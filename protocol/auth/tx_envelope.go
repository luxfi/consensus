// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
	wirezap "github.com/luxfi/consensus/pkg/wire/zap"
)

// tx_envelope.go — the canonical TxAuthEnvelope plus its signing-digest,
// wire codec, and profile-gated verifier.
//
// One envelope, one verifier. Every transaction on a strict-PQ chain
// rides this struct. The signature covers TupleHash256 over every
// security-relevant field (customization "LUX_TX_AUTH_V1"), so any
// post-sign mutation of any bound field breaks signature verification
// at the ML-DSA layer — not just at the envelope-equality layer.
//
// Public surface:
//
//   TxAuthEnvelope.SigningDigest()    canonical 48-byte digest the
//                                     wallet signs over.
//   Marshal / UnmarshalTxAuthEnvelope canonical big-endian wire codec.
//                                     Unmarshal refuses zero enums on
//                                     security-relevant fields.
//   VerifyTxAuthEnvelope              profile-gated entry point. Refuses
//                                     scheme drift, hash-suite drift,
//                                     expired envelopes, and any
//                                     non-PQ wallet scheme under a
//                                     strict-PQ profile. Dispatches the
//                                     signature check through an
//                                     injected verifier so the auth
//                                     package stays below pulsar / coreth
//                                     in the dependency graph.
//
// No panic() anywhere in this file. Every refusal returns a typed error
// from the var block at the bottom of the file.

// txAuthSigningCustomization is the SP 800-185 cSHAKE256 customization
// tag for the TxAuthEnvelope signing transcript. The tag is the schema
// identity; bumping it invalidates every prior signature.
const txAuthSigningCustomization = "LUX_TX_AUTH_V1"

// txAuthProtocolTag is the in-band redundant protocol tag bound as the
// first TupleHash part. Defence-in-depth so a cross-customization-
// collision attacker also has to forge the leading TupleHash part.
const txAuthProtocolTag = "Lux/TxAuth/v1"

// TxAuthEnvelope is the canonical PQ transaction authorization envelope.
// Every byte that determines what the transaction asks the chain to do
// is bound into SigningDigest() via TupleHash256 — a flipped byte in
// any of these fields invalidates the wallet's signature.
//
// All 48-byte fields are PQ AccountID width (SHAKE256-384). All 32-byte
// fields are commitment / root width (SHA3-256). The 16-byte width is
// not used at this layer.
//
// Field roles:
//
//	Version         envelope-format version; bumped on incompatible
//	                layout changes. Current value: 1.
//	ProfileID       ChainSecurityProfile this envelope was produced
//	                under (config.ProfileStrictPQ / Permissive / FIPS).
//	ChainID         L1/L2 chain identifier (cross-chain replay seal).
//	NetworkID       mainnet/testnet/devnet (cross-network replay seal).
//	AccountID       48-byte PQ AccountID of the originator. Equals
//	                DeriveAccountID(ProfileID, ChainID, WalletSchemeID, pubkey).
//	Nonce           per-account monotonic counter (intra-chain replay seal).
//	ExpiryHeight    block height after which the envelope is rejected.
//	                Set to math.MaxUint64 for "never expire" (discouraged).
//	WalletSchemeID  PQ signature scheme used by the wallet. Pinned by
//	                ChainSecurityProfile under strict-PQ; the verifier
//	                refuses any mismatch.
//	HashSuiteID     hash family the transcript binds (orthogonal to
//	                the cSHAKE256 kernel that produces SigningDigest).
//	                Bound as DATA into the digest so a flipped byte
//	                breaks signature verification.
//	FeePayer        AccountID paying fees. Equals AccountID for the
//	                common case; differs for sponsored / meta-tx flows.
//	GasLimit        per-envelope gas cap (caller-side; chain enforces).
//	MaxFee          per-gas maximum fee in chain's native unit (32-byte
//	                big-endian unsigned integer; allows up to 2^256-1).
//	CallRoot        Merkle root over the (target, calldata, value)
//	                tuple sequence. The verifier checks SignatureDigest
//	                over CallRoot, not the calls themselves — call
//	                contents are committed in CallRoot's preimage.
//	AccessListRoot  Merkle root over the access-list set (EIP-2930-style
//	                warm-storage hints).
//	ZIdentityRoot   Merkle root over the originator's Z-Chain identity
//	                commitment (binds identity proofs to this tx).
//	AccountStateRoot 32-byte commitment to the originator's account
//	                state at the time of signing. The verifier rejects
//	                any envelope whose stored AccountStateRoot does not
//	                match the chain's current AccountStateRoot for this
//	                account — closes the "stale state replay" class.
//	PublicKeyRef    32-byte hash reference to the wallet public key
//	                (e.g. SHA3-256 of the ML-DSA-65 pubkey). The
//	                verifier resolves it through AccountStateLookupFn
//	                to obtain the full pubkey bytes for signature
//	                verification.
//	Signature       wallet signature over SigningDigest under
//	                WalletSchemeID. Variable-length.
type TxAuthEnvelope struct {
	Version uint16

	ProfileID config.ProfileID
	ChainID   uint32
	NetworkID uint32

	AccountID [48]byte
	Nonce     uint64

	ExpiryHeight uint64

	WalletSchemeID WalletSchemeID
	HashSuiteID    config.HashSuiteID

	FeePayer [48]byte
	GasLimit uint64
	MaxFee   [32]byte

	CallRoot         [32]byte
	AccessListRoot   [32]byte
	ZIdentityRoot    [32]byte
	AccountStateRoot [32]byte

	PublicKeyRef [32]byte

	Signature []byte
}

