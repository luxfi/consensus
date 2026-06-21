// Copyright (C) 2025-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// round_signer_test.go -- focused tests that the Pulsar cert profile builds,
// implements the luxfi/pulsar RoundSigner contract, and that a round-trip stub
// (Round1 -> Round2 -> Finalize) drives the consensus-owned orchestration
// correctly using the exported pulsar primitives.
//
// These tests deliberately exercise ONLY the consensus-side orchestration this
// package owns. The secret-bearing FIPS 204 signature assembly lives inside
// luxfi/pulsar and is fail-closed until its BCC/CEF ZK proofs are reviewed; we
// assert the fail-closed fallback and (via a stub core) the success wiring,
// without claiming a real threshold signature.

package pulsar

import (
	"bytes"
	"testing"

	pulsarlib "github.com/luxfi/pulsar/ref/go/pkg/pulsar"
)

func init() {
	// The hot path's Round2 calls pulsar.VerifyZPartial, which delegates the
	// zero-knowledge check to the registered PartialZVerifier (fail-closed by
	// default). For the orchestration round-trip we register an accept-bindings
	// verifier exactly as luxfi/pulsar's own partial_test.go does; the public
	// bindings (party/session/nonce/z) are still enforced unconditionally.
	pulsarlib.RegisterPartialZVerifier(acceptBindingsOnly{})
}

type acceptBindingsOnly struct{}

func (acceptBindingsOnly) VerifyPartial(*pulsarlib.Partial, []byte, []byte, []byte) error {
	return nil
}

// stubCore is a TEST-ONLY signature core: it returns a deterministic,
// non-empty Signature so we can exercise the Finalize success path. It is NOT a
// real threshold ML-DSA signer (that must live in luxfi/pulsar). It proves the
// wiring: Finalize hands the core the PUBLIC aggregate + nonce cert + session,
// and threads the returned Signature into the ConsensusCert.
type stubCore struct{ sawZSumLen int }

func (c *stubCore) AssembleSignature(
	_ PulsarSession,
	cert pulsarlib.NonceCert,
	agg pulsarlib.Aggregate,
) (pulsarlib.Signature, error) {
	c.sawZSumLen = len(agg.ZSum)
	return pulsarlib.Signature{Mode: pulsarlib.ModeP65, Bytes: []byte("stub-sig")}, nil
}

// fullSession returns a PulsarSession with every security-relevant root set
// (so Validate passes) and a NoncePoolRoot matching the supplied pool root.
func fullSession(poolRoot [32]byte) PulsarSession {
	mk := func(b byte) [32]byte {
		var x [32]byte
		for i := range x {
			x[i] = b
		}
		return x
	}
	return PulsarSession{
		ChainID:           mk(0x11),
		Epoch:             7,
		Height:            42,
		Round:             3,
		BlockHash:         mk(0x22),
		ValidatorSetRoot:  mk(0x33),
		JointPKID:         mk(0x44),
		DKGTranscriptRoot: mk(0x55),
		CommitteeID:       mk(0x66),
		SignerSetRoot:     mk(0x77),
		NoncePoolRoot:     poolRoot,
		ProtocolVersion:   1,
	}
}

// makePool builds a one-cert in-memory pool whose single NonceCert carries a
// non-zero transcript root (so the ConsensusCert transcript binding is real).
func makePool() (*MemNonceCertPool, [32]byte) {
	var poolRoot [32]byte
	for i := range poolRoot {
		poolRoot[i] = 0xAB
	}
	var nonceID, trRoot [32]byte
	for i := range nonceID {
		nonceID[i] = 0xC0
		trRoot[i] = 0x0D
	}
	cert := pulsarlib.NonceCert{NonceID: nonceID, NonceTranscriptRoot: trRoot}
	return NewMemNonceCertPool([]pulsarlib.NonceCert{cert}, poolRoot), nonceID
}

// TestPulsarRoundSignerImplementsContract is a compile-time + runtime check
// that the profile satisfies luxfi/pulsar's RoundSigner and reports the right
// profile value.
func TestPulsarRoundSignerImplementsContract(t *testing.T) {
	var rs pulsarlib.RoundSigner = &PulsarRoundSigner{}
	if got := rs.Profile(); got != pulsarlib.ProfilePulsar {
		t.Fatalf("Profile() = %v, want ProfilePulsar", got)
	}
}

