// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

// account_id.go — PQ AccountID derivation.
//
// AccountID is the 48-byte (384-bit) canonical identifier of a Lux PQ
// account. It is derived deterministically from (profile_id, chain_id,
// wallet_scheme, compact_pubkey) using SHAKE256-384 with the
// customization tag "LUX_ACCOUNT_ID_V1".
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
// Domain separation: the customization tag "LUX_ACCOUNT_ID_V1" is part
// of the cSHAKE256 customization stream; changing the tag yields a
// digest stream that does not collide with any prior AccountID under a
// different tag. Bumping the tag is a hard fork of account derivation;
// there is no compatibility window.
//
// Canonicality: this derivation is byte-identical to
// protocol/zchain/registry.DeriveAccountID. The two functions vendor
// their own hash kernels (each package owns its TupleHash256 / SHAKE256
// helper to avoid an upward dependency on its parent) but produce
// identical 48-byte output for identical inputs. The parity is
// enforced by TestDeriveAccountID_MatchesRegistryDerivation.
//
// History: an earlier customization "LUX_ACCOUNT_V1" with inputs
// (chainID, scheme, pubkey) — no profile binding — existed in the
// auth-package draft but was never deployed to a live chain. The
// canonical binding is now (profileID, chainID, scheme, pubkey) under
// tag "LUX_ACCOUNT_ID_V1"; the registry path has always used this form,
// so reconciling auth to it is a forward-only change.

// accountIDCustomization is the SP 800-185 cSHAKE256 customization tag
// for AccountID derivation. The tag is the schema identity; bumping it
// produces a digest stream that does not collide with any prior
// AccountID under the previous tag.
//
// Pinned at "LUX_ACCOUNT_ID_V1" to match
// protocol/zchain/registry.AccountIDCustomization. The two MUST agree
// byte-for-byte; the parity test refuses any drift.
const accountIDCustomization = "LUX_ACCOUNT_ID_V1"

// DeriveAccountID returns the 48-byte canonical PQ AccountID for the
// given (profileID, chainID, walletScheme, compactPubkey) tuple:
//
//	AccountID = SHAKE256-384(
//	    profile_id_be4 || chain_id_be4 || u8(scheme) || compact_pubkey,
//	    customization="LUX_ACCOUNT_ID_V1",
//	)
//
// Determinism: identical inputs yield identical outputs across every
// platform and every build. There is no random seed, no nonce, no
// timestamp — every byte of the output is a function of the inputs.
//
// Profile separation: profile_id is bound first so the same wallet
// keypair yields distinct AccountIDs on the strict-PQ vs permissive
// profile (and across Lux / Zoo / Hanzo strict-PQ siblings). A chain
// operator who migrates a key across profiles MUST re-register; the
// AccountIDs do not match. This closes the cross-profile replay class.
//
// Chain separation: chain_id closes the "cross-chain pubkey reuse"
// attack class — an attacker who finds a pubkey collision on one chain
// cannot replay the colliding account on another chain because the
// chain_id prefix forces a distinct digest.
//
// Scheme separation: scheme byte ensures the same pubkey under two
// different schemes yields distinct AccountIDs. Defence-in-depth: even
// if two PQ schemes shared a public-key byte layout (they do not), the
// scheme byte still keeps the accounts distinct on-chain.
//
// Refuses an empty pubkey by returning a defined-but-meaningless
// result; the caller is expected to check pubkey length against the
// scheme's declared public-key width before invoking this function. See
// VerifyTxAuthEnvelope for the policy-gated entry point that performs
// that check.
//
// Hash family: SHAKE256 / cSHAKE256 with output length 48 bytes.
func DeriveAccountID(profileID uint32, chainID uint32, scheme WalletSchemeID, compactPubkey []byte) [48]byte {
	// Length-prefix the variable-length compact_pubkey by binding the
	// fixed-width prefix (4 + 4 + 1 = 9 bytes) first and the pubkey
	// last. cSHAKE256 absorbs in a single pass; the customization tag
	// is the domain separator. Length-extension is impossible against
	// cSHAKE256 (XOF over fixed-output) so we omit the SP 800-185
	// length-prefix framing here.
	//
	// Byte-for-byte identical to protocol/zchain/registry.DeriveAccountID
	// — the two functions agree on the input layout, the customization
	// tag, and the hash kernel.
	var buf []byte
	buf = append(buf, u32BE(profileID)...)
	buf = append(buf, u32BE(chainID)...)
	buf = append(buf, byte(scheme))
	buf = append(buf, compactPubkey...)
	return shake256_384(buf, accountIDCustomization)
}