// SigningDigest returns the 48-byte digest the wallet signs over for
// this envelope. Bound via TupleHash256 (customization "LUX_TX_AUTH_V1")
// so any byte flip on any security-relevant field breaks signature
// verification — not just envelope equality.
//
// The hash family of the digest is fixed at cSHAKE256 (SP 800-185),
// independent of HashSuiteID. HashSuiteID is bound as DATA into the
// transcript, not as the hash family of the transcript itself.
//
// Signature is intentionally NOT bound (the digest is what the signature
// is computed over). All other fields are bound exactly once.
func (e *TxAuthEnvelope) SigningDigest() [48]byte {
	parts := [][]byte{
		[]byte(txAuthProtocolTag),
		u16BE(e.Version),
		{byte(e.ProfileID)},
		u32BE(e.ChainID),
		u32BE(e.NetworkID),
		e.AccountID[:],
		u64BE(e.Nonce),
		u64BE(e.ExpiryHeight),
		{byte(e.WalletSchemeID)},
		{byte(e.HashSuiteID)},
		e.FeePayer[:],
		u64BE(e.GasLimit),
		e.MaxFee[:],
		e.CallRoot[:],
		e.AccessListRoot[:],
		e.ZIdentityRoot[:],
		e.AccountStateRoot[:],
		e.PublicKeyRef[:],
	}
	return tupleHash48(parts, txAuthSigningCustomization)
}

// =============================================================================
// Wire codec — LP-182 schema 0x0A ZAP
// =============================================================================
//
// The wire layer (pkg/wire/zap.TxAuthEnvelope) is the LP-182 canonical
// wire form. This file delegates Marshal / UnmarshalTxAuthEnvelope to
// that package; the Go value-typed TxAuthEnvelope struct above remains
// the in-process representation for VerifyTxAuthEnvelope rule logic and
// SigningDigest construction.

