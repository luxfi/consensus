// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package registry is the Z-Chain PQ identity / authorization registry
// (HIP-0078 §4). It owns the on-chain state machine that produces every
// root the Z-Chain ZPublicInputs (and Q-Chain EpochCommitment) consume:
//
//	identity_root          — committed PQKeyRecord set
//	account_key_root       — same set, indexed by AccountID
//	revocation_root        — revoked / rotated AccountID set
//	session_key_root       — authorised ephemeral session keys
//	contract_policy_root   — per-contract authorisation policy
//	tx_auth_root           — accepted TxAuthEnvelope batch root
//	permit_auth_root       — accepted permit envelope batch root
//
// Producer side: each operation type (register, rotate, revoke,
// authorize-session, commit-tx-auth-batch, commit-permit-batch) carries
// its own ML-DSA-65 signature transcript with an op-specific cSHAKE256
// customization so a signature from one op cannot replay as another op.
// The registry's Apply path runs op.Verify(profile) before mutating
// state; the resulting roots are aggregated via ZRegistryRoots and ride
// in EpochCommitment.
//
// Hot path side: VerifyAuthPassed replaces a direct ML-DSA verify in
// the execution hot path for transactions whose signature was already
// proven in an accepted Z-Chain tx_auth batch. Hot path cost is one
// 48-byte Merkle inclusion check — independent of signature size.
//
// Out of scope for this package:
//   - The STARK circuit body that proves "the registry's Apply step is
//     correct under the input ops" — that lives in luxfi/p3q-zchain.
//   - On-disk persistence of the registry state — coreth / Z-Chain VM
//     owns durable state.
//   - Discovery / gossip of pending ops — Z-Chain VM owns mempool.
//
// All cross-package types this package binds (config.WalletSchemeID,
// config.ChainSecurityProfile, MerkleProof) come from packages that
// already live below the proof / VM layer, so this package stays
// dependency-clean.
package registry
