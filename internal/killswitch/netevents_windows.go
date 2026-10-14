//go:build windows

package killswitch

import (
	"context"
	"time"

	"github.com/Glimor/proxity/internal/wfp"
)

// dropPollInterval is the FwpmNetEventEnum0 fallback cadence.
const dropPollInterval = 5 * time.Second

// WatchDrops streams classify-drop events attributed to this policy's
// filters until ctx ends. Best-effort: it returns nil (disabling drop
// logging) when net-event collection is unavailable.
func (p *Policy) WatchDrops(ctx context.Context, cb func(DropEvent)) error {
	wb, ok := p.b.(*wfpBackend)
	if !ok {
		return nil
	}
	eng := wb.eng
	return wfp.WatchDrops(ctx, p.log, dropPollInterval,
		func(id uint64) bool {
			_, ok := eng.FilterIDs()[id]
			return ok
		},
		func(ev wfp.DropEvent) {
			cb(DropEvent{
				ExePath: wfp.AppIDToPath(ev.AppID),
				Dst:     ev.Dst,
				DstPort: ev.DstPort,
				Proto:   ev.Proto,
				TS:      ev.TS,
			})
		})
}
