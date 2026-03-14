## ADDED Requirements

### Requirement: Remove unused monitoring code

The system SHALL remove orphaned monitoring-related code that has no backend implementation or frontend usage.

#### Scenario: Remove unused logs API

- **WHEN** the cleanup is complete
- **THEN** `admin/src/api/logs.ts` SHALL be deleted and `LogEntry` type SHALL be removed from `admin/src/api/types.ts`

#### Scenario: Remove unused useRecentTraces hook

- **WHEN** the cleanup is complete
- **THEN** the `useRecentTraces` export SHALL be removed from `admin/src/hooks/use-monitor.ts`
