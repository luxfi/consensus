// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
)

// permit.go — PQPermit: the PQ-native replacement for EIP-2612 ERC-20
// permit / EIP-712 typed-data signatures. One owner authorises one
// spender to draw up to a value cap before a deadline, against the
// owner's account-bound nonce.
//
// EIP-2612 binds via EIP-712 typed-data hashing under keccak-256 with
// secp256k1 signatures. PQPermit binds via TupleHash256 under cSHAKE256
// with ML-DSA (or another PQ wallet scheme); the wire layout and
// digest construction are designed so the same shape works for every
// ERC-20-like surface (allowance, swap-permit, transfer-permit) by
// changing the verifying contract address only.
//
// Digest binding is intentionally rich: every byte the EVM would
// otherwise depend on for "is this permit valid right now" is bound
// into Digest() via TupleHash256 customization "LUX_PQ_PERMIT_V1".
// Replay across (chain, contract, owner, spender, value, nonce,
// deadline, scheme, suite) is closed deterministically.

// pqPermitDigestCustomization is the SP 800-185 cSHAKE256 customization
// tag for PQPermit.Digest. Bumping it invalidates every prior permit
// signature.
const pqPermitDigestCustomization = "LUX_PQ_PERMIT_V1"

// pqPermitProtocolTag is the in-band redundant protocol tag bound as
// the first TupleHash part.
const pqPermitProtocolTag = "Lux/PQPermit/v1"

// PQPermit is the PQ-native EIP-2612 replacement.
//
// Field roles:
//
//	Version           permit-format version; bumped on incompatible
//	                  layout changes. Current value: 1.
//	ProfileID         ChainSecurityProfile this permit was produced
//	                  under.
//	ChainID           L1/L2 chain identifier (cross-chain replay seal).
//	VerifyingContract 48-byte AccountID of the contract that consumes
//	                  this permit (e.g. an ERC-20 token contract).
//	OwnerAccountID    48-byte PQ AccountID of the permit issuer.
//	Spender           48-byte PQ AccountID authorised to draw on this
//	                  permit.
//	Value             32-byte big-endian unsigned integer cap. 2^256-1
//	                  means "unlimited" — chains MAY reject that.
//	Nonce             monotonic per-owner permit counter. The verifying
//	                  contract MUST refuse a permit whose Nonce does
//	                  not match its stored next-nonce for the owner.
//	Deadline          UNIX-style or chain-height deadline beyond which
//	                  the permit is invalid. Convention is chain-height;
//	                  the verifying contract decides.
//	AuthSchemeID      contract-side auth scheme (config.ContractAuthID
//	                  equivalent in this package). The verifying
//	                  contract MUST refuse a permit whose AuthSchemeID
//	                  is not in its declared per-contract auth profile.
//	HashSuiteID       hash family bound into the digest as DATA. Must
//	                  match profile.HashSuiteID at verification time.
//	Signature         owner's signature over Digest() under
//	                  AuthSchemeID's primitive (ML-DSA family, etc.).
type PQPermit struct {
	Version uint16

	ProfileID         config.ProfileID
	ChainID           uint32
	VerifyingContract [48]byte
	OwnerAccountID    [48]byte
	Spender           [48]byte
	Value             [32]byte
	Nonce             uint64
	Deadline          uint64

	AuthSchemeID ContractAuthID
	HashSuiteID  config.HashSuiteID

	Signature []byte
}

// Digest returns the 48-byte digest the owner signs over for this
// permit. Bound via TupleHash256 (customization "LUX_PQ_PERMIT_V1") so
// any byte flip on any security-relevant field breaks signature
// verification.
//
// Signature is NOT bound (the digest is what the signature is computed
// over). All other fields are bound exactly once.
func (p *PQPermit) Digest() [48]byte {
	parts := [][]byte{
		[]byte(pqPermitProtocolTag),
		u16BE(p.Version),
		{byte(p.ProfileID)},
		u32BE(p.ChainID),
		p.VerifyingContract[:],
		p.OwnerAccountID[:],
		p.Spender[:],
		p.Value[:],
		u64BE(p.Nonce),
		u64BE(p.Deadline),
		{byte(p.AuthSchemeID)},
		{byte(p.HashSuiteID)},
	}
	return tupleHash48(parts, pqPermitDigestCustomization)
}

