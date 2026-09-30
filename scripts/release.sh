#!/bin/bash
# Publishes a new version by creating a GitHub release (and its tag) from main.
# The Release workflow then does the rest.
# Usage: ./scripts/release.sh v1.2.3
set -e
cd "$(dirname "$0")/.."

version="$1"
if [[ ! "$version" =~ ^v[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
  echo "Usage: $0 vX.Y.Z" >&2
  exit 1
fi
if [[ "$(git branch --show-current)" != "main" || -n "$(git status --porcelain)" ]]; then
  echo "Releases must be made from a clean main branch" >&2
  exit 1
fi

git pull --ff-only
go vet ./...
go test ./...
git push origin main
gh release create "$version" --target main --generate-notes
