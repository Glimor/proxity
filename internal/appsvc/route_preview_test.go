//go:build windows

package appsvc

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/ipc"
	"github.com/google/uuid"
)

type routePreviewEngine struct {
	*fakeEngine
	statuses []domain.ProxyStatus
}

func (e routePreviewEngine) Status() []domain.ProxyStatus { return e.statuses }

func routePreviewClient(t *testing.T, svc *Service) *ipc.Client {
	t.Helper()
	srv := ipc.NewServer()
	svc.RegisterHandlers(srv)
	pipe := `\\.\pipe\proxity-route-preview-test-` + uuid.NewString()
	if err := srv.Listen(pipe); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	t.Cleanup(func() {
		cancel()
		if err := srv.Close(); err != nil {
			t.Error(err)
		}
	})
	go srv.Serve(ctx)
	client := ipc.NewClient(pipe)
	t.Cleanup(func() {
		if err := client.Close(); err != nil {
			t.Error(err)
		}
	})
	if err := client.Connect(ctx); err != nil {
		t.Fatal(err)
	}
	return client
}

func savePreviewRule(t *testing.T, svc *Service, r domain.Rule) {
	t.Helper()
	r.ID = uuid.NewString()
	if err := domain.CanonicalizeRulePath(&r, r.Path); err != nil {
		t.Fatal(err)
	}
	if err := svc.d.Rules.CreateRule(t.Context(), &r); err != nil {
		t.Fatal(err)
	}
}

