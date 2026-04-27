package main

import (
	"database/sql"
	"fmt"
	"strings"
	"testing"
)

// skipIfNoTestMySQL is the canonical gate used by *_test.go to skip when no
// live MySQL target is configured in the environment.
func skipIfNoTestMySQL(t *testing.T) {
	t.Helper()
	if _, ok := testMySQLSectionFromEnv(); !ok {
		t.Skip("TEST_MYSQL_* env not set; skipping MySQL-backed test")
	}
}

// openTestDB opens a fresh connection to the test MySQL database and wipes
// every non-goose table so the test starts from a clean slate. The caller
// must close the returned *sql.DB.
func openTestDB(t *testing.T) *sql.DB {
	t.Helper()
	skipIfNoTestMySQL(t)
	db, err := OpenDB("")
	if err != nil {
		t.Fatalf("open test mysql: %v", err)
	}
	if err := truncateAllQiweiTables(db); err != nil {
		_ = db.Close()
		t.Fatalf("reset mysql tables: %v", err)
	}
	return db
}

func truncateAllQiweiTables(db *sql.DB) error {
	rows, err := db.Query(`SELECT table_name FROM information_schema.tables
		WHERE table_schema = DATABASE()
		  AND table_name NOT LIKE 'goose_%'`)
	if err != nil {
		return err
	}
	defer rows.Close()

	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			return err
		}
		names = append(names, n)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(names) == 0 {
		return nil
	}
	if _, err := db.Exec("SET FOREIGN_KEY_CHECKS=0"); err != nil {
		return fmt.Errorf("disable fk checks: %w", err)
	}
	defer db.Exec("SET FOREIGN_KEY_CHECKS=1")
	for _, n := range names {
		if _, err := db.Exec("TRUNCATE TABLE `" + strings.ReplaceAll(n, "`", "``") + "`"); err != nil {
			return fmt.Errorf("truncate %s: %w", n, err)
		}
	}
	return nil
}
