package catalogue

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

type migration struct {
	version  string
	sql      string
	checksum string
}

var migrationName = regexp.MustCompile(`^[0-9]{4}_[a-z0-9_]+\.sql$`)

func embeddedMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrationFiles, "migrations")
	if err != nil {
		return nil, err
	}
	var result []migration
	for _, entry := range entries {
		if entry.IsDir() || !migrationName.MatchString(entry.Name()) {
			return nil, fmt.Errorf("invalid migration filename %q", entry.Name())
		}
		body, err := migrationFiles.ReadFile("migrations/" + entry.Name())
		if err != nil {
			return nil, err
		}
		hash := sha256.Sum256(body)
		result = append(result, migration{
			version:  strings.TrimSuffix(entry.Name(), ".sql"),
			sql:      string(body),
			checksum: hex.EncodeToString(hash[:]),
		})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].version < result[j].version })
	if len(result) == 0 {
		return nil, errors.New("no migrations embedded")
	}
	for i := 1; i < len(result); i++ {
		if result[i].version[:4] == result[i-1].version[:4] {
			return nil, fmt.Errorf("duplicate migration sequence %s", result[i].version[:4])
		}
	}
	return result, nil
}

// applyMigrations runs the initial baseline and any future migrations
// transactionally, refusing unknown versions or modified migration contents.
// Future upgrades require pre-upgrade snapshots before this is invoked.
func applyMigrations(ctx context.Context, db *sql.DB, migrations []migration) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin catalogue migration: %w", err)
	}
	defer tx.Rollback()

	for _, statement := range []string{
		"CREATE TABLE IF NOT EXISTS schema_migrations(version TEXT PRIMARY KEY, applied_at TEXT NOT NULL)",
		"CREATE TABLE IF NOT EXISTS schema_migration_checksums(version TEXT PRIMARY KEY REFERENCES schema_migrations(version), sha256 TEXT NOT NULL)",
	} {
		if _, err := tx.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("initialise migration register: %w", err)
		}
	}

	rows, err := tx.QueryContext(ctx, `SELECT m.version, c.sha256 FROM schema_migrations m
		LEFT JOIN schema_migration_checksums c ON c.version = m.version`)
	if err != nil {
		return fmt.Errorf("read migration register: %w", err)
	}
	applied := map[string]string{}
	for rows.Next() {
		var version string
		var checksum sql.NullString
		if err := rows.Scan(&version, &checksum); err != nil {
			rows.Close()
			return err
		}
		if !checksum.Valid {
			rows.Close()
			return fmt.Errorf("migration %s has no checksum; refuse legacy/partial state", version)
		}
		applied[version] = checksum.String
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}

	known := map[string]string{}
	for _, m := range migrations {
		known[m.version] = m.checksum
	}
	for version, checksum := range applied {
		expected, exists := known[version]
		if !exists {
			return fmt.Errorf("database has unknown migration %s; downgrade blocked", version)
		}
		if expected != checksum {
			return fmt.Errorf("migration %s checksum mismatch; catalogue unchanged", version)
		}
	}

	for _, m := range migrations {
		if _, exists := applied[m.version]; exists {
			continue
		}
		if _, err := tx.ExecContext(ctx, m.sql); err != nil {
			return fmt.Errorf("apply migration %s: %w", m.version, err)
		}
		timestamp := time.Now().UTC().Format(time.RFC3339Nano)
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES(?, ?)", m.version, timestamp); err != nil {
			return fmt.Errorf("record migration %s: %w", m.version, err)
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migration_checksums(version, sha256) VALUES(?, ?)", m.version, m.checksum); err != nil {
			return fmt.Errorf("record checksum %s: %w", m.version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	return nil
}
