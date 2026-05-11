// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry

import (
	"errors"

	"github.com/luxfi/consensus/config"
)

// key_record.go — the canonical PQKeyRecord plus the supporting enums
// (PubkeyLocation, KeyStatus, RevokeReason) and the DeriveAccountID
// helper that produces the 48-byte AccountID a record is keyed under.
//
// One record per (account_id, scheme) pair. The same compact_pubkey
// under two distinct schemes yields two distinct AccountIDs (the scheme
// byte is bound into the derivation), so cross-scheme key reuse cannot
// collide at the AccountID layer.

// AccountIDCustomization is the SP 800-185 cSHAKE256 customization tag
// for AccountID derivation. The tag is the schema identity; bumping it
// produces a digest stream that does not collide with any prior
// AccountID. Exported (not unexported) so tooling that audits the
// derivation can name the tag without re-typing the string.
const AccountIDCustomization = "LUX_ACCOUNT_ID_V1"

// PQKeyRecord is the canonical Z-Chain identity object. One record per
// active account; rotations create a new record and mark the prior
// record as KeyStatusRotated.
//
// Field roles:
//
//   AccountID            48-byte SHAKE256-384 derivation over (profile_id,
//                        chain_id, scheme, compact_pubkey). Bound first
//                        in every op transcript so a forger who replaces
//                        the record's pubkey bytes still has to forge
//                        the AccountID derivation chain.
//   SchemeID             FIPS 204 ML-DSA parameter set the wallet signs
//                        under. ML-DSA-65 is the production default.
//   CompactPubkey        canonical encoded public key (ML-DSA-65 is
//                        1952 bytes per FIPS 204). Inline-storable.
//   CompactPubkeyHash    SHAKE256-384 hash of CompactPubkey. Indexed
//                        alongside AccountID for fast equality checks.
//   ExpandedPubkeyHash   EIP-8051 / EthDilithium expanded form hash —
//                        the 48-byte commitment over the matrix-A
//                        expansion. Stored when the chain's executor
//                        needs the expanded form (e.g. EVM precompile);
//                        otherwise zero.
//   ExpandedPubkeyLoc    where the expanded pubkey lives (inline /
//                        state-trie / off-chain CAS). Defaults to
//                        PubkeyLocInline; off-chain locations require
//                        the executor to resolve before signature verify.
//   Status               key lifecycle state. KeyStatusActive is the
//                        only state under which signatures verify.
//   ValidFromEpoch       first epoch the key is admissible.
//   ValidUntilEpoch      last epoch the key is admissible (inclusive);
//                        0 means "no expiry."
//   RotationPolicyHash   48-byte commitment to the off-chain rotation
//                        policy (cadence, recovery key list, k-of-n
//                        threshold). Zero means "no rotation policy
//                        pinned" (the chain's default applies).
type PQKeyRecord struct {
	AccountID          [48]byte
	SchemeID           config.WalletSchemeID
	CompactPubkey      []byte
	CompactPubkeyHash  [48]byte
	ExpandedPubkeyHash [48]byte
	ExpandedPubkeyLoc  PubkeyLocation
	Status             KeyStatus
	ValidFromEpoch     uint64
	ValidUntilEpoch    uint64
	RotationPolicyHash [48]byte
}

// PubkeyLocation names where the expanded public key lives. The wire
// byte is bound into every op transcript so a record cannot be
// downgraded from inline to off-chain (or vice-versa) without breaking
// the signature.
type PubkeyLocation uint8

const (
	// PubkeyLocInvalid is the zero value. The registry refuses it.
	PubkeyLocInvalid PubkeyLocation = 0x00

	// PubkeyLocInline — expanded pubkey is stored verbatim inside the
	// record (or recomputable from CompactPubkey on demand).
	PubkeyLocInline PubkeyLocation = 0x01

	// PubkeyLocStateTrie — expanded pubkey lives at a derived key
	// inside the chain's state trie; ExpandedPubkeyHash is its commitment.
	PubkeyLocStateTrie PubkeyLocation = 0x02

	// PubkeyLocOffchain — expanded pubkey lives in a content-addressed
	// store referenced by ExpandedPubkeyHash; executors fetch on demand.
	PubkeyLocOffchain PubkeyLocation = 0x03
)

