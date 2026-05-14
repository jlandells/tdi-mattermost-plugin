#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${ROOT_DIR}"

export GOCACHE="${GOCACHE:-/tmp/tdi-mattermost-go-build-cache}"

go test -race ./...

(
  cd webapp
  npm ci
  npm run build
)

