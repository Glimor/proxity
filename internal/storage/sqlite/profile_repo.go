package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

type ProfileRepo struct{ db *DB }

func (d *DB) Profiles() *ProfileRepo { return &ProfileRepo{db: d} }

const profileCols = `id, name, host, port, username, password_ciphertext, enabled,
	timeout_ms, retry_interval_ms, max_retries, created_at, updated_at`

func (r *ProfileRepo) CreateProfile(ctx context.Context, p *domain.ProxyProfile) error {
	now := time.Now()
	if p.CreatedAt.IsZero() {
		p.CreatedAt = now
	}
	p.UpdatedAt = now
	_, err := r.db.SQL.ExecContext(ctx,
		`INSERT INTO proxy_profiles(`+profileCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.Name, p.Host, p.Port, p.Username, p.PasswordCiphertext,
		boolInt(p.Enabled), p.TimeoutMS, p.RetryIntervalMS, p.MaxRetries,
		storeTime(p.CreatedAt), storeTime(p.UpdatedAt))
	return err
}

func (r *ProfileRepo) UpdateProfile(ctx context.Context, p *domain.ProxyProfile) error {
	p.UpdatedAt = time.Now()
	res, err := r.db.SQL.ExecContext(ctx,
		`UPDATE proxy_profiles SET name=?, host=?, port=?, username=?,
			password_ciphertext=?, enabled=?, timeout_ms=?, retry_interval_ms=?,
			max_retries=?, updated_at=? WHERE id=?`,
		p.Name, p.Host, p.Port, p.Username, p.PasswordCiphertext,
		boolInt(p.Enabled), p.TimeoutMS, p.RetryIntervalMS, p.MaxRetries,
		storeTime(p.UpdatedAt), p.ID)
	if err != nil {
		return err
	}
	return execOne(res, "proxy_profile", p.ID)
}

func (r *ProfileRepo) DeleteProfile(ctx context.Context, id string) error {
	res, err := r.db.SQL.ExecContext(ctx, `DELETE FROM proxy_profiles WHERE id=?`, id)
	if err != nil {
		return err
	}
	return execOne(res, "proxy_profile", id)
}

func (r *ProfileRepo) GetProfile(ctx context.Context, id string) (*domain.ProxyProfile, error) {
	row := r.db.SQL.QueryRowContext(ctx,
		`SELECT `+profileCols+` FROM proxy_profiles WHERE id=?`, id)
	p, err := scanProfile(row)
	if err == sql.ErrNoRows {
		return nil, notFound("proxy_profile", id)
	}
	return p, err
}

func (r *ProfileRepo) ListProfiles(ctx context.Context) ([]domain.ProxyProfile, error) {
	rows, err := r.db.SQL.QueryContext(ctx,
		`SELECT `+profileCols+` FROM proxy_profiles ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []domain.ProxyProfile{}
	for rows.Next() {
		p, err := scanProfile(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

func (r *ProfileRepo) SetProfileEnabled(ctx context.Context, id string, enabled bool) error {
	res, err := r.db.SQL.ExecContext(ctx,
		`UPDATE proxy_profiles SET enabled=?, updated_at=? WHERE id=?`,
		boolInt(enabled), storeTime(time.Now()), id)
	if err != nil {
		return err
	}
	return execOne(res, "proxy_profile", id)
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanProfile(row rowScanner) (*domain.ProxyProfile, error) {
	var p domain.ProxyProfile
	var created, updated string
	var enabled int
	err := row.Scan(&p.ID, &p.Name, &p.Host, &p.Port, &p.Username,
		&p.PasswordCiphertext, &enabled, &p.TimeoutMS, &p.RetryIntervalMS,
		&p.MaxRetries, &created, &updated)
	if err != nil {
		return nil, err
	}
	p.Enabled = enabled != 0
	if p.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if p.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &p, nil
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
