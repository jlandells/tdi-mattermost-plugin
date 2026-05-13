#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <tag>" >&2
  exit 2
fi

TAG="$1"
VERSION="${TAG#v}"

if [[ "${VERSION}" == "${TAG}" || -z "${VERSION}" ]]; then
  echo "release tag must start with v, got: ${TAG}" >&2
  exit 1
fi

PLUGIN_JSON_VERSION="$(node -p "require('./plugin.json').version")"

if [[ "${PLUGIN_JSON_VERSION}" != "${VERSION}" ]]; then
  echo "release tag ${TAG} does not match plugin.json version ${PLUGIN_JSON_VERSION}" >&2
  exit 1
fi

echo "${VERSION}"

