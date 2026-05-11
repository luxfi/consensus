// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry

import (
	"bytes"
	"crypto/rand"
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/crypto/pq/mldsa/mldsa65"
)

// Fixed test profile / chain coordinates so every test computes
// AccountIDs against the same derivation inputs.
const (
	testChainID = uint32(1729)
)

// testProfile returns the locked strict-PQ profile used in every test.
func testProfile(t *testing.T) *config.ChainSecurityProfile {
	t.Helper()
	p := config.LuxStrictPQ()
	if err := p.Validate(); err != nil {
		t.Fatalf("LuxStrictPQ().Validate(): %v", err)
	}
	return p
}

// newFreshKey draws a fresh ML-DSA-65 keypair and returns it along with
// the canonical PQKeyRecord that would be registered under the
// strict-PQ profile.
func newFreshKey(t *testing.T, profile *config.ChainSecurityProfile) (*mldsa65.PublicKey, *mldsa65.PrivateKey, PQKeyRecord) {
	t.Helper()
	pk, sk, err := mldsa65.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("mldsa65.GenerateKey: %v", err)
	}
	compactPubkey, err := pk.MarshalBinary()
	if err != nil {
		t.Fatalf("pk.MarshalBinary: %v", err)
	}
	if len(compactPubkey) != mldsa65.PublicKeySize {
		t.Fatalf("compactPubkey length=%d, want %d", len(compactPubkey), mldsa65.PublicKeySize)
	}
	pubkeyHash := shake256_384(compactPubkey, "LUX_PQ_PUBKEY_HASH_V1")
	accountID := DeriveAccountID(profile.ProfileID, testChainID, config.WalletSchemeMLDSA65, compactPubkey)
	rec := PQKeyRecord{
		AccountID:         accountID,
		SchemeID:          config.WalletSchemeMLDSA65,
		CompactPubkey:     compactPubkey,
		CompactPubkeyHash: pubkeyHash,
		ExpandedPubkeyLoc: PubkeyLocInline,
		Status:            KeyStatusActive,
		ValidFromEpoch:    1,
		ValidUntilEpoch:   0, // no expiry
	}
	return pk, sk, rec
}

// sign signs msg with sk under the registry's ctx string. Tests use this
// to produce the SelfSig / OldSig / NewSig payloads.
func sign(t *testing.T, sk *mldsa65.PrivateKey, msg []byte) []byte {
	t.Helper()
	sig, err := mldsa65.Sign(sk, msg, txAuthRegistryCtx, false)
	if err != nil {
		t.Fatalf("mldsa65.Sign: %v", err)
	}
	return sig
}

// =============================================================================
// OpRegisterKey
// =============================================================================

func TestOpRegisterKey_VerifyHappyPath(t *testing.T) {
	profile := testProfile(t)
	_, sk, rec := newFreshKey(t, profile)
	op := &OpRegisterKey{NewRecord: rec}
	digest := op.SigningDigest()
	op.SelfSig = sign(t, sk, digest[:])
	if err := op.Verify(profile, testChainID); err != nil {
		t.Fatalf("OpRegisterKey.Verify happy path: %v", err)
	}
}

func TestOpRegisterKey_RejectsZeroValues(t *testing.T) {
	profile := testProfile(t)
	t.Run("nil op", func(t *testing.T) {
		var op *OpRegisterKey
		if err := op.Verify(profile, testChainID); !errors.Is(err, ErrOpNil) {
			t.Fatalf("nil op: got %v, want ErrOpNil", err)
		}
	})
	t.Run("zero record (no scheme)", func(t *testing.T) {
		op := &OpRegisterKey{}
		err := op.Verify(profile, testChainID)
		if err == nil {
			t.Fatalf("zero record: want non-nil error")
		}
		if !errors.Is(err, ErrOpRecordInvalid) {
			t.Fatalf("zero record: got %v, want wrapped ErrOpRecordInvalid", err)
		}
	})
	t.Run("status not active", func(t *testing.T) {
		_, _, rec := newFreshKey(t, profile)
		rec.Status = KeyStatusRotated
		op := &OpRegisterKey{NewRecord: rec}
		// Validate refuses terminal status first.
		if err := op.Verify(profile, testChainID); !errors.Is(err, ErrOpRecordInvalid) {
			t.Fatalf("terminal status: got %v, want wrapped ErrOpRecordInvalid", err)
		}
	})
}

