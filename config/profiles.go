// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// profiles.go — canonical ChainSecurityProfile registry.
//
// One locked profile per chain. The constants below are the canonical,
// audit-signed-off-on profile values; their ProfileHash is computed at
// package init via MustComputeHash and pinned into genesis.
//
// To register a new profile:
//
//  1. Claim a free ProfileID byte in security_profile.go's numbering table.
//  2. Add a var <Name>Profile here with every field set explicitly (no
//     zero-value defaults — Validate refuses zero-init).
//  3. Add the profile to init()'s "compute and pin hash" list.
//  4. Add a TestProfilesValidate sub-case in security_profile_test.go.
//
// Profiles are sorted by ProfileID. New profiles append at the bottom.

package config

// StrictPQProfile is the canonical Lux mainnet locked profile. NIST-
// aligned PQ across every axis: SHA3-NIST + STARK_FRI_SHA3_PQ identified
// proofs, raw ML-DSA-65 identity, Pulsar-M-65 finality with M-87 reserved
// for high-value roots. Five production STARK backends; no dev backends;
// no classical SNARK wrappers; no fallbacks.
//
// This is the only profile a Lux mainnet validator may accept. Operators
// cannot override; the profile is pinned into genesis by ProfileHash and
// every cert envelope binds the same hash.
//
// ProfileHash is computed in init() — see the bottom of this file.
var StrictPQProfile = ChainSecurityProfile{
	ProfileID:         uint32(ProfileStrictPQ),
	ProfileName:       ProfileNameStrictPQ,
	HashSuiteID:       HashSuiteSHA3NIST,
	IdentitySchemeID:  SigSchemeMLDSA65,
	FinalitySchemeID:  SigSchemePulsarM65,
	HighValueSchemeID: SigSchemePulsarM87,
	ProofPolicyID:     ProofPolicySTARKFRISHA3PQ,
	AllowedProofBackends: []ProofBackendID{
		ProofBackendSP1CompressedSTARK,
		ProofBackendRISC0SuccinctSTARK,
		ProofBackendP3QSTARKFRISHA3,
		ProofBackendStoneCairoSTARK,
		ProofBackendStwoCircleSTARK,
	},
	AllowedProofFormats: []ProofFormatID{
		ProofFormatSTARKFRIBinaryV1,
		ProofFormatSP1BinaryV1,
		ProofFormatRISC0BinaryV1,
		ProofFormatP3QBinaryV1,
		ProofFormatStoneCairoBinaryV1,
		ProofFormatStwoCircleBinaryV1,
	},
	MinSoundnessBits:      128,
	MinHashOutputBits:     384,
	RequireTransparent:    true,
	ForbidPairings:        true,
	ForbidKZG:             true,
	ForbidTrustedSetup:    true,
	ForbidClassicalSNARKs: true,
	ForbidDevProofs:       true,
	ForbidFallbacks:       true,

	// E2E PQ axes. Wallet / tx / contract-auth pinned at ML-DSA-65 (FIPS
	// 204 Cat 3), KEM at ML-KEM-768 with ML-KEM-1024 reserved for
	// high-value, recovery at SLH-DSA-192 (FIPS 205 Cat 3 stateless
	// backstop). Every classical 0x90 primitive is refused on every axis.
	WalletSchemeID:          WalletSchemeMLDSA65,
	TxSchemeID:              TxSchemeMLDSA65,
	ContractAuthID:          ContractAuthMLDSA65,
	KeyExchangeID:           KeyExchangeMLKEM768,
	HighValueKEM:            KeyExchangeMLKEM1024,
	RecoverySchemeID:        RecoverySchemeSLHDSA192,
	ForbidECDSAWallets:      true,
	ForbidECDSAContractAuth: true,
	ForbidBLSContractAuth:   true,
	ForbidClassicalKEM:      true,
	RequireTypedTxAuth:      true,
}

