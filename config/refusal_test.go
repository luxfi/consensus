package config

import (
	"errors"
	"testing"
)

// A locked profile is refused for exactly one reason per malformation, and the
// reason is typed so a boot failure says which axis is wrong. Each row breaks a
// canonical strict-PQ profile in one field: if any row stops erroring, a chain
// can boot with that field unset or nonsensical, which is the whole failure mode
// this validation exists to prevent.
func TestMalformedProfileIsRefusedPerField(t *testing.T) {
	for name, tc := range map[string]struct {
		break_ func(*ChainSecurityProfile)
		want   error
	}{
		"no name":            {func(p *ChainSecurityProfile) { p.ProfileName = "" }, ErrProfileFieldUnset},
		"no hash suite":      {func(p *ChainSecurityProfile) { p.HashSuiteID = HashSuiteNone }, ErrProfileFieldUnset},
		"unknown hash suite": {func(p *ChainSecurityProfile) { p.HashSuiteID = HashSuiteID(0xEE) }, ErrProfileFieldUnknown},
		"no identity scheme": {func(p *ChainSecurityProfile) { p.IdentitySchemeID = SigSchemeNone }, ErrProfileFieldUnset},
		"no finality scheme": {func(p *ChainSecurityProfile) { p.FinalitySchemeID = SigSchemeNone }, ErrProfileFieldUnset},
		"no proof policy":    {func(p *ChainSecurityProfile) { p.ProofPolicyID = ProofPolicyNone }, ErrProfileFieldUnset},
		"classical policy":   {func(p *ChainSecurityProfile) { p.ProofPolicyID = ProofPolicyPLONKKZGForbid }, ErrProfileFieldInvalid},
		"backend none listed": {func(p *ChainSecurityProfile) {
			p.AllowedProofBackends = append(p.AllowedProofBackends, ProofBackendNone)
		}, ErrProfileFieldInvalid},
		"no formats":          {func(p *ChainSecurityProfile) { p.AllowedProofFormats = nil }, ErrProfileFieldUnset},
		"format none listed":  {func(p *ChainSecurityProfile) { p.AllowedProofFormats = append(p.AllowedProofFormats, ProofFormatNone) }, ErrProfileFieldInvalid},
		"pairings permitted":  {func(p *ChainSecurityProfile) { p.ForbidPairings = false }, ErrProfileFieldInvalid},
		"kzg permitted":       {func(p *ChainSecurityProfile) { p.ForbidKZG = false }, ErrProfileFieldInvalid},
		"trusted setup ok":    {func(p *ChainSecurityProfile) { p.ForbidTrustedSetup = false }, ErrProfileFieldInvalid},
		"classical snarks ok": {func(p *ChainSecurityProfile) { p.ForbidClassicalSNARKs = false }, ErrProfileFieldInvalid},
		"dev proofs ok":       {func(p *ChainSecurityProfile) { p.ForbidDevProofs = false }, ErrProfileFieldInvalid},
		"fallbacks ok":        {func(p *ChainSecurityProfile) { p.ForbidFallbacks = false }, ErrProfileFieldInvalid},
		"ecdsa contract auth": {func(p *ChainSecurityProfile) { p.ForbidECDSAContractAuth = false }, ErrProfileFieldInvalid},
		"legacy hash with pq": {func(p *ChainSecurityProfile) { p.HashSuiteID = HashSuiteBLAKE3Legacy }, ErrProfileFieldInvalid},
	} {
		t.Run(name, func(t *testing.T) {
			p := StrictPQ()
			if err := p.Validate(); err != nil {
				t.Fatalf("the canonical profile does not validate; nothing below means anything: %v", err)
			}

			tc.break_(p)
			err := p.Validate()
			if err == nil {
				t.Fatal("accepted a malformed profile")
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("got %v, want %v", err, tc.want)
			}
		})
	}
}

// A nil profile is refused rather than dereferenced, and a canonical hash is
// only computable for a profile that is structurally sound.
func TestNilProfileAndUncomputableHashAreRefused(t *testing.T) {
	var p *ChainSecurityProfile
	if err := p.Validate(); !errors.Is(err, ErrProfileNil) {
		t.Errorf("nil profile: got %v, want ErrProfileNil", err)
	}

	broken := StrictPQ()
	broken.ProfileName = ""
	if _, err := broken.ComputeHash(); err == nil {
		t.Error("hashed a structurally invalid profile")
	}

	defer func() {
		if recover() == nil {
			t.Error("MustComputeHash returned instead of panicking on an invalid profile")
		}
	}()
	broken.MustComputeHash()
}