// VerifyPQPermit is the profile-gated verifier for a PQPermit.
//
// Checks (in order):
//
//  1. Structural: permit non-nil, profile non-nil, Version supported.
//  2. Profile/id: permit.ProfileID matches profile.ProfileID.
//  3. Profile/suite: permit.HashSuiteID matches profile.HashSuiteID.
//  4. Profile/scheme: permit.AuthSchemeID is post-quantum (refuses
//     ECDSA legacy under any locked profile that pins PQ).
//  5. Owner pubkey shape: ownerPubkey non-empty.
//  6. Signature: a caller-side SignatureVerifierFn validates the
//     signature under the owner's pubkey. The signature kernel here
//     is wallet-grade; we reuse SignatureVerifierFn from
//     tx_envelope.go because the ContractAuthID block parallels
//     WalletSchemeID one-to-one.
//
// On any failure returns a typed error from the ErrPQPermit* set.
func VerifyPQPermit(
	profile *config.ChainSecurityProfile,
	permit *PQPermit,
	ownerPubkey []byte,
	sigVerifier SignatureVerifierFn,
) error {
	if permit == nil {
		return ErrPQPermitNil
	}
	if profile == nil {
		return ErrPQPermitInvalidProfile
	}
	if sigVerifier == nil {
		return ErrPQPermitMissingSigVerifier
	}
	if permit.Version == 0 || permit.Version > PQPermitCurrentVersion {
		return fmt.Errorf("%w: Version=%d", ErrPQPermitVersionUnsupported, permit.Version)
	}

	if uint32(permit.ProfileID) != profile.ProfileID {
		return fmt.Errorf("%w: permit=0x%02x profile=0x%08x",
			ErrPQPermitInvalidProfile, uint8(permit.ProfileID), profile.ProfileID)
	}

	if permit.HashSuiteID != profile.HashSuiteID {
		return fmt.Errorf("%w: permit=%s profile=%s",
			ErrPQPermitHashSuiteMismatch,
			permit.HashSuiteID.String(), profile.HashSuiteID.String())
	}

	if !permit.AuthSchemeID.IsPostQuantum() {
		return fmt.Errorf("%w: %s",
			ErrPQPermitAuthSchemeNotAllowed, permit.AuthSchemeID.String())
	}

	if len(ownerPubkey) == 0 {
		return ErrPQPermitEmptyOwnerPubkey
	}

	if len(permit.Signature) == 0 {
		return ErrPQPermitSignatureInvalid
	}

	// Map ContractAuthID to WalletSchemeID for signature verification:
	// they share byte layout one-to-one because contract-auth signatures
	// over a permit are the same primitive a wallet signs with (single-
	// party ML-DSA / SLH-DSA). The verifier function dispatches on the
	// WalletSchemeID value; the contract-auth wrapper exists so a
	// contract can pin a different scheme than the chain default.
	walletScheme := WalletSchemeID(permit.AuthSchemeID)
	if !walletScheme.IsPostQuantum() {
		// Belt-and-braces: even if a future ContractAuthID maps to a
		// classical wallet, refuse here.
		return fmt.Errorf("%w: mapped-wallet=%s",
			ErrPQPermitAuthSchemeNotAllowed, walletScheme.String())
	}

	digest := permit.Digest()
	ok, vErr := sigVerifier(walletScheme, ownerPubkey, digest[:], permit.Signature)
	if vErr != nil {
		return fmt.Errorf("%w: %v", ErrPQPermitSignatureInvalid, vErr)
	}
	if !ok {
		return ErrPQPermitSignatureInvalid
	}
	return nil
}

// PQPermitCurrentVersion is the current PQPermit wire-format version.
const PQPermitCurrentVersion uint16 = 1

// =============================================================================
// Typed errors
// =============================================================================

var (
	// ErrPQPermitNil — receiver is nil.
	ErrPQPermitNil = errors.New("pqpermit: nil permit")

	// ErrPQPermitInvalidProfile — profile is nil or permit.ProfileID
	// does not match profile.ProfileID.
	ErrPQPermitInvalidProfile = errors.New("pqpermit: invalid or mismatched profile")

	// ErrPQPermitVersionUnsupported — permit.Version is zero or exceeds
	// PQPermitCurrentVersion.
	ErrPQPermitVersionUnsupported = errors.New("pqpermit: Version unsupported")

	// ErrPQPermitHashSuiteMismatch — permit.HashSuiteID does not match
	// profile.HashSuiteID.
	ErrPQPermitHashSuiteMismatch = errors.New("pqpermit: HashSuiteID does not match profile")

	// ErrPQPermitAuthSchemeNotAllowed — permit.AuthSchemeID is either
	// None or a non-PostQuantum scheme.
	ErrPQPermitAuthSchemeNotAllowed = errors.New("pqpermit: AuthSchemeID not allowed under profile")

	// ErrPQPermitEmptyOwnerPubkey — caller passed an empty pubkey slice.
	ErrPQPermitEmptyOwnerPubkey = errors.New("pqpermit: owner pubkey is empty")

	// ErrPQPermitSignatureInvalid — the owner signature did not verify,
	// or no signature bytes were provided.
	ErrPQPermitSignatureInvalid = errors.New("pqpermit: signature does not verify")

	// ErrPQPermitMissingSigVerifier — caller did not inject a
	// SignatureVerifierFn.
	ErrPQPermitMissingSigVerifier = errors.New("pqpermit: SignatureVerifierFn is required")
)
