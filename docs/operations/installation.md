# Installation

This guide covers installing and configuring the Mattermost plugin.

## Prerequisites

- Mattermost Server `9.0.0` or later.
- System administrator access in Mattermost.
- Reachable external policy endpoints for each enabled policy path when policy
  checks are enabled.
- A Mattermost API token if channel classification is enabled.

## Install From Release

1. Download `com.archtis.mattermost-policy-plugin-<version>.tar.gz` from the
   GitHub release.
2. In Mattermost, open System Console > Plugins > Plugin Management.
3. Upload the tarball and enable the plugin.
4. Open the plugin settings page and configure:
   - `TDIURL`
   - `TDINamespace`
   - `TDIAPIKey`, if required by the policy service. The plugin sends this as
     `X-TDI-Key`, not as an `Authorization` bearer token.
   - policy toggles for the endpoints available to this plugin

All policy toggles are disabled by default. This allows setup scripts to enable
the plugin first, then write policy-service settings, then enable specific
policy checks.

## Build Locally

```bash
make verify
make bundle
```

The bundle is written to:

```text
dist/com.archtis.mattermost-policy-plugin-<version>.tar.gz
```

It contains the Linux server binaries (amd64 + arm64), the webapp bundle, and
`plugin.json`.

## Policy Endpoints

For every enabled policy, the plugin sends requests to:

```text
{TDIURL}/ns/{TDINamespace}/policy/v1/{policy-path}
```

When `TDIAPIKey` is configured, requests include:

```http
X-TDI-Key: <configured TDIAPIKey>
```

This repository intentionally does not document how to install or configure the
external policy service. It only documents the endpoint paths and response shape
required by the Mattermost plugin.

The plugin expects a JSON response with an action:

```json
{
  "action": "allow",
  "reason": "optional reason"
}
```

Allowed actions are `allow` and `continue`. Rejected actions are `reject` and
`deny`. Unknown actions, invalid JSON, non-2xx status codes, and timeouts are
treated as denials by blocking hooks.

## Verification

After enabling the plugin:

1. Confirm the plugin starts in Mattermost logs.
2. Confirm the Mattermost server can reach the configured policy service base
   URL.
3. Enable one policy at a time and test the matching endpoint.
4. Verify denied actions include a clear reason in Mattermost.
5. Check service and Mattermost logs for the same `X-Correlation-ID`.

## Troubleshooting

| Symptom | Check |
| --- | --- |
| Plugin does not start | Mattermost plugin logs, server version, plugin tarball architecture |
| Every action is denied | Policy service URL, namespace, endpoint path, API key, response JSON |
| Policy is not called | Matching plugin setting is enabled and hook is supported by server version |
| Channel classification fails | Mattermost API token permissions and access control policy availability |
| File upload denied | `MaxFileInspectionBytes`, file policy response, and policy timeout |
