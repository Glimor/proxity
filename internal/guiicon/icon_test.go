package guiicon

import (
	"bytes"
	"image/png"
	"strings"
	"testing"
)

func TestFallbackIsValidPNGDataURL(t *testing.T) {
	u := DataURL(`C:\definitely\not\a\real\file-xyz.exe`)
	if !strings.HasPrefix(u, "data:image/png;base64,") {
		t.Fatalf("bad data url: %.40s", u)
	}
	raw, err := png.Decode(bytes.NewReader(FallbackPNG()))
	if err != nil {
		t.Fatalf("fallback png decode: %v", err)
	}
	if raw.Bounds().Dx() != 64 {
		t.Fatalf("unexpected size %v", raw.Bounds())
	}
}

func TestExtractRealExe(t *testing.T) {
	for _, p := range []string{
		`C:\Windows\System32\notepad.exe`,
		`C:\Windows\regedit.exe`,
	} {
		b, err := PNG(p)
		if err != nil {
			continue // binary absent on this machine: try the next
		}
		img, err := png.Decode(bytes.NewReader(b))
		if err != nil {
			t.Fatalf("decode %s: %v", p, err)
		}
		if img.Bounds().Dx() != 32 {
			t.Fatalf("unexpected icon size %v", img.Bounds())
		}
		return
	}
	t.Skip("no reference exe found")
}
