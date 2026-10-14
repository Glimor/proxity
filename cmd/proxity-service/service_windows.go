//go:build windows

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Glimor/proxity/internal/wfp"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const (
	serviceName    = "ProxityEngine"
	serviceDisplay = "Proxity Engine"
	serviceDesc    = "Proxity traffic engine: per-app proxy routing, stats and the WFP kill switch."
)

// Power broadcast codes (winuser.h); not in x/sys.
const (
	pbtAPMResumeSuspend   = 0x0007
	pbtAPMResumeAutomatic = 0x0012
)

func isElevated() bool {
	var token windows.Token
	if err := windows.OpenProcessToken(windows.CurrentProcess(), windows.TOKEN_QUERY, &token); err != nil {
		return false
	}
	defer token.Close()
	return token.IsElevated()
}

// installService registers the Windows service (auto-start, restart-on-fail
// x3). Requires elevation.
func installService() error {
	if !isElevated() {
		return errors.New("service install requires an elevated (Administrator) console")
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.Abs(exe); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	cfg := mgr.Config{
		StartType:   mgr.StartAutomatic,
		DisplayName: serviceDisplay,
		Description: serviceDesc,
	}
	sv, err := m.CreateService(serviceName, exe, cfg)
	if err != nil {
		if !errors.Is(err, windows.ERROR_SERVICE_EXISTS) {
			return fmt.Errorf("create service: %w", err)
		}
		// Already installed: refresh the binary path + config.
		if sv, err = m.OpenService(serviceName); err != nil {
			return fmt.Errorf("open existing service: %w", err)
		}
		if err := sv.UpdateConfig(cfg); err != nil {
			sv.Close()
			return fmt.Errorf("update service config: %w", err)
		}
	}
	defer sv.Close()

	restarts := []mgr.RecoveryAction{
		{Type: mgr.ServiceRestart, Delay: 5 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 10 * time.Second},
		{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	}
	if err := sv.SetRecoveryActions(restarts, 3600); err != nil {
		return fmt.Errorf("set recovery actions: %w", err)
	}
	if err := sv.SetRecoveryActionsOnNonCrashFailures(true); err != nil {
		return fmt.Errorf("set non-crash recovery flag: %w", err)
	}
	return nil
}

// uninstallService stops and deletes the service, then removes our WFP
// provider (and with it every Proxity filter).
func uninstallService() error {
	if !isElevated() {
		return errors.New("service uninstall requires an elevated (Administrator) console")
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("connect SCM: %w", err)
	}
	defer m.Disconnect()

	sv, err := m.OpenService(serviceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			goto wfpCleanup
		}
		return fmt.Errorf("open service: %w", err)
	}
	{
		st, err := sv.Query()
		if err == nil && st.State != svc.Stopped {
			if _, err := sv.Control(svc.Stop); err == nil {
				deadline := time.Now().Add(15 * time.Second)
				for time.Now().Before(deadline) {
					st, err := sv.Query()
					if err != nil || st.State == svc.Stopped {
						break
					}
					time.Sleep(300 * time.Millisecond)
				}
			}
		}
	}
	if err := sv.Delete(); err != nil {
		sv.Close()
		return fmt.Errorf("delete service: %w", err)
	}
	sv.Close()

wfpCleanup:
	if err := wfp.RemoveProvider(); err != nil {
		return fmt.Errorf("service removed, but WFP cleanup failed: %w", err)
	}
	return nil
}

// svcHandler adapts runCore to the SCM Execute contract.
type svcHandler struct {
	run      func(ctx context.Context)
	onResume func()
}

func (h *svcHandler) Execute(_ []string, r <-chan svc.ChangeRequest, s chan<- svc.Status) (bool, uint32) {
	s <- svc.Status{State: svc.StartPending, WaitHint: 10000}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() {
		h.run(ctx)
		close(done)
	}()
	s <- svc.Status{
		State:   svc.Running,
		Accepts: svc.AcceptStop | svc.AcceptShutdown | svc.AcceptPowerEvent,
	}
	for {
		select {
		case cr := <-r:
			switch cr.Cmd {
			case svc.Stop, svc.Shutdown:
				s <- svc.Status{State: svc.StopPending, WaitHint: 15000}
				cancel()
				<-done
				return false, 0
			case svc.PowerEvent:
				if cr.EventType == pbtAPMResumeAutomatic || cr.EventType == pbtAPMResumeSuspend {
					if h.onResume != nil {
						h.onResume()
					}
				}
			case svc.Interrogate:
				s <- cr.CurrentStatus
			}
		case <-done:
			// runCore returned on its own (fatal init error etc.)
			return false, 0
		}
	}
}
