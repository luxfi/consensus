package prism

import (
	"context"
	"testing"

	"github.com/luxfi/ids"
)

func TestProposeAddsVertex(t *testing.T) {
	cfg := DefaultConfig
	e := New(&cfg)

	ctx := context.Background()
	err := e.Propose(ctx, []byte("tx-1"))
	if err != nil {
		t.Fatalf("Propose failed: %v", err)
	}

	frontier := e.Frontier()
	if len(frontier) != 1 {
		t.Fatalf("Expected 1 frontier vertex, got %d", len(frontier))
	}

	p, err := e.GetProposal(ctx, frontier[0])
	if err != nil {
		t.Fatalf("GetProposal failed: %v", err)
	}
	if string(p.Data) != "tx-1" {
		t.Errorf("Expected data 'tx-1', got %q", string(p.Data))
	}
	if p.Height != 1 {
		t.Errorf("Expected height 1, got %d", p.Height)
	}
	if p.Accepted {
		t.Error("Proposal should not be accepted before votes")
	}
}

func TestProposeEmptyDataRejected(t *testing.T) {
	cfg := DefaultConfig
	e := New(&cfg)

	err := e.Propose(context.Background(), nil)
	if err != ErrEmptyData {
		t.Fatalf("Expected ErrEmptyData, got %v", err)
	}

	err = e.Propose(context.Background(), []byte{})
	if err != ErrEmptyData {
		t.Fatalf("Expected ErrEmptyData for empty slice, got %v", err)
	}
}

func TestProposeNotInitialized(t *testing.T) {
	e := &Engine{}

	err := e.Propose(context.Background(), []byte("data"))
	if err != ErrNotInitialized {
		t.Fatalf("Expected ErrNotInitialized, got %v", err)
	}
}

func TestVoteFinalizesProposal(t *testing.T) {
	cfg := DefaultConfig
	cfg.AlphaConfidence = 3

	e := New(&cfg)
	ctx := context.Background()

	if err := e.Propose(ctx, []byte("tx-1")); err != nil {
		t.Fatal(err)
	}

	frontier := e.Frontier()
	id := frontier[0]

	// Vote below threshold
	for i := 0; i < 2; i++ {
		if err := e.Vote(ctx, id, true); err != nil {
			t.Fatal(err)
		}
	}

	p, _ := e.GetProposal(ctx, id)
	if p.Accepted {
		t.Error("Proposal should not be accepted with only 2 votes (threshold=3)")
	}
	if p.Votes != 2 {
		t.Errorf("Expected 2 votes, got %d", p.Votes)
	}

	// Third vote should finalize
	if err := e.Vote(ctx, id, true); err != nil {
		t.Fatal(err)
	}

	p, _ = e.GetProposal(ctx, id)
	if !p.Accepted {
		t.Error("Proposal should be accepted after 3 votes (threshold=3)")
	}

	// Frontier should be empty now
	if len(e.Frontier()) != 0 {
		t.Errorf("Frontier should be empty after finalization, got %d", len(e.Frontier()))
	}
}

func TestVoteRejectDoesNotCount(t *testing.T) {
	cfg := DefaultConfig
	cfg.AlphaConfidence = 2

	e := New(&cfg)
	ctx := context.Background()

	if err := e.Propose(ctx, []byte("tx-1")); err != nil {
		t.Fatal(err)
	}

	id := e.Frontier()[0]

	// Reject vote should not increment
	if err := e.Vote(ctx, id, false); err != nil {
		t.Fatal(err)
	}

	p, _ := e.GetProposal(ctx, id)
	if p.Votes != 0 {
		t.Errorf("Reject vote should not increment, got %d", p.Votes)
	}
}

func TestVoteProposalNotFound(t *testing.T) {
	cfg := DefaultConfig
	e := New(&cfg)

	err := e.Vote(context.Background(), ids.Empty, true)
	if err != ErrProposalNotFound {
		t.Fatalf("Expected ErrProposalNotFound, got %v", err)
	}
}

func TestVoteAlreadyFinalized(t *testing.T) {
	cfg := DefaultConfig
	cfg.AlphaConfidence = 1

	e := New(&cfg)
	ctx := context.Background()

	if err := e.Propose(ctx, []byte("tx-1")); err != nil {
		t.Fatal(err)
	}

	id := e.Frontier()[0]

	// First vote finalizes
	if err := e.Vote(ctx, id, true); err != nil {
		t.Fatal(err)
	}

	// Second vote should return ErrAlreadyVoted
	err := e.Vote(ctx, id, true)
	if err != ErrAlreadyVoted {
		t.Fatalf("Expected ErrAlreadyVoted, got %v", err)
	}
}

func TestGetProposalNotFound(t *testing.T) {
	cfg := DefaultConfig
	e := New(&cfg)

	_, err := e.GetProposal(context.Background(), ids.Empty)
	if err != ErrProposalNotFound {
		t.Fatalf("Expected ErrProposalNotFound, got %v", err)
	}
}

func TestMultipleProposals(t *testing.T) {
	cfg := DefaultConfig
	cfg.AlphaConfidence = 1

	e := New(&cfg)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := e.Propose(ctx, []byte{byte(i)}); err != nil {
			t.Fatalf("Propose %d failed: %v", i, err)
		}
	}

	frontier := e.Frontier()
	if len(frontier) != 5 {
		t.Fatalf("Expected 5 frontier vertices, got %d", len(frontier))
	}

	// Finalize first 3
	for i := 0; i < 3; i++ {
		if err := e.Vote(ctx, frontier[i], true); err != nil {
			t.Fatalf("Vote %d failed: %v", i, err)
		}
	}

	remaining := e.Frontier()
	if len(remaining) != 2 {
		t.Errorf("Expected 2 remaining frontier vertices, got %d", len(remaining))
	}

	snap := e.metrics.Snapshot()
	if snap.VerticesCreated != 5 {
		t.Errorf("Expected 5 vertices created, got %d", snap.VerticesCreated)
	}
	if snap.VerticesFinalized != 3 {
		t.Errorf("Expected 3 vertices finalized, got %d", snap.VerticesFinalized)
	}
}

func TestInitializeResetsState(t *testing.T) {
	cfg := DefaultConfig
	e := New(&cfg)
	ctx := context.Background()

	if err := e.Propose(ctx, []byte("tx-1")); err != nil {
		t.Fatal(err)
	}

	// Re-initialize should clear state
	cfg2 := DefaultConfig
	cfg2.AlphaConfidence = 5
	if err := e.Initialize(ctx, &cfg2); err != nil {
		t.Fatal(err)
	}

	if len(e.Frontier()) != 0 {
		t.Error("Frontier should be empty after Initialize")
	}
	if e.config.AlphaConfidence != 5 {
		t.Errorf("Config should be updated, got AlphaConfidence=%d", e.config.AlphaConfidence)
	}
}