// String returns the canonical wire name.
func (l PubkeyLocation) String() string {
	switch l {
	case PubkeyLocInvalid:
		return "invalid"
	case PubkeyLocInline:
		return "inline"
	case PubkeyLocStateTrie:
		return "state-trie"
	case PubkeyLocOffchain:
		return "offchain"
	default:
		return "pubkey-loc(unknown)"
	}
}

// IsKnown reports whether l is a defined PubkeyLocation.
func (l PubkeyLocation) IsKnown() bool {
	switch l {
	case PubkeyLocInline, PubkeyLocStateTrie, PubkeyLocOffchain:
		return true
	}
	return false
}

// KeyStatus names the lifecycle state of a record. Only KeyStatusActive
// admits signatures; the other states are explicit refusal markers so
// audit tooling can name "this account is revoked / rotated" precisely.
type KeyStatus uint8

const (
	// KeyStatusInvalid is the zero value. The registry refuses it.
	KeyStatusInvalid KeyStatus = 0x00

	// KeyStatusActive — record is live; signatures verify under it.
	KeyStatusActive KeyStatus = 0x01

	// KeyStatusRotated — record has been superseded by a successor
	// record. Signatures under the prior record are refused.
	KeyStatusRotated KeyStatus = 0x02

	// KeyStatusRevoked — record has been revoked. Signatures are
	// refused unconditionally.
	KeyStatusRevoked KeyStatus = 0x03
)

// String returns the canonical wire name.
func (s KeyStatus) String() string {
	switch s {
	case KeyStatusInvalid:
		return "invalid"
	case KeyStatusActive:
		return "active"
	case KeyStatusRotated:
		return "rotated"
	case KeyStatusRevoked:
		return "revoked"
	default:
		return "key-status(unknown)"
	}
}

// IsTerminal reports whether the status is one a record cannot leave.
// Both rotated and revoked are terminal; the registry refuses any op
// that would mutate a terminal record's status further.
func (s KeyStatus) IsTerminal() bool {
	return s == KeyStatusRotated || s == KeyStatusRevoked
}

// RevokeReason names why an OpRevokeKey was issued. Bound into the
// revocation transcript so audit pipelines can distinguish a
// compromise-driven revocation from a routine retire.
type RevokeReason uint8

const (
	// RevokeReasonInvalid is the zero value. Refused.
	RevokeReasonInvalid RevokeReason = 0x00

	// RevokeReasonCompromise — the operator believes the private key
	// was disclosed. Recovery key signature is sufficient (master sig
	// may be unavailable to a compromised account).
	RevokeReasonCompromise RevokeReason = 0x01

	// RevokeReasonRetire — operator-initiated retire of an active
	// key. Master sig REQUIRED.
	RevokeReasonRetire RevokeReason = 0x02

	// RevokeReasonGovernance — chain governance forced removal under
	// a slashing / policy ruling. Governance sig REQUIRED.
	RevokeReasonGovernance RevokeReason = 0x03
)

// String returns the canonical wire name.
func (r RevokeReason) String() string {
	switch r {
	case RevokeReasonInvalid:
		return "invalid"
	case RevokeReasonCompromise:
		return "compromise"
	case RevokeReasonRetire:
		return "retire"
	case RevokeReasonGovernance:
		return "governance"
	default:
		return "revoke-reason(unknown)"
	}
}

// IsKnown reports whether r is a defined RevokeReason.
func (r RevokeReason) IsKnown() bool {
	switch r {
	case RevokeReasonCompromise, RevokeReasonRetire, RevokeReasonGovernance:
		return true
	}
	return false
}

