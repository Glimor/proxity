// Package guiprefs is the GUI's offline preference store. It persists
// %LocalAppData%\Proxity\gui.json so the window can render correctly before
// the service connection comes up; once connected, service Settings win for
// the shared keys.
package guiprefs

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// Prefs are the GUI-local settings kept in gui.json.
type Prefs struct {
	Language       string `json:"language"`        // "tr" | "en"
	Theme          string `json:"theme"`           // "light" | "dark" | "system"
	WizardDone     bool   `json:"wizard_done"`     // first-run wizard completed
	StartMinimized bool   `json:"start_minimized"` // open to tray instead of a window
}

func defaults() Prefs {
	return Prefs{Language: "tr", Theme: "system"}
}

// Path resolves %LocalAppData%\Proxity\gui.json (with a temp fallback).
func Path() string {
	base := os.Getenv("LocalAppData")
	if base == "" {
		base = os.TempDir()
	}
	return filepath.Join(base, "Proxity", "gui.json")
}

// Load reads gui.json; a missing or corrupt file yields defaults.
func Load() (Prefs, error) {
	return LoadFrom(Path())
}

// LoadFrom reads prefs from an explicit path (tests).
func LoadFrom(path string) (Prefs, error) {
	p := defaults()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return p, nil
		}
		return p, err
	}
	if err := json.Unmarshal(data, &p); err != nil {
		return defaults(), err
	}
	if p.Language != "tr" && p.Language != "en" {
		p.Language = defaults().Language
	}
	switch p.Theme {
	case "light", "dark", "system":
	default:
		p.Theme = defaults().Theme
	}
	return p, nil
}

// Save writes prefs atomically (tmp + rename).
func Save(p Prefs) error {
	return SaveTo(Path(), p)
}

// SaveTo writes prefs to an explicit path (tests).
func SaveTo(path string, p Prefs) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
