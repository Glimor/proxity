//go:build windows

package main

import (
	"os"
	"path/filepath"
	"runtime/debug"
)

func openCrashOutput(logsDir string) (*os.File, error) {
	f, err := os.OpenFile(filepath.Join(logsDir, "crash.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, err
	}
	if err := debug.SetCrashOutput(f, debug.CrashOptions{}); err != nil {
		_ = f.Close()
		return nil, err
	}
	return f, nil
}
