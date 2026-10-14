//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"image/png"

	"fyne.io/systray"
	"github.com/Glimor/proxity/internal/guiicon"
)

type trayItems struct {
	show   *systray.MenuItem
	engine *systray.MenuItem
	quit   *systray.MenuItem
}

// startTray spins up the notification-area icon on its own goroutine (Wails
// owns the main thread). RunWithExternalLoop is the fyne/systray API for
// embedding inside another toolkit.
func (a *App) startTray() {
	start, end := systray.RunWithExternalLoop(func() {
		systray.SetIcon(trayIconICO())
		systray.SetTooltip("Proxity")

		items := &trayItems{
			show:   systray.AddMenuItem("Proxity'i Göster / Show", "Pencereyi aç / Open the window"),
			engine: systray.AddMenuItemCheckbox("Motor Çalışıyor / Engine", "Motoru başlat/durdur — Start/stop the engine", false),
			quit:   systray.AddMenuItem("Çıkış / Quit", "Proxity'yi tamamen kapatır; koruma durur — fully closes Proxity; protection stops"),
		}
		a.trayMu.Lock()
		a.trayItems = items
		a.trayMu.Unlock()

		go func() {
			for range items.show.ClickedCh {
				a.ShowWindow()
			}
		}()
		go func() {
			for range items.engine.ClickedCh {
				st, err := a.EngineStatus()
				if err != nil {
					continue
				}
				if st.Running {
					_, _ = a.StopEngine()
				} else {
					_, _ = a.StartEngine()
				}
			}
		}()
		go func() {
			<-items.quit.ClickedCh
			a.Quit()
		}()

		// Sync the engine checkbox once the pipe state is known.
		go func() {
			if st, err := a.EngineStatus(); err == nil {
				a.updateTrayEngine(st.Running)
			}
		}()
	}, func() {})
	go start()
	a.trayEnd = end
}

func (a *App) updateTrayEngine(running bool) {
	a.trayMu.Lock()
	items := a.trayItems
	a.trayMu.Unlock()
	if items == nil || items.engine == nil {
		return
	}
	if running {
		items.engine.Check()
	} else {
		items.engine.Uncheck()
	}
}

// trayIconICO wraps the generated PNG in a minimal ICO container — systray's
// Windows SetIcon only accepts .ico file contents.
func trayIconICO() []byte {
	pngBytes := guiicon.FallbackPNG()
	var buf bytes.Buffer
	// ICONDIR: reserved=0, type=1(icon), count=1
	binary.Write(&buf, binary.LittleEndian, uint16(0))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	binary.Write(&buf, binary.LittleEndian, uint16(1))
	// ICONDIRENTRY
	cfg, err := png.DecodeConfig(bytes.NewReader(pngBytes))
	w, h := byte(64), byte(64)
	if err == nil {
		w, h = byte(cfg.Width%256), byte(cfg.Height%256)
	}
	buf.WriteByte(w)
	buf.WriteByte(h)
	buf.WriteByte(0)                                    // palette
	buf.WriteByte(0)                                    // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))  // planes
	binary.Write(&buf, binary.LittleEndian, uint16(32)) // bpp
	binary.Write(&buf, binary.LittleEndian, uint32(len(pngBytes)))
	binary.Write(&buf, binary.LittleEndian, uint32(6+16)) // image offset
	buf.Write(pngBytes)
	return buf.Bytes()
}
