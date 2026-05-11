// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import (
	"errors"
	"fmt"

	"github.com/luxfi/consensus/config"
)

// contract_profile.go — per-contract authorization declaration.
//
// A ContractAuthProfile is what a smart contract declares as its
// admin / upgrade / pause / permit signature schemes. The chain
// enforces that those schemes are consistent with the chain's
// ChainSecurityProfile: under a strict-PQ chain profile, a contract
// MUST NOT declare a classical scheme for admin / upgrade / pause /
// permit unless AllowClassical is explicitly true AND the chain
// profile permits classical.
//
// One contract = one profile. Same contract MAY pin a different scheme
// per action (admin at ML-DSA-87 for high-stakes governance, permit at
// ML-DSA-65 for everyday allowances) but all schemes MUST satisfy the
// chain-wide policy.

// ContractAuthProfile is the per-contract auth declaration. Carried in
// the contract's deployment metadata and re-checked on every
// admin/upgrade/pause/permit action so a contract that drifted past
// the chain policy fails loud, not silent.
type ContractAuthProfile struct {
	// AdminSchemeID is the signature scheme required for admin actions
	// (treasury withdrawals, parameter changes, role grants). Strongest
	// scheme in the profile by convention.
	AdminSchemeID ContractAuthID

	// UpgradeSchemeID is the signature scheme required for code upgrades
	// (proxy implementation swaps, contract upgrades). Equal or
	// stronger than AdminSchemeID.
	UpgradeSchemeID ContractAuthID

	// PauseSchemeID is the signature scheme required for pause / unpause
	// actions. May be weaker than AdminSchemeID (faster reaction time,
	// fewer signatures) but MUST be PQ under any strict-PQ profile.
	PauseSchemeID ContractAuthID

	// PermitSchemeID is the signature scheme accepted for PQPermit
	// (EIP-2612-style allowances). Refused if a permit's AuthSchemeID
	// doesn't match.
	PermitSchemeID ContractAuthID

	// AllowClassical opts the contract into accepting the legacy
	// classical schemes (ECDSA secp256k1). MUST be false under any
	// chain profile that pins strict-PQ; the chain-side Validate gate
	// refuses contracts that set this true on a strict-PQ chain.
	AllowClassical bool
}

