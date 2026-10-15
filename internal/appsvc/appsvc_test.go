//go:build windows

package appsvc

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/killswitch"
	"github.com/Glimor/proxity/internal/storage/sqlite"
)

// --- fakes ------------------------------------------------------------------

type fakeEngine struct {
	running, alive bool
	profiles       []domain.ProxyProfile
	startCtx       context.Context
	rules          []domain.Rule
	settings       domain.Settings
	rechecks       int
}

func (f *fakeEngine) Start(ctx context.Context) error {
	f.startCtx = ctx
	f.running, f.alive = true, true
	return nil
}
func (f *fakeEngine) Stop()              { f.running, f.alive = false, false }
func (f *fakeEngine) Running() bool      { return f.running }
func (f *fakeEngine) CaptureAlive() bool { return f.alive }
func (f *fakeEngine) Listeners() int     { return 2 }
func (f *fakeEngine) UpdateConfig(p []domain.ProxyProfile, r []domain.Rule, s domain.Settings) {
	f.profiles, f.rules, f.settings = p, r, s
}
func (f *fakeEngine) Status() []domain.ProxyStatus           { return nil }
func (f *fakeEngine) Snapshot() domain.LiveSnapshot          { return domain.LiveSnapshot{} }
func (f *fakeEngine) History() []domain.LiveSnapshot         { return nil }
func (f *fakeEngine) ActiveConnections() []domain.Connection { return nil }
func (f *fakeEngine) RecheckProxies()                        { f.rechecks++ }

type fakeKS struct {
	states     []killswitch.State
	healthyLog []bool
}

func (f *fakeKS) Reconcile(st killswitch.State) error {
	f.states = append(f.states, st)
	return nil
}
func (f *fakeKS) SetHealthy(h bool) error {
	f.healthyLog = append(f.healthyLog, h)
	return nil
}

type fakeCipher struct{}

func (fakeCipher) Protect(p []byte) ([]byte, error) {
	out := make([]byte, len(p))
	for i, b := range p {
		out[i] = b ^ 0x5a
	}
	return out, nil
}
func (fakeCipher) Unprotect(c []byte) ([]byte, error) {
	out := make([]byte, len(c))
	for i, b := range c {
		out[i] = b ^ 0x5a
	}
	return out, nil
}

