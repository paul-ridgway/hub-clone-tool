#!/bin/bash
set -e
cd "$(dirname "$0")/.."
npm run build
npm un -g lo.dev
npm install -g .
