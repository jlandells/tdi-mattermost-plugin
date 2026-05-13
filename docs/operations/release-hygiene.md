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

- `go test ./...`
- `npm ci`
- `npm run build` in `webapp/`

## Packaging

Create a distributable Mattermost plugin bundle with:

```bash
make bundle
```

The bundle is written to `dist/` and should be published as a release artifact,
not committed to Git.

CI writes `dist/SHA256SUMS` beside bundle artifacts so releases can be verified
after download.

## GitHub Actions

This repository is configured for GitHub Actions:

- `.github/workflows/ci.yml` runs verification on pull requests and pushes to
  `main`, `master`, and `develop`.
- `.github/workflows/ci.yml` also builds the Mattermost plugin bundle and
  uploads the `.tar.gz` plus `SHA256SUMS` as workflow artifacts.
- `.github/workflows/release.yml` publishes public GitHub Releases when a tag
  matching `v*` is pushed.

The release workflow uses the GitHub CLI with the built-in `GITHUB_TOKEN`; no
third-party release action or extra secret is required.

## Publishing A Release

1. Ensure `PLUGIN_VERSION` in `Makefile` and `version` in `plugin.json` match.
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

## Existing Cleanup Required

At the time this policy was added, historical generated files were still tracked
by Git, including `webapp/node_modules/` and a root-level
`mattermost-policy-plugin` binary. Remove them from source control with:

```bash
git rm -r --cached webapp/node_modules
git rm --cached mattermost-policy-plugin
```

Those commands untrack the files without deleting local dependency folders when
the files still exist in the worktree. Commit the removals together with the
`.gitignore` update.
