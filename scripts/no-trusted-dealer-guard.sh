#!/usr/bin/env bash
# no-trusted-dealer-guard.sh — CI guard: no trusted-dealer keygen in production code.
#
# Trusted-dealer keygen helpers (NewTrustedDealer / GenerateDualKeys /
# GenerateThresholdKeys / GenerateDualThresholdKeys / a `func GenerateKeys`) mint
# EVERY threshold share inside ONE process — a single point of full-key compromise.
# Production chain genesis MUST key off the dealerless Pedersen DKG
# (keyera.Bootstrap, EpochManager.InitializeEpoch). Those helpers were relocated to
# protocol/quasar/keygen_testsupport_test.go (TEST ONLY). This guard fails the build
# if any of them reappear in NON-TEST consensus code, so the regression that dropped
# the relocation (97b2dfe7c) on a merge cannot silently return.
#
#   exit 0 = clean   exit 1 = trusted-dealer keygen found in non-test code
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"

# Bare helper names are never legitimate in non-test consensus code. `GenerateKeys`
# is matched only as a *definition* (`func GenerateKeys`) so the legitimate library
# call coronaThreshold.GenerateKeys(...) — which lives only in the test fixture — is
# never flagged.
patterns='NewTrustedDealer|func[[:space:]]+GenerateKeys|GenerateDualKeys|GenerateThresholdKeys|GenerateDualThresholdKeys'

# Scan every .go file under the consensus tree, excluding *_test.go. Filter on the
# filename field (awk $1) so a comment merely mentioning a *_test.go path can never
# mask a real hit.
hits="$(grep -rnE --include='*.go' "$patterns" "$repo_root" 2>/dev/null \
        | awk -F: '$1 !~ /_test\.go$/' || true)"

if [[ -n "$hits" ]]; then
  echo "FAIL: trusted-dealer keygen found in NON-TEST consensus code:" >&2
  echo "$hits" >&2
  echo >&2
  echo "Relocate these helpers into a *_test.go (see protocol/quasar/keygen_testsupport_test.go)." >&2
  echo "Production genesis keying must use the dealerless Pedersen DKG (keyera.Bootstrap)." >&2
  exit 1
fi

echo "OK: no trusted-dealer keygen in non-test consensus code (protocol/quasar + tree clean)."
