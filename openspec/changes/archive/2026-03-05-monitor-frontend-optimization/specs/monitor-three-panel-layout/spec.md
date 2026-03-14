## ADDED Requirements

### Requirement: Three-panel layout structure

The monitor page SHALL use a three-panel layout: Session List (left), Conversation Timeline (center), Decision Inspector (right).

#### Scenario: Default layout

- **WHEN** the monitor page loads
- **THEN** three panels SHALL be visible: a fixed-width session list on the left, a flexible-width conversation timeline in the center, and a flexible-width decision inspector on the right

#### Scenario: Inspector collapse

- **WHEN** the user clicks the collapse button on the decision inspector
- **THEN** the inspector panel SHALL collapse and the conversation timeline SHALL expand to fill the remaining width

#### Scenario: Inspector expand on exchange click

- **WHEN** the inspector is collapsed AND the user clicks an exchange in the timeline
- **THEN** the inspector SHALL expand to show the decision detail for that exchange

### Requirement: Component decomposition

The monitor page SHALL be decomposed from a single file into focused component files under `monitor/components/`, `monitor/hooks/`, and `monitor/lib/`.

#### Scenario: Component files exist

- **WHEN** the refactoring is complete
- **THEN** the following directory structure SHALL exist under `admin/src/components/features/monitor/`:
  - `monitor-page.tsx` (three-panel layout + top-level state)
  - `components/` (UI components)
  - `hooks/` (data fetching hooks)
  - `lib/` (utility functions and local types)

#### Scenario: Functional equivalence for existing features

- **WHEN** the refactoring is complete
- **THEN** all existing monitoring capabilities (session list, search, deletion, trace viewing, message viewing) SHALL continue to work identically
