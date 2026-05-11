// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package registry

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/crypto/pq/mldsa/mldsa65"
)

// ops.go — the six Z-Chain registry operations.
//
// Each op:
//
//   1. Carries every byte the chain needs to apply it to state.
//   2. Defines its own canonical signing transcript via TupleHash256
//      with an op-specific customization tag so a signature on one op
//      cannot replay as another op.
//   3. Exposes Verify(profile) that runs structural / scheme / sig
//      checks fail-closed. Verify performs NO state mutation.
//
// The signing transcript for every op is consumed by ML-DSA-65 via the
// FIPS 204 §5.2 ctx field (txAuthRegistryCtx), which gives a third
// layer of domain separation on top of:
//
//   - per-op cSHAKE256 customization tag
//   - per-record AccountID derivation
//
// One scheme is wired here: ML-DSA-65 (production default). The op
// types accept any config.WalletSchemeID but Verify refuses anything
// that doesn't match profile.WalletSchemeID, which strict-PQ profiles
// pin to ML-DSA-65. When ML-DSA-87 lands on a profile, the dispatch
// in verifyMLDSA below picks up the additional scheme — no new op
// types required.

// txAuthRegistryCtx is the FIPS 204 §5.2 ctx string bound into every
// ML-DSA signature in this package. Distinct from any other ctx used
// elsewhere in the consensus tree so a registry signature cannot
// replay against a TxAuthEnvelope verifier (or vice-versa).
var txAuthRegistryCtx = []byte("LUX_ZCHAIN_REGISTRY_V1")

// =============================================================================
// OpRegisterKey
// =============================================================================

// OpRegisterKey adds a fresh PQKeyRecord to the identity set. The new
// record's pubkey signs its own AccountID transcript (proof of
// possession), so no prior key is required.
type OpRegisterKey struct {
	// NewRecord is the record to insert. Its Status MUST be
	// KeyStatusActive at submit time; Verify refuses otherwise.
	NewRecord PQKeyRecord

	// SelfSig is the ML-DSA signature produced by NewRecord.CompactPubkey
	// over OpRegisterKey.SigningDigest(). Proof-of-possession: a
	// registrant cannot publish a pubkey without holding the matching
	// secret key.
	SelfSig []byte
}

// SigningDigest returns the 48-byte digest the SelfSig signs over.
// Bound via TupleHash256 with customization "LUX_OP_REGISTER_KEY_V1".
func (o *OpRegisterKey) SigningDigest() [48]byte {
	recordHash := o.NewRecord.Hash()
	parts := [][]byte{
		[]byte("Lux/OpRegisterKey/v1"),
		o.NewRecord.AccountID[:],
		{byte(o.NewRecord.SchemeID)},
		o.NewRecord.CompactPubkeyHash[:],
		recordHash[:],
	}
	return tupleHash48(parts, "LUX_OP_REGISTER_KEY_V1")
}

// Verify runs the fail-closed admissibility check chain for a register
// op. Returns nil iff the op is admissible under profile.
func (o *OpRegisterKey) Verify(profile *config.ChainSecurityProfile, chainID uint32) error {
	if o == nil {
		return ErrOpNil
	}
	if profile == nil {
		return ErrRegistryNilProfile
	}
	// Record must validate against the profile.
	if err := o.NewRecord.Validate(profile, chainID); err != nil {
		return fmt.Errorf("%w: %v", ErrOpRecordInvalid, err)
	}
	// Fresh record MUST be Active.
	if o.NewRecord.Status != KeyStatusActive {
		return ErrOpRecordNotActive
	}
	// Proof of possession over SigningDigest.
	digest := o.SigningDigest()
	if err := verifyMLDSA(o.NewRecord.SchemeID, o.NewRecord.CompactPubkey, digest[:], o.SelfSig); err != nil {
		return fmt.Errorf("%w: %v", ErrOpSelfSigInvalid, err)
	}
	return nil
}

