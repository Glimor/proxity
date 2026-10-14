//go:build windows

package main

import (
	"bytes"
	_ "embed"
	"os"
	"path/filepath"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const appName = "Proxity"

//go:embed assets/branding/logo.png
var appLogoPNG []byte

//go:embed assets/branding/icon.ico
var appIconICO []byte

// setAppIdentity keeps the taskbar and notifications under the same app ID.
func setAppIdentity() error {
	id, err := windows.UTF16PtrFromString(appName)
	if err != nil {
		return err
	}
	hr, _, _ := windows.NewLazySystemDLL("shell32.dll").NewProc("SetCurrentProcessExplicitAppUserModelID").Call(uintptr(unsafe.Pointer(id)))
	if int32(hr) < 0 {
		return windows.Errno(hr)
	}
	return nil
}

// Windows needs a stable on-disk image for notification history. Extract the
// embedded logo and refresh it when the application updates.
func writeNotificationLogo(dir string) (string, error) {
	path := filepath.Join(dir, "logo.png")
	if existing, err := os.ReadFile(path); err == nil && bytes.Equal(existing, appLogoPNG) {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, appLogoPNG, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func registerNotificationBranding(iconPath string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, `Software\Classes\AppUserModelId\`+appName, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	// Refresh existing values too: the library only writes missing values.
	if err := key.SetStringValue("DisplayName", appName); err != nil {
		return err
	}
	return key.SetStringValue("IconUri", iconPath)
}
