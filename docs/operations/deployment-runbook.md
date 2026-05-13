# Deployment Runbook

This is a placeholder runbook to fill as production assumptions are confirmed.

## Required Inputs

- Mattermost server version and edition.
- TDI/Direktiv version and namespace.
- Enabled policy list.
- TDI base URL and authentication method.
- Mattermost API token owner, scope, and rotation process.
- Plugin bundle version and checksum.

## Preflight Checks

- Plugin bundle installs and starts on the target Mattermost version.
- TDI namespace exists.
- Every enabled policy has a deployed TDI gateway and workflow.
- Mattermost can reach TDI from the server network.
- Access control policy APIs are available if channel classification is enabled.
- Bot user exists and can create posts/DMs.

## Failure Modes To Document

- TDI unavailable.
- TDI timeout.
- Invalid TDI response.
- Mattermost API token expired or under-scoped.
- Channel policy assignment API failure.
- File upload policy failure during large uploads.