// =============================================================================
// OpRotateKey
// =============================================================================

// OpRotateKey supersedes an existing active record (OldAccountID) with a
// fresh record (NewRecord). Both keys sign: the old key authorises the
// transition; the new key proves possession of its own private key.
type OpRotateKey struct {
	// OldAccountID is the AccountID being rotated out. The registry's
	// Apply path looks the record up and marks it KeyStatusRotated.
	OldAccountID [48]byte

	// NewRecord is the successor record. Its Status MUST be
	// KeyStatusActive at submit time.
	NewRecord PQKeyRecord

	// OldSig is the signature produced by the old record's key over
	// the rotation transcript. Closes the "anyone can take over an
	// account" attack class — only a holder of the old secret key can
	// authorise the transition.
	OldSig []byte

	// NewSig is the signature produced by the new record's key over
	// the rotation transcript. Proof of possession on the successor
	// key, same as OpRegisterKey.SelfSig.
	NewSig []byte
}

// SigningDigest returns the 48-byte digest both OldSig and NewSig sign
// over. Bound via TupleHash256 with customization "LUX_OP_ROTATE_KEY_V1".
func (o *OpRotateKey) SigningDigest() [48]byte {
	recordHash := o.NewRecord.Hash()
	parts := [][]byte{
		[]byte("Lux/OpRotateKey/v1"),
		o.OldAccountID[:],
		o.NewRecord.AccountID[:],
		{byte(o.NewRecord.SchemeID)},
		o.NewRecord.CompactPubkeyHash[:],
		recordHash[:],
	}
	return tupleHash48(parts, "LUX_OP_ROTATE_KEY_V1")
}

// Verify runs the fail-closed admissibility check chain for a rotate
// op. The caller resolves the old record's pubkey via OldKeyLookup;
// Verify then dispatches both signatures.
//
// OldKeyLookup is the chain-side resolver that maps an AccountID to
// the holder's stored (scheme, compact_pubkey). The registry's Apply
// path injects it; tests inject a fixture. Returning nil pubkey or a
// non-active status is a refusal.
type OldKeyLookup func(accountID [48]byte) (scheme config.WalletSchemeID, compactPubkey []byte, status KeyStatus, err error)

// VerifyWithLookup runs the rotate op's checks given a resolver for the
// old record's key. Splits Verify so the rotate signature surface is
// dependency-injection-clean (no upward import).
func (o *OpRotateKey) VerifyWithLookup(profile *config.ChainSecurityProfile, chainID uint32, lookup OldKeyLookup) error {
	if o == nil {
		return ErrOpNil
	}
	if profile == nil {
		return ErrRegistryNilProfile
	}
	if lookup == nil {
		return ErrOpMissingLookup
	}
	if isZero48(o.OldAccountID) {
		return ErrOpZeroOldAccount
	}
	if o.OldAccountID == o.NewRecord.AccountID {
		return ErrOpRotateSameAccount
	}
	if err := o.NewRecord.Validate(profile, chainID); err != nil {
		return fmt.Errorf("%w: %v", ErrOpRecordInvalid, err)
	}
	if o.NewRecord.Status != KeyStatusActive {
		return ErrOpRecordNotActive
	}
	oldScheme, oldPubkey, oldStatus, err := lookup(o.OldAccountID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrOpOldKeyLookup, err)
	}
	if len(oldPubkey) == 0 {
		return ErrOpOldKeyLookup
	}
	if oldStatus != KeyStatusActive {
		return ErrOpOldKeyNotActive
	}
	digest := o.SigningDigest()
	if err := verifyMLDSA(oldScheme, oldPubkey, digest[:], o.OldSig); err != nil {
		return fmt.Errorf("%w: %v", ErrOpOldSigInvalid, err)
	}
	if err := verifyMLDSA(o.NewRecord.SchemeID, o.NewRecord.CompactPubkey, digest[:], o.NewSig); err != nil {
		return fmt.Errorf("%w: %v", ErrOpNewSigInvalid, err)
	}
	return nil
}

