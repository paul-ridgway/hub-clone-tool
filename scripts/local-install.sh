#!/bin/bash
set -e
cd "$(dirname "$0")/.."
npm run build
npm un -g hub-clone-tool
npm install -g .