// Marshal returns the LP-182 schema 0x0A ZAP wire-bytes for this
// envelope. Returns ErrTxAuthNilEnvelope on a nil receiver.
func (e *TxAuthEnvelope) Marshal() ([]byte, error) {
	if e == nil {
		return nil, ErrTxAuthNilEnvelope
	}
	return wirezap.NewTxAuthEnvelope(wirezap.TxAuthEnvelopeFields{
		Version:          e.Version,
		ProfileID:        uint8(e.ProfileID),
		ChainID:          e.ChainID,
		NetworkID:        e.NetworkID,
		AccountID:        e.AccountID,
		Nonce:            e.Nonce,
		ExpiryHeight:     e.ExpiryHeight,
		WalletSchemeID:   uint8(e.WalletSchemeID),
		HashSuiteID:      uint8(e.HashSuiteID),
		FeePayer:         e.FeePayer,
		GasLimit:         e.GasLimit,
		MaxFee:           e.MaxFee,
		CallRoot:         e.CallRoot,
		AccessListRoot:   e.AccessListRoot,
		ZIdentityRoot:    e.ZIdentityRoot,
		AccountStateRoot: e.AccountStateRoot,
		PublicKeyRef:     e.PublicKeyRef,
		Signature:        e.Signature,
	}).Bytes(), nil
}

// UnmarshalTxAuthEnvelope wraps LP-182 schema 0x0A ZAP wire-bytes and
// projects them back into a Go value-typed TxAuthEnvelope.
//
// Refuses zero values on security-relevant enum fields (ProfileID,
// WalletSchemeID, HashSuiteID): a zero-init envelope is never a
// legitimate wire payload. Removing this degree of freedom shrinks the
// fuzz surface for downstream verifiers.
//
// Refuses trailing bytes beyond the declared ZAP message size. ZAP's
// Parse silently truncates to the declared header size; auth at the
// envelope level demands exact-size input to preserve the historical
// hand-rolled codec's no-trailing-bytes invariant.
func UnmarshalTxAuthEnvelope(data []byte) (*TxAuthEnvelope, error) {
	wrap, err := wirezap.WrapTxAuthEnvelope(data)
	if err != nil {
		return nil, ErrTxAuthTruncated
	}
	if len(wrap.Bytes()) != len(data) {
		return nil, fmt.Errorf("%w: %d trailing bytes", ErrTxAuthTrailingBytes,
			len(data)-len(wrap.Bytes()))
	}
	e := &TxAuthEnvelope{
		Version:          wrap.Version(),
		ProfileID:        config.ProfileID(wrap.ProfileID()),
		ChainID:          wrap.ChainID(),
		NetworkID:        wrap.NetworkID(),
		AccountID:        wrap.AccountID(),
		Nonce:            wrap.Nonce(),
		ExpiryHeight:     wrap.ExpiryHeight(),
		WalletSchemeID:   WalletSchemeID(wrap.WalletSchemeID()),
		HashSuiteID:      config.HashSuiteID(wrap.HashSuiteID()),
		FeePayer:         wrap.FeePayer(),
		GasLimit:         wrap.GasLimit(),
		MaxFee:           wrap.MaxFee(),
		CallRoot:         wrap.CallRoot(),
		AccessListRoot:   wrap.AccessListRoot(),
		ZIdentityRoot:    wrap.ZIdentityRoot(),
		AccountStateRoot: wrap.AccountStateRoot(),
		PublicKeyRef:     wrap.PublicKeyRef(),
	}
	if sig := wrap.Signature(); len(sig) > 0 {
		e.Signature = append([]byte(nil), sig...)
	}

	// Refuse zero values on security-relevant enums. Order matches the
	// field declaration so error messages point at the first violation
	// a reader sees on the wire.
	switch {
	case e.ProfileID == config.ProfileNone:
		return nil, fmt.Errorf("%w: ProfileID", ErrTxAuthZeroEnum)
	case e.WalletSchemeID == WalletSchemeNone:
		return nil, fmt.Errorf("%w: WalletSchemeID", ErrTxAuthZeroEnum)
	case e.HashSuiteID == config.HashSuiteNone:
		return nil, fmt.Errorf("%w: HashSuiteID", ErrTxAuthZeroEnum)
	}

	return e, nil
}

// =============================================================================
// Verifier
// =============================================================================