// =============================================================================
// OpRevokeKey
// =============================================================================

// OpRevokeKey marks an existing record KeyStatusRevoked. Authorised by
// either the record's master key (for retire / governance), or a
// pre-pinned recovery key (for compromise).
type OpRevokeKey struct {
	// AccountID is the record to revoke. Must resolve via the
	// caller-supplied lookup to an active record.
	AccountID [48]byte

	// Reason names why the revocation was issued. Bound into the
	// signing transcript so a retire signature cannot replay as a
	// compromise signature.
	Reason RevokeReason

	// Sig is the signature over SigningDigest. Verified under either
	// the master key (retire / governance) or the recovery key
	// (compromise); see VerifyWithLookup for dispatch.
	Sig []byte
}

// SigningDigest returns the 48-byte digest Sig signs over. Bound via
// TupleHash256 with customization "LUX_OP_REVOKE_KEY_V1".
func (o *OpRevokeKey) SigningDigest() [48]byte {
	parts := [][]byte{
		[]byte("Lux/OpRevokeKey/v1"),
		o.AccountID[:],
		{byte(o.Reason)},
	}
	return tupleHash48(parts, "LUX_OP_REVOKE_KEY_V1")
}

// RecoveryKeyLookup resolves an AccountID's pinned recovery key, when
// one was registered at the account-creation time. Returns (nil, nil)
// when no recovery key was registered; the verifier treats that as
// "compromise reason is not admissible for this account."
type RecoveryKeyLookup func(accountID [48]byte) (scheme config.WalletSchemeID, compactPubkey []byte, err error)

// VerifyWithLookup runs the revoke op's checks given resolvers for the
// master and recovery keys. Dispatch:
//
//   - RevokeReasonRetire / RevokeReasonGovernance — verifies Sig under
//     the master key returned by masterLookup.
//   - RevokeReasonCompromise — verifies Sig under the recovery key
//     returned by recoveryLookup. If no recovery key is pinned, refuses.
func (o *OpRevokeKey) VerifyWithLookup(
	profile *config.ChainSecurityProfile,
	masterLookup OldKeyLookup,
	recoveryLookup RecoveryKeyLookup,
) error {
	if o == nil {
		return ErrOpNil
	}
	if profile == nil {
		return ErrRegistryNilProfile
	}
	if masterLookup == nil || recoveryLookup == nil {
		return ErrOpMissingLookup
	}
	if isZero48(o.AccountID) {
		return ErrOpZeroAccount
	}
	if !o.Reason.IsKnown() {
		return ErrOpUnknownRevokeReason
	}
	digest := o.SigningDigest()

	switch o.Reason {
	case RevokeReasonRetire, RevokeReasonGovernance:
		scheme, pubkey, status, err := masterLookup(o.AccountID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrOpOldKeyLookup, err)
		}
		if len(pubkey) == 0 {
			return ErrOpOldKeyLookup
		}
		if status != KeyStatusActive {
			return ErrOpOldKeyNotActive
		}
		if err := verifyMLDSA(scheme, pubkey, digest[:], o.Sig); err != nil {
			return fmt.Errorf("%w: %v", ErrOpRevokeSigInvalid, err)
		}
	case RevokeReasonCompromise:
		scheme, pubkey, err := recoveryLookup(o.AccountID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrOpRecoveryLookup, err)
		}
		if len(pubkey) == 0 {
			return ErrOpRecoveryNotPinned
		}
		if err := verifyMLDSA(scheme, pubkey, digest[:], o.Sig); err != nil {
			return fmt.Errorf("%w: %v", ErrOpRevokeSigInvalid, err)
		}
	default:
		return ErrOpUnknownRevokeReason
	}
	return nil
}

// =============================================================================
// OpAuthorizeSession
// =============================================================================

