# Production Readiness Plan

## Current Status

Core hardening work is in place: configuration validation, centralized TDI
response handling, correlation IDs, redacted debug logging, bounded file upload
inspection, bot user creation, focused Go tests, GitHub CI, and tag-driven
release artifacts.

Remaining production work is mainly environment validation: integration tests
against Mattermost and the configured policy service, deployment-specific
runbooks, metrics, token rotation ownership, and final public repository
metadata such as license and support policy.

## Phase 1: Reference Baseline

- Mattermost REST API endpoint notes captured in `docs/references/`.
- Mattermost plugin SDK hook notes captured in `docs/references/`.
- Policy endpoint contract documented in `docs/architecture/overview.md`.
- Remaining task: validate all enabled hooks in a deployed Mattermost test
  instance.

## Phase 2: Correctness And Safety

- Plugin bot user creation is implemented.
- Configuration validation during `OnConfigurationChange` is implemented.
- TDI client behavior and response parsing are centralized.
- Correlation IDs and debug payload redaction are implemented.
- Hook behavior is documented as blocking, remediation, or audit-only.

## Phase 3: Scalability And Failure Handling

- Bounded file upload spool/hash behavior is implemented.
- Define timeout budgets per hook.
- Define retry policy only for safe/idempotent calls.
- Add operational metrics for policy latency, denies, timeouts, and TDI errors.

## Phase 4: Test Coverage

- Go tests cover config validation, TDI client parsing, bot behavior, and file
  upload handling.
- Add HTTP handler tests for plugin-local APIs.
- Add webapp tests for channel classification states.
- Add an integration test plan for Mattermost plus a policy-service test double.

## Phase 5: Release And Operations

- GitHub CI verifies Go tests and webapp build.
- GitHub release workflow produces versioned plugin bundles and checksums
  from tags.
- Generated outputs are ignored and not tracked.
- Deployment, rollback, token rotation, and failure-mode runbooks are started in
  `docs/operations/`.
