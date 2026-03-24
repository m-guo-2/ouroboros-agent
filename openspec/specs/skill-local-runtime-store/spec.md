# skill-local-runtime-store Specification

## Purpose
TBD - created by archiving change skill-local-runtime-store. Update Purpose after archive.
## Requirements
### Requirement: Runtime skill metadata SHALL come from the local store
The system SHALL persist each synchronized skill's runtime metadata in the local `skills` store and SHALL compile agent skill snippets, full prompt previews, and skill list reads from that local metadata instead of GitHub in-memory cache state.

#### Scenario: Bound skill appears in prompt from local metadata
- **WHEN** an agent is bound to a skill whose metadata exists locally and is enabled
- **THEN** the system SHALL compile the skill's name and description into the runtime skills snippet from the local store

#### Scenario: Process restart does not remove prompt-visible skills
- **WHEN** the process restarts after a previous successful skill sync and before a new GitHub refresh completes
- **THEN** the system SHALL still be able to compile the bound skill snippet from the local store

#### Scenario: Disabled local skill is excluded deterministically
- **WHEN** a bound skill exists in the local store but is marked disabled
- **THEN** the system SHALL exclude that skill from the runtime snippet based on the local store state

### Requirement: Runtime skill content SHALL come from local files
The system SHALL read `SKILL.md`, `references/`, and `scripts/` from the synchronized local skill directory during runtime operations. Runtime skill detail loading and script execution MUST NOT depend on GitHub store memory state.

#### Scenario: load_skill reads local skill document
- **WHEN** `load_skill` is called for a bound skill whose local `SKILL.md` exists
- **THEN** the system SHALL return the skill content from the local file copy

#### Scenario: load_skill_reference reads local reference file
- **WHEN** `load_skill_reference` is called for a reference listed in the local skill metadata
- **THEN** the system SHALL read and return the content from the local `references/` directory

#### Scenario: run_script executes synchronized local script
- **WHEN** `run_script` is called for a script exposed by a synchronized skill
- **THEN** the system SHALL execute the script from the local skill directory instead of remote or in-memory content

### Requirement: Skill synchronization SHALL materialize a complete local runtime copy
The synchronization pipeline SHALL fetch skills from GitHub, write the local file copy, and upsert local metadata required for runtime reads. A skill SHALL only become visible to runtime readers after its required local files are materialized.

#### Scenario: Successful sync publishes local metadata after files exist
- **WHEN** a skill sync succeeds
- **THEN** the system SHALL write the local `SKILL.md` and related resources before exposing the synchronized metadata to runtime readers

#### Scenario: Sync refresh updates local runtime state
- **WHEN** a periodic or manual refresh pulls a changed skill from GitHub
- **THEN** the system SHALL update both the local files and the local metadata used by runtime prompt compilation

### Requirement: Missing local skill state SHALL be diagnosable
The system SHALL distinguish runtime skill misses caused by unbound skills, absent local metadata, disabled skills, and missing local files. These cases MUST produce explicit runtime diagnostics or errors instead of silently collapsing into an empty skill snippet.

#### Scenario: Bound skill missing from local metadata is reported
- **WHEN** an agent is bound to a skill ID that is not present in the local skill store
- **THEN** the system SHALL emit a diagnostic indicating that local metadata is missing for the bound skill

#### Scenario: Skill detail file missing is reported
- **WHEN** runtime metadata exists for a skill but the local `SKILL.md` file is missing
- **THEN** `load_skill` SHALL fail with an explicit local file missing error

#### Scenario: Unbound skill remains distinguishable from sync failure
- **WHEN** a skill ID is not bound to the current agent
- **THEN** the system SHALL treat it as an authorization/binding miss rather than a synchronization failure

