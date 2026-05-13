# Mattermost REST API Reference Notes

Source:
https://developers.mattermost.com/api-documentation/

Related API reference entry point:
https://api.mattermost.com/

## Purpose In This Plugin

The plugin uses Mattermost REST APIs for operations that are not directly
covered by the plugin API, especially access control policy discovery and
assignment for channel classification.

## Current Plugin Usage

Implemented in `main.go`:

- `POST /api/v4/access_control_policies/search`
- `GET /api/v4/access_control_policies/{policy_id}/resources/channels`
- `POST /api/v4/access_control_policies/{policy_id}/assign`
- `DELETE /api/v4/access_control_policies/{policy_id}/unassign`

## Production Questions To Resolve

- Confirm the minimum Mattermost edition and license needed for access control
  policy APIs.
- Confirm whether access control policy endpoints are public, stable, or
  subject to Enterprise-specific changes.
- Define the narrowest acceptable token scope for `MattermostAPIToken`.
- Confirm pagination semantics for policy resource channel lists.
- Confirm expected response shapes for assign/unassign failure cases.

## Implementation Rules

- Do not build URLs by hand when path components can contain reserved
  characters; use `url.PathEscape` for identifiers.
- Bound all REST calls with context timeouts.
- Redact the Mattermost API token from logs.
- Treat non-2xx responses as operational errors and include enough context for
  administrators to diagnose without leaking secrets.

