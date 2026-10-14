//go:build windows

// Package appsvc is Proxity's application-service layer: the complete
// feature surface the GUI drives over IPC, implemented against the ports
// stores, the traffic engine and the kill-switch policy. Everything is
// directly testable without IPC via the exported methods.
package appsvc

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/engine"
	"github.com/Glimor/proxity/internal/killswitch"
	"github.com/Glimor/proxity/internal/ports"
	"github.com/google/uuid"
)

// Event is fanned out to IPC subscribers.
type Event struct {
	Type string `json:"type"` // "stats" | "status" | "blocked" | "conns"
	Data any    `json:"data,omitempty"`
}

// EngineAPI is the engine surface the service needs (the real
// *engine.Engine satisfies it; tests inject a fake).
type EngineAPI interface {
	Start(ctx context.Context) error
	Stop()
	Running() bool
	CaptureAlive() bool
	Listeners() int
	UpdateConfig(profiles []domain.ProxyProfile, rules []domain.Rule, s domain.Settings)
	Status() []domain.ProxyStatus
	Snapshot() domain.LiveSnapshot
	History() []domain.LiveSnapshot
	ActiveConnections() []domain.Connection
	RecheckProxies()
}

// KillSwitchAPI is the kill-switch surface the service needs.
type KillSwitchAPI interface {
	Reconcile(state killswitch.State) error
	SetHealthy(healthy bool) error
}

// SettingsAPI abstracts the settings store (sqlite.SettingsRepo).
type SettingsAPI interface {
	LoadSettings(ctx context.Context) (domain.Settings, error)
	SaveSettings(ctx context.Context, s domain.Settings) error
}

type Deps struct {
	Profiles   ports.ProfileStore
	Rules      ports.RuleStore
	Settings   SettingsAPI
	Stats      ports.StatsStore
	Blocked    ports.BlockedLogStore
	Cipher     ports.SecretCipher
	Engine     EngineAPI
	KillSwitch KillSwitchAPI
	Log        *slog.Logger
	SelfPath   string // defaults to os.Executable()
}

type Service struct {
	d   Deps
	log *slog.Logger

	mu            sync.Mutex
	engineHealthy bool

	subsMu sync.Mutex
	subs   map[chan Event]struct{}
}

func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	if d.SelfPath == "" {
		if p, err := os.Executable(); err == nil {
			d.SelfPath = p
		}
	}
	return &Service{d: d, log: d.Log, subs: make(map[chan Event]struct{})}
}

// ---------------------------------------------------------------------------
// event bus
// ---------------------------------------------------------------------------

// Subscribe returns an event channel and an unsubscribe func.
func (s *Service) Subscribe() (<-chan Event, func()) {
	ch := make(chan Event, 64)
	s.subsMu.Lock()
	s.subs[ch] = struct{}{}
	s.subsMu.Unlock()
	return ch, func() {
		s.subsMu.Lock()
		delete(s.subs, ch)
		s.subsMu.Unlock()
	}
}

func (s *Service) emit(t string, data any) {
	s.subsMu.Lock()
	defer s.subsMu.Unlock()
	for ch := range s.subs {
		select {
		case ch <- Event{Type: t, Data: data}:
		default: // slow subscriber: drop rather than block the service
		}
	}
}

// ---------------------------------------------------------------------------
// engine / kill-switch state
// ---------------------------------------------------------------------------

// engineAlive is the current healthy predicate: running AND capturing.
func (s *Service) engineAlive() bool {
	return s.d.Engine != nil && s.d.Engine.Running() && s.d.Engine.CaptureAlive()
}

// Refresh reloads persisted profiles/rules/settings into the engine and
// reconciles the kill switch. Called once at startup and internally after
// every mutation.
func (s *Service) Refresh(ctx context.Context) { s.refreshConfig(ctx) }

