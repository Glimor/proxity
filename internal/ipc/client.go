//go:build windows

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Microsoft/go-winio"
)

// ErrDisconnected is returned by Call when the pipe is down and the
// reconnect loop could not restore it before the call timeout.
var ErrDisconnected = errors.New("ipc: service not reachable")

type callResult struct {
	result json.RawMessage
	err    error
}

// Client is a reconnecting named-pipe RPC client for the GUI. One instance
// serializes writes and demultiplexes responses by id.
type Client struct {
	path string

	mu      sync.Mutex
	conn    net.Conn
	wmu     sync.Mutex    // serializes writes on the current conn
	changed chan struct{} // closed on every conn state transition
	pending map[uint64]chan callResult
	lastErr error // last dial/handshake failure, surfaced via Call timeout

	nextID atomic.Uint64

	eventFn atomic.Value // func(Event)

	runOnce  sync.Once
	done     chan struct{}
	doneOnce sync.Once
	wg       sync.WaitGroup
}

func NewClient(path string) *Client {
	if path == "" {
		path = PipePath
	}
	return &Client{
		path:    path,
		pending: make(map[uint64]chan callResult),
		done:    make(chan struct{}),
		changed: make(chan struct{}),
	}
}

// Connect performs a single dial + handshake. For a persistent connection
// use Run, which keeps reconnecting.
func (c *Client) Connect(ctx context.Context) error {
	return c.connect(ctx)
}

// notifyLocked wakes all waiters; caller holds mu.
func (c *Client) notifyLocked() {
	if c.changed != nil {
		close(c.changed)
	}
	c.changed = make(chan struct{})
}

func (c *Client) connect(ctx context.Context) error {
	conn, err := winio.DialPipeContext(ctx, c.path)
	if err != nil {
		c.setLastErr(err)
		return err
	}
	conn.SetDeadline(time.Now().Add(helloTimeout))
	h := hello{Proto: protoName, Version: protoVersion}
	if err := json.NewEncoder(conn).Encode(h); err != nil {
		conn.Close()
		c.setLastErr(err)
		return err
	}
	var ack struct {
		Proto   string `json:"proto"`
		Version int    `json:"version"`
		Error   string `json:"error,omitempty"`
	}
	if err := json.NewDecoder(conn).Decode(&ack); err != nil {
		conn.Close()
		c.setLastErr(err)
		return err
	}
	conn.SetDeadline(time.Time{})
	if ack.Error != "" || ack.Proto != protoName || ack.Version != protoVersion {
		conn.Close()
		err := fmt.Errorf("ipc: protocol handshake failed: %s", ack.Error)
		c.setLastErr(err)
		return err
	}

	c.mu.Lock()
	c.conn = conn
	c.lastErr = nil
	c.notifyLocked()
	c.mu.Unlock()

	go c.readLoop(conn)
	return nil
}

// Run maintains the connection forever: dial, hello, read pump; on drop it
// redials with exponential backoff (1s..5s) until ctx is cancelled or
// Close is called.
func (c *Client) Run(ctx context.Context) {
	c.runOnce.Do(func() {
		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			backoff := time.Second
			for {
				if ctx.Err() != nil || c.isClosed() {
					return
				}
				err := c.connect(ctx)
				if err != nil {
					if !sleepCtx(ctx, c.done, backoff) {
						return
					}
					if backoff < 5*time.Second {
						backoff *= 2
						if backoff > 5*time.Second {
							backoff = 5 * time.Second
						}
					}
					continue
				}
				backoff = time.Second
				select {
				case <-c.waitChanged(): // conn dropped: redial
				case <-ctx.Done():
					return
				case <-c.done:
					return
				}
			}
		}()
	})
}

func (c *Client) isClosed() bool {
	select {
	case <-c.done:
		return true
	default:
		return false
	}
}

func (c *Client) waitChanged() <-chan struct{} {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		// readLoop may have already reported a drop before Run starts waiting.
		ready := make(chan struct{})
		close(ready)
		return ready
	}
	return c.changed
}

// OnEvent registers the event callback (safe to call anytime; latest wins).
func (c *Client) OnEvent(fn func(Event)) { c.eventFn.Store(fn) }