func TestOpRegisterKey_RejectsForgedSig(t *testing.T) {
	profile := testProfile(t)
	_, sk, rec := newFreshKey(t, profile)
	op := &OpRegisterKey{NewRecord: rec}
	digest := op.SigningDigest()
	sig := sign(t, sk, digest[:])
	// Flip one byte. Note the high bit so we don't accidentally hit a
	// no-op shim somewhere in circl's verifier.
	forged := append([]byte(nil), sig...)
	forged[0] ^= 0xFF
	op.SelfSig = forged
	if err := op.Verify(profile, testChainID); !errors.Is(err, ErrOpSelfSigInvalid) {
		t.Fatalf("forged sig: got %v, want wrapped ErrOpSelfSigInvalid", err)
	}
}

// =============================================================================
// OpRotateKey
// =============================================================================

func TestOpRotateKey_RequiresOldSig_AndNewProofOfPossession(t *testing.T) {
	profile := testProfile(t)
	_, oldSK, oldRec := newFreshKey(t, profile)
	_, newSK, newRec := newFreshKey(t, profile)
	// Sanity: distinct AccountIDs.
	if oldRec.AccountID == newRec.AccountID {
		t.Fatalf("test fixture: distinct keys produced equal AccountIDs (collision)")
	}
	lookup := func(id [48]byte) (config.WalletSchemeID, []byte, KeyStatus, error) {
		if id == oldRec.AccountID {
			return oldRec.SchemeID, oldRec.CompactPubkey, KeyStatusActive, nil
		}
		return 0, nil, 0, errors.New("not found")
	}

	t.Run("happy path requires BOTH sigs", func(t *testing.T) {
		op := &OpRotateKey{OldAccountID: oldRec.AccountID, NewRecord: newRec}
		digest := op.SigningDigest()
		op.OldSig = sign(t, oldSK, digest[:])
		op.NewSig = sign(t, newSK, digest[:])
		if err := op.VerifyWithLookup(profile, testChainID, lookup); err != nil {
			t.Fatalf("happy path: %v", err)
		}
	})

	t.Run("missing OldSig rejected", func(t *testing.T) {
		op := &OpRotateKey{OldAccountID: oldRec.AccountID, NewRecord: newRec}
		digest := op.SigningDigest()
		op.NewSig = sign(t, newSK, digest[:])
		// OldSig left empty.
		if err := op.VerifyWithLookup(profile, testChainID, lookup); !errors.Is(err, ErrOpOldSigInvalid) {
			t.Fatalf("missing OldSig: got %v, want wrapped ErrOpOldSigInvalid", err)
		}
	})

	t.Run("missing NewSig rejected", func(t *testing.T) {
		op := &OpRotateKey{OldAccountID: oldRec.AccountID, NewRecord: newRec}
		digest := op.SigningDigest()
		op.OldSig = sign(t, oldSK, digest[:])
		// NewSig left empty — proof of possession on the successor missing.
		if err := op.VerifyWithLookup(profile, testChainID, lookup); !errors.Is(err, ErrOpNewSigInvalid) {
			t.Fatalf("missing NewSig: got %v, want wrapped ErrOpNewSigInvalid", err)
		}
	})

	t.Run("NewSig signed by old key (impostor) rejected", func(t *testing.T) {
		op := &OpRotateKey{OldAccountID: oldRec.AccountID, NewRecord: newRec}
		digest := op.SigningDigest()
		op.OldSig = sign(t, oldSK, digest[:])
		// Forge NewSig by signing under the OLD key — the verifier
		// must check NewSig against newRec.CompactPubkey, not oldRec's.
		op.NewSig = sign(t, oldSK, digest[:])
		if err := op.VerifyWithLookup(profile, testChainID, lookup); !errors.Is(err, ErrOpNewSigInvalid) {
			t.Fatalf("NewSig impostor: got %v, want wrapped ErrOpNewSigInvalid", err)
		}
	})

	t.Run("rotate to same AccountID rejected", func(t *testing.T) {
		op := &OpRotateKey{OldAccountID: oldRec.AccountID, NewRecord: oldRec}
		if err := op.VerifyWithLookup(profile, testChainID, lookup); !errors.Is(err, ErrOpRotateSameAccount) {
			t.Fatalf("rotate-to-self: got %v, want ErrOpRotateSameAccount", err)
		}
	})

	t.Run("old key not active rejected", func(t *testing.T) {
		staleLookup := func(id [48]byte) (config.WalletSchemeID, []byte, KeyStatus, error) {
			if id == oldRec.AccountID {
				return oldRec.SchemeID, oldRec.CompactPubkey, KeyStatusRotated, nil
			}
			return 0, nil, 0, errors.New("not found")
		}
		op := &OpRotateKey{OldAccountID: oldRec.AccountID, NewRecord: newRec}
		digest := op.SigningDigest()
		op.OldSig = sign(t, oldSK, digest[:])
		op.NewSig = sign(t, newSK, digest[:])
		if err := op.VerifyWithLookup(profile, testChainID, staleLookup); !errors.Is(err, ErrOpOldKeyNotActive) {
			t.Fatalf("stale old key: got %v, want ErrOpOldKeyNotActive", err)
		}
	})
}

