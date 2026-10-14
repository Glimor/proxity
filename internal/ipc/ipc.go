//go:build windows

// Package ipc implements the named-pipe RPC channel between the GUI and the
// Proxity service. Transport: \\.\pipe\proxity (go-winio), framing:
// newline-delimited JSON.
//
// Handshake: after connecting the client sends {"proto":"proxity","version":1}
// and the server answers with the same object or closes on version mismatch.
// Requests  {"id":N,"method":"m","params":{...}}
// Responses {"id":N,"result":X} or {"id":N,"error":{"code","user_msg","detail"}}
// Events    {"event":"stats|status|blocked|conns|...","data":X} — pushed to
// subscribed connections only.
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

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Microsoft/go-winio"
)

const (
	// PipePath is the single service endpoint.
	PipePath = `\\.\pipe\proxity`
	// PipeSDDL grants full access to SYSTEM, Administrators and interactive
	// users — the GUI runs elevated or unelevated in the user session.
	PipeSDDL     = `D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;GA;;;IU)`
	protoName    = "proxity"
	protoVersion = 1
	// helloTimeout bounds the handshake on both ends.
	helloTimeout = 10 * time.Second
)

// Handler answers one RPC method. params is the raw "params" object (may be
// null); the result is marshaled verbatim into the response.
type Handler func(ctx context.Context, params json.RawMessage) (any, error)

// ErrObj is the wire shape of an error response.
type ErrObj struct {
	Code    string `json:"code"`
	UserMsg string `json:"user_msg"`
	Detail  string `json:"detail,omitempty"`
}

func (e *ErrObj) Error() string {
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.UserMsg, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.UserMsg)
}