// Validate returns nil iff this contract profile is consistent with the
// chain-wide ChainSecurityProfile. Refuses:
//
//  1. Any zero scheme (None) — every action MUST pin an explicit scheme.
//  2. Any classical (legacy) scheme when chainProfile pins strict-PQ
//     AND this profile has AllowClassical=false.
//  3. Any unknown scheme byte (ContractAuthID not in the defined set).
//
// On a permissive chain profile, classical schemes are permitted only
// when AllowClassical is explicitly true; absence of the flag still
// fails-closed.
func (c *ContractAuthProfile) Validate(chainProfile *config.ChainSecurityProfile) error {
	if c == nil {
		return ErrContractAuthNilProfile
	}
	if chainProfile == nil {
		return ErrContractAuthNilChainProfile
	}

	// 1. Refuse None on every action.
	if c.AdminSchemeID == ContractAuthNone {
		return fmt.Errorf("%w: AdminSchemeID", ErrContractAuthSchemeUnset)
	}
	if c.UpgradeSchemeID == ContractAuthNone {
		return fmt.Errorf("%w: UpgradeSchemeID", ErrContractAuthSchemeUnset)
	}
	if c.PauseSchemeID == ContractAuthNone {
		return fmt.Errorf("%w: PauseSchemeID", ErrContractAuthSchemeUnset)
	}
	if c.PermitSchemeID == ContractAuthNone {
		return fmt.Errorf("%w: PermitSchemeID", ErrContractAuthSchemeUnset)
	}

	// 2. Reject unknown bytes (defensive — keeps a future renumbering
	// accident from silently accepting a foreign byte).
	for label, s := range map[string]ContractAuthID{
		"AdminSchemeID":   c.AdminSchemeID,
		"UpgradeSchemeID": c.UpgradeSchemeID,
		"PauseSchemeID":   c.PauseSchemeID,
		"PermitSchemeID":  c.PermitSchemeID,
	} {
		if !isKnownContractAuth(s) {
			return fmt.Errorf("%w: %s=0x%02x", ErrContractAuthSchemeUnknown, label, uint8(s))
		}
	}

	// 3. Strict-PQ chain profile refuses classical schemes regardless of
	// AllowClassical. ProfileStrictPQ / ProfileFIPS / ProfileZooStrictPQ
	// / ProfileHanzoStrictPQ are the canonical strict profiles; a
	// downstream / white-label strict profile MAY set the same ID block
	// in its ProfileID field.
	chainStrict := chainProfile.ProfileID == uint32(config.ProfileStrictPQ) ||
		chainProfile.ProfileID == uint32(config.ProfileFIPS) ||
		chainProfile.ProfileID == uint32(config.ProfileZooStrictPQ) ||
		chainProfile.ProfileID == uint32(config.ProfileHanzoStrictPQ)

	checkLegacy := func(label string, s ContractAuthID) error {
		if !s.IsLegacyClassical() {
			return nil
		}
		if chainStrict {
			return fmt.Errorf("%w: %s=%s under strict chain profile %s",
				ErrContractAuthClassicalForbidden,
				label, s.String(), chainProfile.ProfileName)
		}
		if !c.AllowClassical {
			return fmt.Errorf("%w: %s=%s requires AllowClassical=true",
				ErrContractAuthClassicalForbidden, label, s.String())
		}
		return nil
	}
	if err := checkLegacy("AdminSchemeID", c.AdminSchemeID); err != nil {
		return err
	}
	if err := checkLegacy("UpgradeSchemeID", c.UpgradeSchemeID); err != nil {
		return err
	}
	if err := checkLegacy("PauseSchemeID", c.PauseSchemeID); err != nil {
		return err
	}
	if err := checkLegacy("PermitSchemeID", c.PermitSchemeID); err != nil {
		return err
	}

	return nil
}

// isKnownContractAuth reports whether s is a defined ContractAuthID
// entry. Refuses unknown bytes so the codec / validator fails loud
// rather than silently accepting a foreign value.
func isKnownContractAuth(s ContractAuthID) bool {
	switch s {
	case ContractAuthECDSASecp256k1Legacy,
		ContractAuthMLDSA44, ContractAuthMLDSA65, ContractAuthMLDSA87,
		ContractAuthSLHDSA128f, ContractAuthSLHDSA192f, ContractAuthSLHDSA256f:
		return true
	}
	return false
}

// =============================================================================
// Typed errors
// =============================================================================

var (
	// ErrContractAuthNilProfile — receiver is nil.
	ErrContractAuthNilProfile = errors.New("contractauth: nil ContractAuthProfile")

	// ErrContractAuthNilChainProfile — chainProfile argument is nil.
	ErrContractAuthNilChainProfile = errors.New("contractauth: nil chain ChainSecurityProfile")

	// ErrContractAuthSchemeUnset — one of the per-action SchemeID fields
	// is ContractAuthNone.
	ErrContractAuthSchemeUnset = errors.New("contractauth: action SchemeID is unset")

	// ErrContractAuthSchemeUnknown — one of the per-action SchemeID
	// fields is not a defined ContractAuthID entry.
	ErrContractAuthSchemeUnknown = errors.New("contractauth: action SchemeID is unknown")

	// ErrContractAuthClassicalForbidden — a classical (legacy) scheme
	// appears on a strict-PQ chain profile, OR appears on a permissive
	// chain profile without AllowClassical=true.
	ErrContractAuthClassicalForbidden = errors.New("contractauth: classical scheme forbidden under chain profile")
)
