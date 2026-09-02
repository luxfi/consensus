// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// cert_reject_table_test.go — every refusal a certificate can be given, one row
// each, named by the clause that gives it.
//
// The predicate in Verify and VerifyWeighted IS the finality rule, and each of
// its clauses returns its own error precisely so one refusal can be told from
// another. A test that only asked "did this cert verify?" would pass with the
// whole predicate replaced by `return errors.New("no")`, which is the shape of
// test that measures nothing. So every row here starts from a certificate proven
// valid by the control above it, changes exactly ONE thing, and names the error
// the clause is required to answer with.
//
// The same table exists in Rust (Cert::verify) and in C++ (verify_cert). Three
// implementations refusing the same certificate for three different reasons is a
// fork waiting for the one cert they disagree about, so the rows are meant to be
// read side by side.
package chain

import (
	"errors"
	"testing"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// answers is a StakeSource whose four numbers are each set by the test, so a row
// can state a source that DISAGREES with itself — one reporting stake it has no
// signers for, or signers it has no stake for. A real source keeps the four in
// step; the fail-closed clauses exist for the one that does not, and stating an
// inconsistent source is the only way to reach them.
type answers struct {
	weight      map[ids.NodeID]uint64
	signerStake uint64
	signers     int
	carried     uint64
}

func (a answers) Weight(n ids.NodeID, _ uint64) uint64 { return a.weight[n] }
func (a answers) SignerStake(uint64) uint64            { return a.signerStake }
func (a answers) SignerCount(uint64) int               { return a.signers }
func (a answers) CarriedStake(uint64) uint64           { return a.carried }

// equalStake is the honest source for a set of n unit-weight signers: the four
// numbers agree, which is what makes it usable as the control.
func equalStake(vs *testValidatorSet) answers {
	a := answers{weight: make(map[ids.NodeID]uint64, len(vs.ids))}
	for _, id := range vs.ids {
		a.weight[id] = 1
	}
	a.signerStake = uint64(len(vs.ids))
	a.signers = len(vs.ids)
	a.carried = a.signerStake
	return a
}

// certFixture is a valid Quasar certificate over a four-validator set, and the
// pieces a row needs to build a different one.
type certFixture struct {
	vs    *testValidatorSet
	pos   VotePosition
	votes []SignedVote
	stake answers
}

func newCertFixture(t *testing.T, n int) certFixture {
	t.Helper()
	vs := newTestValidatorSet(n)
	pos := VotePosition{
		ChainID:            ids.GenerateTestID(),
		Height:             7,
		Round:              1,
		BlockID:            ids.GenerateTestID(),
		ParentID:           ids.GenerateTestID(),
		CanonicalID:        ids.GenerateTestID(),
		ParentCanonicalID:  ids.GenerateTestID(),
		ExecutionStateRoot: ids.GenerateTestID(),
		PayloadRoot:        ids.GenerateTestID(),
	}
	votes := make([]SignedVote, 0, n)
	for i := 0; i < n; i++ {
		votes = append(votes, SignedVote{NodeID: vs.nodeID(i), Accept: true, Signature: vs.sign(i, pos)})
	}
	return certFixture{vs: vs, pos: pos, votes: votes, stake: equalStake(vs)}
}

// cert assembles a certificate from the first k of the fixture's votes.
func (f certFixture) cert(t *testing.T, tier Finality, threshold uint32, k int) *QuorumCert {
	t.Helper()
	c, err := AssembleQuorumCert(f.pos, tier, threshold, f.votes[:k])
	if err != nil {
		t.Fatalf("assemble %s cert over %d votes: %v", tier, k, err)
	}
	return c
}

// TestTheControlCertificateVerifies is the row every refusal below is measured
// against. Without it a table of refusals proves only that the fixture is broken.
func TestTheControlCertificateVerifies(t *testing.T) {
	f := newCertFixture(t, 4)

	quasar := f.cert(t, Quasar, uint32(config.TwoThirdsCount(4)), 4)
	if err := quasar.Verify(f.vs, f.pos.Height); err != nil {
		t.Fatalf("the control quasar cert does not verify: %v", err)
	}
	if err := quasar.VerifyWeighted(f.vs, f.stake, f.pos.Height); err != nil {
		t.Fatalf("the control quasar cert does not verify weighted: %v", err)
	}

	nova := f.cert(t, Nova, uint32(NovaSignerFloor(4)), 3)
	if err := nova.VerifyWeighted(f.vs, f.stake, f.pos.Height); err != nil {
		t.Fatalf("the control nova cert does not verify weighted: %v", err)
	}
}

// TestVerifyRefusalTable walks every clause of Verify. Each row is the control
// certificate with one field changed, and names the error that clause owes.
func TestVerifyRefusalTable(t *testing.T) {
	f := newCertFixture(t, 4)
	threshold := uint32(config.TwoThirdsCount(4))

	// otherPos is a position the votes were NOT signed over: same chain, next
	// height. A signature over it is a real signature for the wrong statement.
	otherPos := f.pos
	otherPos.Height++

	for _, row := range []struct {
		holds string
		cert  func() *QuorumCert
		want  error
	}{
		{
			holds: "a nil cert is a refusal, not a nil-pointer dereference",
			cert:  func() *QuorumCert { return nil },
			want:  ErrQCNil,
		},
		{
			holds: "a cert from a future format is refused rather than read under this one",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Version = QuorumCertVersion + 1
				return c
			},
			want: ErrQCVersion,
		},
		{
			holds: "a cert claiming a role other than finality is not a finality cert",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Type = QCFinality + 1
				return c
			},
			want: ErrQCType,
		},
		{
			holds: "a tier below the two attestable rungs is refused before any signature work",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Tier = Wave
				return c
			},
			want: ErrQCUnknownTier,
		},
		{
			holds: "a tier above them is refused too — Horizon is the PQ seal, not a QuorumCert",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Tier = Horizon
				return c
			},
			want: ErrQCUnknownTier,
		},
		{
			holds: "a zero threshold is a cert that asks for no votes at all",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Threshold = 0
				return c
			},
			want: ErrQCThresholdZero,
		},
		{
			holds: "a cert carrying no votes proves nothing, whatever its threshold says",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Votes = nil
				return c
			},
			want: ErrQCNoVotes,
		},
		{
			holds: "one voter counted twice is not two voters",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Votes[1] = c.Votes[0]
				return c
			},
			want: ErrQCNotStrictlyIncreasing,
		},
		{
			holds: "votes out of canonical order are refused, so a cert has one spelling",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Votes[0], c.Votes[1] = c.Votes[1], c.Votes[0]
				return c
			},
			want: ErrQCNotStrictlyIncreasing,
		},
		{
			holds: "a finality cert carries accept votes only — a reject inside one is a lie",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Votes[2].Accept = false
				return c
			},
			want: ErrQCVoteNotAccept,
		},
		{
			holds: "a signature over a different position does not certify this one",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Votes[1].Signature = f.vs.sign(1, otherPos)
				return c
			},
			want: ErrQCSigInvalid,
		},
		{
			holds: "a voter the verifier cannot resolve contributes nothing",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				// Keep the order strictly increasing by moving the LAST voter's
				// id upward, so the substitution is caught by the signature
				// clause and not by the ordering one above it.
				last := len(c.Votes) - 1
				stranger := c.Votes[last].NodeID
				stranger[0] = 0xFF
				c.Votes[last].NodeID = stranger
				return c
			},
			want: ErrQCSigInvalid,
		},
		{
			holds: "a cert may not claim a quorum larger than the votes it carries",
			cert: func() *QuorumCert {
				c := f.cert(t, Quasar, threshold, 4)
				c.Threshold = uint32(len(c.Votes)) + 1
				return c
			},
			want: ErrQCBelowThreshold,
		},
	} {
		t.Run(row.holds, func(t *testing.T) {
			err := row.cert().Verify(f.vs, f.pos.Height)
			if !errors.Is(err, row.want) {
				t.Fatalf("want %v, got %v", row.want, err)
			}
		})
	}
}

