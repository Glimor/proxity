package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/Glimor/proxity/internal/logging"
	"github.com/Glimor/proxity/internal/storage/sqlite"
)

func main() {
	console := flag.Bool("console", false, "run in console (dev) mode instead of as a service")
	install := flag.Bool("install", false, "install the Windows service")
	uninstall := flag.Bool("uninstall", false, "uninstall the Windows service")
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
}
