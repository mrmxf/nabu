#!/usr/bin/env bash
# Emit the Go -ldflags value that injects Nabu's version/build info into
# github.com/mrmxf/util/buildinfo at link time. This is the SINGLE source of truth
# used by both build sites — scripts/build.sh and, for nabu-deploy,
# internal/deploy.buildDeployBinary — so every built binary reports the same thing.
#
# Version identity comes from GIT (tags are the source of truth): the describe tag,
# the 40-char commit hash, and the build date. A build made on an exact release tag
# is "prod"; anything else is "dev". Prints ONLY the `-X ...=<json>` flag (no spaces
# in the value), which the caller wraps in -ldflags "...".
#
# The -X target path is hardcoded — buildinfo's own LinkerPath() helper is buggy.
set -euo pipefail
cd "$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)" # repo root

hash="$(git rev-list -1 HEAD 2>/dev/null || echo '')"          # 40 hex, or buildinfo dummies it
tag="$(git describe --tags --always 2>/dev/null || echo 'v0.0.0')"
date="$(date +%F)"
if git describe --exact-match --tags HEAD >/dev/null 2>&1; then
	build="prod"
else
	build="dev"
fi

# Matches buildinfo.LinkerDataJSON. No spaces so it stays a single -ldflags token.
json="$(printf '{"build":"%s","tag":"%s","hash":"%s","date":"%s","suffix":"","name":"Nabu","title":"Nabu"}' \
	"$build" "$tag" "$hash" "$date")"

printf -- '-X github.com/mrmxf/util/buildinfo.SemVerJSON=%s' "$json"
