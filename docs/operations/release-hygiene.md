# Release Hygiene

## Source Control Policy

The repository should track source code, configuration, tests, and documentation.
Generated outputs must stay out of source control.

Do not commit:

- `dist/`
- `webapp/dist/`
- `webapp/node_modules/`
- root-level plugin binaries such as `mattermost-policy-plugin`
- generated plugin bundles such as `*.tar.gz`
- platform executables such as `*.exe`

## Verification

Run the same checks locally that CI runs:

```bash
make verify
```

This executes:

- `go test -race ./...`
- `npm ci`
- `npm run build` in `webapp/`

## Packaging

Create the distributable Mattermost plugin bundle with:

```bash
make bundle
```

The bundle contains the Linux server binaries (amd64 + arm64), the webapp
bundle, and `plugin.json`. It is written to `dist/` and should be published as
a release artifact, not committed to Git.

CI writes `dist/SHA256SUMS` beside bundle artifacts so releases can be verified
after download.

## GitHub Actions

This repository is configured for GitHub Actions:

- `.github/workflows/ci.yml` runs verification on pull requests and pushes to
  `main`, `master`, and `develop`.
- `.github/workflows/ci.yml` also builds the plugin bundle and uploads the
  `.tar.gz` plus `SHA256SUMS` as workflow artifacts.
- `.github/workflows/release.yml` publishes public GitHub Releases when a tag
  matching `v*` is pushed.

The release workflow uses the GitHub CLI with the built-in `GITHUB_TOKEN`; no
third-party release action or extra secret is required.

## Publishing A Release

1. Ensure `version` in `plugin.json` matches the release tag without the `v`
   prefix. For example, tag `v1.0.6` requires `"version": "1.0.6"`.
2. Run `make verify`.
3. Commit the release changes.
4. Create and push a version tag:

```bash
git tag v1.0.5
git push origin v1.0.5
```

GitHub Actions will build:

- `dist/com.archtis.mattermost-policy-plugin-<version>.tar.gz`
- `dist/SHA256SUMS`

Both files are attached to the GitHub Release for the tag.

The release workflow derives `PLUGIN_VERSION` from the tag and validates it
against `plugin.json` with `scripts/check-release-version.sh`. If the tag and
manifest do not match, the release fails before publishing artifacts.

## Migrating The Remote To GitHub

After creating the repository on GitHub, update the local remote:

```bash
git remote set-url origin git@github.com:<owner>/<repo>.git
git push -u origin main
git push origin --tags
```

Use the HTTPS remote form instead if that is your standard:

```bash
git remote set-url origin https://github.com/<owner>/<repo>.git
```

## Repository Cleanliness

Before publishing a public release, confirm generated files are ignored and not
tracked:

```bash
git ls-files webapp/node_modules dist webapp/dist mattermost-policy-plugin
git status --ignored --short
```

The first command should print no tracked files for those paths. The second
command may show ignored local build outputs with `!!`, which is expected.

## Public Repository Checklist

Before making the repository public:

- Confirm the Apache-2.0 `LICENSE` file is present.
- Confirm GitHub private vulnerability reporting is enabled or update
  `SECURITY.md` with the approved reporting channel.
- Confirm `make verify` passes.
- Confirm `make bundle` produces a bundle that installs and starts on a target
  Mattermost server.
