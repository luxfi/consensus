// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package auth — canonical PQ authorization surface for the user-facing
// layers (wallets, contracts, EVM precompiles).
//
// This package is intentionally orthogonal to the validator-facing
// consensus envelope: the same chain pins one ChainSecurityProfile and
// uses it to gate BOTH a validator-produced QBlock and a user-produced
// TxAuthEnvelope. The profile is the single safety surface; auth
// envelopes consult it the same way zchain proof envelopes do.
//
// Five PQ-class enums live here:
//
//   WalletSchemeID    — what a user's EOA signs with (raw ML-DSA family
//                       at production; ECDSA marker reserved for the
//                       legacy migration window only)
//   TxSchemeID        — transaction envelope semantics (legacy single-sig
//                       vs PQ-bound TxAuthEnvelope)
//   ContractAuthID    — per-contract action authorization scheme (admin
//                       / upgrade / pause / permit)
//   KeyExchangeID     — KEM used for ephemeral channel keys (ML-KEM
//                       family; X25519 marker reserved as legacy)
//   RecoverySchemeID  — social / threshold account recovery scheme
//
// These enums are scheduled to land in config/ alongside the canonical
// ChainSecurityProfile expansion. Until that lands, they live here as a
// TEMPORARY local type. The merge will reconcile by:
//
//   1. Moving the enum declarations to config/.
//   2. Replacing this file with a re-export `type WalletSchemeID =
//      config.WalletSchemeID` (etc.) until callers update their imports.
//   3. Deleting this file once callers point at config.
//
// Numbering rule: each enum stays inside its own byte block so the wire
// format is unambiguous even if every five enums are flattened into a
// single transcript stream by a downstream digest function. New entries
// claim the next free integer in their block; never reuse a retired ID.

package auth

import "fmt"

// WalletSchemeID is the wire byte that identifies the signature scheme a
// user's externally-owned account (EOA) signs transactions with.
//
// Distinct from config.SigSchemeID (validator threshold/identity) and
// config.IdentitySchemeID (validator identity) because user wallets are
// single-party and the wire format makes that semantic visible in one
// byte. A receiver knows immediately whether a TxAuthEnvelope.Signature
// is single-party ML-DSA or a future threshold-wallet variant without
// having to dispatch on a separate envelope type.
//
// Numbering:
//
//	0x00       — None / unspecified (rejected by every locked profile)
//	0x10..0x1F — Legacy classical (forbidden in strict-PQ; reserved for
//	             the secp256k1 legacy migration window only)
//	0x40..0x4F — Raw FIPS 204 ML-DSA single-party (production wallets):
//	             0x41 = ML-DSA-44 (devnet only, NIST PQ Cat 2)
//	             0x42 = ML-DSA-65 (production default, NIST PQ Cat 3)
//	             0x43 = ML-DSA-87 (high-value wallets, NIST PQ Cat 5)
//	0x60..0x6F — Hash-based (SLH-DSA / SPHINCS+ family):
//	             0x61 = SLH-DSA-128f
//	             0x62 = SLH-DSA-192f
//	             0x63 = SLH-DSA-256f
//	0xE0..0xEF — Hybrid (PQ-primary + classical witness; reserved for
//	             migration. NOT used at production strict-PQ.)
type WalletSchemeID uint8

const (
	WalletSchemeNone WalletSchemeID = 0x00

	// Legacy classical — explicit forbidden marker so audit tooling can
	// name a misconfiguration precisely. Strict-PQ profiles refuse this
	// at VerifyTxAuthEnvelope.
	WalletSchemeECDSASecp256k1Legacy WalletSchemeID = 0x10

	// Production PQ wallets.
	WalletSchemeMLDSA44 WalletSchemeID = 0x41
	WalletSchemeMLDSA65 WalletSchemeID = 0x42 // production default
	WalletSchemeMLDSA87 WalletSchemeID = 0x43

	// Hash-based PQ wallets — FIPS 205 stateless SLH-DSA family.
	WalletSchemeSLHDSA128f WalletSchemeID = 0x61
	WalletSchemeSLHDSA192f WalletSchemeID = 0x62
	WalletSchemeSLHDSA256f WalletSchemeID = 0x63
)

// String returns the canonical wire name.
func (w WalletSchemeID) String() string {
	switch w {
	case WalletSchemeNone:
		return "none"
	case WalletSchemeECDSASecp256k1Legacy:
		return "ecdsa-secp256k1-legacy"
	case WalletSchemeMLDSA44:
		return "ml-dsa-44"
	case WalletSchemeMLDSA65:
		return "ml-dsa-65"
	case WalletSchemeMLDSA87:
		return "ml-dsa-87"
	case WalletSchemeSLHDSA128f:
		return "slh-dsa-128f"
	case WalletSchemeSLHDSA192f:
		return "slh-dsa-192f"
	case WalletSchemeSLHDSA256f:
		return "slh-dsa-256f"
	default:
		return fmt.Sprintf("wallet-scheme(0x%02x)", uint8(w))
	}
}

