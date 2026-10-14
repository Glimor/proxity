package sqlite

import (
	"context"
	"database/sql"
	"time"

	"github.com/Glimor/proxity/internal/domain"
)

type RuleRepo struct{ db *DB }

func (d *DB) Rules() *RuleRepo { return &RuleRepo{db: d} }

const ruleCols = `id, name, kind, path, path_fold, action, proxy_id, enabled,
	kill_switch, include_subdirs, priority, created_at, updated_at`

// canonicalize fills Path/PathFold before write when PathFold is empty.
func canonicalize(r *domain.Rule) error {
	if r.PathFold == "" || r.Path == "" {
		return domain.CanonicalizeRulePath(r, r.Path)
	}
	return nil
}

func (r *RuleRepo) CreateRule(ctx context.Context, rule *domain.Rule) error {
	if err := canonicalize(rule); err != nil {
		return err
	}
	now := time.Now()
	if rule.CreatedAt.IsZero() {
		rule.CreatedAt = now
	}
	rule.UpdatedAt = now
	_, err := r.db.SQL.ExecContext(ctx,
		`INSERT INTO rules(`+ruleCols+`) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?)`,
		rule.ID, rule.Name, string(rule.Kind), rule.Path, rule.PathFold,
		string(rule.Action), rule.ProxyID, boolInt(rule.Enabled),
		boolInt(rule.KillSwitch), boolInt(rule.IncludeSubdirs), rule.Priority,
		storeTime(rule.CreatedAt), storeTime(rule.UpdatedAt))
	return err
}

func (r *RuleRepo) UpdateRule(ctx context.Context, rule *domain.Rule) error {
	if err := canonicalize(rule); err != nil {
		return err
	}
	rule.UpdatedAt = time.Now()
	res, err := r.db.SQL.ExecContext(ctx,
		`UPDATE rules SET name=?, kind=?, path=?, path_fold=?, action=?,
			proxy_id=?, enabled=?, kill_switch=?, include_subdirs=?, priority=?,
			updated_at=? WHERE id=?`,
		rule.Name, string(rule.Kind), rule.Path, rule.PathFold,
		string(rule.Action), rule.ProxyID, boolInt(rule.Enabled),
		boolInt(rule.KillSwitch), boolInt(rule.IncludeSubdirs), rule.Priority,
		storeTime(rule.UpdatedAt), rule.ID)
	if err != nil {
		return err
	}
	return execOne(res, "rule", rule.ID)
}

func (r *RuleRepo) DeleteRule(ctx context.Context, id string) error {
	res, err := r.db.SQL.ExecContext(ctx, `DELETE FROM rules WHERE id=?`, id)
	if err != nil {
		return err
	}
	return execOne(res, "rule", id)
}

func (r *RuleRepo) GetRule(ctx context.Context, id string) (*domain.Rule, error) {
	row := r.db.SQL.QueryRowContext(ctx,
		`SELECT `+ruleCols+` FROM rules WHERE id=?`, id)
	rule, err := scanRule(row)
	if err == sql.ErrNoRows {
		return nil, notFound("rule", id)
	}
	return rule, err
}

// List returns rules in match order: exe rules first, then folder rules by
// descending path depth, then priority.
func (r *RuleRepo) ListRules(ctx context.Context) ([]domain.Rule, error) {
	rows, err := r.db.SQL.QueryContext(ctx,
		`SELECT `+ruleCols+` FROM rules
		 ORDER BY (kind = 'exe') DESC, LENGTH(path_fold) DESC, priority DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Rule
	for rows.Next() {
		rule, err := scanRule(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *rule)
	}
	return out, rows.Err()
}

func scanRule(row rowScanner) (*domain.Rule, error) {
	var r domain.Rule
	var kind, action, created, updated string
	var enabled, killSwitch, includeSubdirs int
	err := row.Scan(&r.ID, &r.Name, &kind, &r.Path, &r.PathFold, &action,
		&r.ProxyID, &enabled, &killSwitch, &includeSubdirs, &r.Priority,
		&created, &updated)
	if err != nil {
		return nil, err
	}
	r.Kind = domain.RuleKind(kind)
	r.Action = domain.RuleAction(action)
	r.Enabled = enabled != 0
	r.KillSwitch = killSwitch != 0
	r.IncludeSubdirs = includeSubdirs != 0
	if r.CreatedAt, err = parseTime(created); err != nil {
		return nil, err
	}
	if r.UpdatedAt, err = parseTime(updated); err != nil {
		return nil, err
	}
	return &r, nil
}
