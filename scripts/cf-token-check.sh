#!/usr/bin/env bash
# cf-token-check.sh — does this Cloudflare token (and R2 key) do what nabu needs?
#
#   infisical run --env dev --path /nabu-core -- scripts/cf-token-check.sh <zone> [r2-bucket ...]
#   e.g. ... -- scripts/cf-token-check.sh example.com nabu-media nabu-state
#
# Reads CLOUDFLARE_API_TOKEN, CLOUDFLARE_ACCOUNT_ID, and optionally
# R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY from the environment. Read-only: it
# lists things and creates nothing. It prints PASS/FAIL per capability and never
# prints a secret, an account id or a zone id. Exits 1 if anything failed.
# See help/using-nabu/reference/06-cloudflare-account.md.
set -u
ZONE="${1:-}"; shift || true
API=https://api.cloudflare.com/client/v4
FAILS=0

pass() { printf '  PASS  %-28s %s\n' "$1" "${2:-}"; }
fail() { printf '  FAIL  %-28s %s\n' "$1" "${2:-}"; FAILS=$((FAILS + 1)); }
get()  { curl -s -m 15 -H "Authorization: Bearer $CLOUDFLARE_API_TOKEN" "$API$1"; }
# check <label> <path> <what it is for>: success:true is a PASS
check() {
  local out; out=$(get "$2")
  if [ "$(jq -r '.success' <<<"$out" 2>/dev/null)" = "true" ]; then pass "$1" "$3"
  else fail "$1" "$(jq -r '.errors[0].message // "no answer"' <<<"$out" 2>/dev/null) — needed for: $3"; fi
}

[ -n "${CLOUDFLARE_API_TOKEN:-}" ]  || { echo "CLOUDFLARE_API_TOKEN is not set (run under infisical run)"; exit 1; }
[ -n "${CLOUDFLARE_ACCOUNT_ID:-}" ] || { echo "CLOUDFLARE_ACCOUNT_ID is not set"; exit 1; }
command -v jq >/dev/null || { echo "jq is required"; exit 1; }
ACC=$CLOUDFLARE_ACCOUNT_ID

echo "Cloudflare API token"
# an account-owned token verifies here; a user token verifies at /user/tokens/verify
v=$(get "/accounts/$ACC/tokens/verify" | jq -r '.result.status // empty')
[ -z "$v" ] && v=$(get "/user/tokens/verify" | jq -r '.result.status // empty') && [ -n "$v" ] && v="$v (user token)"
if [ -n "$v" ]; then pass "token is valid" "$v"; else fail "token is valid" "neither account nor user verify accepts it"; fi

check "Workers"            "/accounts/$ACC/workers/scripts"        "academy and course Workers, Durable Objects"
check "Workers Containers" "/accounts/$ACC/containers/applications" "one container per course"
check "Workers R2 Storage" "/accounts/$ACC/r2/buckets"             "media and Litestream buckets"
check "D1"                 "/accounts/$ACC/d1/database"            "the academy roster"
check "Cloudflare Pages"   "/accounts/$ACC/pages/projects"         "the static tier (nabu build)"
check "Workers domains"    "/accounts/$ACC/workers/domains"        "the course host as a Workers custom domain"

if [ -n "$ZONE" ]; then
  echo "Zone $ZONE"
  Z=$(get "/zones?name=$ZONE" | jq -r '.result[0].id // empty')
  if [ -z "$Z" ]; then fail "zone visible" "the token is not scoped to $ZONE"
  else
    pass "zone visible"
    check "DNS"            "/zones/$Z/dns_records?per_page=1" "the course host record"
    check "Workers Routes" "/zones/$Z/workers/routes"         "Worker routes on the zone"
  fi
else
  echo "Zone: skipped (pass the zone name as the first argument)"
fi

echo "R2 S3 key"
if [ -z "${R2_ACCESS_KEY_ID:-}" ]; then
  echo "  skip  R2_ACCESS_KEY_ID not set"
else
  [ ${#R2_ACCESS_KEY_ID} -eq 32 ]     && pass "key id length" "32" || fail "key id length" "${#R2_ACCESS_KEY_ID}, expected 32: a paste error?"
  [ ${#R2_SECRET_ACCESS_KEY} -eq 64 ] && pass "secret length" "64" || fail "secret length" "${#R2_SECRET_ACCESS_KEY}, expected 64: a paste error?"
  if [ $# -eq 0 ]; then
    echo "  skip  bucket access (name the buckets after the zone: a bucket-scoped key cannot list buckets)"
  elif ! command -v aws >/dev/null; then
    echo "  skip  bucket access (the aws CLI is not installed)"
  else
    for b in "$@"; do
      if AWS_ACCESS_KEY_ID=$R2_ACCESS_KEY_ID AWS_SECRET_ACCESS_KEY=$R2_SECRET_ACCESS_KEY \
         aws s3api list-objects-v2 --bucket "$b" --max-items 1 --region auto \
         --endpoint-url "https://$ACC.r2.cloudflarestorage.com" >/dev/null 2>&1
      then pass "bucket $b" "list ok"; else fail "bucket $b" "access denied or no such bucket"; fi
    done
  fi
fi

echo
if [ $FAILS -eq 0 ]; then echo "All checks passed."; else echo "$FAILS check(s) failed."; exit 1; fi