// AccountStateLookupFn is the chain-side hook that the auth package
// calls to resolve a wallet's stored public key and current account
// state root. The auth package stays orthogonal to any specific
// account-state implementation by going through this function pointer.
//
// Returns:
//
//	pubkey                  the wallet's full public key bytes, in the
//	                        format the WalletSchemeID's verifier
//	                        expects (e.g. ML-DSA-65 encoded pubkey).
//	currentAccountStateRoot the chain's current 32-byte AccountStateRoot
//	                        for this account. The verifier compares it
//	                        against env.AccountStateRoot — mismatch is
//	                        a stale-state-replay refusal.
//
// On lookup failure (unknown account, db error) the function returns an
// error and the verifier surfaces it as ErrTxAuthAccountStateLookup.
type AccountStateLookupFn func(accountID [48]byte, publicKeyRef [32]byte) (pubkey []byte, currentAccountStateRoot [32]byte, err error)

// SignatureVerifierFn is the chain-side hook that the auth package calls
// to verify a wallet signature under a specific WalletSchemeID. The
// auth package does NOT bind to a concrete signature library — it
// would force a cycle (auth → pulsar → consensus → auth). Instead, the
// caller (typically coreth) injects a function that dispatches across
// the scheme block:
//
//	ML-DSA-44 / 65 / 87  -> github.com/luxfi/pulsar mldsa.Verify
//	SLH-DSA-128f / ...   -> golang.org/x/crypto/sphincs.Verify
//	ECDSA legacy         -> rejected before this function fires when
//	                        profile is strict-PQ; otherwise dispatches
//	                        to a legacy verifier.
//
// Returns (true, nil) on a valid signature; (false, nil) on a malformed
// or non-verifying signature; (false, err) on a transient error (e.g.
// a verifier-side allocator failure).
type SignatureVerifierFn func(scheme WalletSchemeID, pubkey, msgDigest, signature []byte) (bool, error)

// CurrentHeightFn returns the chain's current finalized block height.
// Used by VerifyTxAuthEnvelope to enforce ExpiryHeight. Returning
// zero means "do not enforce expiry" (testing / replay tooling); a
// production caller MUST inject a real height source.
type CurrentHeightFn func() uint64