// refreshConfig reloads stores, pushes config into the engine and
// reconciles the kill switch. Called after every mutation.
func (s *Service) refreshConfig(ctx context.Context) {
	profiles, err := s.d.Profiles.ListProfiles(ctx)
	if err != nil {
		s.log.Error("refresh: list profiles", slog.String("err", err.Error()))
		return
	}
	rules, err := s.d.Rules.ListRules(ctx)
	if err != nil {
		s.log.Error("refresh: list rules", slog.String("err", err.Error()))
		return
	}
	settings, err := s.d.Settings.LoadSettings(ctx)
	if err != nil {
		s.log.Error("refresh: load settings", slog.String("err", err.Error()))
		return
	}
	if s.d.Engine != nil {
		s.d.Engine.UpdateConfig(profiles, rules, settings)
	}
	if s.d.KillSwitch != nil {
		s.mu.Lock()
		s.engineHealthy = s.engineAlive()
		s.mu.Unlock()
		if err := s.d.KillSwitch.Reconcile(s.ksState(rules, settings)); err != nil {
			s.log.Error("killswitch reconcile", slog.String("err", err.Error()))
		}
	}
}

// healthy returns the current engine-health flag under the mutex.
func (s *Service) healthy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.engineHealthy
}

// ksState derives the killswitch.State from enabled rules and settings.
// Only exe-kind rules can map to a WFP app-id; folder rules are enforced by
// the engine while it runs and cannot be covered when it is dead.
func (s *Service) ksState(rules []domain.Rule, st domain.Settings) killswitch.State {
	type entry struct{ p killswitch.AppPolicy }
	m := make(map[string]*entry)
	order := []string{}
	for _, r := range rules {
		if !r.Enabled || r.Kind != domain.RuleKindExe {
			continue
		}
		var blockOnly bool
		switch r.Action {
		case domain.RuleActionBlock:
			blockOnly = true
		case domain.RuleActionProxy:
			if !r.KillSwitch {
				continue
			}
		default:
			continue
		}
		fold := r.PathFold
		if fold == "" {
			fold = domain.FoldPath(r.Path)
		}
		if e, ok := m[fold]; ok {
			e.p.BlockOnly = e.p.BlockOnly || blockOnly
			continue
		}
		m[fold] = &entry{p: killswitch.AppPolicy{Path: r.Path, PathFold: fold, BlockOnly: blockOnly}}
		order = append(order, fold)
	}
	apps := make([]killswitch.AppPolicy, 0, len(order))
	for _, f := range order {
		apps = append(apps, m[f].p)
	}
	return killswitch.State{
		Apps:          apps,
		Global:        st.KillSwitchGlobal,
		LANAllow:      st.LANAccess,
		EngineHealthy: s.healthy(),
		SelfPath:      s.d.SelfPath,
	}
}

// OnCaptureDead is the engine.Deps hook: capture died → fail closed.
func (s *Service) OnCaptureDead() {
	s.mu.Lock()
	s.engineHealthy = false
	s.mu.Unlock()
	if s.d.KillSwitch != nil {
		if err := s.d.KillSwitch.SetHealthy(false); err != nil {
			s.log.Error("killswitch fail-closed", slog.String("err", err.Error()))
		}
	}
	s.emit("status", s.EngineStatus())
}

// StartEngine starts the traffic engine and raises the healthy permits.
func (s *Service) StartEngine(ctx context.Context) error {
	if s.d.Engine == nil {
		return domain.NewError("engine.unavailable", "Traffic engine is not available", "")
	}
	if err := s.d.Engine.Start(ctx); err != nil {
		s.OnCaptureDead()
		return domain.WrapError("engine.start_failed", "Traffic engine failed to start", err)
	}
	s.mu.Lock()
	s.engineHealthy = s.engineAlive()
	s.mu.Unlock()
	if s.d.KillSwitch != nil {
		if err := s.d.KillSwitch.SetHealthy(s.engineHealthy); err != nil {
			s.log.Error("killswitch set healthy", slog.String("err", err.Error()))
		}
	}
	s.emit("status", s.EngineStatus())
	return nil
}

func (s *Service) StopEngine() {
	if s.d.Engine != nil {
		s.d.Engine.Stop() // engine invokes OnCaptureDead via the hook
	}
	s.mu.Lock()
	s.engineHealthy = false
	s.mu.Unlock()
	if s.d.KillSwitch != nil {
		if err := s.d.KillSwitch.SetHealthy(false); err != nil {
			s.log.Error("killswitch set unhealthy", slog.String("err", err.Error()))
		}
	}
	s.emit("status", s.EngineStatus())
}

