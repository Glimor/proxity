package ports

import (
	"context"

	"github.com/Glimor/proxity/internal/domain"
)

type ProfileStore interface {
	CreateProfile(ctx context.Context, p *domain.ProxyProfile) error
	UpdateProfile(ctx context.Context, p *domain.ProxyProfile) error
	DeleteProfile(ctx context.Context, id string) error
	GetProfile(ctx context.Context, id string) (*domain.ProxyProfile, error)
	ListProfiles(ctx context.Context) ([]domain.ProxyProfile, error)
	SetProfileEnabled(ctx context.Context, id string, enabled bool) error
}

// RuleStore lists rules ordered so that exact exe rules come first, then
// folder rules deepest-path-first — matching domain.MatchRule semantics.
type RuleStore interface {
	CreateRule(ctx context.Context, r *domain.Rule) error
	UpdateRule(ctx context.Context, r *domain.Rule) error
	DeleteRule(ctx context.Context, id string) error
	GetRule(ctx context.Context, id string) (*domain.Rule, error)
	ListRules(ctx context.Context) ([]domain.Rule, error)
}

// SettingsStore stores each key as a JSON value; Set is an atomic upsert.
type SettingsStore interface {
	Get(ctx context.Context, key string, out any) (bool, error)
	Set(ctx context.Context, key string, value any) error
	Delete(ctx context.Context, key string) error
}

type StatsStore interface {
	// AddFlowDelta accumulates session totals for an exe/proxy pair and the
	// global session totals.
	AddFlowDelta(ctx context.Context, exePath, proxyID string, up, down int64) error
	RecordOutcome(ctx context.Context, status domain.ConnStatus) error
	SessionCounters(ctx context.Context) (domain.SessionCounters, error)
	UpsertExeDaily(ctx context.Context, s domain.ExeDailyStats) error
	UpsertProxyDaily(ctx context.Context, s domain.ProxyDailyStats) error
	ExeDailyRange(ctx context.Context, fromDay, toDay string) ([]domain.ExeDailyStats, error)
	ProxyDailyRange(ctx context.Context, fromDay, toDay string) ([]domain.ProxyDailyStats, error)
	ResetExe(ctx context.Context, exePath string) error
	ResetAll(ctx context.Context) error
}

type BlockedLogStore interface {
	Log(ctx context.Context, e domain.BlockedEntry) error
	List(ctx context.Context, limit int) ([]domain.BlockedEntry, error)
	Clear(ctx context.Context) error
}

// SecretCipher is implemented by the Windows DPAPI wrapper.
type SecretCipher interface {
	Protect(plaintext []byte) ([]byte, error)
	Unprotect(ciphertext []byte) ([]byte, error)
}
