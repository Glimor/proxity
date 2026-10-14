//go:build windows

package killswitch

import (
	"fmt"
	"sync"

	"github.com/Glimor/proxity/internal/wfp"
	"github.com/google/uuid"
)

// wfpBackend implements Backend on top of internal/wfp.
type wfpBackend struct {
	eng *wfp.Engine
	mu  sync.Mutex
}

// OpenBackend opens the WFP engine in a dynamic session and wipes the
// sublayer so stale persistent filters from a previous process run cannot
// linger, then re-creates the base objects.
func OpenBackend() (Backend, error) {
	eng, err := wfp.Open("Proxity")
	if err != nil {
		return nil, fmt.Errorf("open WFP engine: %w", err)
	}
	if err := eng.Reset("Proxity", "Proxity filters"); err != nil {
		eng.Close()
		return nil, fmt.Errorf("reset WFP base objects: %w", err)
	}
	return &wfpBackend{eng: eng}, nil
}

func (b *wfpBackend) EnsureBase() error {
	return b.eng.EnsureBase("Proxity", "Proxity filters")
}

func (b *wfpBackend) ReopenSession() error {
	return b.eng.Reopen()
}

func (b *wfpBackend) Txn(fn func() error) error {
	return b.eng.Txn(fn)
}

func (b *wfpBackend) Add(s Spec) error {
	return b.eng.AddFilter(wfp.FilterSpec{
		Key:        s.Key,
		Name:       s.Name,
		IPv6:       s.IPv6,
		Persistent: s.Persistent,
		Weight:     s.Weight,
		Block:      s.Block,
		AppID:      s.AppID,
		TCPOnly:    s.TCPOnly,
		Remote:     s.Remote,
		RemotePort: s.RemotePort,
		NoLoopback: s.NoLoopback,
	})
}

func (b *wfpBackend) Delete(key uuid.UUID) error {
	return b.eng.DeleteFilter(key)
}

func (b *wfpBackend) AppIDOf(path string) ([]byte, error) {
	return wfp.AppIDFromPath(path)
}

// Close releases the engine handle. Persistent objects stay installed —
// that is the fail-closed property.
func (b *wfpBackend) Close() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.eng.Close()
}
