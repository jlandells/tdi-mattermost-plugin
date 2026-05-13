# Production Readiness Plan

## Phase 1: Reference Baseline

- Verify all Mattermost REST API endpoints used by this plugin.
- Verify all Mattermost plugin SDK hooks and return semantics.
- Verify TDI gateway request/response contracts.
- Convert unresolved questions in `docs/references/` into implementation tasks.

## Phase 2: Correctness And Safety

- Create and persist a real plugin bot user.
- Validate plugin configuration during `OnConfigurationChange`.
- Centralize TDI client behavior and response parsing.
- Add correlation IDs, structured logs, and secret redaction.
- Classify every hook as blocking, remediation, or audit-only in code and docs.

## Phase 3: Scalability And Failure Handling

- Replace full in-memory file upload processing with bounded streaming behavior.
- Define timeout budgets per hook.
- Define retry policy only for safe/idempotent calls.
- Add operational metrics for policy latency, denies, timeouts, and TDI errors.

## Phase 4: Test Coverage

- Add Go tests for config validation, TDI client parsing, and policy decisions.
- Add HTTP handler tests for plugin-local APIs.
- Add webapp tests for channel classification states.
- Add workflow/helper tests for CEL and STANAG mapping.
- Add an integration test plan for Mattermost plus TDI.

## Phase 5: Release And Operations

- Add CI for Go, webapp, and bundle creation.
- Remove generated binaries from source control.
- Produce versioned plugin bundles with checksums.
- Add deployment, rollback, token rotation, and TDI outage runbooks.