type EngineStatus struct {
	Running      bool `json:"running"`
	CaptureAlive bool `json:"capture_alive"`
	Listeners    int  `json:"listeners"`
}

func (s *Service) EngineStatus() EngineStatus {
	var st EngineStatus
	if s.d.Engine != nil {
		st.Running = s.d.Engine.Running()
		st.CaptureAlive = s.d.Engine.CaptureAlive()
		st.Listeners = s.d.Engine.Listeners()
	}
	return st
}

// ---------------------------------------------------------------------------
// profiles
// ---------------------------------------------------------------------------

type ProfileInput struct {
	Name            string  `json:"name"`
	Host            string  `json:"host"`
	Port            int     `json:"port"`
	Username        string  `json:"username"`
	Password        *string `json:"password"` // plaintext; nil = keep (update) / none (create)
	Enabled         bool    `json:"enabled"`
	TimeoutMS       int     `json:"timeout_ms"`
	RetryIntervalMS int     `json:"retry_interval_ms"`
	MaxRetries      int     `json:"max_retries"`
}

func (in *ProfileInput) validate() error {
	if strings.TrimSpace(in.Name) == "" {
		return domain.NewError("profile.empty_name", "Profile name is empty", "")
	}
	if strings.TrimSpace(in.Host) == "" {
		return domain.NewError("profile.empty_host", "Proxy host is empty", "")
	}
	if in.Port <= 0 || in.Port > 65535 {
		return domain.NewError("profile.bad_port", "Proxy port is invalid", "")
	}
	return nil
}

func (s *Service) ciphertext(password *string) ([]byte, error) {
	if password == nil || *password == "" {
		return nil, nil
	}
	if s.d.Cipher == nil {
		return nil, domain.NewError("secrets.unavailable", "Cannot store the password", "")
	}
	return s.d.Cipher.Protect([]byte(*password))
}

func (s *Service) CreateProfile(ctx context.Context, in ProfileInput) (*domain.ProxyProfile, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	ct, err := s.ciphertext(in.Password)
	if err != nil {
		return nil, err
	}
	p := &domain.ProxyProfile{
		ID: uuid.NewString(), Name: in.Name, Host: in.Host, Port: in.Port,
		Username: in.Username, PasswordCiphertext: ct, Enabled: in.Enabled,
		TimeoutMS: in.TimeoutMS, RetryIntervalMS: in.RetryIntervalMS,
		MaxRetries: in.MaxRetries,
	}
	if err := s.d.Profiles.CreateProfile(ctx, p); err != nil {
		return nil, err
	}
	s.refreshConfig(ctx)
	s.emit("status", s.EngineStatus())
	return p, nil
}

func (s *Service) UpdateProfile(ctx context.Context, id string, in ProfileInput) (*domain.ProxyProfile, error) {
	if err := in.validate(); err != nil {
		return nil, err
	}
	p, err := s.d.Profiles.GetProfile(ctx, id)
	if err != nil {
		return nil, err
	}
	p.Name, p.Host, p.Port, p.Username = in.Name, in.Host, in.Port, in.Username
	p.Enabled = in.Enabled
	p.TimeoutMS, p.RetryIntervalMS, p.MaxRetries = in.TimeoutMS, in.RetryIntervalMS, in.MaxRetries
	if in.Password != nil {
		ct, err := s.ciphertext(in.Password)
		if err != nil {
			return nil, err
		}
		p.PasswordCiphertext = ct
	}
	if err := s.d.Profiles.UpdateProfile(ctx, p); err != nil {
		return nil, err
	}
	s.refreshConfig(ctx)
	return p, nil
}

func (s *Service) DeleteProfile(ctx context.Context, id string) error {
	if err := s.d.Profiles.DeleteProfile(ctx, id); err != nil {
		return err
	}
	s.refreshConfig(ctx)
	return nil
}

func (s *Service) ListProfiles(ctx context.Context) ([]domain.ProxyProfile, error) {
	return s.d.Profiles.ListProfiles(ctx)
}

func (s *Service) SetProfileEnabled(ctx context.Context, id string, enabled bool) error {
	if err := s.d.Profiles.SetProfileEnabled(ctx, id, enabled); err != nil {
		return err
	}
	s.refreshConfig(ctx)
	return nil
}