// DeriveAccountID returns the canonical 48-byte AccountID for the given
// (profile_id, chain_id, scheme, compact_pubkey) tuple, computed as:
//
//	AccountID = SHAKE256-384(
//	    profile_id_be4 || chain_id_be4 || u8(scheme) || compact_pubkey,
//	    customization="LUX_ACCOUNT_ID_V1",
//	)
//
// Determinism: identical inputs yield identical outputs across every
// platform and every build. No random seed, no nonce, no timestamp.
//
// Profile separation: profile_id is bound first so the same wallet
// keypair yields distinct AccountIDs on the strict-PQ vs permissive
// profile. A chain operator who migrates a key across profiles MUST
// re-register; the AccountIDs do not match.
//
// Chain separation: chain_id closes the "cross-chain pubkey reuse"
// attack class — an attacker who finds a pubkey collision on one chain
// cannot replay the colliding account on another chain because the
// chain_id prefix forces a distinct digest.
//
// Scheme separation: scheme byte ensures the same compact_pubkey under
// two different schemes yields distinct AccountIDs. Even if two PQ
// schemes shared a public-key byte layout (they do not), the scheme
// byte still keeps the accounts distinct on-chain.
//
// Empty compact_pubkey returns a defined-but-meaningless result; the
// caller is expected to refuse the empty pubkey before calling. See
// PQKeyRecord.Validate for the policy-gated entry point that performs
// that check.
func DeriveAccountID(profileID uint32, chainID uint32, scheme config.WalletSchemeID, compactPubkey []byte) [48]byte {
	// Length-prefix the variable-length compact_pubkey by binding the
	// fixed-width prefix (4 + 4 + 1 = 9 bytes) first and the pubkey
	// last. cSHAKE256 absorbs in a single pass; the customization tag
	// is the domain separator.
	var buf []byte
	buf = append(buf, u32BE(profileID)...)
	buf = append(buf, u32BE(chainID)...)
	buf = append(buf, byte(scheme))
	buf = append(buf, compactPubkey...)
	return shake256_384(buf, AccountIDCustomization)
}

// Hash returns the canonical 48-byte commitment over every record field.
// Bound via TupleHash256 with customization "LUX_PQ_KEY_RECORD_V1" so
// any byte flip on any field changes the digest. Used by the registry
// to build the leaves of identity_root / account_key_root.
//
// Signature: there is no signature on a record itself — the record is
// the witness; the op transcripts (register / rotate / revoke) carry
// the signatures. Record.Hash is what the ops sign over indirectly via
// their bound AccountID + CompactPubkeyHash fields.
func (r *PQKeyRecord) Hash() [48]byte {
	if r == nil {
		// Defined empty digest for the nil case. A nil record is a
		// programmer error at the caller; we surface it as the
		// canonical zero-record digest rather than panic so this is
		// safe to call from hot paths.
		return shake256_384(nil, "LUX_PQ_KEY_RECORD_NIL_V1")
	}
	parts := [][]byte{
		[]byte("Lux/PQKeyRecord/v1"),
		r.AccountID[:],
		{byte(r.SchemeID)},
		r.CompactPubkey,
		r.CompactPubkeyHash[:],
		r.ExpandedPubkeyHash[:],
		{byte(r.ExpandedPubkeyLoc)},
		{byte(r.Status)},
		u64BE(r.ValidFromEpoch),
		u64BE(r.ValidUntilEpoch),
		r.RotationPolicyHash[:],
	}
	return tupleHash48(parts, "LUX_PQ_KEY_RECORD_V1")
}

// Validate runs the structural / policy checks the registry's Apply
// path runs before mutating state. Returns nil iff the record is
// admissible under the supplied profile.
//
// Checks (in order, fail-closed):
//
//  1. SchemeID is non-zero and matches profile.WalletSchemeID byte-for-byte.
//  2. SchemeID.IsPostQuantum() is true (strict-PQ profiles refuse
//     classical wallets via this check, defence-in-depth on top of
//     profile.ForbidECDSAWallets).
//  3. CompactPubkey length matches the scheme's expected width
//     (ML-DSA-65: 1952 bytes; ML-DSA-87: 2592 bytes).
//  4. CompactPubkeyHash equals SHAKE256-384(CompactPubkey).
//  5. AccountID equals DeriveAccountID(profile.ProfileID, chain_id,
//     scheme, compact_pubkey) for the provided chain_id.
//  6. ExpandedPubkeyLoc.IsKnown() is true.
//  7. Status.IsTerminal() is false (a fresh record is never terminal).
//  8. ValidUntilEpoch is 0 OR >= ValidFromEpoch.
func (r *PQKeyRecord) Validate(profile *config.ChainSecurityProfile, chainID uint32) error {
	if r == nil {
		return ErrRegistryNilRecord
	}
	if profile == nil {
		return ErrRegistryNilProfile
	}
	if r.SchemeID == config.WalletSchemeInvalid {
		return ErrRegistryZeroScheme
	}
	if r.SchemeID != profile.WalletSchemeID {
		return ErrRegistrySchemeMismatch
	}
	if !r.SchemeID.IsPostQuantum() {
		return ErrRegistrySchemeForbidden
	}
	if !r.ExpandedPubkeyLoc.IsKnown() {
		return ErrRegistryUnknownPubkeyLoc
	}
	if r.Status.IsTerminal() {
		return ErrRegistryRecordTerminal
	}
	if r.ValidUntilEpoch != 0 && r.ValidUntilEpoch < r.ValidFromEpoch {
		return ErrRegistryValidityInverted
	}
	expectedLen, ok := compactPubkeyLen(r.SchemeID)
	if !ok {
		return ErrRegistrySchemeForbidden
	}
	if len(r.CompactPubkey) != expectedLen {
		return ErrRegistryPubkeyLen
	}
	computedHash := shake256_384(r.CompactPubkey, "LUX_PQ_PUBKEY_HASH_V1")
	if computedHash != r.CompactPubkeyHash {
		return ErrRegistryPubkeyHashMismatch
	}
	derivedID := DeriveAccountID(profile.ProfileID, chainID, r.SchemeID, r.CompactPubkey)
	if derivedID != r.AccountID {
		return ErrRegistryAccountIDMismatch
	}
	return nil
}

