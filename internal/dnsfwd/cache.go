package dnsfwd

import (
	"net/netip"
	"sync"
	"time"
)

// maxCacheTTL caps record TTLs so a poisoned or long-lived upstream answer
// can't pin a stale IP→domain mapping forever.
const maxCacheTTL = time.Hour

type cacheEntry struct {
	name string
	exp  time.Time
}

// domainCache is a TTL'd IP→domain map populated from forwarded responses.
type domainCache struct {
	mu      sync.RWMutex
	m       map[netip.Addr]cacheEntry
	lastGC  time.Time
	nowFunc func() time.Time // test hook
}

func newDomainCache() *domainCache {
	return &domainCache{m: make(map[netip.Addr]cacheEntry), nowFunc: time.Now}
}

func (c *domainCache) put(ip netip.Addr, name string, ttl time.Duration) {
	if name == "" || ttl <= 0 {
		return
	}
	if ttl > maxCacheTTL {
		ttl = maxCacheTTL
	}
	c.mu.Lock()
	c.m[ip.Unmap()] = cacheEntry{name: name, exp: c.nowFunc().Add(ttl)}
	c.gcLocked()
	c.mu.Unlock()
}

// Lookup returns the cached domain for an IP, if any and unexpired.
func (c *domainCache) Lookup(ip netip.Addr) (string, bool) {
	c.mu.RLock()
	e, ok := c.m[ip.Unmap()]
	c.mu.RUnlock()
	if !ok {
		return "", false
	}
	if c.nowFunc().After(e.exp) {
		return "", false
	}
	return e.name, true
}

// gcLocked opportunistically sweeps expired entries, at most once a minute.
func (c *domainCache) gcLocked() {
	now := c.nowFunc()
	if now.Sub(c.lastGC) < time.Minute {
		return
	}
	c.lastGC = now
	for k, e := range c.m {
		if now.After(e.exp) {
			delete(c.m, k)
		}
	}
}
