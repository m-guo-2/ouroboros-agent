## ADDED Requirements

### Requirement: Unified LLM I/O data hook

A single `useLLMIO(traceId, ref)` hook SHALL be the sole entry point for fetching LLM I/O data. All components requiring LLM I/O data SHALL use this hook.

#### Scenario: Single network request for same ref

- **WHEN** multiple components render for the same trace and ref
- **THEN** only one network request SHALL be made; subsequent components SHALL receive data from React Query cache

#### Scenario: Consistent query key

- **WHEN** `useLLMIO` is called with traceId "abc" and ref "def"
- **THEN** the React Query key SHALL be `["llm-io", "abc", "def"]`

### Requirement: LLM I/O cache policy

LLM I/O data SHALL be cached with `staleTime: Infinity` since payloads are immutable.

#### Scenario: No refetch on remount

- **WHEN** a component using `useLLMIO` unmounts and remounts
- **THEN** data SHALL be served from cache without a network request
