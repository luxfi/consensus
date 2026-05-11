// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

// precompile.go — native PQ verification precompile contracts.
//
// Lux pins four PQ verification primitives at fixed EVM precompile
// addresses so smart contracts can call them at native cost (gas
// schedule lives in coreth, not here):
//
//   0x0000000000000000000000000000000000000301  pq_verify_mldsa65
//   0x0000000000000000000000000000000000000302  pq_verify_mldsa87
//   0x0000000000000000000000000000000000000303  pq_verify_slh_dsa
//   0x0000000000000000000000000000000000000304  pq_verify_z_auth_proof
//
// This file declares the canonical function-pointer types every wiring
// point (coreth / evm) MUST satisfy. The wiring lives in
// ~/work/lux/coreth/precompile/pqverify/ as a separate task. This
// package only owns the contract surface — what arguments each
// precompile takes, what it returns, and the typed errors it can
// produce.
//
// Argument shape rationale:
//
//   - pubkey is the raw FIPS 204 / FIPS 205 encoded public key bytes;
//     length-checked at the wiring layer against the scheme's pinned
//     pubkey width before this function fires.
//   - msgDigest is the 32-byte (SHA3-256) or 48-byte (SHA3-384) digest
//     the contract supplies. The precompile does NOT hash the
//     message — that's the contract's responsibility. This separation
//     keeps the precompile constant-time and lets contracts use
//     TupleHash256 or plain SHA3 as they prefer.
//   - signature is the wire-format signature bytes (FIPS 204 ML-DSA
//     for 0x0301/0x0302, FIPS 205 SLH-DSA for 0x0303, Z-Chain auth
//     proof for 0x0304).
//
// Return:
//
//   - bool: true iff the signature verifies against (pubkey, msgDigest).
//   - error: transient failure (e.g. allocator). MUST be a typed error
//     so the wiring layer surfaces it as an EVM revert with a
//     consistent code; never a panic.

// Precompile addresses (low 4 bytes of a 20-byte EVM address).
// Wired into coreth precompile registry as a separate task; the
// constants live here so cross-repo searches find the canonical
// mapping in one place.
const (
	PrecompileAddrPQVerifyMLDSA65    = 0x301
	PrecompileAddrPQVerifyMLDSA87    = 0x302
	PrecompileAddrPQVerifySLHDSA     = 0x303
	PrecompileAddrPQVerifyZAuthProof = 0x304
)

// PQVerifyMLDSA65Fn is the function-pointer type for the
// pq_verify_mldsa65 precompile (address 0x0301). Verifies a FIPS 204
// ML-DSA-65 (NIST PQ Cat 3) signature.
//
// Inputs:
//
//	pubkey     raw ML-DSA-65 encoded public key (FIPS 204 §5.4.2).
//	msgDigest  caller-supplied message digest (typically 32 or 48
//	           bytes; ML-DSA verifies arbitrary-length messages, so
//	           the precompile accepts any length and the contract
//	           decides the binding).
//	signature  raw ML-DSA-65 encoded signature (FIPS 204 §5.4.3).
//
// Output:
//
//	bool   true iff the signature verifies.
//	error  transient failure (allocator, malformed pubkey length).
type PQVerifyMLDSA65Fn func(pubkey, msgDigest, signature []byte) (bool, error)

// PQVerifyMLDSA87Fn is the function-pointer type for the
// pq_verify_mldsa87 precompile (address 0x0302). Verifies a FIPS 204
// ML-DSA-87 (NIST PQ Cat 5) signature. Same argument shape as
// PQVerifyMLDSA65Fn — only the pubkey / signature widths differ.
type PQVerifyMLDSA87Fn func(pubkey, msgDigest, signature []byte) (bool, error)

// PQVerifySLHDSAFn is the function-pointer type for the pq_verify_slh_dsa
// precompile (address 0x0303). Verifies a FIPS 205 SLH-DSA signature.
// The parameter set (128f / 192f / 256f) is implied by the pubkey
// length and signature length; the wiring layer dispatches across
// parameter sets internally.
type PQVerifySLHDSAFn func(pubkey, msgDigest, signature []byte) (bool, error)

// PQVerifyZAuthFn is the function-pointer type for the
// pq_verify_z_auth_proof precompile (address 0x0304). Verifies a
// Z-Chain auth proof binding an AccountID to a transaction hash.
//
// Inputs:
//
//	accountID  48-byte PQ AccountID (SHAKE256-384 derivation, see
//	           DeriveAccountID).
//	txHash     32-byte commitment to the transaction whose auth is
//	           being proven (typically the EVM tx hash).
//	zAuthRoot  32-byte Z-Chain identity-commitment root the proof is
//	           anchored against. The verifier checks proof validity
//	           under this root; the contract MUST supply the
//	           authoritative root from chain state, not an attacker-
//	           controlled value.
//	proofRef   reference / opaque proof bytes the Z-Chain auth
//	           backend consumes. The exact shape is backend-specific
//	           (STARK / FRI envelope, etc.) and lives in zchain.
//
// Output:
//
//	bool   true iff (accountID, txHash) is bound by proofRef under
//	       zAuthRoot.
//	error  transient failure (proof parsing, backend dispatch).
type PQVerifyZAuthFn func(accountID [48]byte, txHash, zAuthRoot, proofRef []byte) (bool, error)
