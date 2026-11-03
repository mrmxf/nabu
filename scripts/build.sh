#!/usr/bin/env bash
# Build Nabu: SvelteKit SPA -> ./pb_public (repo root, the
# sibling of the default ./pb_data), then Nabu (the Go engine).
# Run from anywhere; paths are resolved relative to the repo root.
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

echo "==> building web (SvelteKit -> ./pb_public)"
( cd web && npm install && npm run build )

echo "==> building Nabu (Go)"
# inject git-derived version/build info into buildinfo (see buildinfo.sh)
go build -ldflags "$(bash scripts/buildinfo.sh)" -o bin/nabu ./cmd/nabu

echo "==> done. Run:  ./bin/nabu serve & # then later: ./bin/nabu stop"
