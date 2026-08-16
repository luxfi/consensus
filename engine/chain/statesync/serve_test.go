// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package statesync

import (
	"context"
	"errors"
	"testing"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

// summary is a served summary. Only ID and Bytes are read by the serving path; the
// rest exists so the stub satisfies the VM-facing declaration.
type summary struct {
	id     ids.ID
	height uint64
	bytes  []byte
}

func (s summary) ID() ids.ID     { return s.id }
func (s summary) Height() uint64 { return s.height }
func (s summary) Bytes() []byte  { return s.bytes }
func (s summary) Accept(context.Context) (block.StateSyncMode, error) {
	return block.StateSyncStatic, nil
}

func summaryAt(height uint64) summary {
	return summary{
		id:     ids.ID{byte(height)},
		height: height,
		bytes:  []byte{byte(height), 'b'},
	}
}

// vm serves summaries. reads records every height it was asked for, in order, so a
// test can tell "answered without looking" from "looked and found nothing", and
// syncPreferenceReads counts a consultation of the local sync preference, which the
// serving path must never make.
type vm struct {
	last                block.StateSummary
	lastErr             error
	held                map[uint64]block.StateSummary
	failAt              map[uint64]error
	reads               []uint64
	syncPreferenceReads int
}

func (v *vm) GetLastStateSummary(context.Context) (block.StateSummary, error) {
	return v.last, v.lastErr
}

func (v *vm) GetStateSummary(_ context.Context, height uint64) (block.StateSummary, error) {
	v.reads = append(v.reads, height)
	if err, failing := v.failAt[height]; failing {
		return nil, err
	}
	held, ok := v.held[height]
	if !ok {
		return nil, database.ErrNotFound
	}
	return held, nil
}

func (v *vm) StateSyncEnabled(context.Context) (bool, error) {
	v.syncPreferenceReads++
	return false, nil
}

func holding(heights ...uint64) *vm {
	v := &vm{held: map[uint64]block.StateSummary{}}
	for _, height := range heights {
		v.held[height] = summaryAt(height)
	}
	if len(heights) > 0 {
		v.last = summaryAt(heights[len(heights)-1])
	} else {
		v.lastErr = database.ErrNotFound
	}
	return v
}

func serverFor(v *vm) *Server[block.StateSummary] {
	return NewServer[block.StateSummary](v)
}

// plainVM builds and parses blocks and nothing else — the chain that will never
// state-sync and whose operator never asked it to.
type plainVM struct{}

func (plainVM) LastAccepted(context.Context) (ids.ID, error) { return ids.Empty, nil }

func silentServer() *Server[block.StateSummary] {
	return NewServer[block.StateSummary](plainVM{})
}

func TestFrontierServesTheLastSummary(t *testing.T) {
	served, err := serverFor(holding(4, 9)).Frontier(context.Background())
	if err != nil {
		t.Fatalf("a VM holding a summary must be served, got %v", err)
	}
	if want := summaryAt(9).Bytes(); string(served) != string(want) {
		t.Fatalf("expected the bytes of the last summary %x, got %x", want, served)
	}
}

func TestFrontierIsSilentWhenTheVMHoldsNoSummary(t *testing.T) {
	served, err := serverFor(holding()).Frontier(context.Background())
	if !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("a VM holding nothing must produce silence, got %v", err)
	}
	if served != nil {
		t.Fatalf("silence carries no bytes, got %x", served)
	}
}

func TestFrontierIsSilentWhenTheVMDoesNotServeSummaries(t *testing.T) {
	served, err := silentServer().Frontier(context.Background())
	if !errors.Is(err, block.ErrStateSyncableVMNotImplemented) {
		t.Fatalf("a VM that does not serve summaries must produce silence, got %v", err)
	}
	if served != nil {
		t.Fatalf("silence carries no bytes, got %x", served)
	}
}

// A VM that reports neither a summary nor an error is still holding nothing. The
// answer is silence and not a panic, because the request comes from a peer.
func TestFrontierIsSilentWhenTheVMNamesNothingAndReportsNoError(t *testing.T) {
	served, err := serverFor(&vm{}).Frontier(context.Background())
	if !errors.Is(err, database.ErrNotFound) {
		t.Fatalf("a VM naming nothing must produce silence, got %v", err)
	}
	if served != nil {
		t.Fatalf("silence carries no bytes, got %x", served)
	}
}

func TestAcceptedNamesOnlyTheHeightsHeld(t *testing.T) {
	named, err := serverFor(holding(10, 30)).Accepted(context.Background(), []uint64{10, 20, 30})
	if err != nil {
		t.Fatalf("a missing height is skipped, not fatal, got %v", err)
	}
	want := []ids.ID{summaryAt(10).ID(), summaryAt(30).ID()}
	if len(named) != len(want) {
		t.Fatalf("expected the %d heights held to be named, got %d: %v", len(want), len(named), named)
	}
	for i, id := range want {
		if named[i] != id {
			t.Fatalf("expected %s at position %d, got %s", id, i, named[i])
		}
	}
}

// Holding none of the named heights is an affirmative answer, not silence: it lets the
// requester's tally complete instead of costing it a deadline.
func TestAcceptedAffirmsNoneWhenNoNamedHeightIsHeld(t *testing.T) {
	v := holding(7)
	named, err := serverFor(v).Accepted(context.Background(), []uint64{11, 12})
	if err != nil {
		t.Fatalf("holding none of the named heights is an answer, got %v", err)
	}
	if len(named) != 0 {
		t.Fatalf("expected no ids named, got %v", named)
	}
	if len(v.reads) != 2 {
		t.Fatalf("expected both named heights to be looked up, got %v", v.reads)
	}
}

