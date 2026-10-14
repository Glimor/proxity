//go:build windows

package main

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/appsvc"
	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/ipc"
)

type bridgeClient struct {
	method string
	value  any
	err    error
}

func (c *bridgeClient) Call(method string, _ any, _ time.Duration) (json.RawMessage, error) {
	if method != c.method {
		return nil, errors.New("unexpected RPC method: " + method)
	}
	if c.err != nil {
		return nil, c.err
	}
	return json.Marshal(c.value)
}
func (*bridgeClient) OnEvent(func(ipc.Event)) {}
func (*bridgeClient) Run(context.Context)     {}
func (*bridgeClient) Close() error            { return nil }

// Exercise every response shape at the actual Wails-facing method boundary.
// A successful RPC must return the decoded value, not its pre-call zero value.
func TestBridgeReturnsServiceResults(t *testing.T) {
	profile := domain.ProxyProfile{ID: "proxy-1", Name: "SOCKS5", Host: "localhost", Port: 1080, Username: "alice", Enabled: true}
	rule := domain.Rule{ID: "rule-1", ProxyID: profile.ID, Enabled: true}
	snapshot := domain.LiveSnapshot{TotalUpBps: 123}
	status := appsvc.EngineStatus{Running: true, CaptureAlive: true, Listeners: 2}
	cases := []struct {
		method string
		want   any
		call   func(*App) (any, error)
	}{
		{"profiles.list", []domain.ProxyProfile{profile}, func(a *App) (any, error) { return a.GetProfiles() }},
		{"profiles.create", &profile, func(a *App) (any, error) { return a.CreateProfile(appsvc.ProfileInput{}) }},
		{"profiles.update", &profile, func(a *App) (any, error) { return a.UpdateProfile(profile.ID, appsvc.ProfileInput{}) }},
		{"profiles.test", appsvc.TestResult{LatencyMS: 42, PublicIP: "203.0.113.1"}, func(a *App) (any, error) { return a.TestProfile(profile.ID) }},
		{"profiles.statuses", []domain.ProxyStatus{{ProfileID: profile.ID, State: domain.ProxyConnected}}, func(a *App) (any, error) { return a.GetProfileStatuses() }},
		{"rules.list", []domain.Rule{rule}, func(a *App) (any, error) { return a.GetRules() }},
		{"rules.create", &appsvc.RuleResult{Rule: &rule}, func(a *App) (any, error) { return a.CreateRule(appsvc.RuleInput{}) }},
		{"rules.update", &appsvc.RuleResult{Rule: &rule}, func(a *App) (any, error) { return a.UpdateRule(rule.ID, appsvc.RuleInput{}) }},
		{"processes.list", []appsvc.ProcessInfo{{PID: 123, Name: "app.exe"}}, func(a *App) (any, error) { return a.ListProcesses() }},
		{"engine.start", status, func(a *App) (any, error) { return a.StartEngine() }},
		{"engine.stop", status, func(a *App) (any, error) { return a.StopEngine() }},
		{"engine.status", status, func(a *App) (any, error) { return a.EngineStatus() }},
		{"stats.snapshot", snapshot, func(a *App) (any, error) { return a.GetSnapshot() }},
		{"stats.history", []domain.LiveSnapshot{snapshot}, func(a *App) (any, error) { return a.GetHistory() }},
		{"stats.conns", []domain.Connection{{ID: 123}}, func(a *App) (any, error) { return a.GetConnections() }},
		{"stats.exe", []domain.ExeDailyStats{{Day: "2026-10-14", Up: 123}}, func(a *App) (any, error) { return a.GetExeStats("", "") }},
		{"stats.proxy", []domain.ProxyDailyStats{{Day: "2026-10-14", Up: 123}}, func(a *App) (any, error) { return a.GetProxyStats("", "") }},
		{"stats.lastSession", &domain.LastSession{Snapshot: snapshot}, func(a *App) (any, error) { return a.LastSession() }},
		{"stats.exportCsv", "day,up\n2026-10-14,123\n", func(a *App) (any, error) { return a.ExportStatsCSV("", "", "all") }},
		{"blocked.list", []domain.BlockedEntry{{ID: 123}}, func(a *App) (any, error) { return a.GetBlocked(200) }},
		{"data.export", []appsvc.ExportProfile{{Name: "SOCKS5"}}, func(a *App) (any, error) { return a.ExportProfiles(false) }},
		{"data.import", appsvc.ImportResult{Created: 2, Updated: 1}, func(a *App) (any, error) { return a.ImportProfiles(nil, false) }},
	}
	for _, tc := range cases {
		t.Run(tc.method, func(t *testing.T) {
			a := &App{cli: &bridgeClient{method: tc.method, value: tc.want}}
			got, err := tc.call(a)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("got %#v, want %#v", got, tc.want)
			}
		})
	}
}

func TestBridgePreservesRPCError(t *testing.T) {
	a := &App{cli: &bridgeClient{method: "profiles.create", err: &ipc.ErrObj{Code: "profile.bad_port", UserMsg: "Invalid port"}}}
	profile, err := a.CreateProfile(appsvc.ProfileInput{})
	var rpcErr *rpcError
	if profile != nil || !errors.As(err, &rpcErr) || rpcErr.e.Code != "profile.bad_port" {
		t.Fatalf("got profile=%v error=%v", profile, err)
	}
}
