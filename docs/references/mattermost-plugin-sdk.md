# Mattermost Plugin SDK Reference Notes

Sources:

- Plugin overview: https://developers.mattermost.com/integrate/plugins/
- Server plugin SDK reference: https://developers.mattermost.com/integrate/reference/server/server-reference/
- Webapp plugin SDK reference: https://developers.mattermost.com/integrate/reference/webapp/webapp-reference/
- Integration overview: https://developers.mattermost.com/integrate/getting-started/

## Purpose In This Plugin

The server plugin intercepts Mattermost actions, builds a policy request, and
delegates allow/deny decisions to TDI. The webapp adds the channel
classification right-hand sidebar and the team-scoping admin-console field.

## Server SDK Areas To Track

- Hook timing: whether each hook runs before or after Mattermost commits the
  action.
- Hook return semantics: whether the hook can block, mutate, filter, or only
  observe.
- Plugin HTTP routing and authenticated request headers.
- Bot account lifecycle APIs.
- File upload hook streaming semantics.
- Mattermost version constraints for newer hooks.

## Webapp SDK Areas To Track

- RHS component registration.
- Channel header button action registration.
- Current channel and permission selectors.
- CSRF handling for plugin-local HTTP APIs.
- Compatibility with the bundled Mattermost webapp React/runtime versions.

## Production Questions To Resolve

- Confirm every implemented hook exists in the configured Mattermost minimum
  supported version.
- Confirm which hooks are truly blocking versus remediation/audit only.
- Confirm whether plugin HTTP endpoints receive trusted `Mattermost-User-Id`
  headers only after Mattermost authentication.
- Confirm expected behavior for `NotificationWillBePushed`; the current code
  returns `nil, ""` when allowed.
- Confirm supported React and Mattermost Redux versions for the webapp bundle.

## Implementation Rules

- Keep hook behavior documented in `docs/architecture/hook-behavior-matrix.md`.
- Use the plugin API for Mattermost operations when available; use REST only
  when the plugin API lacks the required capability.
- Create a real bot user during plugin activation before sending DMs or system
  notifications.
- Keep server-side authorization checks authoritative. Webapp permission checks
  are UX only.

