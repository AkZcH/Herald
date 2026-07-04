package store_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/AkZcH/herald/internal/store"
)

func TestMigrationsRun(t *testing.T) {
	dbURL := os.Getenv("HERALD_DB_URL")
	if dbURL == "" {
		t.Skip("HERALD_DB_URL not set — skipping migration test (runs in CI)")
	}

	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	migrationsPath = filepath.Clean(migrationsPath)

	if err := store.RunMigrations(dbURL, migrationsPath); err != nil {
		t.Fatalf("migrations failed: %v", err)
	}
}

func TestMigrationFilesExist(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	migrationsPath = filepath.Clean(migrationsPath)

	expected := []string{
		"000001_create_config_versions_and_raft_log.up.sql",
		"000001_create_config_versions_and_raft_log.down.sql",
		"000002_create_ack_and_agent_tables.up.sql",
		"000002_create_ack_and_agent_tables.down.sql",
		"000003_create_targeting_schema_audit.up.sql",
		"000003_create_targeting_schema_audit.down.sql",
		"000004_audit_trigger_and_seed.up.sql",
		"000004_audit_trigger_and_seed.down.sql",
	}

	for _, f := range expected {
		path := filepath.Join(migrationsPath, f)
		if _, err := os.Stat(path); err != nil {
			t.Errorf("missing migration file: %s", f)
		}
	}
}

func TestMigrationFilesPaired(t *testing.T) {
	_, filename, _, _ := runtime.Caller(0)
	migrationsPath := filepath.Join(filepath.Dir(filename), "..", "..", "migrations")
	migrationsPath = filepath.Clean(migrationsPath)

	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		t.Fatalf("read dir: %v", err)
	}

	ups := map[string]bool{}
	downs := map[string]bool{}

	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".up.sql") {
			ups[strings.TrimSuffix(e.Name(), ".up.sql")] = true
		}
		if strings.HasSuffix(e.Name(), ".down.sql") {
			downs[strings.TrimSuffix(e.Name(), ".down.sql")] = true
		}
	}

	for version := range ups {
		if !downs[version] {
			t.Errorf("migration %s has .up.sql but no .down.sql", version)
		}
	}
}