// IsRawMLDSA reports whether this wallet scheme uses raw FIPS 204 ML-DSA.
func (w WalletSchemeID) IsRawMLDSA() bool {
	return w == WalletSchemeMLDSA44 ||
		w == WalletSchemeMLDSA65 ||
		w == WalletSchemeMLDSA87
}

// IsSLHDSA reports whether this wallet scheme uses FIPS 205 SLH-DSA.
func (w WalletSchemeID) IsSLHDSA() bool {
	return w == WalletSchemeSLHDSA128f ||
		w == WalletSchemeSLHDSA192f ||
		w == WalletSchemeSLHDSA256f
}

// IsPostQuantum reports whether the scheme is acceptable in strict-PQ
// mode. The ECDSA legacy marker returns false; ML-DSA and SLH-DSA
// families return true; None returns false.
func (w WalletSchemeID) IsPostQuantum() bool {
	return w.IsRawMLDSA() || w.IsSLHDSA()
}

// IsLegacyClassical reports whether this wallet scheme is the explicit
// classical legacy marker. Strict-PQ verifiers MUST refuse these.
func (w WalletSchemeID) IsLegacyClassical() bool {
	return w == WalletSchemeECDSASecp256k1Legacy
}

// TxSchemeID is the wire byte that identifies the transaction envelope
// kind. Pinned at the chain level so a single byte in a locked profile
// tells a verifier whether to expect legacy single-sig or PQ-bound
// TxAuthEnvelope semantics.
//
// Numbering:
//
//	0x00 — None / unspecified (rejected by every locked profile)
//	0x10 — TxSchemeLegacyECDSA (forbidden in strict-PQ; migration only)
//	0x42 — TxSchemePQAuthV1 (production: TxAuthEnvelope under ML-DSA family)
type TxSchemeID uint8

const (
	TxSchemeNone        TxSchemeID = 0x00
	TxSchemeLegacyECDSA TxSchemeID = 0x10
	TxSchemePQAuthV1    TxSchemeID = 0x42
)

// String returns the canonical wire name.
func (t TxSchemeID) String() string {
	switch t {
	case TxSchemeNone:
		return "none"
	case TxSchemeLegacyECDSA:
		return "tx-legacy-ecdsa"
	case TxSchemePQAuthV1:
		return "tx-pq-auth-v1"
	default:
		return fmt.Sprintf("tx-scheme(0x%02x)", uint8(t))
	}
}

// IsPostQuantum reports whether this transaction scheme is admissible
// under a strict-PQ profile.
func (t TxSchemeID) IsPostQuantum() bool {
	return t == TxSchemePQAuthV1
}

// ContractAuthID is the wire byte that identifies the signature scheme a
// contract action (admin call, upgrade, pause, permit) is authorised
// under. Distinct from WalletSchemeID because a contract MAY pin a
// different scheme than its caller's wallet — e.g. governance
// multisigs at ML-DSA-87 (Cat 5) on a chain whose default wallet is
// ML-DSA-65 (Cat 3).
//
// Numbering blocks mirror WalletSchemeID so the same byte pattern means
// the same primitive across user / contract context:
//
//	0x00 — None
//	0x10 — ECDSASecp256k1 legacy (forbidden in strict-PQ)
//	0x41 / 0x42 / 0x43 — ML-DSA-44 / 65 / 87
//	0x61..0x63 — SLH-DSA-128f / 192f / 256f
type ContractAuthID uint8

const (
	ContractAuthNone                 ContractAuthID = 0x00
	ContractAuthECDSASecp256k1Legacy ContractAuthID = 0x10
	ContractAuthMLDSA44              ContractAuthID = 0x41
	ContractAuthMLDSA65              ContractAuthID = 0x42 // production default
	ContractAuthMLDSA87              ContractAuthID = 0x43
	ContractAuthSLHDSA128f           ContractAuthID = 0x61
	ContractAuthSLHDSA192f           ContractAuthID = 0x62
	ContractAuthSLHDSA256f           ContractAuthID = 0x63
)

// String returns the canonical wire name.
func (c ContractAuthID) String() string {
	switch c {
	case ContractAuthNone:
		return "none"
	case ContractAuthECDSASecp256k1Legacy:
		return "ecdsa-secp256k1-legacy"
	case ContractAuthMLDSA44:
		return "ml-dsa-44"
	case ContractAuthMLDSA65:
		return "ml-dsa-65"
	case ContractAuthMLDSA87:
		return "ml-dsa-87"
	case ContractAuthSLHDSA128f:
		return "slh-dsa-128f"
	case ContractAuthSLHDSA192f:
		return "slh-dsa-192f"
	case ContractAuthSLHDSA256f:
		return "slh-dsa-256f"
	default:
		return fmt.Sprintf("contract-auth(0x%02x)", uint8(c))
	}
}