// =============================================================================
// OpRevokeKey
// =============================================================================

func TestOpRevokeKey_RecoveryKeyAccepted(t *testing.T) {
	profile := testProfile(t)
	_, masterSK, masterRec := newFreshKey(t, profile)
	_, recoverySK, recoveryRec := newFreshKey(t, profile)

	masterLookup := func(id [48]byte) (config.WalletSchemeID, []byte, KeyStatus, error) {
		if id == masterRec.AccountID {
			return masterRec.SchemeID, masterRec.CompactPubkey, KeyStatusActive, nil
		}
		return 0, nil, 0, errors.New("not found")
	}
	recoveryLookup := func(id [48]byte) (config.WalletSchemeID, []byte, error) {
		if id == masterRec.AccountID {
			return recoveryRec.SchemeID, recoveryRec.CompactPubkey, nil
		}
		return 0, nil, nil // no recovery key
	}

	t.Run("compromise — recovery key accepted", func(t *testing.T) {
		op := &OpRevokeKey{AccountID: masterRec.AccountID, Reason: RevokeReasonCompromise}
		digest := op.SigningDigest()
		op.Sig = sign(t, recoverySK, digest[:])
		if err := op.VerifyWithLookup(profile, masterLookup, recoveryLookup); err != nil {
			t.Fatalf("compromise/recovery: %v", err)
		}
	})

	t.Run("compromise — master key rejected (must be recovery)", func(t *testing.T) {
		op := &OpRevokeKey{AccountID: masterRec.AccountID, Reason: RevokeReasonCompromise}
		digest := op.SigningDigest()
		op.Sig = sign(t, masterSK, digest[:])
		// Signed by master, not recovery — must fail verify.
		if err := op.VerifyWithLookup(profile, masterLookup, recoveryLookup); !errors.Is(err, ErrOpRevokeSigInvalid) {
			t.Fatalf("compromise/master: got %v, want wrapped ErrOpRevokeSigInvalid", err)
		}
	})

	t.Run("retire — master accepted", func(t *testing.T) {
		op := &OpRevokeKey{AccountID: masterRec.AccountID, Reason: RevokeReasonRetire}
		digest := op.SigningDigest()
		op.Sig = sign(t, masterSK, digest[:])
		if err := op.VerifyWithLookup(profile, masterLookup, recoveryLookup); err != nil {
			t.Fatalf("retire/master: %v", err)
		}
	})

	t.Run("retire — recovery rejected (must be master)", func(t *testing.T) {
		op := &OpRevokeKey{AccountID: masterRec.AccountID, Reason: RevokeReasonRetire}
		digest := op.SigningDigest()
		op.Sig = sign(t, recoverySK, digest[:])
		if err := op.VerifyWithLookup(profile, masterLookup, recoveryLookup); !errors.Is(err, ErrOpRevokeSigInvalid) {
			t.Fatalf("retire/recovery: got %v, want wrapped ErrOpRevokeSigInvalid", err)
		}
	})

	t.Run("compromise — no recovery key pinned rejected", func(t *testing.T) {
		emptyRecoveryLookup := func(id [48]byte) (config.WalletSchemeID, []byte, error) {
			return 0, nil, nil
		}
		op := &OpRevokeKey{AccountID: masterRec.AccountID, Reason: RevokeReasonCompromise}
		digest := op.SigningDigest()
		op.Sig = sign(t, recoverySK, digest[:])
		if err := op.VerifyWithLookup(profile, masterLookup, emptyRecoveryLookup); !errors.Is(err, ErrOpRecoveryNotPinned) {
			t.Fatalf("compromise/no-recovery: got %v, want ErrOpRecoveryNotPinned", err)
		}
	})

	t.Run("unknown reason rejected", func(t *testing.T) {
		op := &OpRevokeKey{AccountID: masterRec.AccountID, Reason: RevokeReason(0xEE)}
		op.Sig = []byte{1, 2, 3}
		if err := op.VerifyWithLookup(profile, masterLookup, recoveryLookup); !errors.Is(err, ErrOpUnknownRevokeReason) {
			t.Fatalf("unknown reason: got %v, want ErrOpUnknownRevokeReason", err)
		}
	})
}

