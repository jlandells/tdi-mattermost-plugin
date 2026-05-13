# Deployment Runbook

Use this checklist before installing or upgrading the plugin in a production
Mattermost environment.

## Required Inputs

- Mattermost server version and edition.
- Policy service base URL and namespace.
- Enabled policy list.
- Policy service authentication method.
- Mattermost API token owner, scope, and rotation process.
- Plugin bundle version and checksum.

## Preflight Checks

- Plugin bundle checksum matches the release `SHA256SUMS` file.
- Plugin bundle installs and starts on the target Mattermost version.
- Every enabled policy has a matching external policy endpoint.
- Mattermost can reach the policy service from the server network.
- Access control policy APIs are available if channel classification is enabled.
- Plugin bot user is created or can be created by Mattermost.
- Debug logging is disabled unless actively troubleshooting.
- `PolicyTimeout` is set to a value that matches the site latency budget.
- `MaxFileInspectionBytes` is set to a value the Mattermost server can spool
  safely.

## Deployment

1. Upload and enable the plugin in Mattermost.
2. Configure policy service connection settings.
3. Enable one policy at a time, starting with an audit-only policy where
   possible.
4. Confirm the policy service receives the request and returns a valid action.
5. Enable blocking policies after their endpoints have been tested.
6. Record the plugin version, checksum, policy namespace, and enabled policy
   list.

## Rollback

1. Disable the plugin in Mattermost.
2. Reinstall the previous known-good release bundle.
3. Reapply the previous plugin settings if Mattermost does not preserve them.
4. Confirm blocking hooks are behaving as expected.

## Token Rotation

1. Create a replacement Mattermost API token with the same minimum privileges.
2. Update the plugin `MattermostAPIToken` setting.
3. Test channel policy search and assignment.
4. Revoke the old token.

## Failure Modes

- Policy service unavailable: blocking hooks deny by design.
- Policy service timeout: blocking hooks deny by design.
- Invalid policy response: blocking hooks deny by design.
- Mattermost API token expired or under-scoped: channel classification fails.
- Channel policy assignment API failure: channel creation continues but policy
  assignment may be incomplete.
- File upload policy failure: uploads are denied when the file policy is enabled.
