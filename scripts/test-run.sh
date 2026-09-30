#!/bin/bash
# Builds hct and runs it against a temporary directory, which is removed when
# hct exits or is killed. Extra arguments are passed through to hct.
set -e
cd "$(dirname "$0")/.."
./scripts/build.sh

dir="$(mktemp -d -t hct-test-run.XXXXXX)"
cleanup() {
  echo "Removing $dir"
  rm -rf "$dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

./bin/hct --dir "$dir" "$@"
