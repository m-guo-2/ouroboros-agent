package migrator

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"strings"
)

// tableMigration declares one source-table-to-target-table copy job. The
// children slice lists all child tables that this table fans out to via
// JSON splits; the runner uses it for TRUNCATE order and for the verify
// step.
//
// The actual row-level work happens inside `migrate`, which receives an
// open transaction and a single source row scanner. Implementations build
// the parent INSERT plus any split-child INSERTs and return the number of
// child rows inserted (used in verify).
type tableMigration struct {
	source   string                                                                                            // sqlite table name
	target   string                                                                                            // mysql table name (parent)
	children []string                                                                                          // mysql split-out child tables
	selectAt func(ctx context.Context, db *sql.DB) (*sql.Rows, error)                                          // SELECT * FROM source
	migrate  func(ctx context.Context, tx *sql.Tx, row *sql.Rows, childCounts map[string]int64) (int64, error) // returns parent row count delta (1 for normal rows)
	// verifyChildren, if set, gets called with the source DB so the table
	// can recompute the expected child row count by walking the source
	// JSON columns. It returns map[childTable]count.
	verifyChildren func(ctx context.Context, sqliteDB *sql.DB) (map[string]int64, error)
}

func (s *scope) migrateTable(ctx context.Context, t tableMigration) error {
	srcCount, err := countSQLite(ctx, s.sqlite, t.source)
	if err != nil {
		// missing source table is not fatal: legacy DBs may have been
		// stopped before the table was first touched. Treat as 0-rows.
		if strings.Contains(err.Error(), "no such table") {
			log.Printf("[%s] %s: source table missing in sqlite, skipping", s.name, t.source)
			return nil
		}
		return fmt.Errorf("count source: %w", err)
	}
	log.Printf("[%s] %s -> %s: source rows=%d", s.name, t.source, t.target, srcCount)

	if !s.opts.VerifyOnly && !s.opts.DryRun {
		if err := s.copyRows(ctx, t); err != nil {
			return err
		}
	} else if s.opts.DryRun {
		log.Printf("[%s] %s: dry-run, would copy %d rows", s.name, t.source, srcCount)
	}

	return s.verifyTable(ctx, t, srcCount)
}

func (s *scope) copyRows(ctx context.Context, t tableMigration) error {
	rows, err := t.selectAt(ctx, s.sqlite)
	if err != nil {
		return fmt.Errorf("select source: %w", err)
	}
	defer rows.Close()

	tx, err := s.mysql.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	childCounts := make(map[string]int64)
	batch := 0
	parentRows := int64(0)
	for rows.Next() {
		n, err := t.migrate(ctx, tx, rows, childCounts)
		if err != nil {
			return fmt.Errorf("row: %w", err)
		}
		parentRows += n
		batch++
		if batch >= s.opts.BatchSize {
			if err := tx.Commit(); err != nil {
				return fmt.Errorf("commit batch: %w", err)
			}
			committed = true
			tx, err = s.mysql.BeginTx(ctx, nil)
			if err != nil {
				return fmt.Errorf("begin next tx: %w", err)
			}
			committed = false
			batch = 0
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate source: %w", err)
	}
	if !committed {
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("final commit: %w", err)
		}
		committed = true
	}

	if len(childCounts) > 0 {
		log.Printf("[%s] %s: parent inserted=%d, children=%v", s.name, t.target, parentRows, childCounts)
	}
	return nil
}

func (s *scope) verifyTable(ctx context.Context, t tableMigration, srcCount int64) error {
	dstCount, err := countMySQL(ctx, s.mysql, t.target)
	if err != nil {
		return fmt.Errorf("count target: %w", err)
	}
	if dstCount != srcCount {
		return fmt.Errorf("%s: source=%d, target=%d", t.target, srcCount, dstCount)
	}

	if t.verifyChildren == nil {
		return nil
	}
	expected, err := t.verifyChildren(ctx, s.sqlite)
	if err != nil {
		return fmt.Errorf("verify children of %s: %w", t.target, err)
	}
	for child, want := range expected {
		got, err := countMySQL(ctx, s.mysql, child)
		if err != nil {
			return fmt.Errorf("count child %s: %w", child, err)
		}
		if got != want {
			return fmt.Errorf("child %s: source-expanded=%d, target=%d", child, want, got)
		}
	}
	return nil
}