// PermissiveProfile is the testnet/devnet profile. Accepts the strict-PQ
// production backends plus the dev backends (RISC0_RAW_STARK_DEV,
// SP1_CORE_STARK_DEV) so iteration is possible without lying about
// security level. Still refuses classical wrappers and trusted-setup
// systems. MinSoundnessBits relaxed to 96 — never advertised on mainnet.
//
// NOT a Lux strict-PQ profile. Marketing as such is forbidden by
// `ForbidDevProofs == false`.
var PermissiveProfile = ChainSecurityProfile{
	ProfileID:         uint32(ProfilePermissive),
	ProfileName:       ProfileNamePermissive,
	HashSuiteID:       HashSuiteSHA3NIST,
	IdentitySchemeID:  SigSchemeMLDSA65,
	FinalitySchemeID:  SigSchemePulsarM65,
	HighValueSchemeID: SigSchemePulsarM65,
	ProofPolicyID:     ProofPolicySTARKFRISHA3PQ,
	AllowedProofBackends: []ProofBackendID{
		ProofBackendSP1CompressedSTARK,
		ProofBackendRISC0SuccinctSTARK,
		ProofBackendP3QSTARKFRISHA3,
		ProofBackendStoneCairoSTARK,
		ProofBackendStwoCircleSTARK,
	},
	AllowedProofFormats: []ProofFormatID{
		ProofFormatSTARKFRIBinaryV1,
		ProofFormatSP1BinaryV1,
		ProofFormatRISC0BinaryV1,
		ProofFormatP3QBinaryV1,
		ProofFormatStoneCairoBinaryV1,
		ProofFormatStwoCircleBinaryV1,
	},
	// Permissive uses the same primitive floors as strict-PQ; the
	// distinction is the *backend allowlist* (above) and the absence
	// of ForbidDevProofs / ForbidFallbacks below.
	MinSoundnessBits:      128,
	MinHashOutputBits:     384,
	RequireTransparent:    true,
	ForbidPairings:        true,
	ForbidKZG:             true,
	ForbidTrustedSetup:    true,
	ForbidClassicalSNARKs: true,
	ForbidDevProofs:       false, // dev backends OK on testnet/devnet
	ForbidFallbacks:       false, // fallbacks OK on testnet/devnet

	// E2E PQ axes — same lattice scheme defaults as strict-PQ but the
	// Forbid* bits are left false so experimental classical primitives
	// can ride alongside the lattice path on a permissive testnet.
	// RequireTypedTxAuth stays false so legacy testnet clients without
	// the typed-auth byte still round-trip.
	WalletSchemeID:          WalletSchemeMLDSA65,
	TxSchemeID:              TxSchemeMLDSA65,
	ContractAuthID:          ContractAuthMLDSA65,
	KeyExchangeID:           KeyExchangeMLKEM768,
	HighValueKEM:            KeyExchangeMLKEM1024,
	RecoverySchemeID:        RecoverySchemeSLHDSA192,
	ForbidECDSAWallets:      false,
	ForbidECDSAContractAuth: false,
	ForbidBLSContractAuth:   false,
	ForbidClassicalKEM:      false,
	RequireTypedTxAuth:      false,
}