// TestRoundTripFailClosed drives Round1 -> Round2 -> Finalize with NO signature
// core: the orchestration completes (canonical nonce, canonical signer subset,
// z-aggregate, ConsensusCert), but the FIPS 204 signature is empty and the
// error is the Corona-fallback signal.
func TestRoundTripFailClosed(t *testing.T) {
	pool, nonceID := makePool()
	sess := fullSession(pool.Root())
	if err := sess.Validate(); err != nil {
		t.Fatalf("session Validate: %v", err)
	}
	sid := sess.SessionID()

	s := &PulsarRoundSigner{
		Session:          sess,
		Pool:             pool,
		Threshold:        2,
		ValidatorSetSize: 8,
		L:                5, // ML-DSA-65 secret dimension ℓ
		Core:             nil,
	}

	// Round1: bind the canonical nonce.
	canonical, ok := pool.At(pulsarlib.CanonicalNonceIndex(sid, pool.Root(), pool.Size()))
	if !ok {
		t.Fatal("pool.At canonical index failed")
	}
	r1, err := s.Round1(sid, nonceID, canonical)
	if err != nil {
		t.Fatalf("Round1: %v", err)
	}
	if r1.SessionID != sid || r1.NonceID != nonceID {
		t.Fatal("Round1 did not bind session/nonce")
	}

	// Round2: two signers' z-partials. ZShare length must match L*N*polyBytes
	// only matters to the real core; for aggregation we just need consistent
	// non-empty packed vectors.
	partials := make([]pulsarlib.Partial, 0, 2)
	for _, party := range []uint32{2, 5} {
		in := pulsarlib.PartialInput{
			PartyID: party,
			ZShare:  bytes.Repeat([]byte{byte(party)}, 5*256*4),
		}
		p, err := s.Round2(r1, in)
		if err != nil {
			t.Fatalf("Round2 party %d: %v", party, err)
		}
		partials = append(partials, p)
	}

	// Finalize: structurally complete cert, empty signature, fallback error.
	agg, cert, err := s.Finalize(r1, partials)
	if err != ErrProfileNotReady {
		t.Fatalf("Finalize err = %v, want ErrProfileNotReady", err)
	}
	if len(cert.Signature.Bytes) != 0 {
		t.Fatal("expected empty signature on fail-closed path")
	}
	// The ConsensusCert must still be a valid accountability artifact:
	// quorum=2 over the canonical 2-signer bitmap, all bits in-set.
	if err := cert.VerifyStructure(2, s.ValidatorSetSize); err != nil {
		t.Fatalf("ConsensusCert.VerifyStructure: %v", err)
	}
	if cert.TranscriptRoot != canonical.NonceTranscriptRoot {
		t.Fatal("ConsensusCert did not bind the nonce transcript root")
	}
	if cert.Epoch != sess.Epoch || cert.Height != sess.Height || cert.Round != sess.Round {
		t.Fatal("ConsensusCert did not bind the consensus round")
	}
	if len(agg.ZSum) == 0 {
		t.Fatal("aggregate z-sum is empty")
	}
}

// TestRoundTripWithStubCore proves the success wiring: with a (stub) core
// registered, Finalize threads the assembled signature into the cert and hands
// the core the public aggregate.
func TestRoundTripWithStubCore(t *testing.T) {
	pool, nonceID := makePool()
	sess := fullSession(pool.Root())
	sid := sess.SessionID()
	core := &stubCore{}
	s := &PulsarRoundSigner{
		Session: sess, Pool: pool, Threshold: 2, ValidatorSetSize: 8, L: 5, Core: core,
	}
	canonical, _ := pool.At(pulsarlib.CanonicalNonceIndex(sid, pool.Root(), pool.Size()))
	r1, err := s.Round1(sid, nonceID, canonical)
	if err != nil {
		t.Fatalf("Round1: %v", err)
	}
	var partials []pulsarlib.Partial
	for _, party := range []uint32{1, 4} {
		p, err := s.Round2(r1, pulsarlib.PartialInput{PartyID: party, ZShare: bytes.Repeat([]byte{byte(party)}, 5*256*4)})
		if err != nil {
			t.Fatalf("Round2: %v", err)
		}
		partials = append(partials, p)
	}
	_, cert, err := s.Finalize(r1, partials)
	if err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if string(cert.Signature.Bytes) != "stub-sig" {
		t.Fatal("Finalize did not thread the core's signature into the cert")
	}
	if core.sawZSumLen == 0 {
		t.Fatal("core was not handed the aggregated z-sum")
	}
}

