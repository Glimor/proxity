//go:build windows

package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

func testPath(name string) string {
	return fmt.Sprintf(`\\.\pipe\proxity-test-%s-%d`, name, os.Getpid())
}

func startTestServer(t *testing.T, path string, handlers map[string]Handler) (*Server, context.CancelFunc) {
	t.Helper()
	srv := NewServer()
	for m, h := range handlers {
		srv.Handle(m, h)
	}
	if err := srv.Listen(path); err != nil {
		t.Fatalf("listen: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	go srv.Serve(ctx)
	return srv, cancel
}

func TestCallRoundTrip(t *testing.T) {
	path := testPath("rt")
	srv, cancel := startTestServer(t, path, map[string]Handler{
		"echo": func(ctx context.Context, p json.RawMessage) (any, error) {
			return p, nil
		},
	})
	defer srv.Close()
	defer cancel()

	c := NewClient(path)
	defer c.Close()
	ctx, cc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cc()
	if err := c.Connect(ctx); err != nil {
		t.Fatalf("connect: %v", err)
	}

	res, err := c.Call("echo", map[string]string{"hello": "world"}, 5*time.Second)
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal(res, &out); err != nil || out["hello"] != "world" {
		t.Fatalf("bad result %q err=%v", string(res), err)
	}
}

func TestErrorPropagation(t *testing.T) {
	path := testPath("err")
	srv, cancel := startTestServer(t, path, map[string]Handler{
		"fail": func(ctx context.Context, p json.RawMessage) (any, error) {
			return nil, domain.NewError("app.test", "Something broke", "inner detail")
		},
		"unknown-check": func(ctx context.Context, p json.RawMessage) (any, error) { return nil, nil },
	})
	defer srv.Close()
	defer cancel()

	c := NewClient(path)
	defer c.Close()
	ctx, cc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cc()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	_, err := c.Call("fail", nil, 5*time.Second)
	var eo *ErrObj
	if !errors.As(err, &eo) || eo.Code != "app.test" || eo.Detail != "inner detail" {
		t.Fatalf("expected ErrObj app.test, got %v", err)
	}

	_, err = c.Call("does.not.exist", nil, 5*time.Second)
	if !errors.As(err, &eo) || eo.Code != "ipc.unknown_method" {
		t.Fatalf("expected ipc.unknown_method, got %v", err)
	}
}

func TestBroadcastToSubscribers(t *testing.T) {
	path := testPath("bc")
	srv, cancel := startTestServer(t, path, nil)
	defer srv.Close()
	defer cancel()

	ctx, cc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cc()

	recv := func() (*Client, chan Event) {
		c := NewClient(path)
		if err := c.Connect(ctx); err != nil {
			t.Fatalf("connect: %v", err)
		}
		ch := make(chan Event, 8)
		c.OnEvent(func(e Event) { ch <- e })
		if _, err := c.Call("events.subscribe", nil, 5*time.Second); err != nil {
			t.Fatalf("subscribe: %v", err)
		}
		return c, ch
	}
	c1, ch1 := recv()
	defer c1.Close()
	c2, ch2 := recv()
	defer c2.Close()

	time.Sleep(100 * time.Millisecond) // let subscribe land
	srv.Broadcast("stats", map[string]int{"up": 42})

	for i, ch := range []chan Event{ch1, ch2} {
		select {
		case ev := <-ch:
			if ev.Event != "stats" {
				t.Fatalf("client %d: bad event %q", i, ev.Event)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("client %d: no event received", i)
		}
	}
}

func TestReconnectAfterServerRestart(t *testing.T) {
	path := testPath("re")
	srv, cancel1 := startTestServer(t, path, map[string]Handler{
		"ping": func(ctx context.Context, p json.RawMessage) (any, error) {
			return "pong", nil
		},
	})

	ctx, cc := context.WithTimeout(context.Background(), 30*time.Second)
	defer cc()
	c := NewClient(path)
	defer c.Close()
	c.Run(ctx)

	if _, err := c.Call("ping", nil, 10*time.Second); err != nil {
		t.Fatalf("initial call: %v", err)
	}

	// Kill the server; start a fresh one on the same pipe name.
	srv.Close()
	cancel1()
	time.Sleep(200 * time.Millisecond)

	srv2, cancel2 := startTestServer(t, path, map[string]Handler{
		"ping": func(ctx context.Context, p json.RawMessage) (any, error) {
			return "pong2", nil
		},
	})
	defer srv2.Close()
	defer cancel2()

	var res json.RawMessage
	var err error
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		res, err = c.Call("ping", nil, 3*time.Second)
		if err == nil {
			break
		}
		time.Sleep(200 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("call after reconnect: %v", err)
	}
	var pong string
	json.Unmarshal(res, &pong)
	if pong != "pong2" {
		t.Fatalf("expected pong2 from new server, got %q", pong)
	}
}

func TestConcurrentCalls(t *testing.T) {
	path := testPath("conc")
	srv, cancel := startTestServer(t, path, map[string]Handler{
		"add": func(ctx context.Context, p json.RawMessage) (any, error) {
			var in struct{ A, B int }
			json.Unmarshal(p, &in)
			return in.A + in.B, nil
		},
	})
	defer srv.Close()
	defer cancel()

	c := NewClient(path)
	defer c.Close()
	ctx, cc := context.WithTimeout(context.Background(), 10*time.Second)
	defer cc()
	if err := c.Connect(ctx); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := c.Call("add", map[string]int{"a": i, "b": 1}, 5*time.Second)
			if err != nil {
				t.Errorf("call %d: %v", i, err)
				return
			}
			var n int
			if json.Unmarshal(res, &n) != nil || n != i+1 {
				t.Errorf("call %d: bad result %q", i, string(res))
			}
		}()
	}
	wg.Wait()
}
