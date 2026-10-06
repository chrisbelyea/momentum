#!/usr/bin/env bash
# Validate the documented authentication contract against the implemented route
# surface. This is intentionally dependency-free so it runs in every CI job.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
DOC="$ROOT/docs/auth-api.md"
ONBOARDING="$ROOT/docs/onboarding.md"
SERVER_DOC="$ROOT/cmd/server/README.md"
ROUTES="$ROOT/internal/auth/http.go"

test -s "$DOC" || { echo "missing authentication API contract: $DOC" >&2; exit 1; }
test -s "$ONBOARDING" || { echo "missing onboarding guide: $ONBOARDING" >&2; exit 1; }
test -s "$SERVER_DOC" || { echo "missing server documentation: $SERVER_DOC" >&2; exit 1; }

# Every documented JSON endpoint must still exist in the auth router. Keeping
# this list explicit makes a route removal or rename fail CI until the contract
# and its consumers are deliberately updated together.
for endpoint in '/auth/register' '/auth/login' '/auth/logout'; do
  grep -Fq "$endpoint" "$ROUTES" || {
    echo "authentication route is missing from implementation: $endpoint" >&2
    exit 1
  }
  grep -Fq "$endpoint" "$DOC" || {
    echo "authentication route is missing from contract: $endpoint" >&2
    exit 1
  }
done

required_contract=(
  '201 Created'
  '200'
  '204 No Content'
  '400'
  '401'
  '403'
  'momentum_session'
  'HttpOnly'
  'SameSite=Strict'
  '24 hours'
  'password-backed'
  'MOMENTUM_ENCRYPTION_KEY'
  'trusted TLS'
)
for phrase in "${required_contract[@]}"; do
  grep -Fq "$phrase" "$DOC" || {
    echo "authentication contract is missing required behavior: $phrase" >&2
    exit 1
  }
done

# This was a shipped documentation bug: the adopted compatibility identity is
# not guaranteed to have a particular integer ID. Keep the contract generic.
if grep -Fq '"user_id": 2' "$DOC"; then
  echo 'authentication contract hard-codes an invalid registration user ID' >&2
  exit 1
fi

grep -Fq 'auth-api.md' "$ONBOARDING" || {
  echo 'onboarding guide does not link the authentication contract' >&2
  exit 1
}
grep -Fq 'auth-api.md' "$SERVER_DOC" || {
  echo 'server documentation does not link the authentication contract' >&2
  exit 1
}

echo 'Authentication API documentation contract validated.'