type request struct {
	ID     uint64          `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

type response struct {
	ID     uint64  `json:"id"`
	Result any     `json:"result,omitempty"`
	Error  *ErrObj `json:"error,omitempty"`
}

// Event is a server push to subscribed clients.
type Event struct {
	Event string `json:"event"`
	Data  any    `json:"data,omitempty"`
}

type hello struct {
	Proto   string `json:"proto"`
	Version int    `json:"version"`
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

type srvConn struct {
	conn       net.Conn
	wmu        sync.Mutex
	subscribed atomic.Bool
	cancel     context.CancelFunc
}

func (c *srvConn) write(v any) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()
	c.conn.SetWriteDeadline(time.Now().Add(15 * time.Second))
	defer c.conn.SetWriteDeadline(time.Time{})
	return json.NewEncoder(c.conn).Encode(v)
}

// Server is a multi-client named-pipe RPC endpoint.
type Server struct {
	handlers map[string]Handler

	mu     sync.Mutex
	conns  map[*srvConn]struct{}
	ln     net.Listener
	closed bool
	wg     sync.WaitGroup
}

func NewServer() *Server {
	return &Server{handlers: make(map[string]Handler), conns: make(map[*srvConn]struct{})}
}

// Handle registers a method handler.
func (s *Server) Handle(method string, h Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handlers[method] = h
}

// Listen creates the pipe listener with the service ACL.
func (s *Server) Listen(path string) error {
	if path == "" {
		path = PipePath
	}
	ln, err := winio.ListenPipe(path, &winio.PipeConfig{
		SecurityDescriptor: PipeSDDL,
	})
	if err != nil {
		return fmt.Errorf("listen pipe %s: %w", path, err)
	}
	s.mu.Lock()
	s.ln = ln
	s.mu.Unlock()
	return nil
}

// Serve accepts connections until Close or ctx cancellation.
func (s *Server) Serve(ctx context.Context) error {
	s.mu.Lock()
	ln := s.ln
	s.mu.Unlock()
	if ln == nil {
		return errors.New("ipc: Listen not called")
	}
	go func() {
		<-ctx.Done()
		s.Close()
	}()
	for {
		conn, err := ln.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed || ctx.Err() != nil {
				return nil
			}
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				continue
			}
			return err
		}
		s.wg.Add(1)
		go func() { defer s.wg.Done(); s.serveConn(conn) }()
	}
}

// Broadcast pushes an event to every subscribed client; write failures
// evict the client silently.
func (s *Server) Broadcast(event string, data any) {
	s.mu.Lock()
	targets := make([]*srvConn, 0, len(s.conns))
	for c := range s.conns {
		if c.subscribed.Load() {
			targets = append(targets, c)
		}
	}
	s.mu.Unlock()
	ev := Event{Event: event, Data: data}
	for _, c := range targets {
		if err := c.write(ev); err != nil {
			c.conn.Close()
		}
	}
}

// Subscribe marks the calling connection as an event subscriber. It is
// invoked by the built-in "events.subscribe" handler via the context.
func (s *Server) setSubscribed(c *srvConn, on bool) { c.subscribed.Store(on) }

// Close shuts the listener and every client connection.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	ln := s.ln
	conns := make([]*srvConn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	if ln != nil {
		ln.Close()
	}
	for _, c := range conns {
		c.cancel()
		c.conn.Close()
	}
	s.wg.Wait()
	return nil
}

func (s *Server) serveConn(conn net.Conn) {
	ctx, cancel := context.WithCancel(context.Background())
	c := &srvConn{conn: conn, cancel: cancel}
	defer func() {
		cancel()
		s.mu.Lock()
		delete(s.conns, c)
		s.mu.Unlock()
		conn.Close()
	}()

	// Handshake: first line must be a hello with matching proto/version.
	conn.SetReadDeadline(time.Now().Add(helloTimeout))
	var h hello
	if err := json.NewDecoder(conn).Decode(&h); err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})
	if h.Proto != protoName || h.Version != protoVersion {
		c.write(map[string]any{
			"proto":   protoName,
			"version": protoVersion,
			"error":   "protocol version mismatch",
		})
		return
	}
	if err := c.write(hello{Proto: protoName, Version: protoVersion}); err != nil {
		return
	}

	s.mu.Lock()
	s.conns[c] = struct{}{}
	s.mu.Unlock()

	dec := json.NewDecoder(conn)
	var inFlight sync.WaitGroup
	defer inFlight.Wait()
	for {
		var req request
		if err := dec.Decode(&req); err != nil {
			return
		}
		if req.Method == "" {
			continue
		}
		inFlight.Add(1)
		go func(req request) {
			defer inFlight.Done()
			resp := s.dispatch(ctx, c, req)
			if err := c.write(resp); err != nil && ctx.Err() == nil {
				conn.Close()
			}
		}(req)
	}
}

func (s *Server) dispatch(ctx context.Context, c *srvConn, req request) response {
	// Built-in subscription toggles don't need a registered handler.
	if req.Method == "events.subscribe" || req.Method == "events.unsubscribe" {
		s.setSubscribed(c, req.Method == "events.subscribe")
		return response{ID: req.ID, Result: map[string]bool{"subscribed": req.Method == "events.subscribe"}}
	}
	s.mu.Lock()
	h, ok := s.handlers[req.Method]
	s.mu.Unlock()
	if !ok {
		return response{ID: req.ID, Error: &ErrObj{
			Code: "ipc.unknown_method", UserMsg: "Unknown method", Detail: req.Method}}
	}
	res, err := h(ctx, req.Params)
	if err != nil {
		var ae *domain.AppError
		if errors.As(err, &ae) {
			return response{ID: req.ID, Error: &ErrObj{Code: ae.Code, UserMsg: ae.UserMsg, Detail: ae.Detail}}
		}
		var eo *ErrObj
		if errors.As(err, &eo) {
			return response{ID: req.ID, Error: eo}
		}
		return response{ID: req.ID, Error: &ErrObj{
			Code: "ipc.internal", UserMsg: "Internal error", Detail: err.Error()}}
	}
	return response{ID: req.ID, Result: res}
}
