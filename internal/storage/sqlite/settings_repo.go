package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/Glimor/proxity/internal/domain"
)

const settingsKey = "settings"

type SettingsRepo struct{ db *DB }

func (d *DB) Settings() *SettingsRepo { return &SettingsRepo{db: d} }

func (r *SettingsRepo) Get(ctx context.Context, key string, out any) (bool, error) {
	var raw string
	err := r.db.SQL.QueryRowContext(ctx,
		`SELECT value FROM settings WHERE key=?`, key).Scan(&raw)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return true, json.Unmarshal([]byte(raw), out)
}

func (r *SettingsRepo) Set(ctx context.Context, key string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = r.db.SQL.ExecContext(ctx,
		`INSERT INTO settings(key, value) VALUES(?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, string(raw))
	return err
}

func (r *SettingsRepo) Delete(ctx context.Context, key string) error {
	_, err := r.db.SQL.ExecContext(ctx, `DELETE FROM settings WHERE key=?`, key)
	return err
}

func (r *SettingsRepo) LoadSettings(ctx context.Context) (domain.Settings, error) {
	s := domain.DefaultSettings()
	found, err := r.Get(ctx, settingsKey, &s)
	if err != nil {
		return s, err
	}
	if !found {
		return domain.DefaultSettings(), nil
	}
	s.Normalize()
	return s, nil
}

func (r *SettingsRepo) SaveSettings(ctx context.Context, s domain.Settings) error {
	s.Normalize()
	return r.Set(ctx, settingsKey, s)
}
