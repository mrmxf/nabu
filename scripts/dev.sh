#!/usr/bin/env bash
# Local dev cycle: stop any running Nabu, rebuild the LEAN binary + web, reseed,
# then serve with the AI worker assumed to be running on THIS machine.
#
#   1. stop the running Nabu (clean DB for the rebuild)
#   2. build.sh (web -> pb_public, then the lean ./bin/nabu — no deploy embed)
#   3. seed demo content/accounts (superuser only if NABU_SUPERUSER_PW is present)
#   4. serve ./bin/nabu pointing at a LOCAL worker (127.0.0.1:8091)
#   5. secrets (WORKER_DISPATCH_TOKEN, S3 keys, ...) come from Infisical
#
# This does NOT build the deploy binary — that's `nabu snapshot` (produces
# nabu-deploy). Human-readable output goes through `clog Log` (-I/-E/-W).
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

# Infisical environment to pull secrets from (dev/staging/prod). Override with:
#   INFISICAL_ENV=prod ./scripts/dev.sh
INF_ENV="${INFISICAL_ENV:-dev}"

# The worker runs on this box for local dev; nabu derives the dispatch URL from
# the health URL (…/healthz -> …/v1/jobs/dispatch), so this one export is enough.
export WORKER_HEALTH_URL="${WORKER_HEALTH_URL:-http://127.0.0.1:8091/healthz}"

# ---- 1. stop any running Nabu so the rebuild/reseed starts clean.
clog Log -I "stopping any running Nabu"
go run ./cmd/nabu stop || clog Log -W "no running Nabu to stop (continuing)"

# ---- 2. build the lean binary + web (no deploy embed).
clog Log -I "building web + lean nabu (build.sh)"
./scripts/build.sh

# ---- 3. reseed demo content/accounts (idempotent; seed auto-applies pending
# migrations, so a brand-new ./pb_data works in one step). This runs outside
# Infisical, so no dashboard superuser is created unless NABU_SUPERUSER_PW is
# already in the environment — otherwise create the first one from /_/.
clog Log -I "seeding demo content/accounts"
go run ./cmd/nabu seed || clog Log -W "seed reported issues (continuing)"

# ---- 4+5. serve the freshly built binary with secrets from Infisical, pointing
# at the local worker.
clog Log -I "worker assumed local: WORKER_HEALTH_URL=$WORKER_HEALTH_URL"

if command -v infisical >/dev/null 2>&1; then
	clog Log -I "serving Nabu on :8090 with secrets from Infisical (env: $INF_ENV)"
	clog Log -I "open http://127.0.0.1:8090  ·  stop with: ./bin/nabu stop"
	exec infisical run --env "$INF_ENV" --silent \
		-- ./bin/nabu serve --http=127.0.0.1:8090 &
else
	clog Log -E "infisical not installed — secrets unavailable; the worker dispatch will run without WORKER_DISPATCH_TOKEN"
	clog Log -W "serving Nabu on :8090 WITHOUT Infisical secrets"
	clog Log -I "open http://127.0.0.1:8090  ·  stop with: ./bin/nabu stop"
	exec ./bin/nabu serve --http=127.0.0.1:8090 &
fi
