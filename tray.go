//go:build windows

package main

import "fyne.io/systray"

// startTray spins up the notification-area icon on its own goroutine (Wails
// owns the main thread). RunWithExternalLoop is the fyne/systray API for
// embedding inside another toolkit.
func (a *App) startTray() {
	start, end := systray.RunWithExternalLoop(func() {
		systray.SetIcon(appIconICO)
		systray.SetTooltip(appName)

		// Engine startup belongs to the service, not a tray checkbox.
		show := systray.AddMenuItem("Show Proxity", "Open the window")
		quit := systray.AddMenuItem("Quit Proxity", "Close the Proxity window and tray; the background service keeps running")

		go func() {
			for range show.ClickedCh {
				a.ShowWindow()
			}
		}()
		go func() {
			<-quit.ClickedCh
			a.Quit()
		}()
	}, func() {})
	go start()
	a.trayEnd = end
}
