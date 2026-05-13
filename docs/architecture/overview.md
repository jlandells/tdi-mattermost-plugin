# Architecture Overview

The plugin connects Mattermost plugin hooks to external policy endpoints. Each
enabled policy builds a JSON payload from the Mattermost event, sends it to the
configured policy service, and applies the response according to the hook
timing.

```text
Mattermost event -> plugin hook -> policy endpoint -> allow/reject
```

## Policy Contract

Policy requests are sent to the configured TDI-compatible URL pattern:

```text
{TDIURL}/ns/{TDINamespace}/policy/v1/{policy-path}
```

The expected response is JSON:

```json
{
  "action": "allow",
  "reason": "optional human-readable reason"
}
```

The plugin treats `allow` and `continue` as allowed actions. It treats
`reject`, `deny`, service errors, non-2xx responses, invalid JSON, and unknown
actions as denials for blocking hooks.

## Hook Timing

Mattermost hooks fall into three practical categories:

- Blocking hooks can stop the user action before Mattermost saves or sends it.
- Remediation hooks run after Mattermost has already accepted the action and can
  only undo or follow up where the API permits it.
- Audit hooks report events to TDI without changing the original action.

See `hook-behavior-matrix.md` for the detailed mapping.

## Repository Layout

```text
.
├── .github/workflows/          GitHub CI and release workflows
├── docs/                       Public documentation and reference notes
├── scripts/                    Verification and release helper scripts
├── webapp/                     Optional internal Mattermost webapp bundle
├── main.go                     Server plugin hooks and TDI client logic
├── configuration.go            Plugin configuration and validation
├── plugin.json                 Mattermost plugin manifest and settings schema
└── Makefile                    Build, verify, bundle, and cleanup targets
```

## Operational Notes

- Blocking policy calls fail secure.
- Debug logging redacts sensitive payload fields before logging.
- Each TDI request includes `X-Correlation-ID`.
- File upload checks spool uploads to a temporary file, hash content, and enforce
  `MaxFileInspectionBytes` before sending file metadata to TDI.
- Channel classification requires a Mattermost API token with permissions to
  search and assign access control policies.
