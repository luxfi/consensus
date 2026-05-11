// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry

// roots.go — the aggregator that pins every Z-Chain registry root into
// a single 48-byte commitment.
//
// ZRegistryRoots is what rides in EpochCommitment.zchain_state_root's
// preimage and gets consumed by Q-Chain (HIP-0079) as part of the
// QBlock transcript. Q-Chain doesn't look inside; it consumes the
// aggregated hash and binds it.
//
// Field roles (HIP-0078 §"Z-Chain state" + §"E2E PQ surface"):
//
//   IdentityRoot         Merkle root over the active PQKeyRecord set
//                        (every key that can sign on this chain).
//   AccountKeyRoot       same set indexed by AccountID, sorted lex.
//                        Distinct from IdentityRoot only in the leaf
//                        encoding; the registry maintains both indexes
//                        because IdentityRoot is the consensus-side
//                        accumulator while AccountKeyRoot is the
//                        execution-side lookup tree.
//   RevocationRoot       Merkle root over the set of revoked /
//                        rotated AccountIDs (KeyStatusRevoked +
//                        KeyStatusRotated).
//   SessionKeyRoot       Merkle root over authorised, unexpired
//                        session keys.
//   ContractPolicyRoot   Merkle root over per-contract policy hashes
//                        (admin / upgrade / pause / permit caps).
//   TxAuthRoot           Merkle root over the most recent accepted
//                        OpCommitTxAuthBatch.BatchRoot. VerifyAuthPassed
//                        consumes this on the execution hot path.
//   PermitAuthRoot       Merkle root over the most recent accepted
//                        OpCommitPermitBatch.PermitRoot.

// ZRegistryRoots is the aggregator. Every field is the canonical
// 48-byte Z-Chain root width. There is no version field; bumping the
// roots aggregator is a hard fork of the EpochCommitment binding.
type ZRegistryRoots struct {
	IdentityRoot       [48]byte
	AccountKeyRoot     [48]byte
	RevocationRoot     [48]byte
	SessionKeyRoot     [48]byte
	ContractPolicyRoot [48]byte
	TxAuthRoot         [48]byte
	PermitAuthRoot     [48]byte
}

// Hash returns the 48-byte commitment over every root. Bound via
// TupleHash256 with customization "LUX_ZCHAIN_REGISTRY_ROOTS_V1" so any
// byte flip on any field changes the digest. Used by EpochCommitment
// (and by the Z-Chain STARK's public inputs) to pin "this is the set
// of registry roots accepted at epoch N."
//
// Order is the declaration order above; bumping the customization tag
// is the only way to change the layout. The tag pins:
//
//   1. The exact set of roots bound. Adding a root bumps the tag.
//   2. The exact order of binding. Re-ordering bumps the tag.
//   3. The exact 48-byte width per field. Width changes bump the tag.
func (r *ZRegistryRoots) Hash() [48]byte {
	if r == nil {
		// Defined empty digest for the nil case. Surface as the
		// canonical zero-roots digest rather than panic so this is
		// safe to call from hot paths that may be passed a nil
		// pointer by a buggy upstream.
		return shake256_384(nil, "LUX_ZCHAIN_REGISTRY_ROOTS_NIL_V1")
	}
	parts := [][]byte{
		[]byte("Lux/ZRegistryRoots/v1"),
		r.IdentityRoot[:],
		r.AccountKeyRoot[:],
		r.RevocationRoot[:],
		r.SessionKeyRoot[:],
		r.ContractPolicyRoot[:],
		r.TxAuthRoot[:],
		r.PermitAuthRoot[:],
	}
	return tupleHash48(parts, "LUX_ZCHAIN_REGISTRY_ROOTS_V1")
}