func newTestService(t *testing.T) (*Service, *fakeEngine, *fakeKS) {
	t.Helper()
	db, err := sqlite.Open(filepath.Join(t.TempDir(), "proxity.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	eng := &fakeEngine{}
	ks := &fakeKS{}
	svc := New(Deps{
		Profiles:   db.Profiles(),
		Rules:      db.Rules(),
		Settings:   db.Settings(),
		Stats:      db.Stats(),
		Blocked:    db.BlockedLog(),
		Cipher:     fakeCipher{},
		Engine:     eng,
		KillSwitch: ks,
		SelfPath:   `C:\Proxity\proxity-service.exe`,
	})
	return svc, eng, ks
}

// --- tests ------------------------------------------------------------------

func TestProfileCRUD(t *testing.T) {
	svc, eng, _ := newTestService(t)
	ctx := context.Background()

	pw := "secret"
	p, err := svc.CreateProfile(ctx, ProfileInput{
		Name: "home", Host: "proxy.local", Port: 1080,
		Username: "u", Password: &pw, Enabled: true, TimeoutMS: 5000,
	})
	if err != nil {
		t.Fatal(err)
	}
	if p.ID == "" || len(p.PasswordCiphertext) == 0 {
		t.Fatal("profile not populated")
	}

	list, err := svc.ListProfiles(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v n=%d", err, len(list))
	}
	if eng.profiles[0].ID != p.ID {
		t.Fatal("engine config not refreshed")
	}

	if _, err := svc.UpdateProfile(ctx, p.ID, ProfileInput{
		Name: "home2", Host: "proxy.local", Port: 1080, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.ListProfiles(ctx)
	if got[0].Name != "home2" {
		t.Fatal("update not applied")
	}
	// Password was nil on update → ciphertext preserved.
	if len(got[0].PasswordCiphertext) == 0 {
		t.Fatal("password wiped on update")
	}

	if err := svc.SetProfileEnabled(ctx, p.ID, false); err != nil {
		t.Fatal(err)
	}
	if err := svc.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.ListProfiles(ctx)
	if len(list) != 0 {
		t.Fatal("delete failed")
	}
}

func TestProfileValidation(t *testing.T) {
	svc, _, _ := newTestService(t)
	if _, err := svc.CreateProfile(context.Background(), ProfileInput{Name: "x"}); err == nil {
		t.Fatal("expected validation error for missing host/port")
	}
}

func TestRuleCRUDAndConflicts(t *testing.T) {
	svc, _, ks := newTestService(t)
	ctx := context.Background()

	exe, _ := os.Executable()

	p, err := svc.CreateProfile(ctx, ProfileInput{
		Name: "p", Host: "h", Port: 1080, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	r1, err := svc.CreateRule(ctx, RuleInput{
		Name: "r1", Kind: domain.RuleKindExe, Path: exe,
		Action: domain.RuleActionProxy, ProxyID: p.ID, Enabled: true,
		KillSwitch: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r1.Warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", r1.Warnings)
	}

	// Same-path rule → conflict reported.
	r2, err := svc.CreateRule(ctx, RuleInput{
		Name: "r2", Kind: domain.RuleKindExe, Path: exe,
		Action: domain.RuleActionBlock, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r2.Conflicts) != 1 || r2.Conflicts[0].ID != r1.Rule.ID {
		t.Fatalf("expected conflict with r1, got %+v", r2.Conflicts)
	}

	// Nonexistent exe → warning, not error.
	r3, err := svc.CreateRule(ctx, RuleInput{
		Name: "r3", Kind: domain.RuleKindExe, Path: `C:\nope\missing.exe`,
		Action: domain.RuleActionDirect, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(r3.Warnings) == 0 {
		t.Fatal("expected missing-exe warning")
	}

	// Kill-switch state reached the policy: r1 (proxy+ks) and r2 (block)
	// share the same path → single deduped app, BlockOnly wins.
	last := ks.states[len(ks.states)-1]
	found := 0
	for _, a := range last.Apps {
		if a.PathFold == domain.FoldPath(r1.Rule.Path) {
			found++
			if !a.BlockOnly {
				t.Fatal("expected BlockOnly to win on conflicting rule")
			}
		}
	}
	if found != 1 {
		t.Fatalf("expected 1 deduped app entry, got %d", found)
	}
	// r3 is direct → not protected.
	for _, a := range last.Apps {
		if a.PathFold == domain.FoldPath(r3.Rule.Path) {
			t.Fatal("direct rule should not appear in kill-switch state")
		}
	}

	if err := svc.DeleteRule(ctx, r2.Rule.ID); err != nil {
		t.Fatal(err)
	}
	last = ks.states[len(ks.states)-1]
	for _, a := range last.Apps {
		if a.PathFold == domain.FoldPath(r1.Rule.Path) && a.BlockOnly {
			t.Fatal("block-only flag stuck after deleting block rule")
		}
	}
}

func TestSettingsDriveGlobalKillSwitch(t *testing.T) {
	svc, eng, ks := newTestService(t)
	ctx := context.Background()

	st, err := svc.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st.KillSwitchGlobal = true
	st.LANAccess = false
	if _, err := svc.UpdateSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	last := ks.states[len(ks.states)-1]
	if !last.Global || last.LANAllow {
		t.Fatalf("bad ks state: %+v", last)
	}
	if !eng.settings.KillSwitchGlobal {
		t.Fatal("engine settings not refreshed")
	}
}

func TestEngineStartStopWiresKillSwitch(t *testing.T) {
	svc, eng, ks := newTestService(t)
	ctx := context.Background()

	if err := svc.StartEngine(ctx); err != nil {
		t.Fatal(err)
	}
	if !eng.Running() || len(ks.healthyLog) == 0 || !ks.healthyLog[len(ks.healthyLog)-1] {
		t.Fatal("kill switch not set healthy")
	}
	if s := svc.EngineStatus(); !s.Running || !s.CaptureAlive || s.Listeners != 2 {
		t.Fatalf("bad engine status %+v", s)
	}

	// Capture death → fail closed.
	svc.OnCaptureDead()
	if ks.healthyLog[len(ks.healthyLog)-1] != false {
		t.Fatal("capture-dead did not fail closed")
	}

	svc.StopEngine()
	if eng.Running() {
		t.Fatal("engine still running")
	}
	if ks.healthyLog[len(ks.healthyLog)-1] != false {
		t.Fatal("stop did not unset healthy")
	}
}

func TestExportImportStripsSecrets(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	pw := "secret"
	p, err := svc.CreateProfile(ctx, ProfileInput{
		Name: "a", Host: "h", Port: 1080, Password: &pw, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	exp, err := svc.ExportProfiles(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(exp) != 1 || exp[0].PasswordCiphertext != nil {
		t.Fatal("export leaked ciphertext")
	}
	exp, err = svc.ExportProfiles(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(exp[0].PasswordCiphertext) == 0 {
		t.Fatal("export with secrets lost ciphertext")
	}

	// Reimport same id without overwrite → skipped; with overwrite → updated.
	res, err := svc.ImportProfiles(ctx, exp, false)
	if err != nil || res.Created != 0 || len(res.Skipped) != 1 {
		t.Fatalf("import: %+v err=%v", res, err)
	}
	exp[0].Name = "renamed"
	res, err = svc.ImportProfiles(ctx, exp, true)
	if err != nil || res.Updated != 1 {
		t.Fatalf("import overwrite: %+v err=%v", res, err)
	}
	got, err := svc.ListProfiles(ctx)
	if err != nil || got[0].Name != "renamed" || got[0].ID != p.ID {
		t.Fatal("overwrite import failed")
	}
}

func TestBlockedAndStatsPlumbing(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	if err := svc.d.Blocked.Log(ctx, domain.BlockedEntry{ExePath: "X", Dst: "1.2.3.4", Port: 443}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.BlockedLog(ctx, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("blocked list: %v n=%d", err, len(list))
	}
	if err := svc.ClearBlocked(ctx); err != nil {
		t.Fatal(err)
	}
	list, _ = svc.BlockedLog(ctx, 10)
	if len(list) != 0 {
		t.Fatal("clear failed")
	}

	if err := svc.ResetStats(ctx, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.ExeStats(ctx, "2024-01-01", "2024-12-31"); err != nil {
		t.Fatal(err)
	}
}

// A proxy rule without kill switch still lands in the kill-switch state —
// flagged UDPBlock so its unsupported UDP egress is blocked at WFP level.
// A kill-switch proxy rule on the same path wins over the UDP-only mode.
func TestProxyNoKillSwitchGetsUDPBlock(t *testing.T) {
	svc, _, ks := newTestService(t)
	ctx := context.Background()

	exe, _ := os.Executable()
	exe2 := exe + ".other" // distinct fold, still maps to an app policy

	p, err := svc.CreateProfile(ctx, ProfileInput{
		Name: "p", Host: "h", Port: 1080, Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := svc.CreateRule(ctx, RuleInput{
		Name: "noKS", Kind: domain.RuleKindExe, Path: exe,
		Action: domain.RuleActionProxy, ProxyID: p.ID, Enabled: true,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateRule(ctx, RuleInput{
		Name: "withKS", Kind: domain.RuleKindExe, Path: exe2,
		Action: domain.RuleActionProxy, ProxyID: p.ID, Enabled: true,
		KillSwitch: true,
	}); err != nil {
		t.Fatal(err)
	}

	last := ks.states[len(ks.states)-1]
	seen := map[string]killswitch.AppPolicy{}
	for _, a := range last.Apps {
		seen[a.PathFold] = a
	}
	noKS, ok := seen[domain.FoldPath(exe)]
	if !ok || !noKS.UDPBlock || noKS.BlockOnly {
		t.Fatalf("no-KS proxy app missing UDPBlock: %+v", last.Apps)
	}
	withKS, ok := seen[domain.FoldPath(exe2)]
	if !ok || withKS.UDPBlock || withKS.BlockOnly {
		t.Fatalf("KS proxy app should be plain protected entry: %+v", last.Apps)
	}
}

func TestSecureDNSSettingReachesKillSwitch(t *testing.T) {
	svc, _, ks := newTestService(t)
	ctx := context.Background()

	st, err := svc.GetSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	st.SecureDNS = true
	if _, err := svc.UpdateSettings(ctx, st); err != nil {
		t.Fatal(err)
	}
	last := ks.states[len(ks.states)-1]
	if !last.SecureDNS {
		t.Fatalf("SecureDNS not propagated: %+v", last)
	}
	if last.SelfPath == "" {
		t.Fatal("dns self-permit needs SelfPath")
	}
}

func TestExportStatsCSV(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	err := svc.d.Stats.UpsertExeDaily(ctx, domain.ExeDailyStats{
		Day: "2024-06-01", ExePath: `C:\app.exe`, ProxyID: "p1",
		Down: 100, Up: 50, Conns: 2, Seconds: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	err = svc.d.Stats.UpsertProxyDaily(ctx, domain.ProxyDailyStats{
		Day: "2024-06-01", ProxyID: "p1", Down: 100, Up: 50, Conns: 2, Seconds: 30,
	})
	if err != nil {
		t.Fatal(err)
	}

	csvText, err := svc.ExportStatsCSV(ctx, "2024-05-01", "2024-07-01", "all")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(csvText, "day,exe_path,proxy_id,down,up,conns,seconds") {
		t.Fatalf("missing header: %q", csvText)
	}
	if !strings.Contains(csvText, `2024-06-01,C:\app.exe,p1,100,50,2,30`) {
		t.Fatalf("missing exe row: %q", csvText)
	}
	if !strings.Contains(csvText, `2024-06-01,,p1,100,50,2,30`) {
		t.Fatalf("missing proxy row: %q", csvText)
	}
	// Range exclusion.
	out, err := svc.ExportStatsCSV(ctx, "2024-06-02", "2024-07-01", "exe")
	if err != nil || strings.Contains(out, "app.exe") {
		t.Fatalf("range filter failed: %q %v", out, err)
	}
}

func TestLastSessionAndRetention(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()

	// None yet.
	if s, err := svc.LastSession(ctx); err != nil || s != nil {
		t.Fatalf("expected no last session, got %+v err=%v", s, err)
	}
	if err := svc.PersistSession(ctx); err != nil {
		t.Fatal(err)
	}
	s, err := svc.LastSession(ctx)
	if err != nil || s == nil || s.EndedAt.IsZero() {
		t.Fatalf("last session not persisted: %+v err=%v", s, err)
	}

	// Retention: an old row is purged, a fresh one kept.
	old := time.Now().AddDate(0, 0, -401).Format("2006-01-02")
	today := time.Now().Format("2006-01-02")
	if err := svc.d.Stats.UpsertExeDaily(ctx, domain.ExeDailyStats{
		Day: old, ExePath: "old.exe", Down: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := svc.d.Stats.UpsertExeDaily(ctx, domain.ExeDailyStats{
		Day: today, ExePath: "new.exe", Down: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// Seed blocked log just over the cap with cheap inserts? Insert a few
	// and trim to a small bound via the repo to verify Trim works.
	for i := 0; i < 10; i++ {
		if err := svc.d.Blocked.Log(ctx, domain.BlockedEntry{ExePath: "x", Dst: "1.2.3.4", Port: 443}); err != nil {
			t.Fatal(err)
		}
	}
	if n, err := svc.d.Blocked.Trim(ctx, 5); err != nil || n != 5 {
		t.Fatalf("trim: n=%d err=%v", n, err)
	}
	if err := svc.RunRetention(ctx); err != nil {
		t.Fatal(err)
	}
	rows, err := svc.ExeStats(ctx, "2000-01-01", "2999-12-31")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ExePath != "new.exe" {
		t.Fatalf("retention kept wrong rows: %+v", rows)
	}
}
