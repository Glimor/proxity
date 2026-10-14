//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

const engineServiceName = "ProxityEngine"

// ServiceStatus feeds the dashboard "service unreachable" panel.
type ServiceStatus struct {
	BinaryFound bool   `json:"binary_found"`
	BinaryPath  string `json:"binary_path,omitempty"`
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
}

// findServiceBinary looks for proxity-service.exe next to the GUI, in common
// install folders and on PATH.
func findServiceBinary() (string, error) {
	var candidates []string
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates,
			filepath.Join(dir, "proxity-service.exe"),
			filepath.Join(dir, "..", "proxity-service.exe"), // dev: build/bin next to wails output
		)
	}
	if pf := os.Getenv("ProgramFiles"); pf != "" {
		candidates = append(candidates, filepath.Join(pf, "Proxity", "proxity-service.exe"))
	}
	if local := os.Getenv("LocalAppData"); local != "" {
		candidates = append(candidates,
			filepath.Join(local, "Programs", "Proxity", "proxity-service.exe"),
			filepath.Join(local, "Proxity", "proxity-service.exe"))
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			if abs, err := filepath.Abs(c); err == nil {
				return abs, nil
			}
			return c, nil
		}
	}
	if p, err := exec.LookPath("proxity-service.exe"); err == nil {
		return p, nil
	}
	return "", errors.New("proxity-service.exe not found")
}

func queryServiceStatus() ServiceStatus {
	var st ServiceStatus
	if bin, err := findServiceBinary(); err == nil {
		st.BinaryFound = true
		st.BinaryPath = bin
	}
	m, err := mgr.Connect()
	if err != nil {
		return st
	}
	defer m.Disconnect()
	s, err := m.OpenService(engineServiceName)
	if err != nil {
		return st
	}
	defer s.Close()
	st.Installed = true
	if q, err := s.Query(); err == nil {
		st.Running = q.State == svc.Running || q.State == svc.StartPending
	}
	return st
}

// installService runs "proxity-service.exe -install" elevated (UAC consent).
// The service binary installs and then starts the ProxityEngine service.
func installService() error {
	return runServiceElevated("-install")
}

// uninstallService runs "proxity-service.exe -uninstall" elevated.
func uninstallService() error {
	return runServiceElevated("-uninstall")
}

func runServiceElevated(flag string) error {
	bin, err := findServiceBinary()
	if err != nil {
		return err
	}
	verb, _ := windows.UTF16PtrFromString("runas")
	exe, _ := windows.UTF16PtrFromString(bin)
	args, _ := windows.UTF16PtrFromString(flag)
	dir, _ := windows.UTF16PtrFromString(filepath.Dir(bin))
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	r, _, callErr := shell32.NewProc("ShellExecuteW").Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(exe)),
		uintptr(unsafe.Pointer(args)),
		uintptr(unsafe.Pointer(dir)),
		1 /* SW_SHOWNORMAL */)
	if r > 32 {
		return nil
	}
	if errno, ok := callErr.(windows.Errno); ok && errno == windows.ERROR_CANCELLED {
		return errors.New("uac.cancelled")
	}
	return fmt.Errorf("ShellExecuteW failed: %v", callErr)
}

const (
	guiRunKeyPath  = `Software\Microsoft\Windows\CurrentVersion\Run`
	guiRunKeyValue = "Proxity"
)

// setGuiAutoStart writes/removes the HKCU Run entry for the GUI.
func setGuiAutoStart(enabled bool) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, guiRunKeyPath, registry.SET_VALUE|registry.QUERY_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enabled {
		err := k.DeleteValue(guiRunKeyValue)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.Abs(exe); err != nil {
		return err
	}
	return k.SetStringValue(guiRunKeyValue, `"`+exe+`" --minimized`)
}

func guiAutoStart() bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, guiRunKeyPath, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(guiRunKeyValue)
	return err == nil
}