// FIPSProfile is the FIPS-204-only profile. Drops Pulsar-M (production
// fork of Corona; not yet FIPS-approved) — but the profile still has to
// satisfy `FinalitySchemeID.IsPulsarM()`, so for FIPS deployments the
// chain DOES use Pulsar-M (FIPS 204-compatible output) at M-65 and M-87.
// Only the canonical P3Q STARK/FRI/SHA3 backend is admitted.
//
// Note: this profile is named FIPS but the actual FIPS-only protocol
// stance the FIPS deployment ships is documented per HIP-0077 §"FIPS
// profile". This struct is the consensus-layer allow-list; the larger
// FIPS posture lives in the operator manifest.
var FIPSProfile = ChainSecurityProfile{
	ProfileID:         uint32(ProfileFIPS),
	ProfileName:       ProfileNameFIPS,
	HashSuiteID:       HashSuiteSHA3NIST,
	IdentitySchemeID:  SigSchemeMLDSA65,
	FinalitySchemeID:  SigSchemePulsarM65,
	HighValueSchemeID: SigSchemePulsarM87,
	ProofPolicyID:     ProofPolicySTARKFRISHA3PQ,
	AllowedProofBackends: []ProofBackendID{
		ProofBackendP3QSTARKFRISHA3,
	},
	AllowedProofFormats: []ProofFormatID{
		ProofFormatP3QBinaryV1,
	},
	MinSoundnessBits:      128,
	MinHashOutputBits:     384,
	RequireTransparent:    true,
	ForbidPairings:        true,
	ForbidKZG:             true,
	ForbidTrustedSetup:    true,
	ForbidClassicalSNARKs: true,
	ForbidDevProofs:       true,
	ForbidFallbacks:       true,

	// E2E PQ axes — identical to StrictPQ. FIPS deployments demand
	// every E2E layer sit inside FIPS 203/204/205; the canonical FIPS
	// scheme set is the same as the strict-PQ set.
	WalletSchemeID:          WalletSchemeMLDSA65,
	TxSchemeID:              TxSchemeMLDSA65,
	ContractAuthID:          ContractAuthMLDSA65,
	KeyExchangeID:           KeyExchangeMLKEM768,
	HighValueKEM:            KeyExchangeMLKEM1024,
	RecoverySchemeID:        RecoverySchemeSLHDSA192,
	ForbidECDSAWallets:      true,
	ForbidECDSAContractAuth: true,
	ForbidBLSContractAuth:   true,
	ForbidClassicalKEM:      true,
	RequireTypedTxAuth:      true,
}

// ForkClassicalCompatUnsafeProfileID is the wire byte for the
// classical-compat fork profile. Reserved in the 0x80+ downstream block.
// Any chain that pins this profile MUST NOT be marketed as Lux strict-PQ
// — Validate accepts it (it satisfies all soundness/forbid invariants)
// but its ProofPolicyID is one of the explicit classical-PQ STARK
// policies, and the ChainSecurityProfile.Validate check that refuses a
// classical ProofPolicyID does NOT apply here because we accept the
// keccak-merkle STARK policy (PQ-positive but non-NIST-canonical).
//
// ForkClassicalCompatUnsafeProfile is provided so audit tooling has a
// concrete fork-stance to point at in error messages: "this chain
// pinned the COMPAT_UNSAFE fork profile; the consensus layer is not
// auditing it as Lux strict-PQ."
const ForkClassicalCompatUnsafeProfileID uint32 = 0x80