// SessionPolicy is the on-the-wire policy attached to a session key
// authorisation. Each field bound into the signing transcript so any
// mutation breaks the signature.
type SessionPolicy struct {
	// AllowedContractsRoot is a Merkle root over the set of contract
	// addresses this session key may invoke. Zero means "no contract
	// allowlist" (any contract); production policies SHOULD pin a
	// non-zero root.
	AllowedContractsRoot [48]byte

	// MaxGasPerCall caps the gas this session key may consume in a
	// single call. Zero means "no cap." Production policies SHOULD
	// pin a real cap.
	MaxGasPerCall uint64

	// MaxCallsPerEpoch caps the call count per epoch. Zero means "no
	// cap." Defence against a leaked session key racing the chain
	// before the master key revokes it.
	MaxCallsPerEpoch uint64
}

// Hash returns the 48-byte commitment over the policy. Bound via
// TupleHash256 with customization "LUX_SESSION_POLICY_V1".
func (p *SessionPolicy) Hash() [48]byte {
	if p == nil {
		return shake256_384(nil, "LUX_SESSION_POLICY_NIL_V1")
	}
	parts := [][]byte{
		[]byte("Lux/SessionPolicy/v1"),
		p.AllowedContractsRoot[:],
		u64BE(p.MaxGasPerCall),
		u64BE(p.MaxCallsPerEpoch),
	}
	return tupleHash48(parts, "LUX_SESSION_POLICY_V1")
}

// OpAuthorizeSession authorises an ephemeral session key under an
// existing active record. The master key signs; the session key is
// stored as a 48-byte hash (not the raw bytes) so a public log of
// authorisations does not leak the session pubkey before first use.
type OpAuthorizeSession struct {
	// AccountID is the master account authorising the session.
	AccountID [48]byte

	// SessionPubkey is the raw bytes of the session key. Length must
	// match a known scheme (the policy doesn't fix the session key's
	// scheme — a chain MAY allow ML-DSA-44 session keys under an
	// ML-DSA-65 master, for example).
	SessionPubkey []byte

	// SessionPolicy is the policy attached to this session key.
	SessionPolicy SessionPolicy

	// ValidUntil is the block height (or epoch number — chain-defined)
	// after which the session key is refused. MUST be > 0.
	ValidUntil uint64

	// MasterSig is the signature over SigningDigest produced by the
	// master record's key. Refuses any session that the master did
	// not authorise.
	MasterSig []byte
}

// SigningDigest returns the 48-byte digest MasterSig signs over. Bound
// via TupleHash256 with customization "LUX_OP_AUTHORIZE_SESSION_V1".
func (o *OpAuthorizeSession) SigningDigest() [48]byte {
	sessionHash := shake256_384(o.SessionPubkey, "LUX_SESSION_PUBKEY_HASH_V1")
	policyHash := o.SessionPolicy.Hash()
	parts := [][]byte{
		[]byte("Lux/OpAuthorizeSession/v1"),
		o.AccountID[:],
		sessionHash[:],
		policyHash[:],
		u64BE(o.ValidUntil),
	}
	return tupleHash48(parts, "LUX_OP_AUTHORIZE_SESSION_V1")
}

// VerifyWithLookup runs the fail-closed check chain for a session
// authorisation, looking up the master key via masterLookup.
func (o *OpAuthorizeSession) VerifyWithLookup(
	profile *config.ChainSecurityProfile,
	masterLookup OldKeyLookup,
	currentHeight uint64,
) error {
	if o == nil {
		return ErrOpNil
	}
	if profile == nil {
		return ErrRegistryNilProfile
	}
	if masterLookup == nil {
		return ErrOpMissingLookup
	}
	if isZero48(o.AccountID) {
		return ErrOpZeroAccount
	}
	if len(o.SessionPubkey) == 0 {
		return ErrOpZeroSessionKey
	}
	if o.ValidUntil == 0 {
		return ErrOpSessionNoExpiry
	}
	if currentHeight > 0 && o.ValidUntil <= currentHeight {
		return ErrOpSessionAlreadyExpired
	}
	scheme, pubkey, status, err := masterLookup(o.AccountID)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrOpOldKeyLookup, err)
	}
	if len(pubkey) == 0 {
		return ErrOpOldKeyLookup
	}
	if status != KeyStatusActive {
		return ErrOpOldKeyNotActive
	}
	digest := o.SigningDigest()
	if err := verifyMLDSA(scheme, pubkey, digest[:], o.MasterSig); err != nil {
		return fmt.Errorf("%w: %v", ErrOpMasterSigInvalid, err)
	}
	return nil
}