// TestVerifyWithoutAVerifierFailsClosed keeps the nil verifier off the happy
// path: a cert whose signatures nothing can check is not a cert that passed.
func TestVerifyWithoutAVerifierFailsClosed(t *testing.T) {
	f := newCertFixture(t, 4)
	c := f.cert(t, Quasar, uint32(config.TwoThirdsCount(4)), 4)

	if err := c.Verify(nil, f.pos.Height); !errors.Is(err, ErrQCVerifierNil) {
		t.Fatalf("a nil verifier must be refused, got %v", err)
	}
	if err := c.VerifyWeighted(nil, f.stake, f.pos.Height); !errors.Is(err, ErrQCVerifierNil) {
		t.Fatalf("VerifyWeighted must inherit the refusal, got %v", err)
	}
}

// TestWeightedRefusalTable walks the two rungs' floors. Each row states a stake
// source and a certificate, and names the floor the pair does not clear.
func TestWeightedRefusalTable(t *testing.T) {
	f := newCertFixture(t, 4)
	honest := f.stake

	// unresolved reports stake but no signers: two thirds of no set is not a
	// number, and TwoThirdsCount(0) is 1, which would hand a lone signer a floor
	// of one. Only a source that contradicts itself this way reaches the clause.
	unresolved := honest
	unresolved.signers = 0

	// small reports a signing set of three. f = ⌊(n−1)/3⌋ is 0 there, so a
	// two-thirds supermajority tolerates no fault at all and export is refused
	// however unanimous the certificate is.
	small := honest
	small.signers = 3

	// keyless reports members but no signable stake — the door every floor is
	// read against has closed.
	keyless := honest
	keyless.signerStake = 0

	for _, row := range []struct {
		holds string
		cert  *QuorumCert
		stake StakeSource
		want  error
	}{
		{
			holds: "no stake source at all fails closed, never count-only",
			cert:  f.cert(t, Quasar, uint32(config.TwoThirdsCount(4)), 4),
			stake: nil,
			want:  ErrQCStakeBelowSupermajority,
		},
		{
			holds: "export over a set with no signable stake is refused",
			cert:  f.cert(t, Quasar, uint32(config.TwoThirdsCount(4)), 4),
			stake: keyless,
			want:  ErrQCStakeBelowSupermajority,
		},
		{
			holds: "export over an unresolved signing set is refused",
			cert:  f.cert(t, Quasar, uint32(config.TwoThirdsCount(4)), 4),
			stake: unresolved,
			want:  ErrQCBelowThreshold,
		},
		{
			holds: "export below the minimum Byzantine committee is refused however unanimous",
			cert:  f.cert(t, Quasar, uint32(config.TwoThirdsCount(4)), 4),
			stake: small,
			want:  ErrQCBelowThreshold,
		},
		{
			holds: "nova over an unresolved set cannot assert a majority of it",
			cert:  f.cert(t, Nova, uint32(NovaSignerFloor(4)), 3),
			stake: unresolved,
			want:  ErrQCBelowThreshold,
		},
		{
			holds: "nova needs its distinct-signer floor, so a stake majority cannot self-ignite",
			cert:  f.cert(t, Nova, 2, 2),
			stake: honest,
			want:  ErrQCBelowThreshold,
		},
		{
			holds: "nova over a set with no signable stake is refused",
			cert:  f.cert(t, Nova, uint32(NovaSignerFloor(4)), 3),
			stake: keyless,
			want:  ErrQCStakeBelowMajority,
		},
	} {
		t.Run(row.holds, func(t *testing.T) {
			err := row.cert.VerifyWeighted(f.vs, row.stake, f.pos.Height)
			if !errors.Is(err, row.want) {
				t.Fatalf("want %v, got %v", row.want, err)
			}
		})
	}
}

