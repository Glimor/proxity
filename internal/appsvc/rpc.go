//go:build windows

package appsvc

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	"github.com/Glimor/proxity/internal/ipc"
)

func decode(params json.RawMessage, v any) error {
	if len(params) == 0 || string(params) == "null" {
		return nil
	}
	return json.Unmarshal(params, v)
}

func opCtx(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 30*time.Second)
}

// RegisterHandlers binds the full service surface onto an IPC server.
// Method names are the public RPC contract with the GUI.
func (s *Service) RegisterHandlers(srv *ipc.Server) {
	srv.Handle("ping", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return map[string]any{"pong": true, "version": 1}, nil
	})

	// --- profiles -------------------------------------------------------
	srv.Handle("profiles.create", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in ProfileInput
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.CreateProfile(cctx, in)
	})
	srv.Handle("profiles.update", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ID    string       `json:"id"`
			Input ProfileInput `json:"input"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.UpdateProfile(cctx, in.ID, in.Input)
	})
	srv.Handle("profiles.delete", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return map[string]bool{"ok": true}, s.DeleteProfile(cctx, in.ID)
	})
	srv.Handle("profiles.list", func(ctx context.Context, _ json.RawMessage) (any, error) {
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.ListProfiles(cctx)
	})
	srv.Handle("profiles.setEnabled", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return map[string]bool{"ok": true}, s.SetProfileEnabled(cctx, in.ID, in.Enabled)
	})
	srv.Handle("profiles.test", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		return s.TestProfile(ctx, in.ID), nil // ctx carries disconnect cancel
	})
	srv.Handle("profiles.statuses", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.ProfileStatuses(), nil
	})

	// --- rules ----------------------------------------------------------
	srv.Handle("rules.create", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in RuleInput
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.CreateRule(cctx, in)
	})
	srv.Handle("rules.update", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ID    string    `json:"id"`
			Input RuleInput `json:"input"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.UpdateRule(cctx, in.ID, in.Input)
	})
	srv.Handle("rules.delete", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ID string `json:"id"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return map[string]bool{"ok": true}, s.DeleteRule(cctx, in.ID)
	})
	srv.Handle("rules.list", func(ctx context.Context, _ json.RawMessage) (any, error) {
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.ListRules(cctx)
	})
	srv.Handle("rules.setEnabled", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return map[string]bool{"ok": true}, s.SetRuleEnabled(cctx, in.ID, in.Enabled)
	})
	srv.Handle("rules.toggleKillSwitch", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ID      string `json:"id"`
			Enabled bool   `json:"enabled"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return map[string]bool{"ok": true}, s.ToggleKillSwitch(cctx, in.ID, in.Enabled)
	})
	srv.Handle("processes.list", func(ctx context.Context, _ json.RawMessage) (any, error) {
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.ListRunningProcesses(cctx)
	})

	// --- engine ---------------------------------------------------------
	srv.Handle("engine.start", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.EngineStatus(), s.StartEngine(ctx)
	})
	srv.Handle("engine.stop", func(ctx context.Context, _ json.RawMessage) (any, error) {
		s.StopEngine()
		return s.EngineStatus(), nil
	})
	srv.Handle("engine.status", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.EngineStatus(), nil
	})

	// --- settings -------------------------------------------------------
	srv.Handle("settings.get", func(ctx context.Context, _ json.RawMessage) (any, error) {
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.GetSettings(cctx)
	})
	srv.Handle("settings.update", func(ctx context.Context, p json.RawMessage) (any, error) {
		var st domain.Settings
		if err := decode(p, &st); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.UpdateSettings(cctx, st)
	})

	// --- stats & data ---------------------------------------------------
	srv.Handle("stats.snapshot", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.Snapshot(), nil
	})
	srv.Handle("stats.history", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.History(), nil
	})
	srv.Handle("stats.conns", func(ctx context.Context, _ json.RawMessage) (any, error) {
		return s.ActiveConnections(), nil
	})
	srv.Handle("stats.exe", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.ExeStats(cctx, in.From, in.To)
	})
	srv.Handle("stats.proxy", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			From string `json:"from"`
			To   string `json:"to"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.ProxyStats(cctx, in.From, in.To)
	})
	srv.Handle("stats.lastSession", func(ctx context.Context, _ json.RawMessage) (any, error) {
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.LastSession(cctx)
	})
	srv.Handle("stats.exportCsv", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			From string `json:"from"`
			To   string `json:"to"`
			Kind string `json:"kind"` // "exe" | "proxy" | "all"
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.ExportStatsCSV(cctx, in.From, in.To, in.Kind)
	})
	srv.Handle("stats.reset", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			ExePath string `json:"exe_path"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return map[string]bool{"ok": true}, s.ResetStats(cctx, in.ExePath)
	})
	srv.Handle("blocked.list", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			Limit int `json:"limit"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.BlockedLog(cctx, in.Limit)
	})
	srv.Handle("blocked.clear", func(ctx context.Context, _ json.RawMessage) (any, error) {
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return map[string]bool{"ok": true}, s.ClearBlocked(cctx)
	})
	srv.Handle("data.export", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			IncludeSecrets bool `json:"include_secrets"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.ExportProfiles(cctx, in.IncludeSecrets)
	})
	srv.Handle("data.import", func(ctx context.Context, p json.RawMessage) (any, error) {
		var in struct {
			Profiles  []ExportProfile `json:"profiles"`
			Overwrite bool            `json:"overwrite"`
		}
		if err := decode(p, &in); err != nil {
			return nil, err
		}
		cctx, cancel := opCtx(ctx)
		defer cancel()
		return s.ImportProfiles(cctx, in.Profiles, in.Overwrite)
	})
}
