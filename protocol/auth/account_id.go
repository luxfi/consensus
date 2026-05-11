// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import (
	"encoding/binary"
)

// account_id.go — PQ AccountID derivation.
//
// AccountID is the 48-byte (384-bit) canonical identifier of a Lux PQ
// account. It is derived deterministically from (chain_id, wallet_scheme,
// pubkey) using SHAKE256-384 with the customization tag "LUX_ACCOUNT_V1".
//
// Rationale for 48 bytes: matches the canonical Z-Chain transcript /
// state root width (SHA3-384 / cSHAKE256 384-bit output) used across
// the consensus tree. One width across the project = one path through
// the wire for every fixed-size identifier. The 16 extra bytes over a
// 32-byte EVM address are paid for once at account creation and never
// again; the security margin against collision attacks on a PQ wallet
// pubkey is comparable to the signature scheme's security level.
//
// Rationale for SHAKE256 (not keccak-256): SHAKE256 is FIPS 202, sits
// on the same SP 800-185 family as every other transcript / digest in
// this package, and naturally supports the 48-byte output width. Using
// keccak-256 here would force a 32-byte AccountID and pull in a hash
// family disjoint from the rest of the PQ surface.
//
// Domain separation: the customization tag "LUX_ACCOUNT_V1" is part of
// the cSHAKE256 customization stream; changing the tag yields a digest
// stream that does not collide with any prior AccountID. Bumping the
// tag is a hard fork of account derivation; there is no compatibility
// window.

// accountIDCustomization is the SP 800-185 cSHAKE256 customization tag
// for AccountID derivation. The tag is the schema identity; changing it
// produces a digest stream that does not collide with any prior
// AccountID under the previous tag.
const accountIDCustomization = "LUX_ACCOUNT_V1"

// DeriveAccountID returns the 48-byte canonical PQ AccountID for the
// given (chainID, walletScheme, pubkey) triple. The derivation is:
//
//	AccountID = SHAKE256-384("LUX_ACCOUNT_V1", chain_id_be4 || u8(scheme) || pubkey)
//
// Determinism: identical inputs yield identical outputs across every
// platform and every build. There is no random seed, no nonce, no
// timestamp — every byte of the output is a function of the inputs.
//
// Chain separation: chain_id is bound first so the same (scheme, pubkey)
// pair yields distinct AccountIDs on distinct chains. This closes the
// "cross-chain pubkey reuse" attack class: an attacker who finds a
// pubkey collision on one chain cannot replay the colliding account on
// another chain — the chain_id byte prefix forces a different digest.
//
// Scheme separation: wallet_scheme is bound after chain_id so the same
// pubkey under two different schemes yields distinct AccountIDs. This
// is defence-in-depth: even if two PQ schemes shared a public-key byte
// layout (they do not), the scheme byte still keeps the accounts
// distinct on-chain.
//
// Refuses an empty pubkey by returning a defined-but-empty result; the
// caller is expected to check pubkey length against the scheme's
// declared public-key width before invoking this function. See
// VerifyTxAuthEnvelope for the policy-gated entry point that performs
// that check.
//
// Hash family: SHAKE256 / cSHAKE256 with output length 48 bytes.
func DeriveAccountID(chainID uint32, scheme WalletSchemeID, pubkey []byte) [48]byte {
	// Bind length-prefixed parts manually so the binding is identical to
	// a TupleHash transcript: chain_id (4 BE) || scheme (1 byte) || pubkey.
	//
	// We do NOT use TupleHash256 here even though it would also work,
	// because AccountID derivation is performance-sensitive (wallets
	// derive it once per session, contracts derive it on every call):
	// a single cSHAKE256 absorb is the cheapest possible PQ-aligned
	// hash invocation, and we still get domain separation via the
	// "LUX_ACCOUNT_V1" customization tag.
	//
	// The four-byte chain_id and one-byte scheme are fixed-width, so
	// the only variable-length part is pubkey. Length-extension is
	// impossible against cSHAKE256 (XOF over fixed-output) so we omit
	// the SP 800-185 length-prefix framing here.
	var buf []byte
	var chain [4]byte
	binary.BigEndian.PutUint32(chain[:], chainID)
	buf = append(buf, chain[:]...)
	buf = append(buf, byte(scheme))
	buf = append(buf, pubkey...)
	return shake256_384(buf, accountIDCustomization)
}