// =============================================================================
// OpAuthorizeSession
// =============================================================================

func TestOpAuthorizeSession_TimeBoxed(t *testing.T) {
	profile := testProfile(t)
	_, masterSK, masterRec := newFreshKey(t, profile)
	_, _, sessionRec := newFreshKey(t, profile)
	sessionPubkey := sessionRec.CompactPubkey
	masterLookup := func(id [48]byte) (config.WalletSchemeID, []byte, KeyStatus, error) {
		if id == masterRec.AccountID {
			return masterRec.SchemeID, masterRec.CompactPubkey, KeyStatusActive, nil
		}
		return 0, nil, 0, errors.New("not found")
	}

	t.Run("happy path", func(t *testing.T) {
		op := &OpAuthorizeSession{
			AccountID:     masterRec.AccountID,
			SessionPubkey: sessionPubkey,
			SessionPolicy: SessionPolicy{MaxGasPerCall: 100_000, MaxCallsPerEpoch: 50},
			ValidUntil:    1_000,
		}
		digest := op.SigningDigest()
		op.MasterSig = sign(t, masterSK, digest[:])
		if err := op.VerifyWithLookup(profile, masterLookup, 100); err != nil {
			t.Fatalf("happy path: %v", err)
		}
	})

	t.Run("ValidUntil <= currentHeight rejected", func(t *testing.T) {
		op := &OpAuthorizeSession{
			AccountID:     masterRec.AccountID,
			SessionPubkey: sessionPubkey,
			ValidUntil:    100,
		}
		digest := op.SigningDigest()
		op.MasterSig = sign(t, masterSK, digest[:])
		if err := op.VerifyWithLookup(profile, masterLookup, 200); !errors.Is(err, ErrOpSessionAlreadyExpired) {
			t.Fatalf("expired: got %v, want ErrOpSessionAlreadyExpired", err)
		}
	})

	t.Run("ValidUntil zero rejected", func(t *testing.T) {
		op := &OpAuthorizeSession{
			AccountID:     masterRec.AccountID,
			SessionPubkey: sessionPubkey,
			ValidUntil:    0,
		}
		if err := op.VerifyWithLookup(profile, masterLookup, 100); !errors.Is(err, ErrOpSessionNoExpiry) {
			t.Fatalf("no expiry: got %v, want ErrOpSessionNoExpiry", err)
		}
	})

	t.Run("forged master sig rejected", func(t *testing.T) {
		_, otherSK, _ := newFreshKey(t, profile)
		op := &OpAuthorizeSession{
			AccountID:     masterRec.AccountID,
			SessionPubkey: sessionPubkey,
			ValidUntil:    1_000,
		}
		digest := op.SigningDigest()
		op.MasterSig = sign(t, otherSK, digest[:])
		if err := op.VerifyWithLookup(profile, masterLookup, 100); !errors.Is(err, ErrOpMasterSigInvalid) {
			t.Fatalf("forged master: got %v, want wrapped ErrOpMasterSigInvalid", err)
		}
	})
}

// =============================================================================
// OpCommitTxAuthBatch
// =============================================================================

