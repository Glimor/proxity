package sqlite

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/google/uuid"
)

func openTestDB(t *testing.T) (*DB, string) {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	db, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db, path
}

func TestMigrateIdempotent(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	if err := db.Migrate(ctx); err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	var n int
	if err := db.SQL.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("expected 1 recorded migration, got %d", n)
	}
}

func TestProfileCRUD(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	repo := db.Profiles()

	p := &domain.ProxyProfile{
		ID: uuid.NewString(), Name: "work", Host: "proxy.local", Port: 1080,
		Username: "u", PasswordCiphertext: []byte{1, 2, 3}, Enabled: true,
		TimeoutMS: 8000, RetryIntervalMS: 3000, MaxRetries: 2,
	}
	if err := repo.CreateProfile(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.GetProfile(ctx, p.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "work" || got.Port != 1080 || !got.Enabled ||
		string(got.PasswordCiphertext) != string([]byte{1, 2, 3}) {
		t.Fatalf("round trip mismatch: %+v", got)
	}

	got.Name = "work2"
	got.Enabled = false
	if err := repo.UpdateProfile(ctx, got); err != nil {
		t.Fatalf("update: %v", err)
	}
	got2, _ := repo.GetProfile(ctx, p.ID)
	if got2.Name != "work2" || got2.Enabled {
		t.Fatalf("update mismatch: %+v", got2)
	}

	if err := repo.SetProfileEnabled(ctx, p.ID, true); err != nil {
		t.Fatalf("set enabled: %v", err)
	}
	got3, _ := repo.GetProfile(ctx, p.ID)
	if !got3.Enabled {
		t.Fatal("SetProfileEnabled did not take effect")
	}

	list, err := repo.ListProfiles(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v len=%d", err, len(list))
	}
	if err := repo.DeleteProfile(ctx, p.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if _, err := repo.GetProfile(ctx, p.ID); err == nil {
		t.Fatal("expected not found after delete")
	}
}

func TestRuleCRUDAndOrdering(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	repo := db.Rules()

	mk := func(name string, kind domain.RuleKind, path string, prio int) *domain.Rule {
		r := &domain.Rule{
			ID: uuid.NewString(), Name: name, Kind: kind,
			Action: domain.RuleActionProxy, ProxyID: "p1", Enabled: true,
			IncludeSubdirs: true, Priority: prio,
		}
		if err := domain.CanonicalizeRulePath(r, path); err != nil {
			t.Fatalf("canonicalize %s: %v", path, err)
		}
		return r
	}

	exe := mk("exe-rule", domain.RuleKindExe, `c:\tools\app.exe`, 0)
	shallow := mk("shallow", domain.RuleKindFolder, `C:\`, 0)
	deep := mk("deep", domain.RuleKindFolder, `c:/tools/`, 0)
	for _, r := range []*domain.Rule{shallow, deep, exe} {
		if err := repo.CreateRule(ctx, r); err != nil {
			t.Fatalf("create %s: %v", r.Name, err)
		}
	}

	got, err := repo.GetRule(ctx, exe.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	want := domain.FoldPath(`C:\tools\app.exe`)
	if got.PathFold != want {
		t.Fatalf("path_fold = %q want %q", got.PathFold, want)
	}
	if got.Path != `C:\tools\app.exe` {
		t.Fatalf("path = %q", got.Path)
	}

	rules, err := repo.ListRules(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("len=%d", len(rules))
	}
	if rules[0].ID != exe.ID {
		t.Fatalf("exe rule should sort first, got %s", rules[0].Name)
	}
	if rules[1].ID != deep.ID || rules[2].ID != shallow.ID {
		t.Fatalf("expected deep folder before shallow, got %s then %s",
			rules[1].Name, rules[2].Name)
	}

	exe.Enabled = false
	if err := repo.UpdateRule(ctx, exe); err != nil {
		t.Fatalf("update: %v", err)
	}
	if err := repo.DeleteRule(ctx, shallow.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	rules, _ = repo.ListRules(ctx)
	if len(rules) != 2 {
		t.Fatalf("len after delete=%d", len(rules))
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	repo := db.Settings()

	s := domain.DefaultSettings()
	s.Language = domain.LangTR
	s.Theme = "dark"
	s.SecureDNS = true
	s.DNSUpstream = "https://dns.example/dns-query"
	s.DefaultAction = domain.DecisionBlock
	if err := repo.SaveSettings(ctx, s); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := repo.LoadSettings(ctx)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got.Language != domain.LangTR || got.Theme != "dark" ||
		!got.SecureDNS || got.DefaultAction != domain.DecisionBlock {
		t.Fatalf("settings mismatch: %+v", got)
	}

	var v struct{ A int }
	if err := repo.Set(ctx, "k", struct{ A int }{A: 1}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Set(ctx, "k", struct{ A int }{A: 2}); err != nil {
		t.Fatal(err)
	}
	found, err := repo.Get(ctx, "k", &v)
	if err != nil || !found || v.A != 2 {
		t.Fatalf("upsert: found=%v v=%+v err=%v", found, v, err)
	}
	found, _ = repo.Get(ctx, "missing", &v)
	if found {
		t.Fatal("missing key reported found")
	}
}

func TestStatsAndBlockedLog(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	repo := db.Stats()

	for i := 0; i < 3; i++ {
		if err := repo.AddFlowDelta(ctx, `C:\app.exe`, "p1", 100, 200); err != nil {
			t.Fatal(err)
		}
	}
	up, down, err := repo.FlowTotals(ctx, `flow:exe:C:\app.exe`)
	if err != nil || up != 300 || down != 600 {
		t.Fatalf("flow totals: up=%d down=%d err=%v", up, down, err)
	}
	up, down, _ = repo.FlowTotals(ctx, "flow:total")
	if up != 300 || down != 600 {
		t.Fatalf("global totals: %d/%d", up, down)
	}

	repo.RecordOutcome(ctx, domain.ConnBlocked)
	repo.RecordOutcome(ctx, domain.ConnBlocked)
	repo.RecordOutcome(ctx, domain.ConnFailed)
	repo.RecordOutcome(ctx, domain.ConnClosed)
	c, err := repo.SessionCounters(ctx)
	if err != nil || c.Blocked != 2 || c.Failed != 1 || c.Succeeded != 1 {
		t.Fatalf("counters: %+v err=%v", c, err)
	}

	ds := domain.ExeDailyStats{Day: "2026-01-01", ExePath: `C:\app.exe`, ProxyID: "p1", Down: 10, Up: 5, Conns: 1, Seconds: 30}
	repo.UpsertExeDaily(ctx, ds)
	repo.UpsertExeDaily(ctx, ds)
	repo.UpsertExeDaily(ctx, domain.ExeDailyStats{Day: "2026-01-02", ExePath: `C:\app.exe`, ProxyID: "p1", Down: 1})
	rows, err := repo.ExeDailyRange(ctx, "2026-01-01", "2026-01-01")
	if err != nil || len(rows) != 1 {
		t.Fatalf("range: %v len=%d", err, len(rows))
	}
	if rows[0].Down != 20 || rows[0].Up != 10 || rows[0].Conns != 2 || rows[0].Seconds != 60 {
		t.Fatalf("daily aggregate: %+v", rows[0])
	}

	repo.UpsertProxyDaily(ctx, domain.ProxyDailyStats{Day: "2026-01-01", ProxyID: "p1", Down: 7})
	prows, err := repo.ProxyDailyRange(ctx, "2026-01-01", "2026-01-31")
	if err != nil || len(prows) != 1 || prows[0].Down != 7 {
		t.Fatalf("proxy range: %+v err=%v", prows, err)
	}

	if err := repo.ResetExe(ctx, `C:\app.exe`); err != nil {
		t.Fatal(err)
	}
	rows, _ = repo.ExeDailyRange(ctx, "2026-01-01", "2026-12-31")
	if len(rows) != 0 {
		t.Fatalf("expected no exe rows after reset, got %d", len(rows))
	}
	if err := repo.ResetAll(ctx); err != nil {
		t.Fatal(err)
	}
	c, _ = repo.SessionCounters(ctx)
	if c.Blocked != 0 || c.Failed != 0 {
		t.Fatalf("counters not reset: %+v", c)
	}

	b := db.BlockedLog()
	e1 := domain.BlockedEntry{ExePath: `C:\app.exe`, PID: 42, Dst: "1.2.3.4", Port: 443, Proto: "tcp", Reason: "rule:block"}
	if err := b.Log(ctx, e1); err != nil {
		t.Fatal(err)
	}
	e2 := domain.BlockedEntry{ExePath: `C:\b.exe`, PID: 7, Dst: "5.6.7.8", Port: 80, Proto: "tcp", Reason: "killswitch"}
	b.Log(ctx, e2)
	entries, err := b.List(ctx, 10)
	if err != nil || len(entries) != 2 {
		t.Fatalf("blocked list: %v len=%d", err, len(entries))
	}
	if entries[0].ExePath != `C:\b.exe` {
		t.Fatal("expected newest first")
	}
	if err := b.Clear(ctx); err != nil {
		t.Fatal(err)
	}
	entries, _ = b.List(ctx, 10)
	if len(entries) != 0 {
		t.Fatal("blocked log not cleared")
	}
}

func TestCorruptDBRecovery(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.db")
	if err := os.WriteFile(path, []byte("this is not a sqlite database at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	db, err := Open(path)
	var cerr *CorruptDBError
	if !errors.As(err, &cerr) {
		t.Fatalf("expected CorruptDBError, got %v", err)
	}
	if db == nil {
		t.Fatal("expected usable DB after recovery")
	}
	defer db.Close()
	if cerr.BackupPath == "" {
		t.Fatal("no backup path reported")
	}
	if _, err := os.Stat(cerr.BackupPath); err != nil {
		t.Fatalf("backup file missing: %v", err)
	}
	if err := db.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate on recovered db: %v", err)
	}
}

func TestCanonicalPath(t *testing.T) {
	c, err := domain.CanonicalPath(`c:/tools/../tools/app.exe`)
	if err != nil {
		t.Fatal(err)
	}
	if c != `C:\tools\app.exe` {
		t.Fatalf("canonical = %q", c)
	}
	if domain.FoldPath(`C:\Tools\App.EXE`) != `C:\TOOLS\APP.EXE` {
		t.Fatal("fold")
	}
}

func TestMatchRule(t *testing.T) {
	fold := func(p string) string { c, _ := domain.CanonicalPath(p); return domain.FoldPath(c) }
	rule := func(kind domain.RuleKind, p string, prio int) domain.Rule {
		var r domain.Rule
		r.Kind, r.Enabled, r.IncludeSubdirs, r.Priority = kind, true, true, prio
		domain.CanonicalizeRulePath(&r, p)
		return r
	}

	exe := rule(domain.RuleKindExe, `C:\tools\app.exe`, 0)
	folder := rule(domain.RuleKindFolder, `C:\tools`, 0)
	deeper := rule(domain.RuleKindFolder, `C:\tools\sub`, 0)
	disabled := rule(domain.RuleKindExe, `C:\tools\app.exe`, 99)
	disabled.Enabled = false

	got := domain.MatchRule([]domain.Rule{folder, exe, deeper}, fold(`C:\Tools\App.Exe`))
	if got == nil || got.PathFold != exe.PathFold {
		t.Fatal("exact exe rule must beat folder rules")
	}
	got = domain.MatchRule([]domain.Rule{folder, deeper}, fold(`C:\tools\sub\x\y.exe`))
	if got == nil || got.PathFold != deeper.PathFold {
		t.Fatal("deepest folder must win")
	}
	got = domain.MatchRule([]domain.Rule{exe, deeper, folder}, fold(`C:\tools\app.exe`))
	if got == nil || got.PathFold != exe.PathFold {
		t.Fatal("exe exact match")
	}
	noSub := rule(domain.RuleKindFolder, `C:\tools`, 0)
	noSub.IncludeSubdirs = false
	if domain.MatchRule([]domain.Rule{noSub}, fold(`C:\tools\sub\y.exe`)) != nil {
		t.Fatal("IncludeSubdirs=false must not match nested paths")
	}
	if domain.MatchRule([]domain.Rule{noSub}, fold(`C:\tools\y.exe`)) == nil {
		t.Fatal("direct child of folder must match")
	}
	if domain.MatchRule([]domain.Rule{disabled}, fold(`C:\tools\app.exe`)) != nil {
		t.Fatal("disabled rule must not match")
	}
	if domain.MatchRule(nil, fold(`C:\x.exe`)) != nil {
		t.Fatal("no rules -> nil")
	}
}
