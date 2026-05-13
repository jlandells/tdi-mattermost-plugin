# TDI / Direktiv Reference Notes

Source:
https://docs.direktiv.io/

## Purpose In This Plugin

TDI is the external policy decision point. Mattermost remains the policy
enforcement point. Plugin hooks call TDI gateway endpoints and interpret the
workflow response as either allow or deny.

## Current Contract

Plugin request URL pattern:

```text
{TDIURL}/ns/{TDINamespace}/policy/v1/{policy-path}
```

Expected successful workflow response:

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

## Production Questions To Resolve

- Confirm the stable gateway API shape for the deployed TDI version.
- Confirm authentication mechanism: bearer API key, gateway auth plugin, mTLS,
  or another deployment-specific control.
- Define timeout budgets per hook.
- Define whether each endpoint is idempotent and whether retries are allowed.
- Define workflow deployment/versioning strategy.
- Define expected audit behavior when Mattermost has already committed the
  action.

## Implementation Rules

- Fail secure for pre-action blocking hooks unless an explicit policy says
  otherwise.
- Fail observable for audit-only hooks: log/report failure without disrupting
  already-completed Mattermost actions.
- Include a correlation ID in every policy request and log entry. The plugin
  sends this to TDI in the `X-Correlation-ID` header.
- Do not log raw policy payloads. Debug logs must redact message content,
  channel headers, user attributes, email addresses, file hashes, API keys,
  authorization values, and denial reasons.
- Keep request/response schemas under version control once finalized.