type TestResult struct {
	LatencyMS int64  `json:"latency_ms"`
	PublicIP  string `json:"public_ip"`
	Error     string `json:"error,omitempty"`
}

// TestProfile runs a one-shot SOCKS5 health check through the profile.
func (s *Service) TestProfile(ctx context.Context, id string) TestResult {
	p, err := s.d.Profiles.GetProfile(ctx, id)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	var pw []byte
	if s.d.Cipher != nil && len(p.PasswordCiphertext) > 0 {
		pw, err = s.d.Cipher.Unprotect(p.PasswordCiphertext)
		if err != nil {
			return TestResult{Error: "password decrypt: " + err.Error()}
		}
	}
	tctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	lat, ip, err := engine.TestProxy(tctx, p, pw)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	return TestResult{LatencyMS: lat, PublicIP: ip}
}

func (s *Service) ProfileStatuses() []domain.ProxyStatus {
	if s.d.Engine == nil {
		return nil
	}
	return s.d.Engine.Status()
}

// ---------------------------------------------------------------------------
// rules
// ---------------------------------------------------------------------------

type RuleInput struct {
	Name           string            `json:"name"`
	Kind           domain.RuleKind   `json:"kind"`
	Path           string            `json:"path"`
	Action         domain.RuleAction `json:"action"`
	ProxyID        string            `json:"proxy_id"`
	Enabled        bool              `json:"enabled"`
	KillSwitch     bool              `json:"kill_switch"`
	IncludeSubdirs bool              `json:"include_subdirs"`
	Priority       int               `json:"priority"`
}

type RuleResult struct {
	Rule      *domain.Rule  `json:"rule"`
	Warnings  []string      `json:"warnings,omitempty"`
	Conflicts []domain.Rule `json:"conflicts,omitempty"`
}

func (s *Service) validateRule(ctx context.Context, in *RuleInput, excludeID string) (*domain.Rule, []string, []domain.Rule, error) {
	var warnings []string
	if strings.TrimSpace(in.Name) == "" {
		return nil, nil, nil, domain.NewError("rule.empty_name", "Rule name is empty", "")
	}
	if in.Kind != domain.RuleKindExe && in.Kind != domain.RuleKindFolder {
		return nil, nil, nil, domain.NewError("rule.bad_kind", "Rule kind must be exe or folder", string(in.Kind))
	}
	switch in.Action {
	case domain.RuleActionProxy, domain.RuleActionDirect, domain.RuleActionBlock:
	default:
		return nil, nil, nil, domain.NewError("rule.bad_action", "Rule action is invalid", string(in.Action))
	}
	if in.Action == domain.RuleActionProxy {
		if _, err := s.d.Profiles.GetProfile(ctx, in.ProxyID); err != nil {
			return nil, nil, nil, domain.NewError("rule.bad_proxy", "Selected proxy profile does not exist", in.ProxyID)
		}
	}
	r := &domain.Rule{
		ID: excludeID, Name: in.Name, Kind: in.Kind, Action: in.Action,
		ProxyID: in.ProxyID, Enabled: in.Enabled, KillSwitch: in.KillSwitch,
		IncludeSubdirs: in.IncludeSubdirs, Priority: in.Priority,
	}
	if err := domain.CanonicalizeRulePath(r, in.Path); err != nil {
		return nil, nil, nil, err
	}
	if in.Kind == domain.RuleKindExe {
		if _, err := os.Stat(r.Path); err != nil {
			warnings = append(warnings, "Executable does not exist: "+r.Path)
		}
	} else {
		if st, err := os.Stat(r.Path); err != nil || !st.IsDir() {
			warnings = append(warnings, "Folder does not exist: "+r.Path)
		}
	}
	// Conflicts: other rules targeting the exact same path — the matcher
	// picks one winner, so the GUI should be told.
	var conflicts []domain.Rule
	if all, err := s.d.Rules.ListRules(ctx); err == nil {
		for _, o := range all {
			if o.ID != excludeID && o.PathFold == r.PathFold && o.Kind == r.Kind {
				conflicts = append(conflicts, o)
			}
		}
	}
	return r, warnings, conflicts, nil
}

