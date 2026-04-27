// Command migrate-sqlite-to-mysql is the one-shot, offline migration tool
// that copies legacy SQLite data (agent/data/config.db,
// channel-qiwei/qiwei.db) into the new MySQL schema produced by goose
// migrations.
//
// Usage:
//
//	migrate-sqlite-to-mysql --scope agent  --sqlite PATH --mysql-dsn DSN [--batch-size N] [--truncate-before] [--dry-run] [--verify-only]
//	migrate-sqlite-to-mysql --scope qiwei  --sqlite PATH --mysql-dsn DSN [...]
//	migrate-sqlite-to-mysql --scope all    --sqlite-agent PATH --sqlite-qiwei PATH \
//	                                      --mysql-dsn-agent DSN --mysql-dsn-qiwei DSN [...]
//
// The tool MUST run during a maintenance window with all consumers stopped.
// Source SQLite files are read-only inputs; this binary never modifies them.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/m-guo-2/ouroboros-agent/cmd/migrate-sqlite-to-mysql/internal/migrator"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		log.Printf("migrate-sqlite-to-mysql: %v", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("migrate-sqlite-to-mysql", flag.ContinueOnError)
	var (
		scope          = fs.String("scope", "", "agent|qiwei|all (required)")
		sqlitePath     = fs.String("sqlite", "", "path to source sqlite file (when --scope=agent|qiwei)")
		mysqlDSN       = fs.String("mysql-dsn", "", "target MySQL DSN (when --scope=agent|qiwei)")
		sqliteAgent    = fs.String("sqlite-agent", "", "path to agent sqlite (when --scope=all)")
		sqliteQiwei    = fs.String("sqlite-qiwei", "", "path to qiwei sqlite (when --scope=all)")
		mysqlDSNAgent  = fs.String("mysql-dsn-agent", "", "MySQL DSN for moli_agent (when --scope=all)")
		mysqlDSNQiwei  = fs.String("mysql-dsn-qiwei", "", "MySQL DSN for moli_qiwei (when --scope=all)")
		batchSize      = fs.Int("batch-size", 500, "rows per insert batch")
		truncateBefore = fs.Bool("truncate-before", false, "truncate target tables before importing")
		dryRun         = fs.Bool("dry-run", false, "read source and print plan only; do not write target")
		verifyOnly     = fs.Bool("verify-only", false, "skip writes; only compare row counts")
		allowOrphans   = fs.Bool("allow-orphans", false, "allow orphan FK-like references; will null them out instead of aborting")
	)
	if err := fs.Parse(args); err != nil {
		return err
	}

	scopeNorm := strings.ToLower(strings.TrimSpace(*scope))
	if scopeNorm == "" {
		return errors.New("--scope is required (agent|qiwei|all)")
	}
	if *batchSize <= 0 {
		return errors.New("--batch-size must be > 0")
	}

	opts := migrator.Options{
		BatchSize:      *batchSize,
		TruncateBefore: *truncateBefore,
		DryRun:         *dryRun,
		VerifyOnly:     *verifyOnly,
		AllowOrphans:   *allowOrphans,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	start := time.Now()
	switch scopeNorm {
	case "agent":
		if *sqlitePath == "" || *mysqlDSN == "" {
			return errors.New("--sqlite and --mysql-dsn are required for --scope=agent")
		}
		if err := migrator.RunAgent(ctx, *sqlitePath, *mysqlDSN, opts); err != nil {
			return fmt.Errorf("agent: %w", err)
		}
	case "qiwei":
		if *sqlitePath == "" || *mysqlDSN == "" {
			return errors.New("--sqlite and --mysql-dsn are required for --scope=qiwei")
		}
		if err := migrator.RunQiwei(ctx, *sqlitePath, *mysqlDSN, opts); err != nil {
			return fmt.Errorf("qiwei: %w", err)
		}
	case "all":
		if *sqliteAgent == "" || *sqliteQiwei == "" || *mysqlDSNAgent == "" || *mysqlDSNQiwei == "" {
			return errors.New("--sqlite-agent, --sqlite-qiwei, --mysql-dsn-agent, --mysql-dsn-qiwei are required for --scope=all")
		}
		if err := migrator.RunAgent(ctx, *sqliteAgent, *mysqlDSNAgent, opts); err != nil {
			return fmt.Errorf("agent: %w", err)
		}
		if err := migrator.RunQiwei(ctx, *sqliteQiwei, *mysqlDSNQiwei, opts); err != nil {
			return fmt.Errorf("qiwei: %w", err)
		}
	default:
		return fmt.Errorf("unknown --scope=%q (expected agent|qiwei|all)", scopeNorm)
	}

	log.Printf("done in %s", time.Since(start).Round(time.Millisecond))
	return nil
}