// IsPostQuantum reports whether this contract-auth scheme is admissible
// under strict-PQ. ECDSA legacy returns false.
func (c ContractAuthID) IsPostQuantum() bool {
	switch c {
	case ContractAuthMLDSA44, ContractAuthMLDSA65, ContractAuthMLDSA87,
		ContractAuthSLHDSA128f, ContractAuthSLHDSA192f, ContractAuthSLHDSA256f:
		return true
	}
	return false
}

// IsLegacyClassical reports whether this scheme is the explicit
// classical legacy marker. Strict-PQ refuses these.
func (c ContractAuthID) IsLegacyClassical() bool {
	return c == ContractAuthECDSASecp256k1Legacy
}

// KeyExchangeID is the wire byte that identifies the KEM used for
// ephemeral channel keys (encrypted RPC, account-level encryption,
// channel rekey).
//
// Numbering:
//
//	0x00 — None
//	0x10 — X25519 legacy classical (forbidden in strict-PQ)
//	0x50..0x5F — ML-KEM FIPS 203 family:
//	             0x51 = ML-KEM-512  (NIST Cat 1)
//	             0x52 = ML-KEM-768  (NIST Cat 3, production default)
//	             0x53 = ML-KEM-1024 (NIST Cat 5)
type KeyExchangeID uint8

const (
	KeyExchangeNone         KeyExchangeID = 0x00
	KeyExchangeX25519Legacy KeyExchangeID = 0x10
	KeyExchangeMLKEM512     KeyExchangeID = 0x51
	KeyExchangeMLKEM768     KeyExchangeID = 0x52 // production default
	KeyExchangeMLKEM1024    KeyExchangeID = 0x53
)

// String returns the canonical wire name.
func (k KeyExchangeID) String() string {
	switch k {
	case KeyExchangeNone:
		return "none"
	case KeyExchangeX25519Legacy:
		return "x25519-legacy"
	case KeyExchangeMLKEM512:
		return "ml-kem-512"
	case KeyExchangeMLKEM768:
		return "ml-kem-768"
	case KeyExchangeMLKEM1024:
		return "ml-kem-1024"
	default:
		return fmt.Sprintf("key-exchange(0x%02x)", uint8(k))
	}
}

// IsPostQuantum reports whether this KEM is admissible under strict-PQ.
// X25519 legacy returns false; ML-KEM family returns true.
func (k KeyExchangeID) IsPostQuantum() bool {
	return k == KeyExchangeMLKEM512 ||
		k == KeyExchangeMLKEM768 ||
		k == KeyExchangeMLKEM1024
}

// RecoverySchemeID is the wire byte that identifies the account recovery
// scheme (social recovery, threshold guardians, time-locked PQ recovery).
//
// Numbering:
//
//	0x00 — None
//	0x10 — NoneByDesign (account is non-recoverable; loss is final)
//	0x20 — SocialKofN (k-of-n guardian threshold)
//	0x30 — TimelockPQ (PQ-signed time-locked recovery key)
type RecoverySchemeID uint8

const (
	RecoverySchemeNone         RecoverySchemeID = 0x00
	RecoverySchemeNoneByDesign RecoverySchemeID = 0x10
	RecoverySchemeSocialKofN   RecoverySchemeID = 0x20
	RecoverySchemeTimelockPQ   RecoverySchemeID = 0x30
)

// String returns the canonical wire name.
func (r RecoverySchemeID) String() string {
	switch r {
	case RecoverySchemeNone:
		return "none"
	case RecoverySchemeNoneByDesign:
		return "recovery-none-by-design"
	case RecoverySchemeSocialKofN:
		return "recovery-social-k-of-n"
	case RecoverySchemeTimelockPQ:
		return "recovery-timelock-pq"
	default:
		return fmt.Sprintf("recovery-scheme(0x%02x)", uint8(r))
	}
}

// IsKnown reports whether this RecoverySchemeID is a defined entry. A
// chain MAY set RecoverySchemeNone meaning "no recovery axis pinned by
// this chain"; RecoverySchemeNoneByDesign is the explicit
// non-recoverable choice. Unknown bytes return false so the codec can
// refuse a malformed envelope.
func (r RecoverySchemeID) IsKnown() bool {
	switch r {
	case RecoverySchemeNone,
		RecoverySchemeNoneByDesign,
		RecoverySchemeSocialKofN,
		RecoverySchemeTimelockPQ:
		return true
	}
	return false
}
