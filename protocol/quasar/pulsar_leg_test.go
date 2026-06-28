// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quasar

import (
	"crypto/rand"
	"testing"

	"github.com/luxfi/threshold/protocols/pulsar"
)

// signPulsarLegStock produces a stock single-key FIPS-204 ML-DSA-65 signature
// over msg (empty ctx) and the matching group public key, via the threshold
// pulsar package's single-key path (GenerateKey + Sign).
//
// Why a single key is valid leg evidence: the Pulsar leg of a Quasar cert
// verifies under the UNMODIFIED FIPS-204 verifier (pulsarwire.VerifyBytes /
// pulsar.VerifyCtx, empty ctx). A signature under the group key therefore
// satisfies the leg predicate byte-for-byte — no DKG, no threshold ceremony is
// needed to GENERATE valid leg bytes for a cert-composition test. The
// no-reconstruct t-of-n threshold PRODUCER is gate-C, blocked on a Pulsar
// release that ships a poly-vector secret-share type (ML-DSA's s1 is non-linear
// in the GF(257) seed-share, so the current KeyShare admits only the
// reconstruct path); see the HONEST SCOPE note in consensus_cert_dualpq_test.go
// and luxfi/pulsar BLOCKERS.md PULSAR-V12-PARALLEL-PQ. The permissionless
// guarantee comes from the dealerless Corona leg in AND-mode, exactly as
// dualpq.go documents — this helper does NOT claim a Pulsar threshold ceremony.
//
// The returned *pulsar.Signature / *pulsar.PublicKey are the threshold pulsar
// wire types (aliases of the pulsar kernel types), so they feed ComposePolaris
// (legs.Pulsar.MarshalBinary) and pulsarGroupPK.MarshalBinary() unchanged, and
// verify through VerifyWithRealKeysPolaris' pulsarwire.VerifyBytes path.
func signPulsarLegStock(t *testing.T, params *pulsar.Params, msg []byte) (*pulsar.Signature, *pulsar.PublicKey) {
	t.Helper()
	sk, err := pulsar.GenerateKey(params, rand.Reader)
	if err != nil {
		t.Fatalf("pulsar.GenerateKey: %v", err)
	}
	// ctx=nil, randomized=false ⇒ deterministic, matching the empty-ctx FIPS-204
	// verifier the cert's pulsar leg routes through.
	sig, err := pulsar.Sign(params, sk, msg, nil, false, nil)
	if err != nil {
		t.Fatalf("pulsar.Sign: %v", err)
	}
	return sig, sk.Pub
}
