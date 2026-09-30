#!/bin/bash
# Publishes a new version of the Go module by tagging main and pushing the tag.
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
git tag -a "$version" -m "$version"
git push origin main "$version"