func (s *Service) CreateRule(ctx context.Context, in RuleInput) (*RuleResult, error) {
	r, warnings, conflicts, err := s.validateRule(ctx, &in, "")
	if err != nil {
		return nil, err
	}
	r.ID = uuid.NewString()
	if err := s.d.Rules.CreateRule(ctx, r); err != nil {
		return nil, err
	}
	s.refreshConfig(ctx)
	return &RuleResult{Rule: r, Warnings: warnings, Conflicts: conflicts}, nil
}

func (s *Service) UpdateRule(ctx context.Context, id string, in RuleInput) (*RuleResult, error) {
	r, warnings, conflicts, err := s.validateRule(ctx, &in, id)
	if err != nil {
		return nil, err
	}
	if err := s.d.Rules.UpdateRule(ctx, r); err != nil {
		return nil, err
	}
	s.refreshConfig(ctx)
	return &RuleResult{Rule: r, Warnings: warnings, Conflicts: conflicts}, nil
}

func (s *Service) DeleteRule(ctx context.Context, id string) error {
	if err := s.d.Rules.DeleteRule(ctx, id); err != nil {
		return err
	}
	s.refreshConfig(ctx)
	return nil
}

func (s *Service) ListRules(ctx context.Context) ([]domain.Rule, error) {
	return s.d.Rules.ListRules(ctx)
}

func (s *Service) SetRuleEnabled(ctx context.Context, id string, enabled bool) error {
	r, err := s.d.Rules.GetRule(ctx, id)
	if err != nil {
		return err
	}
	r.Enabled = enabled
	if err := s.d.Rules.UpdateRule(ctx, r); err != nil {
		return err
	}
	s.refreshConfig(ctx)
	return nil
}

func (s *Service) ToggleKillSwitch(ctx context.Context, id string, on bool) error {
	r, err := s.d.Rules.GetRule(ctx, id)
	if err != nil {
		return err
	}
	r.KillSwitch = on
	if err := s.d.Rules.UpdateRule(ctx, r); err != nil {
		return err
	}
	s.refreshConfig(ctx)
	return nil
}

// ---------------------------------------------------------------------------
// settings
// ---------------------------------------------------------------------------

func (s *Service) GetSettings(ctx context.Context) (domain.Settings, error) {
	return s.d.Settings.LoadSettings(ctx)
}

func (s *Service) UpdateSettings(ctx context.Context, st domain.Settings) (domain.Settings, error) {
	if err := s.d.Settings.SaveSettings(ctx, st); err != nil {
		return st, err
	}
	s.refreshConfig(ctx)
	return st, nil
}

// ---------------------------------------------------------------------------
// data / stats
// ---------------------------------------------------------------------------

func (s *Service) Snapshot() domain.LiveSnapshot {
	if s.d.Engine == nil {
		return domain.LiveSnapshot{}
	}
	return s.d.Engine.Snapshot()
}

func (s *Service) History() []domain.LiveSnapshot {
	if s.d.Engine == nil {
		return nil
	}
	return s.d.Engine.History()
}

func (s *Service) ActiveConnections() []domain.Connection {
	if s.d.Engine == nil {
		return nil
	}
	return s.d.Engine.ActiveConnections()
}

func (s *Service) BlockedLog(ctx context.Context, limit int) ([]domain.BlockedEntry, error) {
	return s.d.Blocked.List(ctx, limit)
}

func (s *Service) ClearBlocked(ctx context.Context) error {
	return s.d.Blocked.Clear(ctx)
}

func (s *Service) ExeStats(ctx context.Context, fromDay, toDay string) ([]domain.ExeDailyStats, error) {
	return s.d.Stats.ExeDailyRange(ctx, fromDay, toDay)
}

func (s *Service) ProxyStats(ctx context.Context, fromDay, toDay string) ([]domain.ProxyDailyStats, error) {
	return s.d.Stats.ProxyDailyRange(ctx, fromDay, toDay)
}

// ResetStats clears stats for one exe (exePath non-empty) or all stats.
func (s *Service) ResetStats(ctx context.Context, exePath string) error {
	if exePath == "" {
		return s.d.Stats.ResetAll(ctx)
	}
	c, err := domain.CanonicalPath(exePath)
	if err != nil {
		return err
	}
	return s.d.Stats.ResetExe(ctx, domain.FoldPath(c))
}

