//go:build windows

// Package svccfg wraps the small Service Control Manager operations Proxity
// needs outside the service entrypoint. Kept in its own package so the app
// service layer (internal/appsvc) can apply settings without importing the
// service's install machinery.
package svccfg

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// ServiceName is the SCM name of the Proxity engine service.
const ServiceName = "ProxityEngine"

// ErrNotInstalled means the named service is not registered with the SCM.
var ErrNotInstalled = errors.New("service not installed")

// SetStartType switches the ProxityEngine service between Automatic and
// Manual (demand) start. Callers must run elevated or as SYSTEM — the app
// service already runs as SYSTEM. Returns ErrNotInstalled when the service
// is not registered (e.g. the GUI uninstall path already removed it).
func SetStartType(automatic bool) error {
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()
	s, err := m.OpenService(ServiceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return ErrNotInstalled
		}
		return fmt.Errorf("open service %q: %w", ServiceName, err)
	}
	defer s.Close()

	cfg, err := s.Config()
	if err != nil {
		return fmt.Errorf("query service config: %w", err)
	}
	want := uint32(mgr.StartManual)
	if automatic {
		want = mgr.StartAutomatic
	}
	if cfg.StartType == want {
		return nil
	}
	cfg.StartType = want
	// DelayedAutoStart is meaningless on a demand-start service; clear it
	// so the written-back config stays coherent.
	if !automatic {
		cfg.DelayedAutoStart = false
	}
	if err := s.UpdateConfig(cfg); err != nil {
		return fmt.Errorf("update service config: %w", err)
	}
	return nil
}