func TestOpCommitTxAuthBatch_RootDeterministicOverSorted(t *testing.T) {
	profile := testProfile(t)
	// Build a small TxAuthBatch root over a sorted set of (account, tx)
	// leaves; assert (a) Verify accepts the result and (b) shuffling the
	// commitment fields changes the SigningDigest.
	var (
		acct1 [48]byte
		acct2 [48]byte
		tx1   [48]byte
		tx2   [48]byte
	)
	for i := range acct1 {
		acct1[i] = 0x11
		acct2[i] = 0x22
		tx1[i] = 0xA1
		tx2[i] = 0xA2
	}
	leaf1 := LeafDigest(acct1, tx1)
	leaf2 := LeafDigest(acct2, tx2)
	// Sort by leaf hash (canonical), then bind.
	var lo, hi [48]byte
	if bytes.Compare(leaf1[:], leaf2[:]) < 0 {
		lo, hi = leaf1, leaf2
	} else {
		lo, hi = leaf2, leaf1
	}
	root := nodeDigest(lo, hi)

	op := &OpCommitTxAuthBatch{
		Epoch:         7,
		BatchRoot:     root,
		SignatureRoot: nodeDigest(lo, hi), // distinct preimage in production
		AccountRoot:   nodeDigest(hi, lo), // sorted differently
		BatchCount:    2,
	}
	if err := op.Verify(profile); err != nil {
		t.Fatalf("Verify happy path: %v", err)
	}

	// Determinism: re-compute SigningDigest, should match.
	d1 := op.SigningDigest()
	d2 := op.SigningDigest()
	if d1 != d2 {
		t.Fatalf("SigningDigest non-deterministic")
	}

	// Mutation in any commitment field changes the digest.
	mutants := []struct {
		name string
		mut  func(*OpCommitTxAuthBatch)
	}{
		{"epoch", func(o *OpCommitTxAuthBatch) { o.Epoch ^= 1 }},
		{"batch_root", func(o *OpCommitTxAuthBatch) { o.BatchRoot[0] ^= 1 }},
		{"signature_root", func(o *OpCommitTxAuthBatch) { o.SignatureRoot[0] ^= 1 }},
		{"account_root", func(o *OpCommitTxAuthBatch) { o.AccountRoot[0] ^= 1 }},
		{"batch_count", func(o *OpCommitTxAuthBatch) { o.BatchCount ^= 1 }},
	}
	base := op.SigningDigest()
	for _, mut := range mutants {
		t.Run(mut.name, func(t *testing.T) {
			clone := *op
			mut.mut(&clone)
			if clone.SigningDigest() == base {
				t.Fatalf("mutating %s did NOT change SigningDigest", mut.name)
			}
		})
	}
}

func TestOpCommitTxAuthBatch_RejectsZeroFields(t *testing.T) {
	profile := testProfile(t)
	nonZero := [48]byte{1}
	good := OpCommitTxAuthBatch{
		Epoch: 1, BatchRoot: nonZero, SignatureRoot: nonZero, AccountRoot: nonZero, BatchCount: 1,
	}
	if err := good.Verify(profile); err != nil {
		t.Fatalf("good fixture verify: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*OpCommitTxAuthBatch)
		want error
	}{
		{"zero epoch", func(o *OpCommitTxAuthBatch) { o.Epoch = 0 }, ErrOpBatchZeroEpoch},
		{"zero batch_root", func(o *OpCommitTxAuthBatch) { o.BatchRoot = [48]byte{} }, ErrOpBatchZeroRoot},
		{"zero sig_root", func(o *OpCommitTxAuthBatch) { o.SignatureRoot = [48]byte{} }, ErrOpBatchZeroRoot},
		{"zero account_root", func(o *OpCommitTxAuthBatch) { o.AccountRoot = [48]byte{} }, ErrOpBatchZeroRoot},
		{"zero count", func(o *OpCommitTxAuthBatch) { o.BatchCount = 0 }, ErrOpBatchZeroCount},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			clone := good
			c.mut(&clone)
			if err := clone.Verify(profile); !errors.Is(err, c.want) {
				t.Fatalf("%s: got %v, want %v", c.name, err, c.want)
			}
		})
	}
}

// =============================================================================
// ZRegistryRoots.Hash — every field must bind
// =============================================================================