func (c *Client) readLoop(conn net.Conn) {
	dec := json.NewDecoder(conn)
	for {
		var raw json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			break
		}
		var head struct {
			ID    uint64 `json:"id"`
			Event string `json:"event"`
		}
		if err := json.Unmarshal(raw, &head); err != nil {
			continue
		}
		if head.Event != "" {
			var ev Event
			if json.Unmarshal(raw, &ev) == nil {
				if fn, ok := c.eventFn.Load().(func(Event)); ok && fn != nil {
					fn(ev)
				}
			}
			continue
		}
		var resp struct {
			ID     uint64          `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *ErrObj         `json:"error"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			continue
		}
		c.mu.Lock()
		ch, ok := c.pending[resp.ID]
		delete(c.pending, resp.ID)
		c.mu.Unlock()
		if ok {
			if resp.Error != nil {
				ch <- callResult{err: resp.Error}
			} else {
				ch <- callResult{result: resp.Result}
			}
		}
	}

	c.mu.Lock()
	if c.conn == conn {
		c.conn = nil
	}
	pending := c.pending
	c.pending = make(map[uint64]chan callResult)
	c.notifyLocked()
	c.mu.Unlock()
	for _, ch := range pending {
		ch <- callResult{err: ErrDisconnected}
	}
	conn.Close()
}

// Call invokes method with params (marshaled to JSON) and waits up to
// timeout for a response. Returns ErrDisconnected when no connection comes
// up in time, or *ErrObj for remote errors.
func (c *Client) Call(method string, params any, timeout time.Duration) (json.RawMessage, error) {
	deadline := time.Now().Add(timeout)
	conn, err := c.waitConn(deadline)
	if err != nil {
		return nil, err
	}

	var p json.RawMessage
	if params != nil {
		if p, err = json.Marshal(params); err != nil {
			return nil, err
		}
	}
	id := c.nextID.Add(1)
	ch := make(chan callResult, 1)
	c.mu.Lock()
	c.pending[id] = ch
	c.mu.Unlock()

	req := request{ID: id, Method: method, Params: p}
	if err := c.write(conn, req); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, err
	}

	remain := time.Until(deadline)
	if remain <= 0 {
		remain = time.Millisecond
	}
	t := time.NewTimer(remain)
	defer t.Stop()
	select {
	case r := <-ch:
		return r.result, r.err
	case <-t.C:
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("ipc: call %s timed out", method)
	case <-c.done:
		return nil, ErrDisconnected
	}
}

func (c *Client) write(conn net.Conn, v any) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	defer conn.SetWriteDeadline(time.Time{})
	return json.NewEncoder(conn).Encode(v)
}

// waitConn returns the live conn, waiting for the reconnect loop to bring
// one up until deadline.
func (c *Client) waitConn(deadline time.Time) (net.Conn, error) {
	for {
		c.mu.Lock()
		if c.conn != nil {
			conn := c.conn
			c.mu.Unlock()
			return conn, nil
		}
		ch := c.changed
		lastErr := c.lastErr
		closed := c.isClosed()
		c.mu.Unlock()
		if closed {
			return nil, ErrDisconnected
		}
		remain := time.Until(deadline)
		if remain <= 0 {
			if lastErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrDisconnected, lastErr)
			}
			return nil, ErrDisconnected
		}
		t := time.NewTimer(remain)
		select {
		case <-ch:
			t.Stop()
		case <-t.C:
			if lastErr != nil {
				return nil, fmt.Errorf("%w: %v", ErrDisconnected, lastErr)
			}
			return nil, ErrDisconnected
		case <-c.done:
			t.Stop()
			return nil, ErrDisconnected
		}
	}
}

func (c *Client) setLastErr(err error) {
	c.mu.Lock()
	c.lastErr = err
	c.mu.Unlock()
}

// Close drops the connection and stops the reconnect loop.
func (c *Client) Close() error {
	c.doneOnce.Do(func() { close(c.done) })
	c.mu.Lock()
	conn := c.conn
	c.mu.Unlock()
	if conn != nil {
		conn.Close()
	}
	c.wg.Wait()
	return nil
}

func sleepCtx(ctx context.Context, done <-chan struct{}, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-done:
		return false
	case <-t.C:
		return true
	}
}
