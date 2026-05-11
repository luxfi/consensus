// Copyright (C) 2019-2026, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// validator_scheme.go — cross-axis gate between ChainSecurityProfile and
// the NodeIDScheme byte that travels with every peer / validator
// registration / block-proposer attribution. Lives here (not in
// security_profile.go) so the surface stays tightly scoped: one file
// owns the rule "the NodeID scheme on the wire MUST match the scheme
// pinned by the chain profile, unless classical-compat is on".
//
// The matching wire byte enum is luxfi/ids.NodeIDScheme. The two enums
// share the same numeric assignment by design (0x42 = ML-DSA-65,
// 0x43 = ML-DSA-87, 0x90 = secp256k1 classical-compat-unsafe), so the
// byte itself reads identically in every transcript. SigSchemeID names
// the signature primitive; NodeIDScheme names the identity-derivation
// primitive; the wire byte is shared because the two travel together.
//
// This file deliberately does NOT import luxfi/ids — consensus/config
// sits below ids in the dependency graph. The cross-axis check is
// performed by callers (peer/upgrader.go, vms/proposervm/block) that
// import both packages; this file owns the byte-level rule.

package config

import (
	"errors"
	"fmt"
)

// ValidatorSchemeID is the SigSchemeID byte the chain pins for validator
// identity. It is the single source of truth for "what NodeIDScheme MUST
// a peer present in this chain's handshake / validator registration /
// block-proposer attribution". The wire byte is shared with
// luxfi/ids.NodeIDScheme so a verifier can match without conversion.
//
// Type alias — not a new struct field. The pinned scheme is the
// IdentitySchemeID byte the profile already carries; this alias names
// the role explicitly so call sites read self-documenting.
type ValidatorSchemeID = SigSchemeID

// ValidatorSchemeID returns the SigSchemeID byte that pins which
// NodeIDScheme byte is admissible for validator identity on this
// chain. It is the IdentitySchemeID byte by definition: the wire
// transcript pins one ML-DSA family across "registration signature"
// and "NodeID derivation" so they cannot drift.
//
// Callers comparing a wire-decoded ids.NodeIDScheme byte against the
// chain pin SHOULD cast the byte directly (both enums are uint8) and
// match via this accessor. Returned bytes are SigSchemeMLDSA65 (0x42)
// or SigSchemeMLDSA87 (0x43) on a locked strict-PQ profile.
func (p *ChainSecurityProfile) ValidatorSchemeID() ValidatorSchemeID {
	return p.IdentitySchemeID
}

// AcceptsValidatorScheme reports whether the wire scheme byte presented
// by a peer / proposer / registrant is admissible under this profile.
//
// The check has three branches:
//
//  1. presented == ValidatorSchemeID()  → accept (the matched path).
//  2. presented is classical (0x90 block) AND classicalCompatUnsafe is
//     true → accept (the operator opted into legacy classical NodeIDs).
//  3. anything else → refuse with ErrValidatorSchemeMismatch.
//
// classicalCompatUnsafe MUST be derived from the operator's explicit
// opt-in (LUX_CLASSICAL_COMPAT_UNSAFE env / config flag); it is NOT a
// default. Strict-PQ profiles refuse classical schemes even when the
// flag is set: this function is the cross-axis primitive-mismatch
// gate; the strict-PQ refusal lives at AcceptsValidatorScheme's caller
// which checks the profile class first.
//
// classicalCompatUnsafe interacts with profile class as follows:
//
//   - strict-PQ profile (ProfileLuxStrictPQ / ProfileLuxFIPS):
//     classicalCompatUnsafe MUST be false. The caller refuses the flag
//     before reaching this gate.
//   - permissive profile (ProfileLuxPermissive): classicalCompatUnsafe
//     MAY be true; the operator owns the decision.
func (p *ChainSecurityProfile) AcceptsValidatorScheme(
	presented SigSchemeID,
	classicalCompatUnsafe bool,
) error {
	pinned := p.ValidatorSchemeID()
	if presented == pinned {
		return nil
	}

	// Classical-compat path: only the named classical block byte is
	// admissible, and only when the operator opted in. An arbitrary
	// 0x90+ byte is still refused.
	if classicalCompatUnsafe && presented.IsClassicalCompatUnsafe() {
		// Strict-PQ profiles refuse classical regardless of the flag.
		// The flag's only effect is on permissive profiles.
		if p.ProfileID == uint32(ProfileLuxStrictPQ) ||
			p.ProfileID == uint32(ProfileLuxFIPS) {
			return fmt.Errorf("%w: presented=%s pinned=%s; strict-PQ refuses classical even under LUX_CLASSICAL_COMPAT_UNSAFE",
				ErrValidatorSchemeMismatch, presented.String(), pinned.String())
		}
		return nil
	}

	return fmt.Errorf("%w: presented=%s pinned=%s",
		ErrValidatorSchemeMismatch, presented.String(), pinned.String())
}

// IsClassicalCompatUnsafe reports whether this SigSchemeID is in the
// classical compat block (0x90+). Mirrors the matching helper on
// luxfi/ids.NodeIDScheme. The wire byte is shared between the two
// enums; both enums classify the byte identically.
//
// At present the only enrolled classical byte is secp256k1 (0x90);
// other 0x90+ bytes are reserved and refused by every gate.
func (s SigSchemeID) IsClassicalCompatUnsafe() bool {
	return s == sigSchemeSecp256k1Classical
}

// sigSchemeSecp256k1Classical is the SigSchemeID byte that mirrors
// luxfi/ids.NodeIDSchemeSecp256k1 (0x90). Named at this package level
// so the byte assignment lives in one place and the wire enums stay
// aligned.
//
// Not a public constant: callers outside this file SHOULD route through
// IsClassicalCompatUnsafe so the byte assignment can be tightened later
// without churning every call site.
const sigSchemeSecp256k1Classical SigSchemeID = 0x90

// ErrValidatorSchemeMismatch — the wire NodeIDScheme byte presented by
// a peer does not match what the chain profile pins. Returned by
// AcceptsValidatorScheme and by node-level callers that funnel the
// cross-axis check through this single error.
//
// Mirrors luxfi/ids.ErrNodeIDSchemeMismatch; consumers that need to
// match either error MAY wrap both. The errors are not identical
// because they live in different packages — one is the wire-decode
// gate (ids), the other is the chain-profile gate (consensus/config).
var ErrValidatorSchemeMismatch = errors.New("config: NodeIDScheme does not match ChainSecurityProfile.ValidatorSchemeID")
