# agent/data — historical SQLite seed scripts

This directory keeps the legacy `*.sql` runbook scripts that operators used
to seed `data/config.db` while the agent ran on SQLite. After the move to
MySQL the canonical schema + seeds live under
`agent/internal/storage/migrations/` and are applied automatically by goose
on startup.

| File                              | Status                | New home                                          |
| --------------------------------- | --------------------- | ------------------------------------------------- |
| `043-wecom-skills.sql`            | superseded            | `migrations/00002_seed_wecom_skills.sql`           |
| `043-wecom-skills-apply.py`       | superseded (one-off)  | `cmd/migrate-sqlite-to-mysql/` handles legacy data |
| `047-prompt-transparent.sql`      | absorbed into 043 / 00002 prompt | n/a (logical superset already in `system_prompt`) |
| `053-wechat-builtin-agent.sql`    | not auto-applied      | apply manually via repo SQL if you still want this agent variant |
| `055-proactive-delayed-tasks.sql` | not auto-applied      | apply manually via repo SQL if you still want the proactive prompt |

Files are kept here for historical reference and for the data-migration
tool. New seeds should go into `agent/internal/storage/migrations/` as a
`0000N_*.sql` goose file.
