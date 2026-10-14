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

	"github.com/Glimor/proxity/internal/engine"
	"github.com/Glimor/proxity/internal/logging"
	"github.com/Glimor/proxity/internal/secrets"
	"github.com/Glimor/proxity/internal/storage/sqlite"
)

func main() {
	console := flag.Bool("console", false, "run in console (dev) mode instead of as a service")
	install := flag.Bool("install", false, "install the Windows service")
	uninstall := flag.Bool("uninstall", false, "uninstall the Windows service")
	engineMode := flag.Bool("engine", false, "run the traffic engine in the foreground (dev harness, needs admin)")
	flag.Parse()

	if *install {
		fmt.Println("service install: not implemented")
		return
	}
	if *uninstall {
		fmt.Println("service uninstall: not implemented")
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

	logger.Info("ok: database open, migrations applied")

	if *engineMode {
		runEngine(context.Background(), db, logger)
	}
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
		V6Pool:  os.Getenv("PROXITY_V6_NAT") == "pool",
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
