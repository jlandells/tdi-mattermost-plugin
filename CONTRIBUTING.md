# Contributing

Thanks for taking the time to improve this Mattermost plugin.

## Scope

This repository is focused on the Mattermost plugin only:

- Mattermost server plugin hooks.
- Optional internal Mattermost webapp bundle.
- Plugin packaging and GitHub release automation.
- The policy endpoint contract consumed by the plugin.

Do not add instructions for installing or operating the external policy service.
Keep those details in the system that owns that service.

## Development Setup

Requirements:

- Go version from `go.mod`.
- Node.js 22.
- npm.

Run the full local verification path before opening a pull request:

```bash
make verify
```

Build the public server-only plugin bundle:

```bash
make bundle
```

Build the internal bundle with the optional Mattermost webapp:

```bash
make bundle INCLUDE_WEBAPP=true
```

## Repository Hygiene

Do not commit generated outputs:

- `dist/`
- `webapp/dist/`
- `webapp/node_modules/`
- generated `*.tar.gz` bundles
- root-level plugin binaries

Keep changes scoped to the issue or pull request. Avoid unrelated formatting,
renames, and broad refactors.

## Tests

Add or update focused tests when changing:

- configuration validation
- TDI-compatible policy response parsing
- hook allow/deny behavior
- file upload handling
- plugin-local HTTP handlers
- optional webapp state or API behavior

## Documentation

Update documentation when behavior, configuration, packaging, or public release
processes change. Keep documentation Mattermost-plugin focused and retain only
the policy endpoint contract needed by this plugin.