func TestZRegistryRoots_Hash_BindsEveryField(t *testing.T) {
	base := &ZRegistryRoots{
		IdentityRoot:       [48]byte{0x01},
		AccountKeyRoot:     [48]byte{0x02},
		RevocationRoot:     [48]byte{0x03},
		SessionKeyRoot:     [48]byte{0x04},
		ContractPolicyRoot: [48]byte{0x05},
		TxAuthRoot:         [48]byte{0x06},
		PermitAuthRoot:     [48]byte{0x07},
	}
	baseHash := base.Hash()

	// Re-hash MUST be deterministic.
	if baseHash != base.Hash() {
		t.Fatalf("Hash non-deterministic")
	}

	// One mutation subtest per field.
	mutants := []struct {
		name string
		mut  func(*ZRegistryRoots)
	}{
		{"identity_root", func(r *ZRegistryRoots) { r.IdentityRoot[0] ^= 0xFF }},
		{"account_key_root", func(r *ZRegistryRoots) { r.AccountKeyRoot[0] ^= 0xFF }},
		{"revocation_root", func(r *ZRegistryRoots) { r.RevocationRoot[0] ^= 0xFF }},
		{"session_key_root", func(r *ZRegistryRoots) { r.SessionKeyRoot[0] ^= 0xFF }},
		{"contract_policy_root", func(r *ZRegistryRoots) { r.ContractPolicyRoot[0] ^= 0xFF }},
		{"tx_auth_root", func(r *ZRegistryRoots) { r.TxAuthRoot[0] ^= 0xFF }},
		{"permit_auth_root", func(r *ZRegistryRoots) { r.PermitAuthRoot[0] ^= 0xFF }},
	}
	for _, mut := range mutants {
		t.Run(mut.name, func(t *testing.T) {
			clone := *base
			mut.mut(&clone)
			if clone.Hash() == baseHash {
				t.Fatalf("mutating %s did NOT change ZRegistryRoots.Hash()", mut.name)
			}
		})
	}

	// Also: distinct customization-tag separation — a TupleHash over the
	// same parts under a different customization MUST produce a different
	// digest. We exercise this implicitly above (the customization is the
	// schema identity); add a defensive nil-receiver check.
	var nilRoots *ZRegistryRoots
	if nilRoots.Hash() == baseHash {
		t.Fatalf("nil receiver hash collided with populated roots hash")
	}
}

// =============================================================================
// VerifyAuthPassed — execution hot-path inclusion check
// =============================================================================

// buildBatchRoot is a tiny canonical Merkle-root builder for tests.
// Tree shape: balanced binary, left-to-right by leaf index. Caller
// pre-sorts the leaves; this helper does NOT sort.
//
// Returns (root, proofForLeafN, err).
func buildBatchRoot(leaves [][48]byte) (root [48]byte, proofs []MerkleProof) {
	if len(leaves) == 0 {
		return root, nil
	}
	// Round up to next power of two by duplicating last leaf — same
	// convention used elsewhere in the consensus tree for balanced trees.
	depth := 0
	for (1 << depth) < len(leaves) {
		depth++
	}
	padded := make([][48]byte, 1<<depth)
	for i, l := range leaves {
		padded[i] = l
	}
	for i := len(leaves); i < len(padded); i++ {
		padded[i] = leaves[len(leaves)-1]
	}
	// Build all levels.
	levels := [][][48]byte{padded}
	for cur := padded; len(cur) > 1; {
		next := make([][48]byte, len(cur)/2)
		for i := 0; i < len(cur); i += 2 {
			next[i/2] = nodeDigest(cur[i], cur[i+1])
		}
		levels = append(levels, next)
		cur = next
	}
	root = levels[len(levels)-1][0]

	// Emit a proof for each of the original (un-padded) leaves.
	proofs = make([]MerkleProof, len(leaves))
	for idx := 0; idx < len(leaves); idx++ {
		var siblings [][48]byte
		cur := idx
		for level := 0; level < depth; level++ {
			var sibIdx int
			if cur%2 == 0 {
				sibIdx = cur + 1
			} else {
				sibIdx = cur - 1
			}
			siblings = append(siblings, levels[level][sibIdx])
			cur /= 2
		}
		proofs[idx] = MerkleProof{
			LeafIndex: uint64(idx),
			LeafHash:  leaves[idx],
			Siblings:  siblings,
		}
	}
	return root, proofs
}