// =============================================================================
// OpCommitTxAuthBatch — the rollup's hot-path commitment
// =============================================================================

// OpCommitTxAuthBatch publishes the roots of an accepted batch of
// TxAuthEnvelope signatures the rollup has verified off the execution
// hot path. The execution layer subsequently checks transaction
// admission via VerifyAuthPassed, which only needs the BatchRoot — no
// ML-DSA verify on the hot path.
//
// There is no signature on this op type: the act of committing is
// authorised by the Z-Chain proof system itself (a STARK proof that
// every signature in the batch verified). The proof lives outside this
// struct (it rides in the ZProofEnvelope that accompanies the op);
// what's bound here is the batch's commitments.
type OpCommitTxAuthBatch struct {
	// Epoch is the Z-Chain epoch at which the batch was accepted.
	Epoch uint64

	// BatchRoot is the 48-byte Merkle root over the sorted set of
	// (account_id, tx_digest) tuples whose signatures verified.
	// VerifyAuthPassed in auth_proof.go consumes this root.
	BatchRoot [48]byte

	// SignatureRoot is the 48-byte Merkle root over the signature
	// blobs (one leaf per accepted signature). Stored for off-chain
	// audit / dispute paths; the execution layer does not consult it.
	SignatureRoot [48]byte

	// AccountRoot is the 48-byte Merkle root over the set of
	// (account_id, public_key_ref) tuples the batch references.
	// Lets a verifier prove "this batch's signatures verified against
	// THESE keys, not different ones" without re-walking the registry.
	AccountRoot [48]byte

	// BatchCount is the number of (account, tx) tuples in the batch.
	// Bound into the digest so a downstream auditor can refuse a
	// commitment whose claimed count contradicts the leaf count of
	// BatchRoot's preimage.
	BatchCount uint32
}

// SigningDigest returns the 48-byte commitment over every field. No
// signature is attached at this layer — the digest is the public-input
// commitment the Z-Chain STARK proves over.
func (o *OpCommitTxAuthBatch) SigningDigest() [48]byte {
	parts := [][]byte{
		[]byte("Lux/OpCommitTxAuthBatch/v1"),
		u64BE(o.Epoch),
		o.BatchRoot[:],
		o.SignatureRoot[:],
		o.AccountRoot[:],
		u32BE(o.BatchCount),
	}
	return tupleHash48(parts, "LUX_OP_COMMIT_TX_AUTH_BATCH_V1")
}

// Verify runs structural checks. Returns nil iff every root is non-zero
// and BatchCount > 0. Signature verification of the batch's individual
// envelopes happens upstream inside the STARK proof; this op's
// admissibility is "every commitment is structurally well-formed."
func (o *OpCommitTxAuthBatch) Verify(profile *config.ChainSecurityProfile) error {
	if o == nil {
		return ErrOpNil
	}
	if profile == nil {
		return ErrRegistryNilProfile
	}
	if o.Epoch == 0 {
		return ErrOpBatchZeroEpoch
	}
	if isZero48(o.BatchRoot) {
		return ErrOpBatchZeroRoot
	}
	if isZero48(o.SignatureRoot) {
		return ErrOpBatchZeroRoot
	}
	if isZero48(o.AccountRoot) {
		return ErrOpBatchZeroRoot
	}
	if o.BatchCount == 0 {
		return ErrOpBatchZeroCount
	}
	return nil
}

