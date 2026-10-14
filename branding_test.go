//go:build windows

package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"testing"
)

func TestNotificationLogoRefreshesFromEmbeddedBranding(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "branding")
	path, err := writeNotificationLogo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("old logo"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := writeNotificationLogo(dir); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, appLogoPNG) {
		t.Fatalf("cached logo does not match embedded logo: %v", err)
	}
}

func TestPackagedBranding(t *testing.T) {
	public, err := os.ReadFile("frontend/public/logo.png")
	if err != nil || !bytes.Equal(public, appLogoPNG) {
		t.Fatalf("frontend branding is out of sync: run scripts/update-branding.ps1: %v", err)
	}
	if _, err := png.DecodeConfig(bytes.NewReader(appLogoPNG)); err != nil {
		t.Fatal(err)
	}
	if len(appIconICO) < 6 || binary.LittleEndian.Uint16(appIconICO[2:4]) != 1 {
		t.Fatal("tray icon must be a valid ICO")
	}
	count := int(binary.LittleEndian.Uint16(appIconICO[4:6]))
	if count == 0 || len(appIconICO) < 6+16*count {
		t.Fatal("ICO directory is missing")
	}
	for i := 0; i < count; i++ {
		entry := appIconICO[6+16*i : 6+16*(i+1)]
		size := int(binary.LittleEndian.Uint32(entry[8:12]))
		offset := int(binary.LittleEndian.Uint32(entry[12:16]))
		if offset < 6+16*count || offset+size > len(appIconICO) {
			t.Fatalf("invalid ICO image bounds: frame %d", i)
		}
	}
}
