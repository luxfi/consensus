package config

import (
	"errors"
	"testing"
)

// A bridge profile that leaves any pinned field unset is refused before it can
// be used, one typed reason per field. A bridge whose hash family or admin
// scheme is unpinned cannot commit to a transcript or authorise a transfer, so
// accepting one is worse than refusing to start.
func TestIncompleteBridgeProfileIsRefusedPerField(t *testing.T) {
	for name, tc := range map[string]struct {
		break_ func(*BridgeProfile)
		want   error
	}{
		"no name":            {func(p *BridgeProfile) { p.Name = "" }, ErrBridgeProfileFieldUnset},
		"no source finality": {func(p *BridgeProfile) { p.SourceFinalityScheme = SigSchemeNone }, ErrBridgeProfileFieldUnset},
		"no dest finality":   {func(p *BridgeProfile) { p.DestFinalityScheme = SigSchemeNone }, ErrBridgeProfileFieldUnset},
		"no hash suite":      {func(p *BridgeProfile) { p.HashSuiteID = HashSuiteNone }, ErrBridgeProfileFieldUnset},
		"unknown hash suite": {func(p *BridgeProfile) { p.HashSuiteID = HashSuiteID(0xEE) }, ErrBridgeProfileFieldUnknown},
		"no proof policy":    {func(p *BridgeProfile) { p.ProofPolicyID = ProofPolicyNone }, ErrBridgeProfileFieldUnset},
		"no admin scheme":    {func(p *BridgeProfile) { p.BridgeAdminScheme = ContractAuthInvalid }, ErrBridgeProfileFieldUnset},
		"no pause scheme":    {func(p *BridgeProfile) { p.BridgePauseScheme = ContractAuthInvalid }, ErrBridgeProfileFieldUnset},
	} {
		t.Run(name, func(t *testing.T) {
			p := strictPQBridgeProfile
			if err := p.Validate(); err != nil {
				t.Fatalf("the canonical bridge profile does not validate: %v", err)
			}

			tc.break_(&p)
			err := p.Validate()
			if err == nil {
				t.Fatal("accepted an incomplete bridge profile")
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}

	var nilProfile *BridgeProfile
	if err := nilProfile.Validate(); !errors.Is(err, ErrBridgeProfileNil) {
		t.Errorf("nil bridge profile: got %v, want ErrBridgeProfileNil", err)
	}
	if err := nilProfile.RequireAdminPQ(); !errors.Is(err, ErrBridgeProfileNil) {
		t.Errorf("nil bridge profile admin check: got %v, want ErrBridgeProfileNil", err)
	}
}

// The admin gate is what every ecrecover call site consults. A strict-PQ bridge
// must refuse a classical admin scheme even though the call site would have
// accepted it; a compat bridge is the only thing that may say yes. Breaks if a
// strict profile ever admits a classical authorisation.
func TestAdminGateRefusesClassicalOnStrictBridges(t *testing.T) {
	if err := strictPQBridgeProfile.RequireAdminPQ(); err != nil {
		t.Errorf("strict bridge refused its own PQ admin scheme: %v", err)
	}
	if err := bridgeClassicalCompat.RequireAdminPQ(); err != nil {
		t.Errorf("compat bridge refused a classical admin it explicitly allows: %v", err)
	}

	strictWithClassicalAdmin := strictPQBridgeProfile
	strictWithClassicalAdmin.BridgeAdminScheme = ContractAuthECDSAUnsafe
	err := strictWithClassicalAdmin.RequireAdminPQ()
	if err == nil {
		t.Fatal("strict bridge admitted an ECDSA admin scheme")
	}
	if !errors.Is(err, ErrBridgeProfileForbidden) {
		t.Errorf("got %v, want ErrBridgeProfileForbidden", err)
	}
}

// MustValidate is what init() uses, so a build carrying a malformed canonical
// bridge profile must not start. Breaks if it ever returns on a bad profile.
func TestMustValidatePanicsOnAMalformedBridgeProfile(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MustValidate returned on a profile that does not validate")
		}
	}()

	p := strictPQBridgeProfile
	p.Name = ""
	p.MustValidate()
}
