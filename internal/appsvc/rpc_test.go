//go:build windows

package appsvc

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/ipc"
)

func TestEngineStartRPCReportsNewStatusAndSurvivesGUIDisconnect(t *testing.T) {
	svc, eng, _ := newTestService(t)
	srv := ipc.NewServer()
	svc.RegisterHandlers(srv)
	path := fmt.Sprintf(`\\.\pipe\proxity-start-test-%d`, os.Getpid())
	if err := srv.Listen(path); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer srv.Close()
	go srv.Serve(ctx)
	client := ipc.NewClient(path)
	defer client.Close()
	if err := client.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	raw, err := client.Call("engine.start", nil, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var status EngineStatus
	if err := json.Unmarshal(raw, &status); err != nil {
		t.Fatal(err)
	}
	if !status.Running || !status.CaptureAlive || status.Listeners != 2 {
		t.Fatalf("RPC reported stale start status: %+v", status)
	}
	client.Close()
	if eng.startCtx.Done() != nil {
		t.Fatal("engine lifetime is tied to GUI connection")
	}
	svc.StopEngine()
}
