package catalogue

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// Store owns one SQLite connection, so connection-scoped safety pragmas
// apply uniformly to catalogue operations.
type Store struct {
	db   *sql.DB
	path string
}

// Open creates the local catalogue at path or checks an existing catalogue.
// It never contacts a Supabase service. Only absolute paths are permitted.
func Open(ctx context.Context, path string) (_ *Store, err error) {
	if !filepath.IsAbs(path) || strings.TrimSpace(path) == "" {
		return nil, errors.New("catalogue path must be absolute")
	}
	path = filepath.Clean(path)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, fmt.Errorf("prepare catalogue directory: %w", err)
	}
	if fi, err := os.Lstat(path); err == nil {
		if !fi.Mode().IsRegular() || fi.Mode().Perm()&0077 != 0 {
			return nil, errors.New("existing catalogue must be a regular private file")
		}
	} else if errors.Is(err, os.ErrNotExist) {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
		if err != nil {
			return nil, fmt.Errorf("create catalogue: %w", err)
		}
		if err := f.Close(); err != nil {
			return nil, err
		}
	} else {
		return nil, fmt.Errorf("inspect catalogue: %w", err)
	}

	dsn := (&url.URL{
		Scheme:   "file",
		Path:     path,
		RawQuery: "_busy_timeout=5000&_foreign_keys=on&_journal_mode=WAL",
	}).String()
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("open catalogue: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	defer func() {
		if err != nil {
			db.Close()
		}
	}()
	if err = db.PingContext(ctx); err != nil {
		return nil, fmt.Errorf("connect catalogue: %w", err)
	}
	if err = checkPragmas(ctx, db); err != nil {
		return nil, err
	}
	if err = verifyIntegrity(ctx, db); err != nil {
		return nil, err
	}
	migrations, err := embeddedMigrations()
	if err != nil {
		return nil, err
	}
	if err = applyMigrations(ctx, db, migrations); err != nil {
		return nil, err
	}
	if err = verifyIntegrity(ctx, db); err != nil {
		return nil, err
	}
	return &Store{db: db, path: path}, nil
}

func checkPragmas(ctx context.Context, db *sql.DB) error {
	var fk int
	if err := db.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fk); err != nil || fk != 1 {
		return fmt.Errorf("catalogue foreign keys not enabled (value %d): %v", fk, err)
	}
	var journal string
	if err := db.QueryRowContext(ctx, "PRAGMA journal_mode").Scan(&journal); err != nil || !strings.EqualFold(journal, "wal") {
		return fmt.Errorf("catalogue WAL mode not enabled (value %q): %v", journal, err)
	}
	return nil
}

func verifyIntegrity(ctx context.Context, db *sql.DB) error {
	rows, err := db.QueryContext(ctx, "PRAGMA integrity_check")
	if err != nil {
		return err
	}
	for rows.Next() {
		var result string
		if err := rows.Scan(&result); err != nil {
			rows.Close()
			return err
		}
		if result != "ok" {
			rows.Close()
			return fmt.Errorf("catalogue integrity check failed: %s", result)
		}
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	rows, err = db.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	if rows.Next() {
		rows.Close()
		return errors.New("catalogue foreign-key integrity failed")
	}
	err = rows.Err()
	rows.Close()
	return err
}

// Verify checks the operational catalogue's consistency independently.
func (s *Store) Verify(ctx context.Context) error {
	if err := checkPragmas(ctx, s.db); err != nil {
		return err
	}
	return verifyIntegrity(ctx, s.db)
}

// Close releases SQLite and its write-ahead-log handles.
func (s *Store) Close() error { return s.db.Close() }

// DB is deliberately package-private: only catalogue internals must perform
// queries until project-scoped authorisation is implemented.
func (s *Store) pathForTests() string { return s.path }

// Backup creates a consistent SQLite snapshot with VACUUM INTO. The target
// must not already exist, and a partial snapshot is removed on error.
// Only catalogue metadata is backed up; never Supabase component archives.
func (s *Store) Backup(ctx context.Context, destination string) (err error) {
	if !filepath.IsAbs(destination) {
		return errors.New("snapshot destination must be absolute")
	}
	destination = filepath.Clean(destination)
	if destination == s.path {
		return errors.New("cannot replace active catalogue with snapshot")
	}
	f, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("create private snapshot: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(destination)
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(destination)
		}
	}()
	if _, err = s.db.ExecContext(ctx, "VACUUM INTO ?", destination); err != nil {
		return fmt.Errorf("SQLite snapshot: %w", err)
	}
	// Verify by opening a separate SQLite connection without applying migrations.
	checkDB, openErr := sql.Open("sqlite3", (&url.URL{Scheme: "file", Path: destination, RawQuery: "mode=ro&_foreign_keys=on"}).String())
	if openErr != nil {
		return openErr
	}
	defer checkDB.Close()
	if err = verifyIntegrity(ctx, checkDB); err != nil {
		return fmt.Errorf("snapshot integrity: %w", err)
	}
	return nil
}