// TestExportRefusesTheSeatsWithoutTheStake is the lopsided row read the other
// way round, and the reason the export rung reads two floors instead of one: the
// three small holders are a two-thirds supermajority of the SEATS and hold three
// units of a hundred. Adding the large holder clears both.
func TestExportRefusesTheSeatsWithoutTheStake(t *testing.T) {
	f := newCertFixture(t, 4)
	lopsided := answers{weight: map[ids.NodeID]uint64{
		f.vs.nodeID(0): 97, f.vs.nodeID(1): 1, f.vs.nodeID(2): 1, f.vs.nodeID(3): 1,
	}, signerStake: 100, signers: 4, carried: 100}

	seatsOnly, err := AssembleQuorumCert(f.pos, Quasar, 3, f.votes[1:4])
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := seatsOnly.Verify(f.vs, f.pos.Height); err != nil {
		t.Fatalf("the seats-only cert must be signature-valid, or the row proves nothing: %v", err)
	}
	if err := seatsOnly.VerifyWeighted(f.vs, lopsided, f.pos.Height); !errors.Is(err, ErrQCStakeBelowSupermajority) {
		t.Fatalf("three of four seats holding 3%% of stake must not export, got %v", err)
	}

	withWeight, err := AssembleQuorumCert(f.pos, Quasar, 3, f.votes[0:3])
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := withWeight.VerifyWeighted(f.vs, lopsided, f.pos.Height); err != nil {
		t.Fatalf("three seats holding 99%% of stake must export: %v", err)
	}
}

