package storage

import (
	"fmt"
	"os"
	"strings"
	"testing"
)

// SetupTestDB connects to a MySQL test database using the TEST_MYSQL_*
// environment variables. If any required variable is missing the test is
// skipped; the post-SQLite migration stance is that persistence tests
// must run against MySQL or not at all. Returns the effective config for
// inspection.
//
// Required env vars:
//   - TEST_MYSQL_HOST, TEST_MYSQL_PORT, TEST_MYSQL_USER, TEST_MYSQL_PASSWORD
//   - TEST_MYSQL_DATABASE (will be reset between tests)
func SetupTestDB(t *testing.T) MySQLConfig {
	t.Helper()

	host := os.Getenv("TEST_MYSQL_HOST")
	port := os.Getenv("TEST_MYSQL_PORT")
	user := os.Getenv("TEST_MYSQL_USER")
	pass := os.Getenv("TEST_MYSQL_PASSWORD")
	db := os.Getenv("TEST_MYSQL_DATABASE")
	if host == "" || user == "" || db == "" {
		t.Skip("TEST_MYSQL_* not configured; skipping MySQL-backed test")
	}
	if port == "" {
		port = "3306"
	}

	cfg := MySQLConfig{
		Host: host, Port: port, User: user, Password: pass, Database: db,
		Params: "charset=utf8mb4&collation=utf8mb4_bin&loc=UTC&multiStatements=true",
	}
	if err := Init(cfg); err != nil {
		t.Fatalf("connect test MySQL %s/%s: %v", host, db, err)
	}
	if err := resetTestDB(); err != nil {
		t.Fatalf("reset test db: %v", err)
	}
	if err := SeedDefaults(); err != nil {
		t.Fatalf("seed defaults: %v", err)
	}
	return cfg
}

// resetTestDB drops all non-goose tables in the current database and
// re-runs migrations. Calls TRUNCATE-like cleanup to give each test a
// blank slate.
func resetTestDB() error {
	if DB == nil {
		return fmt.Errorf("storage.DB is nil")
	}
	rows, err := DB.Query(`SHOW TABLES`)
	if err != nil {
		return err
	}
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		tables = append(tables, name)
	}
	rows.Close()

	if _, err := DB.Exec(`SET FOREIGN_KEY_CHECKS = 0`); err != nil {
		return err
	}
	defer DB.Exec(`SET FOREIGN_KEY_CHECKS = 1`)
	for _, t := range tables {
		if strings.HasPrefix(t, "goose_") {
			continue
		}
		if _, err := DB.Exec("TRUNCATE TABLE `" + t + "`"); err != nil {
			return fmt.Errorf("truncate %s: %w", t, err)
		}
	}
	return nil
}