// TestEmptyPoolFallsBack: an empty NonceCert pool yields the Corona-fallback
// signal, never an inline nonce.
func TestEmptyPoolFallsBack(t *testing.T) {
	var root [32]byte
	root[0] = 1
	empty := NewMemNonceCertPool(nil, root)
	sess := fullSession(root)
	s := &PulsarRoundSigner{Session: sess, Pool: empty, Threshold: 2, ValidatorSetSize: 8, L: 5}
	var nid [32]byte
	nid[0] = 9
	if _, err := s.Round1(sess.SessionID(), nid, pulsarlib.NonceCert{NonceID: nid}); err != ErrNonceCertPoolEmpty {
		t.Fatalf("Round1 on empty pool err = %v, want ErrNonceCertPoolEmpty", err)
	}
}

// TestNonCanonicalNonceRejected: a cert that is not the canonical pool
// selection for the session is refused (anti-grind).
func TestNonCanonicalNonceRejected(t *testing.T) {
	pool, _ := makePool()
	sess := fullSession(pool.Root())
	s := &PulsarRoundSigner{Session: sess, Pool: pool, Threshold: 2, ValidatorSetSize: 8, L: 5}
	var bogus [32]byte
	bogus[0] = 0xFF // not the canonical nonce id
	if _, err := s.Round1(sess.SessionID(), bogus, pulsarlib.NonceCert{NonceID: bogus}); err != ErrNonCanonicalNonce {
		t.Fatalf("Round1 with non-canonical nonce err = %v, want ErrNonCanonicalNonce", err)
	}
}

// TestSessionIDBindsAllFields: changing any one binding field changes the
// session id (the binding is load-bearing across the whole round context).
func TestSessionIDBindsAllFields(t *testing.T) {
	var root [32]byte
	root[0] = 0xAB
	base := fullSession(root)
	baseID := base.SessionID()

	mutators := map[string]func(*PulsarSession){
		"chain_id":            func(s *PulsarSession) { s.ChainID[0] ^= 1 },
		"epoch":               func(s *PulsarSession) { s.Epoch ^= 1 },
		"height":              func(s *PulsarSession) { s.Height ^= 1 },
		"round":               func(s *PulsarSession) { s.Round ^= 1 },
		"block_hash":          func(s *PulsarSession) { s.BlockHash[0] ^= 1 },
		"validator_set_root":  func(s *PulsarSession) { s.ValidatorSetRoot[0] ^= 1 },
		"joint_pk_id":         func(s *PulsarSession) { s.JointPKID[0] ^= 1 },
		"dkg_transcript_root": func(s *PulsarSession) { s.DKGTranscriptRoot[0] ^= 1 },
		"committee_id":        func(s *PulsarSession) { s.CommitteeID[0] ^= 1 },
		"signer_set_root":     func(s *PulsarSession) { s.SignerSetRoot[0] ^= 1 },
		"nonce_pool_root":     func(s *PulsarSession) { s.NoncePoolRoot[0] ^= 1 },
		"protocol_version":    func(s *PulsarSession) { s.ProtocolVersion ^= 1 },
	}
	for name, mut := range mutators {
		s := base
		mut(&s)
		if s.SessionID() == baseID {
			t.Fatalf("mutating %q did not change the session id", name)
		}
	}
}

// TestSessionValidateRejectsUnsetRoots: an all-zero security-relevant root is
// refused.
func TestSessionValidateRejectsUnsetRoots(t *testing.T) {
	var root [32]byte
	root[0] = 1
	good := fullSession(root)
	if err := good.Validate(); err != nil {
		t.Fatalf("full session must validate, got %v", err)
	}
	bad := good
	bad.DKGTranscriptRoot = [32]byte{} // unbind a field
	if err := bad.Validate(); err != ErrSessionFieldUnset {
		t.Fatalf("unset DKG transcript root err = %v, want ErrSessionFieldUnset", err)
	}
}
