// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package auth

import (
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
)

// TestContractAuthProfile_Validate_HappyPath — every action pinned at
// a PQ scheme under a strict-PQ chain validates cleanly.
func TestContractAuthProfile_Validate_HappyPath(t *testing.T) {
	chain := config.LuxStrictPQ()
	c := &ContractAuthProfile{
		AdminSchemeID:   ContractAuthMLDSA87,
		UpgradeSchemeID: ContractAuthMLDSA87,
		PauseSchemeID:   ContractAuthMLDSA65,
		PermitSchemeID:  ContractAuthMLDSA65,
		AllowClassical:  false,
	}
	if err := c.Validate(chain); err != nil {
		t.Fatalf("strict-PQ happy path failed: %v", err)
	}
}

// TestContractAuthProfile_Validate_RejectsClassicalUnderStrictPQ — a
// strict-PQ chain refuses ANY classical scheme regardless of the
// contract's AllowClassical flag.
func TestContractAuthProfile_Validate_RejectsClassicalUnderStrictPQ(t *testing.T) {
	chain := config.LuxStrictPQ()

	// Trying each per-action field with the classical marker MUST fail.
	cases := []struct {
		name string
		mut  func(*ContractAuthProfile)
	}{
		{
			"Admin classical",
			func(c *ContractAuthProfile) { c.AdminSchemeID = ContractAuthECDSASecp256k1Legacy },
		},
		{
			"Upgrade classical",
			func(c *ContractAuthProfile) { c.UpgradeSchemeID = ContractAuthECDSASecp256k1Legacy },
		},
		{
			"Pause classical",
			func(c *ContractAuthProfile) { c.PauseSchemeID = ContractAuthECDSASecp256k1Legacy },
		},
		{
			"Permit classical",
			func(c *ContractAuthProfile) { c.PermitSchemeID = ContractAuthECDSASecp256k1Legacy },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// AllowClassical=true MUST NOT save us under a strict-PQ chain.
			c := &ContractAuthProfile{
				AdminSchemeID:   ContractAuthMLDSA65,
				UpgradeSchemeID: ContractAuthMLDSA65,
				PauseSchemeID:   ContractAuthMLDSA65,
				PermitSchemeID:  ContractAuthMLDSA65,
				AllowClassical:  true,
			}
			tc.mut(c)
			if err := c.Validate(chain); !errors.Is(err, ErrContractAuthClassicalForbidden) {
				t.Fatalf("classical scheme accepted under strict-PQ with AllowClassical=true: %v", err)
			}
		})
	}
}

// TestContractAuthProfile_Validate_RejectsFIPSClassical — FIPS profile
// also refuses classical regardless of AllowClassical.
func TestContractAuthProfile_Validate_RejectsFIPSClassical(t *testing.T) {
	chain := config.LuxFIPS()
	c := &ContractAuthProfile{
		AdminSchemeID:   ContractAuthECDSASecp256k1Legacy,
		UpgradeSchemeID: ContractAuthMLDSA65,
		PauseSchemeID:   ContractAuthMLDSA65,
		PermitSchemeID:  ContractAuthMLDSA65,
		AllowClassical:  true,
	}
	if err := c.Validate(chain); !errors.Is(err, ErrContractAuthClassicalForbidden) {
		t.Fatalf("FIPS profile accepted classical: %v", err)
	}
}

// TestContractAuthProfile_Validate_PermissiveAllowsClassicalOnlyWithFlag
// — under a permissive chain profile, classical schemes pass ONLY when
// AllowClassical=true.
func TestContractAuthProfile_Validate_PermissiveAllowsClassicalOnlyWithFlag(t *testing.T) {
	chain := config.LuxPermissive()

	// Without flag — refused.
	cNoFlag := &ContractAuthProfile{
		AdminSchemeID:   ContractAuthECDSASecp256k1Legacy,
		UpgradeSchemeID: ContractAuthMLDSA65,
		PauseSchemeID:   ContractAuthMLDSA65,
		PermitSchemeID:  ContractAuthMLDSA65,
		AllowClassical:  false,
	}
	if err := cNoFlag.Validate(chain); !errors.Is(err, ErrContractAuthClassicalForbidden) {
		t.Fatalf("permissive without flag accepted classical: %v", err)
	}

	// With flag — permitted (testnet relaxation).
	cFlag := *cNoFlag
	cFlag.AllowClassical = true
	if err := cFlag.Validate(chain); err != nil {
		t.Fatalf("permissive with flag rejected classical: %v", err)
	}
}

// TestContractAuthProfile_Validate_RejectsUnsetScheme — None on any
// action is a typed error.
func TestContractAuthProfile_Validate_RejectsUnsetScheme(t *testing.T) {
	chain := config.LuxStrictPQ()
	cases := []struct {
		name string
		mut  func(*ContractAuthProfile)
	}{
		{"Admin", func(c *ContractAuthProfile) { c.AdminSchemeID = ContractAuthNone }},
		{"Upgrade", func(c *ContractAuthProfile) { c.UpgradeSchemeID = ContractAuthNone }},
		{"Pause", func(c *ContractAuthProfile) { c.PauseSchemeID = ContractAuthNone }},
		{"Permit", func(c *ContractAuthProfile) { c.PermitSchemeID = ContractAuthNone }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := &ContractAuthProfile{
				AdminSchemeID:   ContractAuthMLDSA65,
				UpgradeSchemeID: ContractAuthMLDSA65,
				PauseSchemeID:   ContractAuthMLDSA65,
				PermitSchemeID:  ContractAuthMLDSA65,
			}
			tc.mut(c)
			if err := c.Validate(chain); !errors.Is(err, ErrContractAuthSchemeUnset) {
				t.Fatalf("%s=None accepted: %v", tc.name, err)
			}
		})
	}
}

// TestContractAuthProfile_Validate_RejectsUnknownScheme — an out-of-band
// byte (not in the defined enum set) is refused.
func TestContractAuthProfile_Validate_RejectsUnknownScheme(t *testing.T) {
	chain := config.LuxStrictPQ()
	c := &ContractAuthProfile{
		AdminSchemeID:   ContractAuthID(0x7F), // unknown
		UpgradeSchemeID: ContractAuthMLDSA65,
		PauseSchemeID:   ContractAuthMLDSA65,
		PermitSchemeID:  ContractAuthMLDSA65,
	}
	if err := c.Validate(chain); !errors.Is(err, ErrContractAuthSchemeUnknown) {
		t.Fatalf("unknown scheme accepted: %v", err)
	}
}

// TestContractAuthProfile_Validate_RejectsNilArguments — nil receiver
// or nil chain profile are typed errors.
func TestContractAuthProfile_Validate_RejectsNilArguments(t *testing.T) {
	chain := config.LuxStrictPQ()

	var nilProfile *ContractAuthProfile
	if err := nilProfile.Validate(chain); !errors.Is(err, ErrContractAuthNilProfile) {
		t.Errorf("nil profile: got %v", err)
	}

	c := &ContractAuthProfile{
		AdminSchemeID:   ContractAuthMLDSA65,
		UpgradeSchemeID: ContractAuthMLDSA65,
		PauseSchemeID:   ContractAuthMLDSA65,
		PermitSchemeID:  ContractAuthMLDSA65,
	}
	if err := c.Validate(nil); !errors.Is(err, ErrContractAuthNilChainProfile) {
		t.Errorf("nil chain profile: got %v", err)
	}
}
