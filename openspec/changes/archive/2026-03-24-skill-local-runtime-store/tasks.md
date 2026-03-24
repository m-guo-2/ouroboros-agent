# Tasks

## 1. Local skill metadata store

- [x] 1.1 Define the runtime metadata contract for the local `skills` table, including how `base_path`, `scripts`, `references`, `synced_at`, and source revision are stored
- [x] 1.2 Update the skill storage helpers to read runtime skill snippets, skill lists, and bound-skill lookups from SQLite instead of GitHub cache accessors
- [x] 1.3 Add explicit diagnostics for bound-skill misses, disabled skills, and missing local metadata during snippet compilation

## 2. Synchronization pipeline

- [x] 2.1 Refactor the GitHub skill store so refresh writes local `SKILL.md`, `scripts/`, and `references/` before publishing updated runtime metadata
- [x] 2.2 Upsert synchronized skill metadata into the local `skills` table after local files are materialized and validated
- [x] 2.3 Ensure manual refresh and periodic sync keep the local DB and local file tree in sync for changed and removed skills

## 3. Runtime local file reads

- [x] 3.1 Update `GetSkillDetail()` to resolve skill metadata from the local store and read `SKILL.md` from the synchronized local directory
- [x] 3.2 Update `GetSkillReference()` to validate references against local metadata and read from the local `references/` directory
- [x] 3.3 Update `run_script` to resolve and execute scripts from the synchronized local skill directory, with explicit errors for missing files

## 4. Validation and observability

- [x] 4.1 Verify that runtime prompt compilation still includes bound skills after process restart before a new GitHub refresh completes
- [x] 4.2 Add regression coverage or diagnostics checks for local-only runtime reads, including missing file and missing metadata cases
- [x] 4.3 Confirm `openspec` acceptance by checking that proposal, design, spec, and tasks remain consistent with the local-runtime-source-of-truth design
