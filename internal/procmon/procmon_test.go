//go:build windows

package procmon

import (
	"log/slog"
	"net/netip"
	"os"
	"path/filepath"
	"testing"

	"github.com/Glimor/proxity/internal/domain"
)

// Works without admin: only needs OpenProcess on our own process.
func TestProcessPathSelf(t *testing.T) {
	m := New(slog.Default())
	got := m.ProcessPath(os.Getpid())
	if got == "" {
		t.Fatal("empty path for self")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Skip(err)
	}
	// Test binaries run from a temp dir; compare canonical folds.
	want, err := domain.CanonicalPath(exe)
	if err != nil {
		t.Skip(err)
	}
	if got != domain.FoldPath(want) && filepath.Base(got) != filepath.Base(domain.FoldPath(want)) {
		t.Fatalf("got %q want %q", got, domain.FoldPath(want))
	}
}

func TestLookupMiss(t *testing.T) {
	m := New(slog.Default())
	if pid, _, ok := m.Lookup(netip.MustParseAddr("127.0.0.1"), 1); ok || pid != 0 {
		t.Fatal("unexpected hit on unused port")
	}
}