// VerifyTxAuthEnvelope is the profile-gated entry point for a transaction
// authorization envelope. Runs the following checks in order:
//
//  1. Structural: env non-nil, profile non-nil, Version supported.
//  2. Profile/scheme: env.WalletSchemeID is admissible under profile.
//     Strict-PQ profiles refuse any IsLegacyClassical / non-PostQuantum
//     wallet scheme.
//  3. Profile/suite: env.HashSuiteID matches profile.HashSuiteID.
//  4. Profile/id: env.ProfileID matches profile.ProfileID.
//  5. Expiry: env.ExpiryHeight > currentHeight (when currentHeight > 0).
//  6. AccountID derivation: DeriveAccountID(env.ProfileID, env.ChainID,
//     env.WalletSchemeID, pubkey) MUST equal env.AccountID. Closes the
//     "wrong key for the account" attack class deterministically.
//  7. AccountStateRoot: env.AccountStateRoot equals the chain's current
//     AccountStateRoot for this account.
//  8. Signature: SignatureVerifierFn returns true on
//     (env.WalletSchemeID, pubkey, env.SigningDigest(), env.Signature).
//
// On any failure returns a typed error from the ErrTxAuth* set. No
// panic() on any path.
func VerifyTxAuthEnvelope(
	profile *config.ChainSecurityProfile,
	env *TxAuthEnvelope,
	accountStateLookup AccountStateLookupFn,
	sigVerifier SignatureVerifierFn,
	currentHeight CurrentHeightFn,
) error {
	// 1. Structural.
	if env == nil {
		return ErrTxAuthNilEnvelope
	}
	if profile == nil {
		return ErrTxAuthInvalidProfile
	}
	if accountStateLookup == nil {
		return ErrTxAuthMissingAccountLookup
	}
	if sigVerifier == nil {
		return ErrTxAuthMissingSigVerifier
	}
	if env.Version == 0 {
		return fmt.Errorf("%w: Version=0", ErrTxAuthVersionUnsupported)
	}
	if env.Version > TxAuthCurrentVersion {
		return fmt.Errorf("%w: Version=%d > current=%d",
			ErrTxAuthVersionUnsupported, env.Version, TxAuthCurrentVersion)
	}

	// 2. Profile/scheme. Strict-PQ profile must enforce IsPostQuantum.
	if !env.WalletSchemeID.IsPostQuantum() && profile.ForbidClassicalSNARKs {
		// ForbidClassicalSNARKs subsumes "no classical primitives" on
		// strict-PQ profiles; the wallet-scheme check piggy-backs on
		// it so we don't add a new profile knob just for wallets.
		return fmt.Errorf("%w: %s under profile %s",
			ErrTxAuthWalletSchemeNotAllowed,
			env.WalletSchemeID.String(), profile.ProfileName)
	}
	if env.WalletSchemeID == WalletSchemeNone {
		return fmt.Errorf("%w: WalletSchemeNone", ErrTxAuthWalletSchemeNotAllowed)
	}

	// 3. Profile/suite. The envelope's HashSuiteID MUST match the
	// profile's HashSuiteID byte-for-byte. Closes the "BLAKE3 envelope
	// against a SHA3 profile" class deterministically.
	if env.HashSuiteID != profile.HashSuiteID {
		return fmt.Errorf("%w: env=%s profile=%s",
			ErrTxAuthHashSuiteMismatch,
			env.HashSuiteID.String(), profile.HashSuiteID.String())
	}

	// 4. Profile/id. ProfileID byte must match. Refuses profile drift
	// between signing time and verification time.
	if uint32(env.ProfileID) != profile.ProfileID {
		return fmt.Errorf("%w: env=0x%02x profile=0x%08x",
			ErrTxAuthInvalidProfile, uint8(env.ProfileID), profile.ProfileID)
	}

	// 5. Expiry. currentHeight==nil or returning 0 means "do not
	// enforce expiry" — testing / replay tooling. Production callers
	// MUST inject a real height source.
	if currentHeight != nil {
		h := currentHeight()
		if h > 0 && env.ExpiryHeight <= h {
			return fmt.Errorf("%w: expiry=%d current=%d",
				ErrTxAuthExpired, env.ExpiryHeight, h)
		}
	}

	// 6. Resolve pubkey + current account-state root via injected lookup.
	pubkey, currentAccountStateRoot, err := accountStateLookup(env.AccountID, env.PublicKeyRef)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTxAuthAccountStateLookup, err)
	}
	if len(pubkey) == 0 {
		return fmt.Errorf("%w: empty pubkey", ErrTxAuthAccountStateLookup)
	}

	// 7. AccountID derivation. Closes the "wrong key for the account"
	// class: even if a verifier somehow got handed a foreign pubkey, the
	// derived AccountID would not match env.AccountID and the envelope
	// would be rejected. The derivation binds the profile byte first so
	// the same wallet cannot pose as the same account across two
	// profiles (cross-profile replay class).
	derivedAccountID := DeriveAccountID(uint32(env.ProfileID), env.ChainID, env.WalletSchemeID, pubkey)
	if derivedAccountID != env.AccountID {
		return fmt.Errorf("%w: derived=%x env=%x",
			ErrTxAuthAccountIDMismatch,
			derivedAccountID[:8], env.AccountID[:8])
	}

	// 8. AccountStateRoot match. The chain's current state root for
	// this account MUST equal the root the wallet signed over. Closes
	// the "stale state replay" attack class deterministically: a sender
	// who signs an envelope against an old state and rebroadcasts it
	// against a new state is rejected at this check.
	if env.AccountStateRoot != currentAccountStateRoot {
		return fmt.Errorf("%w: env=%x chain=%x",
			ErrTxAuthAccountStateRoot,
			env.AccountStateRoot[:8], currentAccountStateRoot[:8])
	}

	// 9. Signature verify. The envelope's SigningDigest is what the
	// wallet signed over. Dispatch through the injected verifier so the
	// auth package has no upward dependency on pulsar / coreth.
	if len(env.Signature) == 0 {
		return ErrTxAuthSignatureInvalid
	}
	digest := env.SigningDigest()
	ok, vErr := sigVerifier(env.WalletSchemeID, pubkey, digest[:], env.Signature)
	if vErr != nil {
		return fmt.Errorf("%w: %v", ErrTxAuthSignatureInvalid, vErr)
	}
	if !ok {
		return ErrTxAuthSignatureInvalid
	}

	return nil
}

