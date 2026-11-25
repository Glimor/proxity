//go:build windows

// Package svccfg manages Proxity's Windows Service Control Manager startup policy.
package svccfg

import (
	"errors"
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc/mgr"
)

// ServiceName is the SCM name of the Proxity engine service.
const ServiceName = "ProxityEngine"

// ErrNotInstalled means the service is not registered with the SCM.
var ErrNotInstalled = errors.New("service not installed")

// Startup reads and updates SCM configuration without starting or stopping the service.
type Startup struct{}

func (Startup) Automatic() (bool, error) {
	s, err := openService(windows.SERVICE_QUERY_CONFIG)
	if err != nil {
		return false, err
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return false, fmt.Errorf("query service config: %w", err)
	}
	return cfg.StartType == mgr.StartAutomatic, nil
}

// SetAutomatic changes the next-boot mode and returns a rollback of the exact
// prior startup policy, including an administrator's non-delayed or disabled mode.
func (Startup) SetAutomatic(enabled bool) (func() error, error) {
	s, err := openService(windows.SERVICE_QUERY_CONFIG | windows.SERVICE_CHANGE_CONFIG)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	cfg, err := s.Config()
	if err != nil {
		return nil, fmt.Errorf("query service config: %w", err)
	}
	want := uint32(mgr.StartManual)
	if enabled {
		want = mgr.StartAutomatic
	}
	if err := setStartType(s, want); err != nil {
		return nil, err
	}
	// The delayed flag only affects Automatic services. Leave it alone when
	// switching to Manual, which also preserves the prior policy on rollback.
	delayedChanged := enabled && !cfg.DelayedAutoStart
	if delayedChanged {
		if err := setDelayed(s, true); err != nil {
			return nil, errors.Join(err, setStartType(s, cfg.StartType))
		}
	}
	return func() error {
		restore, err := openService(windows.SERVICE_CHANGE_CONFIG)
		if err != nil {
			return err
		}
		defer restore.Close()
		if delayedChanged {
			err = setDelayed(restore, cfg.DelayedAutoStart)
		}
		return errors.Join(err, setStartType(restore, cfg.StartType))
	}, nil
}

func openService(access uint32) (*mgr.Service, error) {
	m, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return nil, fmt.Errorf("connect SCM: %w", err)
	}
	defer windows.CloseServiceHandle(m)
	name, err := windows.UTF16PtrFromString(ServiceName)
	if err != nil {
		return nil, fmt.Errorf("encode service name: %w", err)
	}
	h, err := windows.OpenService(m, name, access)
	if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
		return nil, ErrNotInstalled
	}
	if err != nil {
		return nil, fmt.Errorf("open service %q: %w", ServiceName, err)
	}
	return &mgr.Service{Name: ServiceName, Handle: h}, nil
}

func setStartType(s *mgr.Service, startType uint32) error {
	if err := windows.ChangeServiceConfig(s.Handle, windows.SERVICE_NO_CHANGE, startType,
		windows.SERVICE_NO_CHANGE, nil, nil, nil, nil, nil, nil, nil); err != nil {
		return fmt.Errorf("set service start type: %w", err)
	}
	return nil
}

func setDelayed(s *mgr.Service, enabled bool) error {
	var info windows.SERVICE_DELAYED_AUTO_START_INFO
	if enabled {
		info.IsDelayedAutoStartUp = 1
	}
	// ChangeServiceConfig2 interprets this pointer according to the info level.
	if err := windows.ChangeServiceConfig2(s.Handle, windows.SERVICE_CONFIG_DELAYED_AUTO_START_INFO,
		(*byte)(unsafe.Pointer(&info))); err != nil {
		return fmt.Errorf("set delayed service start: %w", err)
	}
	return nil
}
