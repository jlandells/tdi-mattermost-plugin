# Security Policy

## Supported Versions

Security fixes are expected to target the latest released version of the plugin.
Older releases may receive fixes when maintainers determine that backporting is
practical and necessary.

## Reporting A Vulnerability

Do not report security vulnerabilities in public GitHub issues.

Use GitHub private vulnerability reporting if it is enabled for this repository.
If private reporting is not enabled, contact the repository owner or maintainer
through the organization’s approved security reporting channel.

Include:

- affected plugin version or commit
- Mattermost server version
- configuration required to reproduce the issue
- impact and exploitability notes
- minimal reproduction steps

## Handling Secrets

Never include real API keys, Mattermost tokens, session cookies, private URLs,
or user data in issues, pull requests, logs, or screenshots.

The plugin redacts sensitive policy payload fields in debug logs, but debug
logging should remain disabled in production unless actively troubleshooting.
