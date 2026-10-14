//go:build !windows

package killswitch

import "context"

// WatchDrops is a no-op off Windows: WFP net events only exist there.
func (p *Policy) WatchDrops(ctx context.Context, cb func(DropEvent)) error {
	<-ctx.Done()
	return nil
}
