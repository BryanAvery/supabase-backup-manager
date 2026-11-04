package catalogue

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "catalogue.sqlite")
	store, err := Open(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestInitialMigrationMatchesSpecification(t *testing.T) {
	original, err := os.ReadFile("../../docs/spec/v1.0.0/data/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	expected := strings.TrimPrefix(string(original), "PRAGMA foreign_keys=ON;\nPRAGMA journal_mode=WAL;\n")
	if expected == string(original) {
		t.Fatal("unexpected canonical schema header")
	}
	embedded, err := migrationFiles.ReadFile("migrations/0001_initial_catalogue.sql")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(expected) != strings.TrimSpace(string(embedded)) {
		t.Fatal("embedded initial migration diverges from engineering specification")
	}
}

func TestInitialiseAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "operational.sqlite")
	store, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if fi, err := os.Stat(path); err != nil || fi.Mode().Perm() != 0600 {
		t.Fatalf("catalogue file mode: %v, %v", fi, err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 2 {
		t.Fatalf("migration register count %d: %v", count, err)
	}
	if err := store.Verify(ctx); err != nil {
		t.Fatal(err)
	}
	// The schema's role membership constraints must be enforced, not bypassed.
	_, err = store.db.ExecContext(ctx, "INSERT INTO project_members(project_id,user_id,role) VALUES ('missing','missing','viewer')")
	if err == nil {
		t.Fatal("foreign-key constraints were not enforced")
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	if err := reopened.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 2 {
		t.Fatalf("migration rerun created duplicates: %d, %v", count, err)
	}
}

func TestMigrationFailureRollsBack(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	migrations, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	failureSQL := "CREATE TABLE partial_example(id INTEGER); INSERT INTO nonexistent_table VALUES (1);"
	hash := sha256.Sum256([]byte(failureSQL))
	migrations = append(migrations, migration{version: "0003_fail", sql: failureSQL, checksum: hex.EncodeToString(hash[:])})
	err = applyMigrations(ctx, store.db, migrations)
	if err == nil {
		t.Fatal("expected migration failure")
	}
	var found int
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE name='partial_example'").Scan(&found); err != nil || found != 0 {
		t.Fatalf("partial DDL persisted: %d %v", found, err)
	}
	if err := store.db.QueryRowContext(ctx, "SELECT count(*) FROM schema_migrations").Scan(&found); err != nil || found != 2 {
		t.Fatalf("migration recorded despite rollback: %d %v", found, err)
	}
}

func TestUnknownOrModifiedMigrationFailsClosed(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	ms, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	tampered := append([]migration(nil), ms...)
	tampered[0].checksum = strings.Repeat("0", 64)
	if err := applyMigrations(ctx, store.db, tampered); err == nil {
		t.Fatal("changed migration not rejected")
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO schema_migrations(version, applied_at) VALUES ('9999_unknown', '2026-11-04T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.ExecContext(ctx, "INSERT INTO schema_migration_checksums(version,sha256) VALUES ('9999_unknown', 'fake')"); err != nil {
		t.Fatal(err)
	}
	if err := applyMigrations(ctx, store.db, ms); err == nil {
		t.Fatal("future/unknown migration not rejected")
	}
}

func TestSnapshotRestoreAndExclusiveDestination(t *testing.T) {
	store := testStore(t)
	ctx := context.Background()
	_, err := store.db.ExecContext(ctx,
		"INSERT INTO users(id,display_name,created_at) VALUES('synthetic-id','Synthetic Tester','2026-11-04T00:00:00Z')")
	if err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(t.TempDir(), "snapshot.sqlite")
	if err := store.Backup(ctx, target); err != nil {
		t.Fatal(err)
	}
	if err := store.Backup(ctx, target); err == nil {
		t.Fatal("existing snapshot must never be overwritten")
	}
	restored, err := Open(ctx, target)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	var name string
	if err := restored.db.QueryRowContext(ctx, "SELECT display_name FROM users WHERE id='synthetic-id'").Scan(&name); err != nil || name != "Synthetic Tester" {
		t.Fatalf("restored data incorrect: %q, %v", name, err)
	}
	fi, err := os.Stat(target)
	if err != nil || fi.Mode().Perm()&0077 != 0 {
		t.Fatalf("snapshot permissions unsafe: %v, %v", fi, err)
	}
}

func TestRejectUnsafePaths(t *testing.T) {
	ctx := context.Background()
	if _, err := Open(ctx, "relative.sqlite"); err == nil {
		t.Fatal("relative path accepted")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "public.sqlite")
	if err := os.WriteFile(path, []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, path); err == nil {
		t.Fatal("world-readable catalogue accepted")
	}
	src := filepath.Join(dir, "private.sqlite")
	if err := os.WriteFile(src, []byte{}, 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.sqlite")
	if err := os.Symlink(src, link); err != nil {
		t.Skipf("filesystem does not support symlinks: %v", err)
	}
	if _, err := Open(ctx, link); err == nil {
		t.Fatal("symlink catalogue accepted")
	}
	store := testStore(t)
	if err := store.Backup(ctx, store.pathForTests()); err == nil {
		t.Fatal("source overwritten by snapshot")
	}
	if err := store.Backup(ctx, "relative.sqlite"); err == nil {
		t.Fatal("relative snapshot path accepted")
	}
}

func TestSnapshotAllowsRollbackToOlderSchemaOnCopy(t *testing.T) {
	ctx := context.Background()
	store := testStore(t)
	if _, err := store.db.ExecContext(ctx,
		"INSERT INTO users(id,display_name,created_at) VALUES('rollback-user','Before Upgrade','2026-11-04T00:00:00Z')"); err != nil {
		t.Fatal(err)
	}
	backupPath := filepath.Join(t.TempDir(), "pre-upgrade.sqlite")
	if err := store.Backup(ctx, backupPath); err != nil {
		t.Fatal(err)
	}
	// Simulate a later committed migration without altering the embedded baseline.
	migrations, err := embeddedMigrations()
	if err != nil {
		t.Fatal(err)
	}
	sqlText := "CREATE TABLE upgrade_only (id TEXT PRIMARY KEY)"
	hash := sha256.Sum256([]byte(sqlText))
	migrations = append(migrations, migration{
		version: "0003_new_feature",
		sql: sqlText, checksum: hex.EncodeToString(hash[:]),
	})
	if err := applyMigrations(ctx, store.db, migrations); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRowContext(ctx,
		"SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 3 {
		t.Fatalf("upgraded migration count %d: %v", count, err)
	}
	// A complete snapshot, rather than reverse SQL, is the rollback source.
	rollback, err := Open(ctx, backupPath)
	if err != nil {
		t.Fatal(err)
	}
	defer rollback.Close()
	if err := rollback.db.QueryRowContext(ctx,
		"SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 2 {
		t.Fatalf("snapshot did not preserve previous schema version: %d, %v", count, err)
	}
	if err := rollback.db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE name='upgrade_only'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("future table leaked into rollback snapshot: %d, %v", count, err)
	}
	var name string
	if err := rollback.db.QueryRowContext(ctx,
		"SELECT display_name FROM users WHERE id='rollback-user'").Scan(&name); err != nil || name != "Before Upgrade" {
		t.Fatalf("rollback snapshot lost original row: %q, %v", name, err)
	}
}
