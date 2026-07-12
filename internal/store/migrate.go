package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	_ "github.com/lib/pq"
)

var migrationMu sync.Mutex

func RunMigrations(dbURL string, migrationsPath string) error {
	migrationMu.Lock()
	defer migrationMu.Unlock()

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		return fmt.Errorf("ping: %w", err)
	}

		// use advisory lock to serialize concurrent migration runs
	_, err = db.Exec("SELECT pg_advisory_lock(999)")
	if err != nil {
		return fmt.Errorf("advisory lock: %w", err)
	}
	defer db.Exec("SELECT pg_advisory_unlock(999)")

	_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		)
	`)
	if err != nil {
		return fmt.Errorf("create schema_migrations: %w", err)
	}

	entries, err := os.ReadDir(migrationsPath)
	if err != nil {
		return fmt.Errorf("read migrations dir: %w", err)
	}

	var upFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".up.sql") {
			upFiles = append(upFiles, e.Name())
		}
	}
	sort.Strings(upFiles)

	for _, filename := range upFiles {
		version := strings.TrimSuffix(filename, ".up.sql")

		var exists bool
		err := db.QueryRow(
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=$1)", version,
		).Scan(&exists)
		if err != nil {
			return fmt.Errorf("check migration %s: %w", version, err)
		}
		if exists {
			continue
		}

		content, err := os.ReadFile(filepath.Join(migrationsPath, filename))
		if err != nil {
			return fmt.Errorf("read %s: %w", filename, err)
		}

		if _, err := db.Exec(string(content)); err != nil {
			return fmt.Errorf("exec %s: %w", filename, err)
		}

		if _, err := db.Exec(
			"INSERT INTO schema_migrations(version) VALUES($1)", version,
		); err != nil {
			return fmt.Errorf("record %s: %w", version, err)
		}

		fmt.Printf("  applied %s\n", filename)
	}

	return nil
}