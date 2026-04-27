// Package migrator implements the SQLite → MySQL one-shot copy + cleanse
// pipeline used by cmd/migrate-sqlite-to-mysql. The pipeline is intentionally
// sequential (no parallelism) and stops at the first row count mismatch or
// SQL error, on the assumption that operators run it during a maintenance
// window and would rather human-investigate one bad scope than chase a
// silently half-imported target.
package migrator

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// Options carries CLI knobs into the table-by-table executor.
type Options struct {
	BatchSize      int
	TruncateBefore bool
	DryRun         bool
	VerifyOnly     bool
	AllowOrphans   bool
}

// scope owns the connection pair plus the table descriptor list for one
// service ("agent" or "qiwei").
type scope struct {
	name   string // "agent" | "qiwei"
	sqlite *sql.DB
	mysql  *sql.DB
	opts   Options
	tables []tableMigration
}

// RunAgent imports moli_agent. Tables are migrated in dependency order;
// JSON columns are split into the new child tables in the same transaction
// as their parent rows.
func RunAgent(ctx context.Context, sqlitePath, mysqlDSN string, opts Options) error {
	sq, my, err := openPair(sqlitePath, mysqlDSN)
	if err != nil {
		return err
	}
	defer sq.Close()
	defer my.Close()

	s := &scope{name: "agent", sqlite: sq, mysql: my, opts: opts, tables: agentTables()}
	return s.run(ctx)
}

// RunQiwei imports moli_qiwei.
func RunQiwei(ctx context.Context, sqlitePath, mysqlDSN string, opts Options) error {
	sq, my, err := openPair(sqlitePath, mysqlDSN)
	if err != nil {
		return err
	}
	defer sq.Close()
	defer my.Close()

	s := &scope{name: "qiwei", sqlite: sq, mysql: my, opts: opts, tables: qiweiTables()}
	return s.run(ctx)
}

func openPair(sqlitePath, mysqlDSN string) (*sql.DB, *sql.DB, error) {
	if _, err := os.Stat(sqlitePath); err != nil {
		return nil, nil, fmt.Errorf("sqlite source %q: %w", sqlitePath, err)
	}
	sq, err := sql.Open("sqlite", sqlitePath+"?mode=ro")
	if err != nil {
		return nil, nil, fmt.Errorf("open sqlite: %w", err)
	}
	if err := sq.Ping(); err != nil {
		sq.Close()
		return nil, nil, fmt.Errorf("ping sqlite: %w", err)
	}

	dsn := mysqlDSN
	if !strings.Contains(dsn, "multiStatements") {
		if strings.Contains(dsn, "?") {
			dsn += "&multiStatements=true"
		} else {
			dsn += "?multiStatements=true"
		}
	}
	my, err := sql.Open("mysql", dsn)
	if err != nil {
		sq.Close()
		return nil, nil, fmt.Errorf("open mysql: %w", err)
	}
	if err := my.Ping(); err != nil {
		sq.Close()
		my.Close()
		return nil, nil, fmt.Errorf("ping mysql: %w", err)
	}
	return sq, my, nil
}

func (s *scope) run(ctx context.Context) error {
	log.Printf("[%s] preflight checks...", s.name)
	if err := s.preflight(ctx); err != nil {
		return err
	}

	if s.opts.TruncateBefore && !s.opts.DryRun && !s.opts.VerifyOnly {
		log.Printf("[%s] truncating target tables...", s.name)
		if err := s.truncateAll(ctx); err != nil {
			return fmt.Errorf("truncate: %w", err)
		}
	}

	for _, t := range s.tables {
		if err := s.migrateTable(ctx, t); err != nil {
			return fmt.Errorf("table %s: %w", t.source, err)
		}
	}
	log.Printf("[%s] all tables migrated.", s.name)
	return nil
}

// preflight asserts that target schema is up to date (goose has run) and
// guards against the operator pointing at a still-live source DB.
func (s *scope) preflight(ctx context.Context) error {
	var n int
	if err := s.mysql.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = 'goose_db_version'`,
	).Scan(&n); err != nil {
		return fmt.Errorf("check goose_db_version: %w", err)
	}
	if n == 0 {
		return errors.New("target MySQL has no goose_db_version table; start the service binary once to apply migrations before running this tool")
	}

	if !s.opts.TruncateBefore && !s.opts.DryRun && !s.opts.VerifyOnly {
		for _, t := range s.tables {
			rows, err := countMySQL(ctx, s.mysql, t.target)
			if err != nil {
				return fmt.Errorf("count %s: %w", t.target, err)
			}
			if rows > 0 {
				return fmt.Errorf("target table %s not empty (rows=%d); pass --truncate-before to overwrite", t.target, rows)
			}
		}
	}

	if !s.opts.AllowOrphans {
		if err := s.checkOrphans(ctx); err != nil {
			return err
		}
	}

	return nil
}

func (s *scope) truncateAll(ctx context.Context) error {
	if _, err := s.mysql.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 0"); err != nil {
		return err
	}
	defer s.mysql.ExecContext(ctx, "SET FOREIGN_KEY_CHECKS = 1")

	// Truncate in reverse migration order so children are wiped first; the
	// `tableMigration.children` slice carries every split-out child table.
	for i := len(s.tables) - 1; i >= 0; i-- {
		t := s.tables[i]
		for j := len(t.children) - 1; j >= 0; j-- {
			if _, err := s.mysql.ExecContext(ctx, "TRUNCATE TABLE `"+t.children[j]+"`"); err != nil {
				return fmt.Errorf("truncate child %s: %w", t.children[j], err)
			}
		}
		if _, err := s.mysql.ExecContext(ctx, "TRUNCATE TABLE `"+t.target+"`"); err != nil {
			return fmt.Errorf("truncate %s: %w", t.target, err)
		}
	}
	return nil
}

// checkOrphans only validates the few cross-table refs we know to be
// hot. The goal is to surface obviously-bad data ahead of a write storm,
// not to be exhaustive.
func (s *scope) checkOrphans(ctx context.Context) error {
	if s.name != "agent" {
		return nil
	}
	rows, err := s.sqlite.QueryContext(ctx, `
		SELECT ac.id, ac.model_id
		FROM agent_configs ac
		LEFT JOIN models m ON m.id = ac.model_id
		WHERE ac.model_id IS NOT NULL AND ac.model_id <> '' AND m.id IS NULL
	`)
	if err != nil {
		// agent_configs may not exist yet on first run; silently ignore.
		return nil
	}
	defer rows.Close()
	var bad []string
	for rows.Next() {
		var id, modelID string
		if err := rows.Scan(&id, &modelID); err != nil {
			return err
		}
		bad = append(bad, fmt.Sprintf("agent_configs.id=%s -> model_id=%s missing in models", id, modelID))
	}
	if len(bad) > 0 {
		return fmt.Errorf("orphan rows detected (pass --allow-orphans to clear them):\n  %s", strings.Join(bad, "\n  "))
	}
	return nil
}

func countMySQL(ctx context.Context, db *sql.DB, table string) (int64, error) {
	var n int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM `"+table+"`").Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func countSQLite(ctx context.Context, db *sql.DB, table string) (int64, error) {
	var n int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}
