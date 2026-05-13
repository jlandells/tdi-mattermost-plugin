# Policy Endpoint Contract

## Purpose In This Plugin

This repository is focused on the Mattermost plugin. It does not document how to
install, deploy, or configure the external policy service.

The plugin treats the configured policy service as the external policy decision
point. Mattermost remains the policy enforcement point. Plugin hooks call
TDI-compatible endpoint paths and interpret the response as either allow or
deny.

## Current Contract

Plugin request URL pattern:

```text
{TDIURL}/ns/{TDINamespace}/policy/v1/{policy-path}
```

Request headers:

```http
Content-Type: application/json
X-Correlation-ID: <generated-correlation-id>
X-TDI-Key: <configured TDIAPIKey>
```

`X-TDI-Key` is sent only when `TDIAPIKey` is configured.

Expected allow response:

```json
{
  "status": "success",
  "action": "continue",
  "result": {}
}
```

Expected deny response:

```json
{
  "status": "success",
  "action": "reject",
  "result": {
    "reason": "Human-readable denial reason"
  }
}
```

## Implementation Rules

- Fail secure for pre-action blocking hooks unless an explicit policy says
  otherwise.
- Fail observable for audit-only hooks: log/report failure without disrupting
  already-completed Mattermost actions.
- Include a correlation ID in every policy request and log entry. The plugin
  sends this to the policy service in the `X-Correlation-ID` header.
- If `TDIAPIKey` is configured, send it only in `X-TDI-Key`. Do not send it as
  an `Authorization` bearer token.
- Do not log raw policy payloads. Debug logs must redact message content,
  channel headers, user attributes, email addresses, file hashes, API keys,
  authorization values, and denial reasons.
- Keep request/response schemas under version control once finalized.