// compactPubkeyLen returns the canonical compact-pubkey byte length for
// the named scheme. Only schemes that are admissible on production
// strict-PQ profiles are listed; any other scheme returns (0, false)
// which Validate translates into ErrRegistrySchemeForbidden.
func compactPubkeyLen(s config.WalletSchemeID) (int, bool) {
	switch s {
	case config.WalletSchemeMLDSA65:
		return 1952, true // FIPS 204 ML-DSA-65 public-key byte length.
	case config.WalletSchemeMLDSA87:
		return 2592, true // FIPS 204 ML-DSA-87 public-key byte length.
	}
	return 0, false
}

// =============================================================================
// Typed errors
// =============================================================================

var (
	// ErrRegistryNilRecord — a Validate / Hash receiver was nil where a
	// real value is required.
	ErrRegistryNilRecord = errors.New("registry: nil record")

	// ErrRegistryNilProfile — caller passed nil profile to Validate.
	ErrRegistryNilProfile = errors.New("registry: nil profile")

	// ErrRegistryZeroScheme — SchemeID was config.WalletSchemeInvalid.
	ErrRegistryZeroScheme = errors.New("registry: SchemeID is zero")

	// ErrRegistrySchemeMismatch — SchemeID does not match the profile's
	// pinned WalletSchemeID.
	ErrRegistrySchemeMismatch = errors.New("registry: SchemeID does not match profile.WalletSchemeID")

	// ErrRegistrySchemeForbidden — SchemeID is not in the admissible
	// strict-PQ set (e.g. classical ECDSA marker).
	ErrRegistrySchemeForbidden = errors.New("registry: SchemeID forbidden under strict-PQ")

	// ErrRegistryUnknownPubkeyLoc — ExpandedPubkeyLoc was not one of
	// {inline, state-trie, offchain}.
	ErrRegistryUnknownPubkeyLoc = errors.New("registry: ExpandedPubkeyLoc unknown")

	// ErrRegistryRecordTerminal — record is in a terminal status
	// (rotated / revoked) and cannot be registered as a new record.
	ErrRegistryRecordTerminal = errors.New("registry: record status is terminal")

	// ErrRegistryValidityInverted — ValidUntilEpoch < ValidFromEpoch
	// (with no-expiry as the only exception, encoded as ValidUntil=0).
	ErrRegistryValidityInverted = errors.New("registry: ValidUntilEpoch precedes ValidFromEpoch")

	// ErrRegistryPubkeyLen — CompactPubkey length did not match the
	// scheme's declared public-key byte length.
	ErrRegistryPubkeyLen = errors.New("registry: CompactPubkey length does not match scheme")

	// ErrRegistryPubkeyHashMismatch — CompactPubkeyHash did not equal
	// SHAKE256-384(CompactPubkey).
	ErrRegistryPubkeyHashMismatch = errors.New("registry: CompactPubkeyHash mismatch")

	// ErrRegistryAccountIDMismatch — DeriveAccountID(...) did not
	// equal record.AccountID. Wrong derivation inputs.
	ErrRegistryAccountIDMismatch = errors.New("registry: AccountID does not match DeriveAccountID")
)
