//go:build windows

package main

import (
	"embed"
	"fmt"
	"os"
	"unsafe"

	"github.com/Glimor/proxity/internal/guiprefs"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	wailswindows "github.com/wailsapp/wails/v2/pkg/options/windows"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	prefs, _ := guiprefs.Load()

	if !wailsBindingGeneration && !webview2Available() {
		showWebview2Missing()
		return
	}

	app := NewApp(prefs)
	if !wailsBindingGeneration {
		if err := setAppIdentity(); err != nil {
			fmt.Fprintln(os.Stderr, "Proxity application identity:", err)
		}
	}

	startHidden := prefs.StartMinimized
	if len(os.Args) > 1 && os.Args[1] == "--minimized" {
		startHidden = true
	}

	err := wails.Run(&options.App{
		Title:            appName,
		Width:            1180,
		Height:           780,
		MinWidth:         940,
		MinHeight:        600,
		StartHidden:      startHidden,
		BackgroundColour: options.NewRGB(15, 17, 23),
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnBeforeClose:    app.beforeClose,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		DragAndDrop:      &options.DragAndDrop{EnableFileDrop: true},
		Windows: &wailswindows.Options{
			Theme:             wailswindows.SystemDefault,
			DisableWindowIcon: false,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "e5f3a1b2-proxity-4c7d-9e8f-0a1b2c3d4e5f",
			OnSecondInstanceLaunch: func(_ options.SecondInstanceData) {
				app.ShowWindow()
			},
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "wails run:", err)
		showFatalBox("Proxity could not start", err.Error())
	}
}

// webview2Available checks the registry for the WebView2 evergreen runtime
// (per-machine and per-user install keys).
func webview2Available() bool {
	const clientGUID = `{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`
	roots := []registry.Key{registry.LOCAL_MACHINE, registry.CURRENT_USER}
	paths := []string{
		`SOFTWARE\Microsoft\EdgeUpdate\Clients\` + clientGUID,
		`SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\` + clientGUID,
	}
	for _, root := range roots {
		for _, p := range paths {
			k, err := registry.OpenKey(root, p, registry.QUERY_VALUE)
			if err != nil {
				continue
			}
			pv, _, err := k.GetStringValue("pv")
			k.Close()
			if err == nil && pv != "" && pv != "0.0.0.0" {
				return true
			}
		}
	}
	return false
}

// showWebview2Missing shows a plain-language dialog and, on OK, opens the
// runtime bootstrapper download page.
func showWebview2Missing() {
	msg := "Proxity needs the Microsoft WebView2 Runtime to show its window.\n\n" +
		"Click OK to download it (free, ~2 MB installer from Microsoft), " +
		"then start Proxity again."
	if messageBox("Proxity", msg, 0x00000041 /* MB_OKCANCEL|MB_ICONINFORMATION */) == 1 {
		openURL("https://go.microsoft.com/fwlink/p/?LinkId=2124703")
	}
}

func showFatalBox(title, msg string) {
	messageBox(title, msg, 0x00000010 /* MB_ICONERROR */)
}

func messageBox(title, text string, flags uintptr) int {
	user32 := windows.NewLazySystemDLL("user32.dll")
	t, _ := windows.UTF16PtrFromString(title)
	m, _ := windows.UTF16PtrFromString(text)
	r, _, _ := user32.NewProc("MessageBoxW").Call(0,
		uintptr(unsafe.Pointer(m)),
		uintptr(unsafe.Pointer(t)),
		flags)
	return int(r)
}

func openURL(url string) {
	u, err := windows.UTF16PtrFromString(url)
	if err != nil {
		return
	}
	verb, _ := windows.UTF16PtrFromString("open")
	shell32 := windows.NewLazySystemDLL("shell32.dll")
	shell32.NewProc("ShellExecuteW").Call(0,
		uintptr(unsafe.Pointer(verb)),
		uintptr(unsafe.Pointer(u)),
		0, 0, 1 /* SW_SHOWNORMAL */)
}