// ---------------------------------------------------------------------------
// import / export
// ---------------------------------------------------------------------------

type ExportProfile struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Host               string `json:"host"`
	Port               int    `json:"port"`
	Username           string `json:"username"`
	PasswordCiphertext []byte `json:"password_ciphertext,omitempty"` // base64 in JSON
	Enabled            bool   `json:"enabled"`
	TimeoutMS          int    `json:"timeout_ms"`
	RetryIntervalMS    int    `json:"retry_interval_ms"`
	MaxRetries         int    `json:"max_retries"`
}

// ExportProfiles dumps all profiles; ciphertext is stripped unless
// includeSecrets is set.
func (s *Service) ExportProfiles(ctx context.Context, includeSecrets bool) ([]ExportProfile, error) {
	profiles, err := s.d.Profiles.ListProfiles(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]ExportProfile, 0, len(profiles))
	for _, p := range profiles {
		ep := ExportProfile{
			ID: p.ID, Name: p.Name, Host: p.Host, Port: p.Port,
			Username: p.Username, Enabled: p.Enabled, TimeoutMS: p.TimeoutMS,
			RetryIntervalMS: p.RetryIntervalMS, MaxRetries: p.MaxRetries,
		}
		if includeSecrets {
			ep.PasswordCiphertext = p.PasswordCiphertext
		}
		out = append(out, ep)
	}
	return out, nil
}

type ImportResult struct {
	Created int      `json:"created"`
	Updated int      `json:"updated"`
	Skipped []string `json:"skipped,omitempty"`
}

// ImportProfiles upserts exported profiles. Existing ids are skipped unless
// overwrite is set (then updated). Entries without an id get a fresh one.
func (s *Service) ImportProfiles(ctx context.Context, list []ExportProfile, overwrite bool) (ImportResult, error) {
	var res ImportResult
	for _, ep := range list {
		p := &domain.ProxyProfile{
			ID: ep.ID, Name: ep.Name, Host: ep.Host, Port: ep.Port,
			Username: ep.Username, PasswordCiphertext: ep.PasswordCiphertext,
			Enabled: ep.Enabled, TimeoutMS: ep.TimeoutMS,
			RetryIntervalMS: ep.RetryIntervalMS, MaxRetries: ep.MaxRetries,
		}
		if p.ID != "" {
			if _, err := s.d.Profiles.GetProfile(ctx, p.ID); err == nil {
				if !overwrite {
					res.Skipped = append(res.Skipped, p.Name)
					continue
				}
				if err := s.d.Profiles.UpdateProfile(ctx, p); err != nil {
					return res, err
				}
				res.Updated++
				continue
			}
		}
		if p.ID == "" {
			p.ID = uuid.NewString()
		}
		if err := s.d.Profiles.CreateProfile(ctx, p); err != nil {
			return res, err
		}
		res.Created++
	}
	s.refreshConfig(ctx)
	return res, nil
}

// ---------------------------------------------------------------------------
// misc
// ---------------------------------------------------------------------------

// ProcessInfo describes one running process for the rule picker.
type ProcessInfo struct {
	PID  int    `json:"pid"`
	Name string `json:"name"`
	Path string `json:"path"` // canonical exe path ("" when not resolvable)
}

// RunEventPump broadcasts "stats"/"conns" events on a ticker and "blocked"
// whenever new blocked entries appear. Runs until ctx is done.
func (s *Service) RunEventPump(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = time.Second
	}
	t := time.NewTicker(interval)
	defer t.Stop()
	var lastBlockedID int64 = -1
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.emit("stats", s.Snapshot())
			s.emit("conns", s.ActiveConnections())
			if s.d.Blocked != nil {
				if list, err := s.d.Blocked.List(ctx, 20); err == nil {
					var newest int64
					if len(list) > 0 {
						newest = list[0].ID
					}
					if lastBlockedID < 0 {
						lastBlockedID = newest
					} else if newest != lastBlockedID {
						lastBlockedID = newest
						s.emit("blocked", list)
					}
				}
			}
		}
	}
}
