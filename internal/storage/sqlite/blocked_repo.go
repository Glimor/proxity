package sqlite

import (
	"context"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

type BlockedRepo struct{ db *DB }

func (d *DB) BlockedLog() *BlockedRepo { return &BlockedRepo{db: d} }

func (r *BlockedRepo) Log(ctx context.Context, e domain.BlockedEntry) error {
	if e.TS.IsZero() {
		e.TS = time.Now()
	}
	res, err := r.db.SQL.ExecContext(ctx,
		`INSERT INTO blocked_log(ts, exe_path, pid, dst, port, proto, reason)
		 VALUES(?,?,?,?,?,?,?)`,
		storeTime(e.TS), e.ExePath, e.PID, e.Dst, e.Port, e.Proto, e.Reason)
	if err != nil {
		return err
	}
	e.ID, _ = res.LastInsertId()
	return nil
}

func (r *BlockedRepo) List(ctx context.Context, limit int) ([]domain.BlockedEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.SQL.QueryContext(ctx,
		`SELECT id, ts, exe_path, pid, dst, port, proto, reason
		 FROM blocked_log ORDER BY ts DESC, id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.BlockedEntry
	for rows.Next() {
		var e domain.BlockedEntry
		var ts string
		if err := rows.Scan(&e.ID, &ts, &e.ExePath, &e.PID, &e.Dst, &e.Port, &e.Proto, &e.Reason); err != nil {
			return nil, err
		}
		if e.TS, err = parseTime(ts); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (r *BlockedRepo) Clear(ctx context.Context) error {
	_, err := r.db.SQL.ExecContext(ctx, `DELETE FROM blocked_log`)
	return err
}

// Trim deletes all but the newest maxRows entries (by id). Returns rows
// deleted.
func (r *BlockedRepo) Trim(ctx context.Context, maxRows int) (int64, error) {
	res, err := r.db.SQL.ExecContext(ctx,
		`DELETE FROM blocked_log WHERE id NOT IN
		 (SELECT id FROM blocked_log ORDER BY id DESC LIMIT ?)`, maxRows)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return n, nil
}
