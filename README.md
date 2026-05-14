# TDI Mattermost Policy Plugin

This repository contains a Mattermost plugin that sends selected Mattermost
events to TDI, formerly Direktiv, for external policy decisions and audit.

The plugin supports blocking checks where Mattermost exposes a pre-action hook
and audit or remediation checks where Mattermost only exposes an after-action
hook. Enable only the policies that have matching external policy endpoints in
your environment.

## Capabilities

- Message, message edit, file upload, login, push notification, and server
  config policy checks.
- Channel join, team join, reaction, user lifecycle, message audit, and SAML
  login events.
- Channel classification UI in the Mattermost right-hand sidebar.
- Restrict policy enforcement to a chosen list of teams; default is all teams.
- Fail-secure policy calls: unavailable, invalid, or rejected TDI responses
  deny blocking actions.
- Correlation IDs and redacted debug payloads for troubleshooting.

## Requirements

- Mattermost Server `9.0.0` or later. Some optional hooks require newer server
  versions; see the hook matrix in `docs/architecture/`.
- Go version from `go.mod`.
- Node.js 22 to build the webapp bundle and read `plugin.json` during release.
- Reachable external policy endpoints matching the plugin's configured TDI URL,
  namespace, and enabled policy paths when policy checks are enabled.

## Install

Download the plugin bundle from a GitHub release and upload the tarball in
Mattermost:

1. Open Mattermost as a system administrator.
2. Go to System Console > Plugins > Plugin Management.
3. Upload `com.archtis.mattermost-policy-plugin-<version>.tar.gz`.
4. Enable the plugin.
5. Configure the policy service URL, namespace, optional API key, and policy
   toggles.

The plugin is safe to enable before policy-service configuration is written.
Policy checks are disabled by default and become active only after the
corresponding policy toggles are enabled.

See `docs/operations/installation.md` for full setup and verification steps.

## Build

```bash
make verify
make bundle
```

The bundle is written to `dist/com.archtis.mattermost-policy-plugin-<version>.tar.gz`
and contains the Linux server binaries (amd64 + arm64), the webapp bundle,
and `plugin.json`.

The version is read from `plugin.json`; override it with `PLUGIN_VERSION`:

```bash
make bundle PLUGIN_VERSION=1.0.5
```

## Release

GitHub Actions builds and publishes release tarballs when a `v*` tag is pushed.
The tag version must match `plugin.json`.

```bash
git tag v1.0.5
git push origin v1.0.5
```

See `docs/operations/release-hygiene.md` for the release process.

## Documentation

- `docs/architecture/overview.md` explains the plugin flow and repository
  layout.
- `docs/architecture/hook-behavior-matrix.md` maps Mattermost hooks to policy
  paths and enforcement behavior.
- `docs/operations/installation.md` covers deployment and configuration.
- `docs/operations/deployment-runbook.md` lists production preflight checks.
- `docs/references/` contains concise notes and source links for Mattermost
  plugin and REST APIs.

## Contributing And Security

- See `CONTRIBUTING.md` for the development workflow and repository hygiene
  rules.
- See `SECURITY.md` for vulnerability reporting guidance.

## License

This project is licensed under the Apache License 2.0. See `LICENSE`.