// =============================================================================
// OpCommitPermitBatch — same shape, different domain
// =============================================================================

// OpCommitPermitBatch publishes the roots of an accepted batch of
// EIP-2612-style permit envelopes the rollup has verified. Same shape
// as OpCommitTxAuthBatch but distinct customization so the two batch
// roots cannot collide on the wire.
type OpCommitPermitBatch struct {
	Epoch         uint64
	PermitRoot    [48]byte
	SignatureRoot [48]byte
	OwnerRoot     [48]byte
	BatchCount    uint32
}

// SigningDigest returns the 48-byte commitment over every field.
func (o *OpCommitPermitBatch) SigningDigest() [48]byte {
	parts := [][]byte{
		[]byte("Lux/OpCommitPermitBatch/v1"),
		u64BE(o.Epoch),
		o.PermitRoot[:],
		o.SignatureRoot[:],
		o.OwnerRoot[:],
		u32BE(o.BatchCount),
	}
	return tupleHash48(parts, "LUX_OP_COMMIT_PERMIT_BATCH_V1")
}

// Verify runs structural checks. Same shape as OpCommitTxAuthBatch.Verify.
func (o *OpCommitPermitBatch) Verify(profile *config.ChainSecurityProfile) error {
	if o == nil {
		return ErrOpNil
	}
	if profile == nil {
		return ErrRegistryNilProfile
	}
	if o.Epoch == 0 {
		return ErrOpBatchZeroEpoch
	}
	if isZero48(o.PermitRoot) {
		return ErrOpBatchZeroRoot
	}
	if isZero48(o.SignatureRoot) {
		return ErrOpBatchZeroRoot
	}
	if isZero48(o.OwnerRoot) {
		return ErrOpBatchZeroRoot
	}
	if o.BatchCount == 0 {
		return ErrOpBatchZeroCount
	}
	return nil
}

// =============================================================================
// ML-DSA signature dispatch
// =============================================================================

// verifyMLDSA dispatches a signature check across the admissible
// wallet-scheme set. Refuses anything that is not ML-DSA-65 today; the
// dispatch grows when ML-DSA-87 lands on a profile.
func verifyMLDSA(scheme config.WalletSchemeID, compactPubkey, msg, sig []byte) error {
	switch scheme {
	case config.WalletSchemeMLDSA65:
		if len(compactPubkey) != mldsa65.PublicKeySize {
			return ErrOpPubkeyLen
		}
		if len(sig) == 0 {
			return ErrOpSigEmpty
		}
		var pk mldsa65.PublicKey
		if err := pk.UnmarshalBinary(compactPubkey); err != nil {
			return fmt.Errorf("%w: %v", ErrOpPubkeyDecode, err)
		}
		if !mldsa65.Verify(&pk, msg, txAuthRegistryCtx, sig) {
			return ErrOpSigInvalid
		}
		return nil
	default:
		return ErrOpSchemeUnsupported
	}
}

// isZero48 reports whether every byte of b is zero.
func isZero48(b [48]byte) bool {
	for _, x := range b {
		if x != 0 {
			return false
		}
	}
	return true
}

// =============================================================================
// Typed errors
// =============================================================================