// TestNovaRefusesTheStakeWithoutTheSeats is the Nova rung's independent guard,
// which is the same shape one rung down: the single 97-of-100 holder IS a strict
// majority of stake on its own signature, and must not ignite alone.
func TestNovaRefusesTheStakeWithoutTheSeats(t *testing.T) {
	f := newCertFixture(t, 4)
	lopsided := answers{weight: map[ids.NodeID]uint64{
		f.vs.nodeID(0): 97, f.vs.nodeID(1): 1, f.vs.nodeID(2): 1, f.vs.nodeID(3): 1,
	}, signerStake: 100, signers: 4, carried: 100}

	alone, err := AssembleQuorumCert(f.pos, Nova, 1, f.votes[0:1])
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	if err := alone.Verify(f.vs, f.pos.Height); err != nil {
		t.Fatalf("the lone-holder cert must be signature-valid: %v", err)
	}
	if err := alone.VerifyWeighted(f.vs, lopsided, f.pos.Height); !errors.Is(err, ErrQCBelowThreshold) {
		t.Fatalf("a stake majority on one signature must not ignite nova, got %v", err)
	}
}

// TestATierIsRefusedByVerifyBeforeVerifyWeightedSeesIt pins the ORDER of the two
// refusals. VerifyWeighted carries its own unknown-tier arm, and it is
// unreachable through this API precisely because Verify runs first and answers
// for the tier — this row is the proof of which of the two answers, so a future
// reordering that let a garbage tier reach the stake switch is a failing test
// and not a silent change.
func TestATierIsRefusedByVerifyBeforeVerifyWeightedSeesIt(t *testing.T) {
	f := newCertFixture(t, 4)
	c := f.cert(t, Quasar, uint32(config.TwoThirdsCount(4)), 4)
	c.Tier = Photon

	err := c.VerifyWeighted(f.vs, f.stake, f.pos.Height)
	if !errors.Is(err, ErrQCUnknownTier) {
		t.Fatalf("want the unknown-tier refusal, got %v", err)
	}
	// Verify alone gives the same answer, which is what makes the weighted arm
	// unreachable rather than merely untested.
	if err := c.Verify(f.vs, f.pos.Height); !errors.Is(err, ErrQCUnknownTier) {
		t.Fatalf("Verify must be the one that refuses the tier, got %v", err)
	}
}

// TestAssembleRefusalTable walks the assembly door. Assembly is orthogonal to
// verification — it does not check signatures — but it must never produce a cert
// that is structurally impossible, because a relaying node assembles and a
// verifier is entitled to a well-formed object.
func TestAssembleRefusalTable(t *testing.T) {
	f := newCertFixture(t, 4)

	reject := f.votes[0]
	reject.Accept = false

	for _, row := range []struct {
		holds     string
		tier      Finality
		threshold uint32
		votes     []SignedVote
		want      error
	}{
		{
			holds: "a cert attests one of the two accept rungs and no other",
			tier:  Wave, threshold: 3, votes: f.votes, want: ErrQCUnknownTier,
		},
		{
			holds: "horizon is the PQ seal layer, not a rung a QuorumCert may claim",
			tier:  Horizon, threshold: 3, votes: f.votes, want: ErrQCUnknownTier,
		},
		{
			holds: "a zero threshold is refused at assembly, not left for the verifier",
			tier:  Quasar, threshold: 0, votes: f.votes, want: ErrQCThresholdZero,
		},
		{
			holds: "there is no cert over no votes",
			tier:  Quasar, threshold: 3, votes: nil, want: ErrQCNoVotes,
		},
		{
			holds: "a reject vote never enters a finality cert",
			tier:  Quasar, threshold: 1, votes: []SignedVote{reject}, want: ErrQCVoteNotAccept,
		},
		{
			holds: "a duplicate voter is an error, never a last-writer-wins overwrite",
			tier:  Quasar, threshold: 2, votes: []SignedVote{f.votes[0], f.votes[0]},
			want: ErrQCNotStrictlyIncreasing,
		},
		{
			holds: "assembly succeeds only once the quorum is actually present",
			tier:  Quasar, threshold: 4, votes: f.votes[:3], want: ErrQCBelowThreshold,
		},
	} {
		t.Run(row.holds, func(t *testing.T) {
			c, err := AssembleQuorumCert(f.pos, row.tier, row.threshold, row.votes)
			if !errors.Is(err, row.want) {
				t.Fatalf("want %v, got %v", row.want, err)
			}
			if c != nil {
				t.Fatal("a refused assembly must return no cert — a caller must not read one out of a no")
			}
		})
	}
}