func TestRoutePreviewRPCExplainsSavedRoute(t *testing.T) {
	svc, eng, _ := newTestService(t)
	eng.running, eng.alive = true, true
	svc.d.Engine = routePreviewEngine{eng, []domain.ProxyStatus{
		{ProfileID: "online", State: domain.ProxyConnected, PublicIP: "private-health-ip"},
		{ProfileID: "offline", State: domain.ProxyError, LastError: "private-health-error"},
		{ProfileID: "disabled", State: domain.ProxyConnected},
	}}
	for _, p := range []domain.ProxyProfile{
		{ID: "online", Name: "Connected Office", Enabled: true},
		{ID: "offline", Name: "Offline Office", Enabled: true},
		{ID: "disabled", Name: "Disabled Office"},
	} {
		p.Protocol, p.Host, p.Port = domain.ProxySOCKS5, "private-proxy-host", 1080
		p.Username, p.PasswordCiphertext = "private-proxy-user", []byte("private-proxy-password")
		if err := svc.d.Profiles.CreateProfile(t.Context(), &p); err != nil {
			t.Fatal(err)
		}
	}
	st := domain.DefaultSettings()
	st.DefaultAction = domain.DecisionBlock
	if err := svc.d.Settings.SaveSettings(t.Context(), st); err != nil {
		t.Fatal(err)
	}
	for _, r := range []domain.Rule{
		{Name: "Outer", Kind: domain.RuleKindFolder, Path: `C:\Apps`, Action: domain.RuleActionBlock, Enabled: true, IncludeSubdirs: true, Priority: 100},
		{Name: "Inner", Kind: domain.RuleKindFolder, Path: `C:\Apps\Office`, Action: domain.RuleActionDirect, Enabled: true},
		{Name: "Exact", Kind: domain.RuleKindExe, Path: `C:\Apps\Office\report.exe`, Action: domain.RuleActionProxy, ProxyID: "online", Enabled: true, Priority: 5},
		{Name: "Lower priority", Kind: domain.RuleKindExe, Path: `C:\Apps\Office\report.exe`, Action: domain.RuleActionBlock, Enabled: true},
		{Name: "Disabled high priority", Kind: domain.RuleKindExe, Path: `C:\Apps\Office\report.exe`, Action: domain.RuleActionBlock, Priority: 999},
		{Name: "Alpha tie", Kind: domain.RuleKindExe, Path: `C:\Apps\tie.exe`, Action: domain.RuleActionDirect, Enabled: true, Priority: 10},
		{Name: "Beta tie", Kind: domain.RuleKindExe, Path: `C:\Apps\tie.exe`, Action: domain.RuleActionBlock, Enabled: true, Priority: 10},
		{Name: "Gamma lower", Kind: domain.RuleKindExe, Path: `C:\Apps\tie.exe`, Action: domain.RuleActionBlock, Enabled: true},
		{Name: "Offline", Kind: domain.RuleKindExe, Path: `C:\Apps\offline.exe`, Action: domain.RuleActionProxy, ProxyID: "offline", Enabled: true, KillSwitch: true},
		{Name: "Missing", Kind: domain.RuleKindExe, Path: `C:\Apps\missing.exe`, Action: domain.RuleActionProxy, ProxyID: "missing", Enabled: true},
		{Name: "Disabled", Kind: domain.RuleKindExe, Path: `C:\Apps\disabled.exe`, Action: domain.RuleActionProxy, ProxyID: "disabled", Enabled: true},
	} {
		savePreviewRule(t, svc, r)
	}
	client := routePreviewClient(t, svc)
	for _, tc := range []struct {
		name, input, path string
		reason            RouteMatchReason
		action            domain.RuleAction
		policy            RouteExePolicy
		rule              *RoutePreviewRule
		proxy             *RoutePreviewProxy
		savedDefault      domain.Decision
	}{
		{"exact beats folders; priority selects exact winner", `c:/apps/office/report.EXE`, `C:\apps\office\report.EXE`,
			RouteExactExe, domain.RuleActionProxy, RouteExeBlock, &RoutePreviewRule{"Exact", `C:\Apps\Office\report.exe`, domain.RuleKindExe, new(5)},
			&RoutePreviewProxy{"Connected Office", RouteProxyConnected, domain.RuleActionDirect}, ""},
		{"deepest folder beats higher priority parent", `C:\Apps\Office\other.exe`, `C:\Apps\Office\other.exe`,
			RouteDeepestFolder, domain.RuleActionDirect, RouteExeNone, &RoutePreviewRule{"Inner", `C:\Apps\Office`, domain.RuleKindFolder, nil}, nil, ""},
		{"top tie does not claim priority", `C:\Apps\tie.exe`, `C:\Apps\tie.exe`,
			RouteExactExe, domain.RuleActionDirect, RouteExeBlock, &RoutePreviewRule{"Alpha tie", `C:\Apps\tie.exe`, domain.RuleKindExe, nil}, nil, ""},
		{"no match passes direct despite saved default", `C:\Apps2\other.exe`, `C:\Apps2\other.exe`,
			RouteDefault, domain.RuleActionDirect, RouteExeNone, nil, nil, domain.DecisionBlock},
		{"offline kill switch blocks fallback", `C:\Apps\offline.exe`, `C:\Apps\offline.exe`,
			RouteExactExe, domain.RuleActionProxy, RouteExeKillSwitch, &RoutePreviewRule{"Offline", `C:\Apps\offline.exe`, domain.RuleKindExe, nil},
			&RoutePreviewProxy{"Offline Office", RouteProxyUnavailable, domain.RuleActionBlock}, ""},
		{"missing profile falls back to direct", `C:\Apps\missing.exe`, `C:\Apps\missing.exe`,
			RouteExactExe, domain.RuleActionProxy, RouteExeUDPOnly, &RoutePreviewRule{"Missing", `C:\Apps\missing.exe`, domain.RuleKindExe, nil},
			&RoutePreviewProxy{"", RouteProxyMissing, domain.RuleActionDirect}, ""},
		{"disabled profile is not connected even with a cached status", `C:\Apps\disabled.exe`, `C:\Apps\disabled.exe`,
			RouteExactExe, domain.RuleActionProxy, RouteExeUDPOnly, &RoutePreviewRule{"Disabled", `C:\Apps\disabled.exe`, domain.RuleKindExe, nil},
			&RoutePreviewProxy{"Disabled Office", RouteProxyDisabled, domain.RuleActionDirect}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			raw, err := client.Call("rules.previewRoute", map[string]string{"exe_path": tc.input}, 3*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			var got RoutePreview
			if err := json.Unmarshal(raw, &got); err != nil {
				t.Fatal(err)
			}
			if got.ExePath != tc.path || got.Reason != tc.reason || got.Action != tc.action ||
				got.ExePolicy != tc.policy || got.SavedDefaultAction != tc.savedDefault {
				t.Fatalf("unexpected route: %s", raw)
			}
			if tc.rule == nil && got.Rule != nil || tc.rule != nil && (got.Rule == nil ||
				got.Rule.Name != tc.rule.Name || got.Rule.Path != tc.rule.Path || got.Rule.Kind != tc.rule.Kind ||
				(tc.rule.Priority == nil) != (got.Rule.Priority == nil) ||
				tc.rule.Priority != nil && *got.Rule.Priority != *tc.rule.Priority) {
				t.Fatalf("unexpected winning rule: %s", raw)
			}
			if tc.proxy == nil && got.Proxy != nil || tc.proxy != nil && (got.Proxy == nil || *got.Proxy != *tc.proxy) {
				t.Fatalf("unexpected proxy outcome: %s", raw)
			}
			if strings.Contains(string(raw), "private-") {
				t.Fatal("route preview leaked a proxy endpoint, credential, or health detail")
			}
		})
	}
}