var (
	// ErrOpNil — the op receiver was nil.
	ErrOpNil = errors.New("registry: nil op")

	// ErrOpRecordInvalid — NewRecord.Validate returned an error.
	ErrOpRecordInvalid = errors.New("registry: op record validate failed")

	// ErrOpRecordNotActive — submit-time record status was not Active.
	ErrOpRecordNotActive = errors.New("registry: op record status is not Active")

	// ErrOpSelfSigInvalid — proof-of-possession signature did not verify.
	ErrOpSelfSigInvalid = errors.New("registry: SelfSig does not verify")

	// ErrOpMissingLookup — caller did not inject a required resolver.
	ErrOpMissingLookup = errors.New("registry: required lookup function missing")

	// ErrOpZeroOldAccount — OpRotateKey.OldAccountID was zero.
	ErrOpZeroOldAccount = errors.New("registry: OldAccountID is zero")

	// ErrOpZeroAccount — op.AccountID was zero where a real value is required.
	ErrOpZeroAccount = errors.New("registry: AccountID is zero")

	// ErrOpRotateSameAccount — old and new AccountIDs were identical.
	ErrOpRotateSameAccount = errors.New("registry: rotate to the same AccountID")

	// ErrOpOldKeyLookup — masterLookup returned an error or empty pubkey.
	ErrOpOldKeyLookup = errors.New("registry: master-key lookup failed")

	// ErrOpOldKeyNotActive — looked-up old record was not in Active status.
	ErrOpOldKeyNotActive = errors.New("registry: old key is not Active")

	// ErrOpOldSigInvalid — OldSig did not verify under the old record's key.
	ErrOpOldSigInvalid = errors.New("registry: OldSig does not verify")

	// ErrOpNewSigInvalid — NewSig did not verify under the new record's key.
	ErrOpNewSigInvalid = errors.New("registry: NewSig does not verify")

	// ErrOpUnknownRevokeReason — Reason was not one of the defined entries.
	ErrOpUnknownRevokeReason = errors.New("registry: unknown RevokeReason")

	// ErrOpRecoveryLookup — recoveryLookup returned an error.
	ErrOpRecoveryLookup = errors.New("registry: recovery-key lookup failed")

	// ErrOpRecoveryNotPinned — compromise reason was issued but no
	// recovery key is registered for the account.
	ErrOpRecoveryNotPinned = errors.New("registry: no recovery key pinned for account")

	// ErrOpRevokeSigInvalid — revoke signature did not verify.
	ErrOpRevokeSigInvalid = errors.New("registry: revoke signature does not verify")

	// ErrOpZeroSessionKey — SessionPubkey was empty.
	ErrOpZeroSessionKey = errors.New("registry: SessionPubkey is empty")

	// ErrOpSessionNoExpiry — ValidUntil was zero (must be > 0).
	ErrOpSessionNoExpiry = errors.New("registry: session ValidUntil is zero")

	// ErrOpSessionAlreadyExpired — ValidUntil <= currentHeight at submit.
	ErrOpSessionAlreadyExpired = errors.New("registry: session already expired at submit")

	// ErrOpMasterSigInvalid — MasterSig did not verify.
	ErrOpMasterSigInvalid = errors.New("registry: MasterSig does not verify")

	// ErrOpBatchZeroEpoch — batch op Epoch was zero.
	ErrOpBatchZeroEpoch = errors.New("registry: batch Epoch is zero")

	// ErrOpBatchZeroRoot — batch op had a zero-valued root field.
	ErrOpBatchZeroRoot = errors.New("registry: batch root is zero")

	// ErrOpBatchZeroCount — batch op BatchCount was zero.
	ErrOpBatchZeroCount = errors.New("registry: batch count is zero")

	// ErrOpSchemeUnsupported — scheme byte is not implemented in the
	// signature dispatch. Production strict-PQ pins ML-DSA-65.
	ErrOpSchemeUnsupported = errors.New("registry: signature scheme not supported")

	// ErrOpPubkeyLen — compact pubkey did not match the scheme width.
	ErrOpPubkeyLen = errors.New("registry: compact pubkey length mismatch")

	// ErrOpPubkeyDecode — compact pubkey failed to decode under the scheme.
	ErrOpPubkeyDecode = errors.New("registry: compact pubkey decode failed")

	// ErrOpSigEmpty — signature byte slice was empty.
	ErrOpSigEmpty = errors.New("registry: signature is empty")

	// ErrOpSigInvalid — signature did not verify under (pubkey, msg, ctx).
	ErrOpSigInvalid = errors.New("registry: signature does not verify")
)
