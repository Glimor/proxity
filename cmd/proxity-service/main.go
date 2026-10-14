//go:build windows

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/Glimor/proxity/internal/appsvc"
	"github.com/Glimor/proxity/internal/engine"
	"github.com/Glimor/proxity/internal/ipc"
	"github.com/Glimor/proxity/internal/killswitch"
	"github.com/Glimor/proxity/internal/logging"
	"github.com/Glimor/proxity/internal/secrets"
	"github.com/Glimor/proxity/internal/storage/sqlite"
	"golang.org/x/sys/windows/svc"
)

func main() {
	console := flag.Bool("console", false, "run in console (dev) mode instead of as a service")
	install := flag.Bool("install", false, "install the Windows service")
	uninstall := flag.Bool("uninstall", false, "uninstall the Windows service")
	engineMode := flag.Bool("engine", false, "run the traffic engine in the foreground (dev harness, needs admin)")
	flag.Parse()

	if *install {
		if err := installService(); err != nil {
			fmt.Fprintln(os.Stderr, "install failed:", err)
			os.Exit(1)
		}
		if err := startService(); err != nil {
			fmt.Fprintln(os.Stderr, "service installed but failed to start:", err)
			os.Exit(1)
		}
		fmt.Printf("service %q installed and started (auto-start, restart-on-failure)\n", serviceName)
		return
	}
	if *uninstall {
		if err := uninstallService(); err != nil {
			fmt.Fprintln(os.Stderr, "uninstall failed:", err)
			os.Exit(1)
		}
		fmt.Printf("service %q removed; WFP provider and filters deleted\n", serviceName)
		return
	}

	dbPath := sqlite.ResolveDBPath(nil)
	logDir := filepath.Dir(dbPath)
	logger, closer, err := logging.Init("debug", filepath.Join(logDir, "logs"), *console)
	if err != nil {
		fmt.Fprintf(os.Stderr, "init logging: %v\n", err)
		os.Exit(1)
	}
	defer closer.Close()

	logger.Info("proxity-service starting",
		slog.Bool("console", *console), slog.String("db", dbPath))

	db, err := sqlite.Open(dbPath)
	var corrupt *sqlite.CorruptDBError
	if errors.As(err, &corrupt) {
		logger.Warn("corrupt database moved aside, starting fresh",
			slog.String("backup", corrupt.BackupPath))
	} else if err != nil {
		logger.Error("open database", slog.String("err", err.Error()))
		os.Exit(1)
	}
	defer db.Close()

	if err := db.Migrate(context.Background()); err != nil {
		logger.Error("migrate", slog.String("err", err.Error()))
		os.Exit(1)
	}

	if *engineMode {
		runEngine(context.Background(), db, logger)
		return
	}

	isSvc, err := svc.IsWindowsService()
	if err != nil {
		logger.Warn("cannot detect SCM mode", slog.String("err", err.Error()))
	}
	if isSvc && !*console {
		h := &svcHandler{}
		var eng *engine.Engine
		h.onResume = func() {
			if eng != nil {
				eng.RecheckProxies()
			}
		}
		h.run = func(ctx context.Context) {
			_, eng = runServiceCore(ctx, db, logger)
		}
		if err := svc.Run(serviceName, h); err != nil {
			logger.Error("svc.Run", slog.String("err", err.Error()))
		}
		return
	}

	// Console path: same stack, stopped by Ctrl+C.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	runServiceCore(ctx, db, logger)
}

