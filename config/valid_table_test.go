// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// valid_table_test.go — every clause of the parameter doors, one row each.
//
// Valid is the last thing between an operator's config file and a running
// engine, and each of its clauses refuses one shape of nonsense. A table that
// only asked "was this refused?" would pass with the whole body replaced by
// `return ErrParametersInvalid`, so each row here starts from a parameter set
// proven valid by the control, changes exactly ONE field, and names the door
// that must close.
//
// ValidQuorum is the safety-critical subset — K, the two α's, and the BFT
// overlap bound — and the engine enforces it independently at Start. Valid runs
// it and then checks the operational fields around it, so the rows are split the
// same way the code is.
package config

import (
	"errors"
	"testing"
	"time"
)

// TestTheDefaultParametersAreValid is the control. Every refusal below is the
// default with one field moved, so a broken default would make the whole table
// pass for the wrong reason.
func TestTheDefaultParametersAreValid(t *testing.T) {
	p := DefaultParams()
	if err := p.ValidQuorum(); err != nil {
		t.Fatalf("the default quorum is not valid: %v", err)
	}
	if err := p.Valid(); err != nil {
		t.Fatalf("the default parameters are not valid: %v", err)
	}
}

// TestValidQuorumRefusalTable walks the safety-critical subset. The last row is
// the one the others exist to protect: two α-quorums must overlap in more than f
// nodes, or two disjoint quorums each certify a conflicting block.
func TestValidQuorumRefusalTable(t *testing.T) {
	for _, row := range []struct {
		holds  string
		change func(*Parameters)
		want   error
	}{
		{
			holds:  "a sample of no validators is not a sample",
			change: func(p *Parameters) { p.K = 0 },
			want:   ErrInvalidK,
		},
		{
			holds:  "a negative sample size is refused as the same nonsense",
			change: func(p *Parameters) { p.K = -1 },
			want:   ErrInvalidK,
		},
		{
			holds:  "a preference quorum of zero would accept on no votes",
			change: func(p *Parameters) { p.AlphaPreference = 0 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "a preference quorum larger than the sample can never be reached",
			change: func(p *Parameters) { p.AlphaPreference = p.K + 1 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "a confidence quorum of zero would confirm on no votes",
			change: func(p *Parameters) { p.AlphaConfidence = 0 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "a confidence quorum larger than the sample can never be reached",
			change: func(p *Parameters) { p.AlphaConfidence = p.K + 1 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "confidence is the stronger commitment and may not cost fewer votes than preference",
			change: func(p *Parameters) { p.AlphaConfidence = p.AlphaPreference - 1 },
			want:   ErrAlphaConfidenceBelowPreference,
		},
		{
			holds:  "an alpha below the BFT overlap bound lets two disjoint quorums certify a fork",
			change: func(p *Parameters) { p.AlphaPreference, p.AlphaConfidence = p.K/2+1, p.K/2+1 },
			want:   ErrAlphaBelowBFTQuorum,
		},
	} {
		t.Run(row.holds, func(t *testing.T) {
			p := DefaultParams()
			row.change(&p)
			if err := p.ValidQuorum(); !errors.Is(err, row.want) {
				t.Fatalf("ValidQuorum: want %v, got %v", err, row.want)
			}
			// Valid runs the quorum door first, so the same parameters are
			// refused there with the same reason — one definition, two callers.
			if err := p.Valid(); !errors.Is(err, row.want) {
				t.Fatalf("Valid: want %v, got %v", row.want, err)
			}
		})
	}
}

// TestValidRefusalTable walks the operational clauses — the ones outside the
// safety-critical subset. They bound the fields that size queues and timers, and
// each refuses only the values that make no sense rather than every value an
// operator might reasonably choose, so zero is left meaning "unset" throughout.
func TestValidRefusalTable(t *testing.T) {
	for _, row := range []struct {
		holds  string
		change func(*Parameters)
		want   error
	}{
		{
			holds:  "a threshold below two thirds is not a supermajority",
			change: func(p *Parameters) { p.Alpha = 0.5 },
			want:   ErrInvalidAlpha,
		},
		{
			holds:  "a threshold above unity asks for more votes than exist",
			change: func(p *Parameters) { p.Alpha = 1.5 },
			want:   ErrInvalidAlpha,
		},
		{
			holds:  "a confidence depth of zero accepts on the first round it sees",
			change: func(p *Parameters) { p.Beta = 0 },
			want:   ErrInvalidBeta,
		},
		{
			holds:  "a block time below a millisecond is a timer the scheduler cannot honour",
			change: func(p *Parameters) { p.BlockTime = time.Microsecond },
			want:   ErrBlockTimeTooLow,
		},
		{
			holds:  "a round shorter than a block cannot contain one",
			change: func(p *Parameters) { p.RoundTO = p.BlockTime - time.Millisecond },
			want:   ErrRoundTimeoutTooLow,
		},
		{
			holds:  "a negative virtuous depth is not a depth",
			change: func(p *Parameters) { p.BetaVirtuous = -1 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "a rogue depth below the virtuous one would confirm a contested block sooner",
			change: func(p *Parameters) { p.BetaRogue = p.BetaVirtuous - 1 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "a negative poll concurrency is not a count of polls",
			change: func(p *Parameters) { p.ConcurrentPolls = -1 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "a negative processing target is not a count of items",
			change: func(p *Parameters) { p.OptimalProcessing = -1 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "a negative outstanding-item bound is not a bound",
			change: func(p *Parameters) { p.MaxOutstandingItems = -1 },
			want:   ErrParametersInvalid,
		},
		{
			holds:  "a negative processing deadline is a deadline already past",
			change: func(p *Parameters) { p.MaxItemProcessingTime = -time.Second },
			want:   ErrParametersInvalid,
		},
	} {
		t.Run(row.holds, func(t *testing.T) {
			p := DefaultParams()
			row.change(&p)
			if err := p.Valid(); !errors.Is(err, row.want) {
				t.Fatalf("want %v, got %v", row.want, err)
			}
		})
	}
}

// TestZeroMeansUnsetForTheOperationalFields is the other half of the clauses
// above, and the reason they are written `!= 0 && < 1` rather than `< 1`: an
// operator that leaves a field out gets the engine's own default, not a refusal.
// Testing only the refusals would let the guards tighten onto zero unnoticed,
// which would reject every partially-specified config in the fleet.
func TestZeroMeansUnsetForTheOperationalFields(t *testing.T) {
	p := DefaultParams()
	p.BetaRogue = 0
	p.ConcurrentPolls = 0
	p.OptimalProcessing = 0
	p.MaxOutstandingItems = 0
	p.MaxItemProcessingTime = 0
	p.BlockTime = 0
	p.RoundTO = 0

	if err := p.Valid(); err != nil {
		t.Fatalf("an unset operational field must mean unset, not invalid: %v", err)
	}
}