// The registry is the one place a wire ProfileID becomes a profile. Every
// canonical byte must resolve to a profile that validates, and every other byte
// — including the reserved zero — must be refused rather than defaulting to a
// posture the sender never asked for.
func TestProfileRegistryResolvesOnlyCanonicalBytes(t *testing.T) {
	for _, id := range []ProfileID{ProfileStrictPQ, ProfilePermissive, ProfileFIPS} {
		p, err := ProfileByID(id)
		if err != nil {
			t.Fatalf("ProfileByID(%v): %v", id, err)
		}
		if err := p.Validate(); err != nil {
			t.Errorf("ProfileByID(%v) gave a profile that does not validate: %v", id, err)
		}
	}

	for _, id := range []ProfileID{ProfileNone, ProfileID(0x04), ProfileID(0x80), ProfileID(0xFF)} {
		if p, err := ProfileByID(id); err == nil {
			t.Errorf("ProfileByID(0x%02x) resolved to %v; want refusal", uint8(id), p.ProfileName)
		} else if !errors.Is(err, ErrProfileUnknown) {
			t.Errorf("ProfileByID(0x%02x): got %v, want ErrProfileUnknown", uint8(id), err)
		}
	}
}

// The allowlists are allowlists: a backend or format that is not named is
// refused even when it is otherwise a perfectly good production PQ choice.
// Breaks if either check ever falls through to permitting the unlisted.
func TestUnlistedBackendAndFormatAreRefused(t *testing.T) {
	p := StrictPQ()

	if len(p.AllowedProofBackends) == 0 || len(p.AllowedProofFormats) == 0 {
		t.Fatal("the canonical profile lists nothing; this test cannot distinguish listed from unlisted")
	}
	if !p.AllowsBackend(p.AllowedProofBackends[0]) {
		t.Error("refused a backend it lists")
	}
	if !p.AllowsFormat(p.AllowedProofFormats[0]) {
		t.Error("refused a format it lists")
	}

	unlisted := StrictPQ()
	unlisted.AllowedProofBackends = nil
	unlisted.AllowedProofFormats = nil
	if unlisted.AllowsBackend(p.AllowedProofBackends[0]) {
		t.Error("permitted a backend that is not on the list")
	}
	if unlisted.AllowsFormat(p.AllowedProofFormats[0]) {
		t.Error("permitted a format that is not on the list")
	}
}

// The proof axis has two independent refusals that the field table above cannot
// reach, because each needs a value that is neither the reserved zero nor one of
// the explicitly forbidden markers: a policy that is simply not post-quantum,
// and a backend that is real but not production-grade. Both must be refused on a
// locked profile — an unproven policy or a dev backend on mainnet is the exact
// downgrade the allowlists exist to stop.
func TestNonPQPolicyAndDevBackendAreRefused(t *testing.T) {
	unprovenPolicy := StrictPQ()
	unprovenPolicy.ProofPolicyID = ProofPolicyID(0x01)
	if err := unprovenPolicy.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("policy that is not post-quantum: got %v, want ErrProfileFieldInvalid", err)
	}

	devBackend := StrictPQ()
	devBackend.AllowedProofBackends = []ProofBackendID{ProofBackendRISC0RawSTARKDev}
	if err := devBackend.Validate(); !errors.Is(err, ErrProfileFieldInvalid) {
		t.Errorf("non-production backend: got %v, want ErrProfileFieldInvalid", err)
	}
}

// The profile hash is a domain-separated SP 800-185 encoding, and its length
// prefixes must match the standard byte for byte — protocol/zchain hashes the
// same shape independently, so a divergence here silently splits two hashes that
// have to agree. The zero length is the case the standard calls out explicitly
// and the one no profile field happens to produce today.
func TestSP800185LengthPrefixesFollowTheStandard(t *testing.T) {
	for x, want := range map[uint64]string{
		0:     "\x01\x00",
		1:     "\x01\x01",
		384:   "\x02\x01\x80",
		65536: "\x03\x01\x00\x00",
	} {
		if got := string(leftEncodeSP800185Profile(x)); got != want {
			t.Errorf("leftEncode(%d) = % x, want % x", x, got, want)
		}
	}

	for x, want := range map[uint64]string{
		0:     "\x00\x01",
		1:     "\x01\x01",
		384:   "\x01\x80\x02",
		65536: "\x01\x00\x00\x03",
	} {
		if got := string(rightEncodeSP800185Profile(x)); got != want {
			t.Errorf("rightEncode(%d) = % x, want % x", x, got, want)
		}
	}
}