// runServiceCore builds the full service stack — kill switch, engine, app
// service, IPC pipe — loads persisted config, auto-starts the engine and
// blocks until ctx is cancelled. Returns the app service and engine for the
// service handler's power-event wiring.
func runServiceCore(ctx context.Context, db *sqlite.DB, logger *slog.Logger) (*appsvc.Service, *engine.Engine) {
	cipher := secrets.NewCipher(true) // machine scope: SYSTEM service

	var ks *killswitch.Policy
	if b, err := killswitch.OpenBackend(); err != nil {
		logger.Error("WFP backend unavailable; kill switch disabled",
			slog.String("err", err.Error()))
	} else {
		ks = killswitch.New(b, logger)
	}

	var app *appsvc.Service
	eng := engine.New(engine.Deps{
		Stats:   db.Stats(),
		Blocked: db.BlockedLog(),
		Cipher:  cipher,
		Log:     logger,
		OnCaptureDead: func() {
			if app != nil {
				app.OnCaptureDead()
			}
		},
	})
	app = appsvc.New(appsvc.Deps{
		Profiles:   db.Profiles(),
		Rules:      db.Rules(),
		Settings:   db.Settings(),
		Stats:      db.Stats(),
		Blocked:    db.BlockedLog(),
		Cipher:     cipher,
		Engine:     eng,
		KillSwitch: ks,
		Log:        logger,
	})

	// Load persisted config into engine + kill switch, then start capture.
	app.Refresh(ctx)
	if err := app.StartEngine(ctx); err != nil {
		logger.Error("engine auto-start failed; continuing (kill switch stays closed)",
			slog.String("err", err.Error()))
	}

	srv := ipc.NewServer()
	app.RegisterHandlers(srv)
	if err := srv.Listen(ipc.PipePath); err != nil {
		logger.Error("IPC listen failed", slog.String("err", err.Error()))
	} else {
		go srv.Serve(ctx)
		// Bridge app events to subscribed pipe clients.
		evCh, unsub := app.Subscribe()
		defer unsub()
		go func() {
			for {
				select {
				case <-ctx.Done():
					return
				case ev, ok := <-evCh:
					if !ok {
						return
					}
					srv.Broadcast(ev.Type, ev.Data)
				}
			}
		}()
		go app.RunEventPump(ctx, time.Second)
		logger.Info("IPC pipe listening", slog.String("path", ipc.PipePath))
	}

	logger.Info("proxity-service running")
	<-ctx.Done()
	logger.Info("proxity-service stopping")
	srv.Close()
	app.StopEngine() // invokes OnCaptureDead -> kill switch fails closed
	if ks != nil {
		ks.Close() // drops the dynamic session; persistent blocks remain
	}
	return app, eng
}

// runEngine is the Phase-1 manual test harness: loads rules/profiles from the
// DB and runs the traffic engine until Ctrl+C, logging a stats snapshot every
// 2 seconds.
func runEngine(ctx context.Context, db *sqlite.DB, logger *slog.Logger) {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	eng := engine.New(engine.Deps{
		Stats:   db.Stats(),
		Blocked: db.BlockedLog(),
		Cipher:  secrets.NewCipher(false),
		Log:     logger,
	})

	profiles, err := db.Profiles().ListProfiles(ctx)
	if err != nil {
		logger.Error("list profiles", slog.String("err", err.Error()))
		return
	}
	rules, err := db.Rules().ListRules(ctx)
	if err != nil {
		logger.Error("list rules", slog.String("err", err.Error()))
		return
	}
	settings, err := db.Settings().LoadSettings(ctx)
	if err != nil {
		logger.Error("load settings", slog.String("err", err.Error()))
		return
	}
	eng.UpdateConfig(profiles, rules, settings)

	if err := eng.Start(ctx); err != nil {
		logger.Error("engine start", slog.String("err", err.Error()))
		os.Exit(1)
	}
	logger.Info("engine running; Ctrl+C to stop",
		slog.Int("profiles", len(profiles)), slog.Int("rules", len(rules)))

	t := time.NewTicker(2 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			eng.Stop()
			return
		case <-t.C:
			s := eng.Snapshot()
			logger.Info("stats",
				slog.Int64("up_bps", s.TotalUpBps),
				slog.Int64("down_bps", s.TotalDownBps),
				slog.Int("active", s.ActiveConns),
				slog.Int64("blocked", s.BlockedCount),
				slog.Int64("failed", s.FailedCount),
				slog.Int64("succeeded", s.SucceededCount))
			for _, ps := range eng.Status() {
				logger.Info("proxy", slog.String("id", ps.ProfileID),
					slog.String("state", ps.State.String()),
					slog.Int64("latency_ms", ps.LatencyMS),
					slog.String("public_ip", ps.PublicIP),
					slog.String("err", ps.LastError))
			}
		}
	}
}