func TestRoutePreviewRPCDistinguishesEngineOffFromProtection(t *testing.T) {
	svc, _, _ := newTestService(t)
	st := domain.DefaultSettings()
	st.KillSwitchGlobal, st.LANAccess, st.SecureDNS = true, false, true
	if err := svc.d.Settings.SaveSettings(t.Context(), st); err != nil {
		t.Fatal(err)
	}
	p := domain.ProxyProfile{ID: "active", Name: "Active", Host: "unused", Port: 1080, Enabled: true}
	if err := svc.d.Profiles.CreateProfile(t.Context(), &p); err != nil {
		t.Fatal(err)
	}
	path := `C:\Proxity\proxity-service.exe`
	savePreviewRule(t, svc, domain.Rule{
		Name: "Folder policy", Kind: domain.RuleKindFolder, Path: `C:\Proxity`,
		Action: domain.RuleActionProxy, ProxyID: p.ID, Enabled: true, KillSwitch: true,
	})
	client := routePreviewClient(t, svc)

	raw, err := client.Call("rules.previewRoute", map[string]string{"exe_path": path}, 3*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	var got RoutePreview
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if got.Engine.Running || got.Engine.CaptureAlive || !got.KillSwitchGlobal || !got.SecureDNS ||
		got.LANAccess || !got.ServiceExecutable || got.ExePolicy != RouteExeNone ||
		got.Action != domain.RuleActionProxy || got.Reason != RouteDeepestFolder ||
		got.Proxy == nil || got.Proxy.Availability != RouteProxyNotChecked || got.Proxy.FallbackAction != domain.RuleActionBlock {
		t.Fatalf("unexpected saved route or protection context: %s", raw)
	}
}

func TestRoutePreviewRPCRejectsMalformedExecutablePaths(t *testing.T) {
	svc, _, _ := newTestService(t)
	client := routePreviewClient(t, svc)
	for _, path := range []string{
		"", `report.exe`, `C:report.exe`, `C:\report.txt`, `C:\report.exe\`, `C:\no:ads\report.exe`,
		`\\.\C:\report.exe`, `\\?\C:\report.exe`, `C:\Apps\NUL.exe`, "C:\\\x00report.exe",
	} {
		t.Run(path, func(t *testing.T) {
			_, err := client.Call("rules.previewRoute", map[string]string{"exe_path": path}, 3*time.Second)
			var wireError *ipc.ErrObj
			if !errors.As(err, &wireError) || wireError.Code != "route.bad_exe_path" || wireError.Detail != "" {
				t.Fatalf("unexpected error for malformed path: %v", err)
			}
		})
	}
}
