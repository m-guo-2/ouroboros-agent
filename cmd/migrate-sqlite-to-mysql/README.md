# migrate-sqlite-to-mysql

One-shot, offline tool that copies legacy SQLite data
(`agent/data/config.db`, `channel-qiwei/qiwei.db`) into the new MySQL
schema produced by goose migrations.

> ⚠️ Run during a maintenance window with `agent` and `channel-qiwei`
> stopped. The tool reads SQLite files, never writes to them; the source
> remains a clean rollback point.

## Prereqs

1. MySQL 8.0 reachable; `moli_agent` and `moli_qiwei` databases created
   with `utf8mb4 / utf8mb4_bin`.
2. The `agent` and `channel-qiwei` binaries (or `goose up`) have been run
   once against the empty databases so the schema and `goose_db_version`
   are in place.
3. SQLite source file paths confirmed; no other process holds a write
   lock on them.

## Usage

```bash
# Build
go build -o bin/migrate-sqlite-to-mysql ./cmd/migrate-sqlite-to-mysql

# One scope at a time
bin/migrate-sqlite-to-mysql \
  --scope agent \
  --sqlite agent/data/config.db \
  --mysql-dsn "moli:secret@tcp(127.0.0.1:3306)/moli_agent?charset=utf8mb4&collation=utf8mb4_bin&loc=UTC&multiStatements=true"

bin/migrate-sqlite-to-mysql \
  --scope qiwei \
  --sqlite channel-qiwei/qiwei.db \
  --mysql-dsn "moli:secret@tcp(127.0.0.1:3306)/moli_qiwei?charset=utf8mb4&collation=utf8mb4_bin&loc=UTC&multiStatements=true"

# Both scopes in order
bin/migrate-sqlite-to-mysql --scope all \
  --sqlite-agent agent/data/config.db \
  --sqlite-qiwei channel-qiwei/qiwei.db \
  --mysql-dsn-agent "moli:secret@tcp(.../moli_agent?...)" \
  --mysql-dsn-qiwei "moli:secret@tcp(.../moli_qiwei?...)"
```

### Flags

| Flag                   | Default | Notes                                                |
| ---------------------- | ------- | ---------------------------------------------------- |
| `--scope`              | (req)   | `agent` / `qiwei` / `all`                            |
| `--sqlite`             | (req)   | Source SQLite (when scope is agent or qiwei)         |
| `--mysql-dsn`          | (req)   | Target MySQL DSN                                     |
| `--sqlite-agent` / `--sqlite-qiwei` / `--mysql-dsn-agent` / `--mysql-dsn-qiwei` | — | Used together with `--scope=all` |
| `--batch-size`         | 500     | Rows per insert batch                                |
| `--truncate-before`    | false   | Wipe target tables before importing (rerun support)  |
| `--dry-run`            | false   | Read + plan only; no writes                          |
| `--verify-only`        | false   | Skip writes; only compare counts                     |
| `--allow-orphans`      | false   | Allow `agent_configs.model_id → models.id` mismatches |

## What gets cleansed on the way through

- `agent_personas.created_at / updated_at` and
  `group_persona_assignments.*_at`: parsed from `YYYY-MM-DD HH:MM:SS`
  UTC strings into epoch ms. Empty → 0.
- `qiwei_*` epoch-second columns: multiplied by 1000.
- `agent_configs.skills`: both `["x"]` and `[{"id":"x","mode":"y"}]`
  shapes accepted, fanned into `agent_skill_bindings`.
- `agent_configs.{channels, hooks, subagent_models, subagent_skills}`:
  fanned into the matching child tables.
- `agent_personas.{skills, subagent_models, subagent_skills}`: same.
- `messages.tool_calls` / `messages.attachments_json`: split into
  `message_tool_calls` (only `tool_use` blocks) and `message_attachments`.
- `context_compaction_archives.archived_messages`: split into
  `context_compaction_archived_messages`.
- `qiwei_contacts.follow_user_json`: split into `qiwei_contact_followers`.
- `session_active_skills.activation_order`: when 0/empty, backfilled per
  `session_id` from sqlite `rowid` order, replacing the old
  `backfillSessionActiveSkillOrder` runtime helper.
- All new rows get `deleted_at = 0` regardless of source `enabled`.

## Verification

After copying each table the tool runs `COUNT(*)` on source vs target
and aborts on any mismatch. For tables with split children it also walks
the source JSON arrays and asserts the child row count matches the
sum-of-elements. Any mismatch terminates the migration with a `source=N
target=M` message.

## Safety contract

- The source SQLite files are opened read-only and never written.
- `--truncate-before` is the only way to overwrite a non-empty target
  database; otherwise the preflight aborts with `target table not empty`.
- All inserts for one parent row plus its split children land in a
  single transaction, so there is no half-imported row.
