package guiprefs

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsDefaults(t *testing.T) {
	p, err := LoadFrom(filepath.Join(t.TempDir(), "nope", "gui.json"))
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Language != "tr" || p.Theme != "system" || p.WizardDone || p.StartMinimized {
		t.Fatalf("unexpected defaults: %+v", p)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gui.json")
	want := Prefs{Language: "en", Theme: "dark", WizardDone: true, StartMinimized: true}
	if err := SaveTo(path, want); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got != want {
		t.Fatalf("round trip: got %+v want %+v", got, want)
	}
}

func TestLoadCorruptFallsBackToDefaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gui.json")
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadFrom(path)
	if err == nil {
		t.Fatal("expected error for corrupt file")
	}
	if p != defaults() {
		t.Fatalf("expected defaults on corrupt file, got %+v", p)
	}
}

func TestInvalidValuesSanitized(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "gui.json")
	if err := os.WriteFile(path, []byte(`{"language":"de","theme":"neon","wizard_done":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	p, err := LoadFrom(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if p.Language != "tr" || p.Theme != "system" || !p.WizardDone {
		t.Fatalf("sanitize failed: %+v", p)
	}
}
