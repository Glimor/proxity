package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/Glimor/proxity/internal/domain"
)

type StatsRepo struct{ db *DB }

func (d *DB) Stats() *StatsRepo { return &StatsRepo{db: d} }

func (r *StatsRepo) AddFlowDelta(ctx context.Context, exePath, proxyID string, up, down int64) error {
	tx, err := r.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, key := range []string{
		"flow:total",
		"flow:exe:" + exePath,
		"flow:proxy:" + proxyID,
	} {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO session_stats(key, value)
			 VALUES(?, json_object('up', ?, 'down', ?))
			 ON CONFLICT(key) DO UPDATE SET value = json_object(
				'up',   json_extract(session_stats.value,'$.up')   + json_extract(excluded.value,'$.up'),
				'down', json_extract(session_stats.value,'$.down') + json_extract(excluded.value,'$.down'))`,
			key, up, down); err != nil {
			return fmt.Errorf("flow delta %s: %w", key, err)
		}
	}
	return tx.Commit()
}

func (r *StatsRepo) FlowTotals(ctx context.Context, key string) (up, down int64, err error) {
	var raw string
	err = r.db.SQL.QueryRowContext(ctx,
		`SELECT value FROM session_stats WHERE key=?`, key).Scan(&raw)
	if err != nil {
		return 0, 0, err
	}
	var v struct {
		Up   int64 `json:"up"`
		Down int64 `json:"down"`
	}
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return 0, 0, err
	}
	return v.Up, v.Down, nil
}

func (r *StatsRepo) RecordOutcome(ctx context.Context, status domain.ConnStatus) error {
	var key string
	switch status {
	case domain.ConnBlocked:
		key = "cnt:blocked"
	case domain.ConnFailed:
		key = "cnt:failed"
	case domain.ConnClosed:
		key = "cnt:succeeded"
	default:
		return nil
	}
	_, err := r.db.SQL.ExecContext(ctx,
		`INSERT INTO session_stats(key, value) VALUES(?, '1')
		 ON CONFLICT(key) DO UPDATE SET value = CAST(value AS INTEGER) + 1`, key)
	return err
}

func (r *StatsRepo) SessionCounters(ctx context.Context) (domain.SessionCounters, error) {
	var c domain.SessionCounters
	get := func(key string, dst *int64) error {
		return r.db.SQL.QueryRowContext(ctx,
			`SELECT CAST(value AS INTEGER) FROM session_stats WHERE key=?`, key).Scan(dst)
	}
	for key, dst := range map[string]*int64{
		"cnt:blocked":   &c.Blocked,
		"cnt:failed":    &c.Failed,
		"cnt:succeeded": &c.Succeeded,
	} {
		if err := get(key, dst); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return c, err
		}
	}
	return c, nil
}

// DeleteDailyBefore removes exe/proxy daily rows with day < cutoffDay
// (YYYY-MM-DD). Returns the number of deleted rows.
func (r *StatsRepo) DeleteDailyBefore(ctx context.Context, cutoffDay string) (int64, error) {
	var total int64
	for _, table := range []string{"exe_daily_stats", "proxy_daily_stats"} {
		res, err := r.db.SQL.ExecContext(ctx,
			fmt.Sprintf(`DELETE FROM %s WHERE day < ?`, table), cutoffDay)
		if err != nil {
			return total, err
		}
		if n, err := res.RowsAffected(); err == nil {
			total += n
		}
	}
	return total, nil
}

// SaveLastSession stores the final snapshot under the "last_session"
// session_stats key (value column holds JSON text).
func (r *StatsRepo) SaveLastSession(ctx context.Context, s domain.LastSession) error {
	raw, err := json.Marshal(s)
	if err != nil {
		return err
	}
	_, err = r.db.SQL.ExecContext(ctx,
		`INSERT INTO session_stats(key, value) VALUES('last_session', ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`, string(raw))
	return err
}

// LastSession returns the persisted previous-session snapshot, or nil (no
// error) when none exists / it fails to decode.
func (r *StatsRepo) LastSession(ctx context.Context) (*domain.LastSession, error) {
	var raw string
	err := r.db.SQL.QueryRowContext(ctx,
		`SELECT value FROM session_stats WHERE key='last_session'`).Scan(&raw)
	if err != nil {
		return nil, nil // sql.ErrNoRows and scan errors both mean "no session"
	}
	var s domain.LastSession
	if json.Unmarshal([]byte(raw), &s) != nil {
		return nil, nil
	}
	return &s, nil
}

func (r *StatsRepo) UpsertExeDaily(ctx context.Context, s domain.ExeDailyStats) error {
	_, err := r.db.SQL.ExecContext(ctx,
		`INSERT INTO exe_daily_stats(day, exe_path, proxy_id, down, up, conns, seconds)
		 VALUES(?,?,?,?,?,?,?)
		 ON CONFLICT(day, exe_path, proxy_id) DO UPDATE SET
			down = down + excluded.down, up = up + excluded.up,
			conns = conns + excluded.conns, seconds = seconds + excluded.seconds`,
		s.Day, s.ExePath, s.ProxyID, s.Down, s.Up, s.Conns, s.Seconds)
	return err
}

func (r *StatsRepo) UpsertProxyDaily(ctx context.Context, s domain.ProxyDailyStats) error {
	_, err := r.db.SQL.ExecContext(ctx,
		`INSERT INTO proxy_daily_stats(day, proxy_id, down, up, conns, seconds)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(day, proxy_id) DO UPDATE SET
			down = down + excluded.down, up = up + excluded.up,
			conns = conns + excluded.conns, seconds = seconds + excluded.seconds`,
		s.Day, s.ProxyID, s.Down, s.Up, s.Conns, s.Seconds)
	return err
}

func (r *StatsRepo) ExeDailyRange(ctx context.Context, fromDay, toDay string) ([]domain.ExeDailyStats, error) {
	rows, err := r.db.SQL.QueryContext(ctx,
		`SELECT day, exe_path, proxy_id, down, up, conns, seconds
		 FROM exe_daily_stats WHERE day BETWEEN ? AND ?
		 ORDER BY day, exe_path`, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ExeDailyStats
	for rows.Next() {
		var s domain.ExeDailyStats
		if err := rows.Scan(&s.Day, &s.ExePath, &s.ProxyID, &s.Down, &s.Up, &s.Conns, &s.Seconds); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *StatsRepo) ProxyDailyRange(ctx context.Context, fromDay, toDay string) ([]domain.ProxyDailyStats, error) {
	rows, err := r.db.SQL.QueryContext(ctx,
		`SELECT day, proxy_id, down, up, conns, seconds
		 FROM proxy_daily_stats WHERE day BETWEEN ? AND ?
		 ORDER BY day, proxy_id`, fromDay, toDay)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.ProxyDailyStats
	for rows.Next() {
		var s domain.ProxyDailyStats
		if err := rows.Scan(&s.Day, &s.ProxyID, &s.Down, &s.Up, &s.Conns, &s.Seconds); err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (r *StatsRepo) ResetExe(ctx context.Context, exePath string) error {
	tx, err := r.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM exe_daily_stats WHERE exe_path=?`, exePath); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM session_stats WHERE key=?`, "flow:exe:"+exePath); err != nil {
		return err
	}
	return tx.Commit()
}

func (r *StatsRepo) ResetAll(ctx context.Context) error {
	tx, err := r.db.SQL.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, t := range []string{"session_stats", "exe_daily_stats", "proxy_daily_stats"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t); err != nil {
			return err
		}
	}
	return tx.Commit()
}
