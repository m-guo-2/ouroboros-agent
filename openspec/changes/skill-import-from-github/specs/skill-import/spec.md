## ADDED Requirements

### Requirement: System SHALL parse GitHub URLs into structured source parameters
The system SHALL accept a GitHub URL string and extract the owner, repository name, branch, and sub-path. The parser SHALL support full browser URLs (`https://github.com/owner/repo/tree/branch/path`) and shorthand notation (`owner/repo`). When branch is not present in the URL, the system SHALL default to `main`. When path is not present, the system SHALL default to the repository root.

#### Scenario: Full URL with branch and path
- **WHEN** the URL is `https://github.com/user/awesome-skills/tree/main/skills`
- **THEN** the system SHALL extract owner=`user`, repo=`awesome-skills`, branch=`main`, path=`skills`

#### Scenario: Repo URL without path
- **WHEN** the URL is `https://github.com/user/awesome-skills`
- **THEN** the system SHALL extract owner=`user`, repo=`awesome-skills`, branch=`main`, path=`` (root)

#### Scenario: Shorthand notation
- **WHEN** the URL is `user/awesome-skills`
- **THEN** the system SHALL extract owner=`user`, repo=`awesome-skills`, branch=`main`, path=`` (root)

#### Scenario: Blob URL pointing to a file
- **WHEN** the URL is `https://github.com/user/repo/blob/main/skills/my-skill/SKILL.md`
- **THEN** the system SHALL extract the parent directory path as `skills/my-skill`

#### Scenario: Invalid URL
- **WHEN** the URL cannot be parsed into owner and repo
- **THEN** the system SHALL return a descriptive error

### Requirement: System SHALL browse available skills from a public GitHub repository
The system SHALL list all skill directories at the specified path in the target public repository. A directory is considered a skill if it contains a `SKILL.md` file. For each discovered skill, the system SHALL read the SKILL.md frontmatter to extract the skill name and description. The system SHALL also indicate whether each skill already exists in the local skills repository.

#### Scenario: Browse a path containing multiple skills
- **WHEN** the user requests to browse `https://github.com/user/repo/tree/main/skills` and that path contains subdirectories `coding-style/`, `api-design/`, and `readme.md`
- **THEN** the system SHALL return skill entries for `coding-style` and `api-design` (directories containing SKILL.md) and SHALL exclude `readme.md` (not a directory)

#### Scenario: Browse a path where some skills already exist locally
- **WHEN** the browsed repository contains skill `coding-style` and the local skills repo already has a skill with ID `coding-style`
- **THEN** the system SHALL mark that skill entry with `exists: true`

#### Scenario: Browse a path with no skills
- **WHEN** the browsed path has no subdirectories containing SKILL.md
- **THEN** the system SHALL return an empty skill list without error

#### Scenario: Browse a non-existent path
- **WHEN** the specified path does not exist in the target repository
- **THEN** the system SHALL return an error indicating the path was not found

#### Scenario: Browse uses unauthenticated requests
- **WHEN** the system browses a public GitHub repository
- **THEN** the system SHALL NOT send an authentication token in the request

### Requirement: System SHALL import selected skills by committing them to the user's skills GitHub repository
The system SHALL read the complete skill directory (`SKILL.md`, `scripts/`, `references/`) from the source public repository and commit each file to the user's configured `skills_repo` GitHub repository via the GitHub Contents API (`PutFile`). This ensures imported skills are persistently stored in the same way as manually created skills. After committing all files, the system SHALL trigger a skills refresh to update the local cache.

#### Scenario: Import a single skill commits to GitHub
- **WHEN** the user selects skill `api-design` for import from `user/repo` at path `skills`
- **THEN** the system SHALL read `skills/api-design/SKILL.md` from the source repo and commit it to `{skills_path}/api-design/SKILL.md` in the user's skills GitHub repo via PutFile
- **AND** the system SHALL commit `scripts/` and `references/` files if they exist in the source

#### Scenario: Import multiple skills in batch
- **WHEN** the user selects skills `coding-style` and `api-design` for import
- **THEN** the system SHALL import both skills and return the count of successfully imported skills

#### Scenario: Import triggers refresh
- **WHEN** skill import completes successfully
- **THEN** the system SHALL trigger a skills refresh so the imported skills appear in the runtime cache

#### Scenario: Import skill with ID conflict is rejected by default
- **WHEN** the user attempts to import skill `coding-style` and a skill with that ID already exists in the local repo
- **THEN** the system SHALL reject the import for that skill with a conflict error

#### Scenario: Import skill with ID conflict and overwrite flag
- **WHEN** the user attempts to import skill `coding-style` with `overwrite: true` and a skill with that ID already exists
- **THEN** the system SHALL overwrite the existing skill files with the source content

#### Scenario: Source skill missing SKILL.md
- **WHEN** a selected skill directory does not contain a SKILL.md file in the source repo
- **THEN** the system SHALL skip that skill and include it in the error report

### Requirement: Admin UI SHALL provide a skill import dialog
The admin interface SHALL provide an "Import" entry point on the skills list page. The dialog SHALL support a two-phase flow: first entering a GitHub URL and browsing available skills, then selecting and importing the desired skills.

#### Scenario: User opens the import dialog
- **WHEN** the user clicks the "Import" button on the skills list page
- **THEN** the system SHALL display a dialog with a URL input field and a "Browse" action

#### Scenario: User browses and sees available skills
- **WHEN** the user enters a URL and clicks "Browse"
- **THEN** the system SHALL display a list of available skills with checkboxes, showing name, description, and whether each skill already exists locally

#### Scenario: User imports selected skills
- **WHEN** the user selects one or more skills and clicks "Import"
- **THEN** the system SHALL call the import API and refresh the skills list upon success

#### Scenario: Import result feedback
- **WHEN** the import operation completes
- **THEN** the system SHALL display the number of successfully imported skills and list any failures with reasons