// ForkClassicalCompatUnsafeProfile is the locked profile for downstream
// forks that have an external requirement to maintain compatibility with
// classical-compat verifiers (e.g. forks tied to an L1 that requires a
// Groth16 wrapper for cheap EVM verification). The profile uses the
// non-NIST-canonical STARK_FRI_Keccak policy and allows backends that
// emit STARKs verifiable under Keccak Merkle trees.
//
// CRITICAL: This profile MUST NOT be marketed as "Lux strict-PQ." The
// ProfileName explicitly says "FORK_CLASSICAL_COMPAT_UNSAFE" so an
// operator who deploys it cannot accidentally claim the strict-PQ
// posture; audit tooling matches on ProfileName and refuses to issue
// "strict-PQ" attestations to a chain on this profile.
var ForkClassicalCompatUnsafeProfile = ChainSecurityProfile{
	ProfileID:         ForkClassicalCompatUnsafeProfileID,
	ProfileName:       "FORK_CLASSICAL_COMPAT_UNSAFE",
	HashSuiteID:       HashSuiteSHA3NIST,
	IdentitySchemeID:  SigSchemeMLDSA65,
	FinalitySchemeID:  SigSchemePulsarM65,
	HighValueSchemeID: SigSchemePulsarM87,
	ProofPolicyID:     ProofPolicySTARKFRIKeccak,
	AllowedProofBackends: []ProofBackendID{
		ProofBackendP3QSTARKFRISHA3,
		ProofBackendSP1CompressedSTARK,
		ProofBackendRISC0SuccinctSTARK,
	},
	AllowedProofFormats: []ProofFormatID{
		ProofFormatSTARKFRIBinaryV1,
		ProofFormatSP1BinaryV1,
		ProofFormatRISC0BinaryV1,
		ProofFormatP3QBinaryV1,
	},
	MinSoundnessBits:      128,
	MinHashOutputBits:     384,
	RequireTransparent:    true,
	ForbidPairings:        true,
	ForbidKZG:             true,
	ForbidTrustedSetup:    true,
	ForbidClassicalSNARKs: true, // even the fork refuses Groth16/PLONK wrappers
	ForbidDevProofs:       true,
	ForbidFallbacks:       false, // forks may fall back; strict mainnet may not

	// E2E PQ axes — the fork explicitly opts INTO classical primitives
	// on every axis so existing EVM-classical wallets / contracts /
	// session keys keep working. RecoverySchemeNone is permitted here
	// because the high-value scheme is Pulsar-M-87 (Cat 5). Every
	// Forbid* bit is false; RequireTypedTxAuth is false. A chain that
	// pins this profile MUST NOT be marketed as Lux strict-PQ.
	WalletSchemeID:          WalletSchemeECDSAUnsafe,
	TxSchemeID:              TxSchemeECDSAUnsafe,
	ContractAuthID:          ContractAuthECDSAUnsafe,
	KeyExchangeID:           KeyExchangeX25519Unsafe,
	HighValueKEM:            KeyExchangeX25519Unsafe,
	RecoverySchemeID:        RecoverySchemeNone,
	ForbidECDSAWallets:      false,
	ForbidECDSAContractAuth: false,
	ForbidBLSContractAuth:   false,
	ForbidClassicalKEM:      false,
	RequireTypedTxAuth:      false,
}

// HybridProfile is the transitional production posture: BLS + PQ both
// required at the consensus / finality layer, classical contract-auth
// permitted at the EVM precompile boundary. Used by chains migrating
// from classical into strict without a hard fork — PQ witness + BLS
// quorum land in the cert envelope side-by-side, while wallets and
// dApps keep using ECDSA.
//
// ProfileID 0x04, name "HYBRID". ProfileHash computed in init().
var HybridProfile = ChainSecurityProfile{
	ProfileID:         uint32(ProfileHybridID),
	ProfileName:       ProfileNameHybrid,
	HashSuiteID:       HashSuiteSHA3NIST,
	IdentitySchemeID:  SigSchemeMLDSA65,
	FinalitySchemeID:  SigSchemePulsarM65,
	HighValueSchemeID: SigSchemePulsarM87,
	ProofPolicyID:     ProofPolicySTARKFRISHA3PQ,
	AllowedProofBackends: []ProofBackendID{
		ProofBackendSP1CompressedSTARK,
		ProofBackendRISC0SuccinctSTARK,
		ProofBackendP3QSTARKFRISHA3,
		ProofBackendStoneCairoSTARK,
		ProofBackendStwoCircleSTARK,
	},
	AllowedProofFormats: []ProofFormatID{
		ProofFormatSTARKFRIBinaryV1,
		ProofFormatSP1BinaryV1,
		ProofFormatRISC0BinaryV1,
		ProofFormatP3QBinaryV1,
		ProofFormatStoneCairoBinaryV1,
		ProofFormatStwoCircleBinaryV1,
	},
	MinSoundnessBits:      128,
	MinHashOutputBits:     384,
	RequireTransparent:    true,
	ForbidPairings:        false, // BLS aggregate still active
	ForbidKZG:             false,
	ForbidTrustedSetup:    false,
	ForbidClassicalSNARKs: false,
	ForbidDevProofs:       true,
	ForbidFallbacks:       false,

	// E2E PQ axes — classical wallet/auth still permitted, PQ KEM
	// required, recovery via SLH-DSA. RequireTypedTxAuth=true so a
	// receiver always sees the auth primitive named on the wire.
	WalletSchemeID:          WalletSchemeECDSAUnsafe,
	TxSchemeID:              TxSchemeECDSAUnsafe,
	ContractAuthID:          ContractAuthECDSAUnsafe,
	KeyExchangeID:           KeyExchangeMLKEM768,
	HighValueKEM:            KeyExchangeMLKEM1024,
	RecoverySchemeID:        RecoverySchemeSLHDSA192,
	ForbidECDSAWallets:      false,
	ForbidECDSAContractAuth: false,
	ForbidBLSContractAuth:   false,
	ForbidClassicalKEM:      false,
	RequireTypedTxAuth:      true,
}