func TestVerifyAuthPassed_HappyPath(t *testing.T) {
	// 4 leaves: balanced depth-2 tree.
	var (
		accts  [4][48]byte
		txs    [4][48]byte
		leaves [4][48]byte
	)
	for i := 0; i < 4; i++ {
		accts[i] = [48]byte{byte(0x10 + i)}
		txs[i] = [48]byte{byte(0xA0 + i)}
		leaves[i] = LeafDigest(accts[i], txs[i])
	}
	root, proofs := buildBatchRoot(leaves[:])
	for i := 0; i < 4; i++ {
		err := VerifyAuthPassed(root, accts[i], txs[i], proofs[i])
		if err != nil {
			t.Fatalf("leaf %d: %v", i, err)
		}
	}
}

func TestVerifyAuthPassed_RejectsStaleEpoch(t *testing.T) {
	var zeroRoot, acct, tx [48]byte
	proof := MerkleProof{LeafHash: LeafDigest(acct, tx)}
	if err := VerifyAuthPassed(zeroRoot, acct, tx, proof); !errors.Is(err, ErrAuthProofStaleEpoch) {
		t.Fatalf("zero root: got %v, want ErrAuthProofStaleEpoch", err)
	}
}

func TestVerifyAuthPassed_RejectsWrongMerkleProof(t *testing.T) {
	var (
		accts  [4][48]byte
		txs    [4][48]byte
		leaves [4][48]byte
	)
	for i := 0; i < 4; i++ {
		accts[i] = [48]byte{byte(0x10 + i)}
		txs[i] = [48]byte{byte(0xA0 + i)}
		leaves[i] = LeafDigest(accts[i], txs[i])
	}
	root, proofs := buildBatchRoot(leaves[:])

	t.Run("flipped sibling", func(t *testing.T) {
		bad := proofs[0]
		bad.Siblings = append([][48]byte(nil), proofs[0].Siblings...)
		bad.Siblings[0][0] ^= 0xFF
		if err := VerifyAuthPassed(root, accts[0], txs[0], bad); !errors.Is(err, ErrAuthProofWrongRoot) {
			t.Fatalf("flipped sibling: got %v, want ErrAuthProofWrongRoot", err)
		}
	})

	t.Run("wrong leaf for (account, tx)", func(t *testing.T) {
		bad := proofs[0]
		// Replace LeafHash with leaf-1's hash; the LeafHash precondition
		// should refuse before walking.
		bad.LeafHash = leaves[1]
		if err := VerifyAuthPassed(root, accts[0], txs[0], bad); !errors.Is(err, ErrAuthProofLeafMismatch) {
			t.Fatalf("wrong leaf: got %v, want ErrAuthProofLeafMismatch", err)
		}
	})

	t.Run("wrong leaf index", func(t *testing.T) {
		bad := proofs[0]
		bad.LeafIndex = 99 // depth=2 admits indices 0..3.
		if err := VerifyAuthPassed(root, accts[0], txs[0], bad); !errors.Is(err, ErrAuthProofIndexOutOfRange) {
			t.Fatalf("oob index: got %v, want ErrAuthProofIndexOutOfRange", err)
		}
	})

	t.Run("swapped leaf path direction", func(t *testing.T) {
		// Take leaf 0's proof but report it under leaf 1's coordinates.
		// LeafIndex now flips combine direction at level 0 → wrong root.
		bad := proofs[0]
		bad.LeafIndex = 1
		// Keep LeafHash matching (account[0], tx[0]) so we get past the
		// leaf-mismatch check and into the walk.
		if err := VerifyAuthPassed(root, accts[0], txs[0], bad); !errors.Is(err, ErrAuthProofWrongRoot) {
			t.Fatalf("swapped direction: got %v, want ErrAuthProofWrongRoot", err)
		}
	})
}

// =============================================================================
// DeriveAccountID — sanity (defence-in-depth, not the main surface)
// =============================================================================

func TestDeriveAccountID_DistinctOverProfileChainScheme(t *testing.T) {
	pubkey := bytes.Repeat([]byte{0xAB}, 1952)
	a := DeriveAccountID(1, 1, config.WalletSchemeMLDSA65, pubkey)
	b := DeriveAccountID(2, 1, config.WalletSchemeMLDSA65, pubkey)
	c := DeriveAccountID(1, 2, config.WalletSchemeMLDSA65, pubkey)
	d := DeriveAccountID(1, 1, config.WalletSchemeMLDSA87, pubkey)
	if a == b || a == c || a == d || b == c || b == d || c == d {
		t.Fatalf("DeriveAccountID collisions across (profile, chain, scheme)")
	}
}