func TestAcceptedAnswersAnEmptyRequestWithoutReadingTheVM(t *testing.T) {
	v := holding(1, 2, 3)
	named, err := serverFor(v).Accepted(context.Background(), nil)
	if err != nil {
		t.Fatalf("a request naming no heights is answered, got %v", err)
	}
	if len(named) != 0 {
		t.Fatalf("expected no ids named, got %v", named)
	}
	if len(v.reads) != 0 {
		t.Fatalf("nothing was named, so nothing may be looked up, got %v", v.reads)
	}
}

func TestAcceptedAnswersAnEmptyRequestFromAVMThatDoesNotServe(t *testing.T) {
	named, err := silentServer().Accepted(context.Background(), nil)
	if err != nil {
		t.Fatalf("nothing was named, so nothing can fail, got %v", err)
	}
	if len(named) != 0 {
		t.Fatalf("expected no ids named, got %v", named)
	}
}

func TestAcceptedIsSilentWhenTheVMDoesNotServeSummaries(t *testing.T) {
	named, err := silentServer().Accepted(context.Background(), []uint64{5})
	if !errors.Is(err, block.ErrStateSyncableVMNotImplemented) {
		t.Fatalf("a VM that does not serve summaries must produce silence, got %v", err)
	}
	if named != nil {
		t.Fatalf("silence names nothing, got %v", named)
	}
}

// An empty reply asserts "I hold none of those". A VM that could not read has not
// established that, so the request is abandoned even though an earlier height was held.
func TestAcceptedAbandonsTheRequestWhenAReadFails(t *testing.T) {
	unreadable := errors.New("state store unreadable")
	v := holding(10, 30)
	v.failAt = map[uint64]error{30: unreadable}

	named, err := serverFor(v).Accepted(context.Background(), []uint64{10, 30})
	if !errors.Is(err, unreadable) {
		t.Fatalf("a read failure must abandon the request, got %v", err)
	}
	if named != nil {
		t.Fatalf("an abandoned request names nothing, got %v", named)
	}
}

func TestAcceptedReadsARepeatedHeightOnce(t *testing.T) {
	v := holding(7)
	named, err := serverFor(v).Accepted(context.Background(), []uint64{7, 7, 7})
	if err != nil {
		t.Fatalf("a repeated height is not an error, got %v", err)
	}
	if len(named) != 1 || named[0] != summaryAt(7).ID() {
		t.Fatalf("expected the height named once, got %v", named)
	}
	if len(v.reads) != 1 {
		t.Fatalf("expected one lookup for a repeated height, got %v", v.reads)
	}
}

func TestAcceptedSkipsAHeightTheVMNamesAsNothing(t *testing.T) {
	v := holding(10, 20)
	v.held[20] = nil

	named, err := serverFor(v).Accepted(context.Background(), []uint64{10, 20})
	if err != nil {
		t.Fatalf("a VM naming nothing at a height is not fatal, got %v", err)
	}
	if len(named) != 1 || named[0] != summaryAt(10).ID() {
		t.Fatalf("expected only the held height named, got %v", named)
	}
}

// A node serves its peers whatever its own sync preference is: the preference governs
// what this node does with someone else's summary, never what it hands out.
func TestServingIgnoresTheLocalSyncPreference(t *testing.T) {
	v := holding(9)
	server := serverFor(v)

	if _, err := server.Frontier(context.Background()); err != nil {
		t.Fatalf("a VM with sync disabled locally still serves, got %v", err)
	}
	if named, err := server.Accepted(context.Background(), []uint64{9}); err != nil || len(named) != 1 {
		t.Fatalf("a VM with sync disabled locally still names its heights, got %v %v", named, err)
	}
	if v.syncPreferenceReads != 0 {
		t.Fatalf("the local sync preference was consulted %d times", v.syncPreferenceReads)
	}
}

// otherSummary is the same two reads declared elsewhere: a VM built against its own
// summary interface, whose Accept does not even agree in type with this module's.
type otherSummary interface {
	ID() ids.ID
	Height() uint64
	Bytes() []byte
	Accept(context.Context) (uint8, error)
}

type otherSummaryValue struct{ id ids.ID }

func (s otherSummaryValue) ID() ids.ID                            { return s.id }
func (s otherSummaryValue) Height() uint64                        { return 1 }
func (s otherSummaryValue) Bytes() []byte                         { return []byte("elsewhere") }
func (s otherSummaryValue) Accept(context.Context) (uint8, error) { return 1, nil }

type otherVM struct{ held otherSummary }

func (v otherVM) GetLastStateSummary(context.Context) (otherSummary, error) {
	return v.held, nil
}

func (v otherVM) GetStateSummary(context.Context, uint64) (otherSummary, error) {
	return v.held, nil
}

func TestServesAVMBuiltAgainstAnotherSummaryDeclaration(t *testing.T) {
	held := otherSummaryValue{id: ids.ID{42}}
	v := otherVM{held: held}

	served, err := NewServer[otherSummary](v).Frontier(context.Background())
	if err != nil {
		t.Fatalf("a VM built against its own summary declaration must be served, got %v", err)
	}
	if string(served) != string(held.Bytes()) {
		t.Fatalf("expected %x, got %x", held.Bytes(), served)
	}

	named, err := NewServer[otherSummary](v).Accepted(context.Background(), []uint64{1})
	if err != nil || len(named) != 1 || named[0] != held.ID() {
		t.Fatalf("expected the held height named, got %v %v", named, err)
	}

	// Naming the wrong declaration is a silent server, never a wrong answer.
	if _, err := NewServer[block.StateSummary](v).Frontier(context.Background()); !errors.Is(err, block.ErrStateSyncableVMNotImplemented) {
		t.Fatalf("a mismatched summary declaration must produce silence, got %v", err)
	}
}
