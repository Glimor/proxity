package sqlite

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Glimor/proxity/internal/domain"
	_ "modernc.org/sqlite"
)

const dbFileName = "proxity.db"

// CorruptDBError is returned by Open when the database file failed its
// integrity check and was moved aside. The returned *DB is a fresh, usable
// database; the error is informational so callers can warn the user.
type CorruptDBError struct {
	Path       string
	BackupPath string
	Err        error
}

func (e *CorruptDBError) Error() string {
	return fmt.Sprintf("corrupt database %q moved to %q: %v", e.Path, e.BackupPath, e.Err)
}

func (e *CorruptDBError) Unwrap() error { return e.Err }

type DB struct {
	SQL *sql.DB
}

// Open opens (creating if needed) the database at path, applies pragmas and
// verifies integrity. On corruption it renames the file to
// "<path>.corrupt-<timestamp>" and opens a fresh database, returning it
// together with a *CorruptDBError.
func Open(path string) (*DB, error) {
	db, recovered, err := openOnce(path)
	if err != nil {
		return nil, err
	}
	if recovered == nil {
		return db, nil
	}
	return db, recovered
}

func openOnce(path string) (*DB, *CorruptDBError, error) {
	sqldb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqldb.SetMaxOpenConns(1)

	d := &DB{SQL: sqldb}
	// quick_check must run before pragmas: a corrupt/non-db file already fails
	// on the first pragma, which would mask the corruption.
	if err := d.quickCheck(); err != nil {
		sqldb.Close()
		backup := path + ".corrupt-" + time.Now().Format("20060102-150405")
		rerr := os.Rename(path, backup)
		if rerr != nil {
			return nil, nil, fmt.Errorf("database corrupt (%v) and rename failed: %w", err, rerr)
		}
		sqldb, oerr := sql.Open("sqlite", path)
		if oerr != nil {
			return nil, nil, fmt.Errorf("reopen after corruption recovery: %w", oerr)
		}
		sqldb.SetMaxOpenConns(1)
		d = &DB{SQL: sqldb}
		if oerr = d.applyPragmas(); oerr != nil {
			sqldb.Close()
			return nil, nil, fmt.Errorf("apply pragmas after recovery: %w", oerr)
		}
		return d, &CorruptDBError{Path: path, BackupPath: backup, Err: err}, nil
	}
	if err := d.applyPragmas(); err != nil {
		sqldb.Close()
		return nil, nil, fmt.Errorf("apply pragmas: %w", err)
	}
	return d, nil, nil
}

func (d *DB) Close() error { return d.SQL.Close() }

func (d *DB) applyPragmas() error {
	for _, p := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA busy_timeout=5000",
		"PRAGMA foreign_keys=1",
		"PRAGMA synchronous=NORMAL",
	} {
		if _, err := d.SQL.Exec(p); err != nil {
			return fmt.Errorf("%s: %w", p, err)
		}
	}
	return nil
}

func (d *DB) quickCheck() error {
	rows, err := d.SQL.Query("PRAGMA quick_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var res string
		if err := rows.Scan(&res); err != nil {
			return err
		}
		if res != "ok" {
			return errors.New("integrity check failed: " + res)
		}
	}
	return rows.Err()
}

// ResolveDBPath picks %ProgramData%\Proxity\proxity.db when writable,
// otherwise falls back to %LocalAppData%\Proxity\proxity.db.
func ResolveDBPath(log *slog.Logger) string {
	if pd := os.Getenv("ProgramData"); pd != "" {
		dir := filepath.Join(pd, "Proxity")
		if err := ensureWritableDir(dir); err == nil {
			return filepath.Join(dir, dbFileName)
		} else if log != nil {
			log.Warn("ProgramData dir not writable, falling back to LocalAppData",
				"dir", dir, "err", err)
		}
	}
	lad := os.Getenv("LocalAppData")
	if lad == "" {
		lad, _ = os.UserCacheDir()
	}
	dir := filepath.Join(lad, "Proxity")
	if err := ensureWritableDir(dir); err != nil && log != nil {
		log.Error("LocalAppData dir not writable either", "dir", dir, "err", err)
	}
	return filepath.Join(dir, dbFileName)
}

func ensureWritableDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return err
	}
	name := f.Name()
	f.Close()
	return os.Remove(name)
}

func storeTime(t time.Time) string {
	return t.UTC().Format(time.RFC3339Nano)
}

func parseTime(s string) (time.Time, error) {
	return time.Parse(time.RFC3339Nano, s)
}

func notFound(kind, id string) error {
	return domain.NewError("store.not_found", "Record not found",
		fmt.Sprintf("%s id=%s", kind, id))
}

func execOne(res sql.Result, kind, id string) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound(kind, id)
	}
	return nil
}