// ClassicalProfile is the explicit "no PQ" stance — no enforced
// refusals at any layer. Legacy chains pre-PQ migration pin this so
// audit tooling can unambiguously identify the classical posture
// (instead of inferring it from the absence of a profile).
//
// ProfileID 0x05, name "CLASSICAL". Distinct from
// ForkClassicalCompatUnsafeProfile (0x80): Classical is the clean
// "no PQ here" marker; the Fork-Classical-Compat-Unsafe byte is
// reserved for downstream forks with an explicit anti-strict-PQ
// stance flagged loudly in the profile name.
//
// ProfileHash computed in init().
var ClassicalProfile = ChainSecurityProfile{
	ProfileID:         uint32(ProfileClassicID),
	ProfileName:       ProfileNameClassical,
	HashSuiteID:       HashSuiteSHA3NIST,
	IdentitySchemeID:  SigSchemeMLDSA65,
	FinalitySchemeID:  SigSchemePulsarM65,
	HighValueSchemeID: SigSchemePulsarM87,
	ProofPolicyID:     ProofPolicySTARKFRISHA3PQ,
	AllowedProofBackends: []ProofBackendID{
		ProofBackendSP1CompressedSTARK,
		ProofBackendRISC0SuccinctSTARK,
		ProofBackendP3QSTARKFRISHA3,
	},
	AllowedProofFormats: []ProofFormatID{
		ProofFormatSTARKFRIBinaryV1,
		ProofFormatSP1BinaryV1,
		ProofFormatRISC0BinaryV1,
		ProofFormatP3QBinaryV1,
	},
	MinSoundnessBits:      128,
	MinHashOutputBits:     384,
	RequireTransparent:    false,
	ForbidPairings:        false,
	ForbidKZG:             false,
	ForbidTrustedSetup:    false,
	ForbidClassicalSNARKs: false,
	ForbidDevProofs:       true, // dev backends still out — orthogonal to PQ stance
	ForbidFallbacks:       false,

	// E2E axes — every classical primitive permitted.
	WalletSchemeID:          WalletSchemeECDSAUnsafe,
	TxSchemeID:              TxSchemeECDSAUnsafe,
	ContractAuthID:          ContractAuthECDSAUnsafe,
	KeyExchangeID:           KeyExchangeX25519Unsafe,
	HighValueKEM:            KeyExchangeX25519Unsafe,
	RecoverySchemeID:        RecoverySchemeSLHDSA192,
	ForbidECDSAWallets:      false,
	ForbidECDSAContractAuth: false,
	ForbidBLSContractAuth:   false,
	ForbidClassicalKEM:      false,
	RequireTypedTxAuth:      false,
}

// init computes and pins ProfileHash for every canonical profile. Runs
// at package load; a build whose canonical profiles fail Validate cannot
// initialise and therefore cannot ship. This is the genesis-pinning
// guarantee at the binary level.
//
// MustComputeHash panics on validation failure; the panic message names
// the failing profile so a misconfiguration is immediate and visible in
// the boot log.
func init() {
	StrictPQProfile.ProfileHash = StrictPQProfile.MustComputeHash()
	PermissiveProfile.ProfileHash = PermissiveProfile.MustComputeHash()
	FIPSProfile.ProfileHash = FIPSProfile.MustComputeHash()
	ForkClassicalCompatUnsafeProfile.ProfileHash = ForkClassicalCompatUnsafeProfile.MustComputeHash()
	HybridProfile.ProfileHash = HybridProfile.MustComputeHash()
	ClassicalProfile.ProfileHash = ClassicalProfile.MustComputeHash()
}