// TestAssemblySortsAndCopies holds the two properties assembly adds beyond
// refusing: the votes come out in canonical order whatever order they went in,
// and the signature bytes are the cert's own, so a caller mutating its slice
// afterwards cannot change what the cert says.
func TestAssemblySortsAndCopies(t *testing.T) {
	f := newCertFixture(t, 4)

	shuffled := []SignedVote{f.votes[3], f.votes[0], f.votes[2], f.votes[1]}
	c, err := AssembleQuorumCert(f.pos, Quasar, 3, shuffled)
	if err != nil {
		t.Fatalf("assemble: %v", err)
	}
	for i := 1; i < len(c.Votes); i++ {
		if string(c.Votes[i-1].NodeID[:]) >= string(c.Votes[i].NodeID[:]) {
			t.Fatalf("votes %d and %d are not strictly increasing", i-1, i)
		}
	}
	if err := c.Verify(f.vs, f.pos.Height); err != nil {
		t.Fatalf("a cert assembled from shuffled votes must verify: %v", err)
	}

	// The caller's slice is not the cert's.
	shuffled[0].Signature[0] ^= 0xFF
	if err := c.Verify(f.vs, f.pos.Height); err != nil {
		t.Fatalf("mutating the caller's votes changed the cert: %v", err)
	}
}

// TestVoterCountAndEqual covers the two accessors a cert store and a round trip
// lean on. Both are asked about nil, because both are called on certs that may
// not have been fetched.
func TestVoterCountAndEqual(t *testing.T) {
	f := newCertFixture(t, 4)
	c := f.cert(t, Quasar, 3, 4)

	if n := (*QuorumCert)(nil).VoterCount(); n != 0 {
		t.Fatalf("a nil cert carries no voters, got %d", n)
	}
	if n := c.VoterCount(); n != 4 {
		t.Fatalf("VoterCount is the distinct voters carried, want 4 got %d", n)
	}

	if !(*QuorumCert)(nil).Equal(nil) {
		t.Fatal("two absent certs are the same absence")
	}
	if c.Equal(nil) || (*QuorumCert)(nil).Equal(c) {
		t.Fatal("a cert is not equal to no cert, in either direction")
	}
	if !c.Equal(f.cert(t, Quasar, 3, 4)) {
		t.Fatal("assembly is deterministic — the same votes must build an equal cert")
	}

	// One changed field at a time, each of which makes it a different cert.
	for _, row := range []struct {
		holds  string
		change func(*QuorumCert)
	}{
		{"a different format is a different cert", func(o *QuorumCert) { o.Version++ }},
		{"a different role is a different cert", func(o *QuorumCert) { o.Type++ }},
		{"a different rung is a different cert", func(o *QuorumCert) { o.Tier = Nova }},
		{"a different position is a different cert", func(o *QuorumCert) { o.Position.Height++ }},
		{"a different threshold is a different cert", func(o *QuorumCert) { o.Threshold++ }},
		{"fewer votes is a different cert", func(o *QuorumCert) { o.Votes = o.Votes[:3] }},
		{"a different voter is a different cert", func(o *QuorumCert) { o.Votes[0].NodeID[0] ^= 0xFF }},
		{"a different decision is a different cert", func(o *QuorumCert) { o.Votes[0].Accept = false }},
		{"a different signature is a different cert", func(o *QuorumCert) { o.Votes[0].Signature[0] ^= 0xFF }},
	} {
		t.Run(row.holds, func(t *testing.T) {
			other := f.cert(t, Quasar, 3, 4)
			row.change(other)
			if c.Equal(other) {
				t.Fatal("Equal reported two different certs as the same")
			}
		})
	}
}

// TestAuthorizesExportIsTheCertLevelBoundary holds the projection a bridge gates
// on: a Nova cert is a valid, signature-checked majority certificate and is
// still not export-grade, and an absent cert is never export-grade.
func TestAuthorizesExportIsTheCertLevelBoundary(t *testing.T) {
	f := newCertFixture(t, 4)

	nova := f.cert(t, Nova, uint32(NovaSignerFloor(4)), 3)
	if err := nova.VerifyWeighted(f.vs, f.stake, f.pos.Height); err != nil {
		t.Fatalf("the nova cert must be valid, or the row says nothing: %v", err)
	}
	if nova.AuthorizesExport() {
		t.Fatal("a nova cert authorizes local execution only")
	}
	if !f.cert(t, Quasar, uint32(config.TwoThirdsCount(4)), 4).AuthorizesExport() {
		t.Fatal("a quasar cert is the export rung")
	}
	if (*QuorumCert)(nil).AuthorizesExport() {
		t.Fatal("no cert is not export-grade finality")
	}
}