// TxAuthCurrentVersion is the current TxAuthEnvelope wire-format version.
// Bumped only on incompatible layout changes; an envelope whose Version
// exceeds this value is refused by VerifyTxAuthEnvelope.
const TxAuthCurrentVersion uint16 = 1

// =============================================================================
// Typed errors
// =============================================================================

var (
	// ErrTxAuthNilEnvelope — receiver is nil. Codec + verifier surface.
	ErrTxAuthNilEnvelope = errors.New("txauth: nil envelope")

	// ErrTxAuthInvalidProfile — profile is nil or env.ProfileID does
	// not match profile.ProfileID.
	ErrTxAuthInvalidProfile = errors.New("txauth: invalid or mismatched profile")

	// ErrTxAuthVersionUnsupported — env.Version is zero or exceeds
	// TxAuthCurrentVersion.
	ErrTxAuthVersionUnsupported = errors.New("txauth: envelope Version unsupported")

	// ErrTxAuthWalletSchemeNotAllowed — env.WalletSchemeID is either
	// WalletSchemeNone or a non-PostQuantum scheme on a strict-PQ
	// profile.
	ErrTxAuthWalletSchemeNotAllowed = errors.New("txauth: WalletSchemeID not allowed under profile")

	// ErrTxAuthHashSuiteMismatch — env.HashSuiteID does not match the
	// profile's pinned HashSuiteID.
	ErrTxAuthHashSuiteMismatch = errors.New("txauth: HashSuiteID does not match profile")

	// ErrTxAuthZeroEnum — the codec saw a zero value on a
	// security-relevant enum (ProfileID, WalletSchemeID, HashSuiteID)
	// where a real value is required.
	ErrTxAuthZeroEnum = errors.New("txauth: zero-value security-relevant enum")

	// ErrTxAuthExpired — env.ExpiryHeight is at or below the chain's
	// current height.
	ErrTxAuthExpired = errors.New("txauth: envelope expired")

	// ErrTxAuthAccountStateLookup — the injected AccountStateLookupFn
	// returned an error or empty pubkey.
	ErrTxAuthAccountStateLookup = errors.New("txauth: account-state lookup failed")

	// ErrTxAuthAccountIDMismatch — DeriveAccountID(...) did not match
	// env.AccountID. Wrong key for the account.
	ErrTxAuthAccountIDMismatch = errors.New("txauth: derived AccountID does not match envelope")

	// ErrTxAuthAccountStateRoot — env.AccountStateRoot does not match
	// the chain's current AccountStateRoot for this account.
	ErrTxAuthAccountStateRoot = errors.New("txauth: AccountStateRoot stale")

	// ErrTxAuthSignatureInvalid — the wallet signature did not verify,
	// or no signature bytes were provided.
	ErrTxAuthSignatureInvalid = errors.New("txauth: signature does not verify")

	// ErrTxAuthMissingAccountLookup — caller did not inject an
	// AccountStateLookupFn.
	ErrTxAuthMissingAccountLookup = errors.New("txauth: AccountStateLookupFn is required")

	// ErrTxAuthMissingSigVerifier — caller did not inject a
	// SignatureVerifierFn.
	ErrTxAuthMissingSigVerifier = errors.New("txauth: SignatureVerifierFn is required")

	// ErrTxAuthTruncated — codec ran out of input before reading every
	// required field.
	ErrTxAuthTruncated = errors.New("txauth: input truncated")

	// ErrTxAuthTrailingBytes — codec finished reading every required
	// field but data remained in the buffer.
	ErrTxAuthTrailingBytes = errors.New("txauth: trailing bytes after envelope decode")

	// ErrTxAuthSignatureTooLong — declared signature_len exceeds the
	// remaining buffer.
	ErrTxAuthSignatureTooLong = errors.New("txauth: declared signature_len exceeds input")
)

