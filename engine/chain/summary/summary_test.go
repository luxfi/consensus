// Copyright (C) 2019-2026, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// summary_test.go — drives the round against an in-memory beacon set and VM. Each
// case describes a world: which summaries exist, which beacons offer them, how the
// stake votes, and what the VM does with the winner. The assertions are the two
// things the caller acts on — the Outcome, and which summary (if any) had its state
// destroyed.
package summary

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/database"
	"github.com/luxfi/ids"
)

const (
	// none addresses "no interrupted sync" and "nothing adopted".
	none = -1
	// junk is an offer whose bytes the VM cannot read.
	junk = -2
)

var errWorld = errors.New("the world said no")

// testSummary is an identity-preserving summary: its bytes name it, so a parse
// round-trip recovers the same id — the property ratification counts on.
type testSummary struct {
	id        ids.ID
	height    uint64
	bytes     []byte
	mode      block.StateSyncMode
	acceptErr error

	accepts int // observed: how many times the state was destroyed
}

func (s *testSummary) ID() ids.ID     { return s.id }
func (s *testSummary) Height() uint64 { return s.height }
func (s *testSummary) Bytes() []byte  { return s.bytes }
func (s *testSummary) Accept(context.Context) (block.StateSyncMode, error) {
	s.accepts++
	if s.acceptErr != nil {
		return block.StateSyncSkipped, s.acceptErr
	}
	return s.mode, nil
}

// tipBlock is the local last-accepted block; only its height is read.
type tipBlock struct{ height uint64 }

func (b *tipBlock) ID() ids.ID                   { return ids.Empty }
func (b *tipBlock) Parent() ids.ID               { return ids.Empty }
func (b *tipBlock) ParentID() ids.ID             { return ids.Empty }
func (b *tipBlock) Height() uint64               { return b.height }
func (b *tipBlock) Timestamp() time.Time         { return time.Unix(int64(b.height), 0) }
func (b *tipBlock) Status() uint8                { return 0 }
func (b *tipBlock) Verify(context.Context) error { return nil }
func (b *tipBlock) Accept(context.Context) error { return nil }
func (b *tipBlock) Reject(context.Context) error { return nil }
func (b *tipBlock) Bytes() []byte                { return nil }

// testVM is the local VM: a tip height, an optional interrupted sync, and a registry
// that turns offered bytes back into the summary that produced them.
type testVM struct {
	tip       uint64
	tipErr    error
	blockErr  error
	resume    block.StateSummary
	resumeErr error
	known     map[string]*testSummary
}

func (v *testVM) Tip(context.Context) (uint64, error) {
	if v.tipErr != nil {
		return 0, v.tipErr
	}
	if v.blockErr != nil {
		return 0, v.blockErr
	}
	return v.tip, nil
}

func (v *testVM) GetOngoingSyncStateSummary(context.Context) (block.StateSummary, error) {
	switch {
	case v.resumeErr != nil:
		return nil, v.resumeErr
	case v.resume == nil:
		return nil, database.ErrNotFound
	}
	return v.resume, nil
}

func (v *testVM) ParseStateSummary(_ context.Context, b []byte) (block.StateSummary, error) {
	s, ok := v.known[string(b)]
	if !ok {
		return nil, errors.New("unreadable summary")
	}
	return s, nil
}

// testNet is the beacon set: what it offers, how it votes, and what it was asked.
type testNet struct {
	offers     []Offer
	offersErr  error
	ballots    []Ballot
	total      uint64
	ballotsErr error

	asked [][]uint64 // observed: the height lists ratification was run over
}

func (n *testNet) Offers(context.Context) ([]Offer, error) {
	if n.offersErr != nil {
		return nil, n.offersErr
	}
	return n.offers, nil
}

func (n *testNet) Ballots(_ context.Context, heights []uint64) ([]Ballot, uint64, error) {
	n.asked = append(n.asked, heights)
	if n.ballotsErr != nil {
		return nil, 0, n.ballotsErr
	}
	return n.ballots, n.total, nil
}

var (
	_ VM[block.StateSummary] = (*testVM)(nil)
	_ Source                 = (*testNet)(nil)
)

// voter is one beacon in the ratification round.
type voter struct {
	weight uint64
	holds  []int // indices into the case's heights
	twice  bool  // send the same ballot again, under the same identity
}

func TestRun(t *testing.T) {
	tests := []struct {
		name string

		heights []uint64 // the summaries this world contains, addressed by index
		tip     uint64   // the height this node already stands at
		offers  []int    // what discovery returns: an index, or junk
		resume  int      // the interrupted sync, or none
		voters  []voter
		total   uint64 // whole-beacon-set stake; 0 means "the voters are the set"

		offersErr  error
		ballotsErr error
		tipErr     error
		resumeErr  error
		mode       block.StateSyncMode // what Accept returns; zero value is Skipped
		acceptErr  error

		wantOutcome Outcome
		wantErr     bool
		wantAdopted int      // the index whose state was destroyed, or none
		wantAsked   []uint64 // the heights ratification ran over; nil to skip the check
		wantNoVote  bool     // ratification must never have been reached
	}{
		{
			// Every beacon stayed silent. Nothing to ratify, nothing destroyed, and the
			// caller bootstraps from where it stands.
			name:        "no beacon answers",
			heights:     []uint64{100},
			offers:      nil,
			resume:      none,
			wantOutcome: OutcomeSkipped,
			wantAdopted: none,
			wantNoVote:  true,
		},
		{
			// Not being able to ASK is the one condition that is not an answer: the node
			// has learned nothing about the network, so it must not conclude anything.
			name:        "the beacons cannot be asked",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			offersErr:   errWorld,
			wantOutcome: OutcomeInvalid,
			wantErr:     true,
			wantAdopted: none,
			wantNoVote:  true,
		},
		{
			name:        "a clear majority adopts",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}},
			mode:        block.StateSyncStatic,
			wantOutcome: OutcomeAdopted,
			wantAdopted: 0,
			wantAsked:   []uint64{100},
		},
		{
			// 30 of 50 answering stake is a majority and not a supermajority. The votes
			// split across two summaries and neither clears ⅔, which is terminal: asking
			// the same beacons again returns the same split.
			name:        "support below the ⅔ bar adopts nothing",
			heights:     []uint64{100, 200},
			offers:      []int{0, 1},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{1}}, {weight: 10, holds: []int{1}}},
			wantOutcome: OutcomeSkipped,
			wantAdopted: none,
			wantAsked:   []uint64{100, 200},
		},
		{
			// Unanimous among those who spoke, but they hold a fifth of the stake. A
			// denominator that small is exactly what an eclipse manufactures, so ⅔ of it
			// means nothing.
			name:        "too little stake answers to judge",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}},
			total:       100,
			wantOutcome: OutcomeSkipped,
			wantAdopted: none,
		},
		{
			// The network ratified it and the VM still said no. The summary was offered —
			// Accept ran — but nothing was destroyed, so the caller bootstraps.
			name:        "the VM refuses the summary",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}},
			mode:        block.StateSyncSkipped,
			wantOutcome: OutcomeSkipped,
			wantAdopted: 0,
		},
		{
			// Restarted mid-sync. Index 0 is the summary this node was already fetching;
			// index 1 is taller. Both are ratified, and the partly-fetched trie wins —
			// resuming is the whole reason the VM remembers it. Note the request covers
			// both heights: the interrupted sync stands for ratification like any other
			// candidate, even though no beacon offered it.
			name:        "an interrupted sync is resumed over a taller summary",
			heights:     []uint64{100, 200},
			offers:      []int{1},
			resume:      0,
			voters:      []voter{{weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{0, 1}}},
			mode:        block.StateSyncStatic,
			wantOutcome: OutcomeAdopted,
			wantAdopted: 0,
			wantAsked:   []uint64{100, 200},
		},
		{
			// Same restart, but the network has moved past what this node was fetching.
			// Preference is not a veto: an abandoned sync loses to the ratified summary.
			name:        "an abandoned sync loses to the ratified summary",
			heights:     []uint64{100, 200},
			offers:      []int{1},
			resume:      0,
			voters:      []voter{{weight: 10, holds: []int{1}}, {weight: 10, holds: []int{1}}, {weight: 10, holds: []int{1}}, {weight: 10, holds: []int{1}}},
			mode:        block.StateSyncStatic,
			wantOutcome: OutcomeAdopted,
			wantAdopted: 1,
		},
		{
			// The whole beacon set agrees on a summary this node is already past. Adopting
			// it would trade real history for nothing, so it never reaches a vote.
			name:        "a summary at the local tip is refused before any vote",
			heights:     []uint64{100},
			tip:         100,
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}},
			wantOutcome: OutcomeSkipped,
			wantAdopted: none,
			wantNoVote:  true,
		},
		{
			name:        "an unreadable reply drops only that summary",
			heights:     []uint64{100},
			offers:      []int{junk, 0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}},
			mode:        block.StateSyncStatic,
			wantOutcome: OutcomeAdopted,
			wantAdopted: 0,
		},
		{
			// Index 1 was never offered, so it is not a candidate. Beacons naming it are
			// naming nothing: were their stake counted, 50 of 50 would carry it. Index 0
			// holds 30 of 50 and falls short on its own.
			name:        "a beacon cannot introduce a candidate during the vote",
			heights:     []uint64{100, 200},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{1}}, {weight: 10, holds: []int{1}}},
			wantOutcome: OutcomeSkipped,
			wantAdopted: none,
			wantAsked:   []uint64{100},
		},
		{
			// One beacon, one voice. Counted twice, 30 of 50 becomes 60 of 80 and carries
			// the summary; counted once it falls short.
			name:        "a beacon cannot vote twice",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 30, holds: []int{0}, twice: true}, {weight: 10}, {weight: 10}},
			wantOutcome: OutcomeSkipped,
			wantAdopted: none,
		},
		{
			// Naming the same summary twice in one reply is the same trick inside a single
			// ballot.
			name:        "naming a summary twice does not double a beacon's stake",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 30, holds: []int{0, 0}}, {weight: 10}, {weight: 10}},
			total:       50,
			wantOutcome: OutcomeSkipped,
			wantAdopted: none,
		},
		{
			// Real stake cannot overflow a uint64. Saturating would hand the round an
			// infinitely trusted summary, so the arithmetic fault refuses instead.
			name:        "overflowing stake is refused, not saturated",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: ^uint64(0), holds: []int{0}}, {weight: 1, holds: []int{0}}},
			total:       1000,
			wantOutcome: OutcomeSkipped,
			wantAdopted: none,
		},
		{
			name:        "the taller of two ratified summaries wins",
			heights:     []uint64{100, 200},
			offers:      []int{0, 1},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{0, 1}}, {weight: 10, holds: []int{0, 1}}},
			mode:        block.StateSyncStatic,
			wantOutcome: OutcomeAdopted,
			wantAdopted: 1,
		},
		{
			// A VM that fetches in the background still leaves the state below the summary
			// incomplete, and bootstrap re-executes against exactly that state. One path:
			// adopt and wait.
			name:        "a background fetch waits like any other",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}},
			mode:        block.StateSyncDynamic,
			wantOutcome: OutcomeAdopted,
			wantAdopted: 0,
		},
		{
			// The state is gone but nothing promises this VM will ever signal that it
			// finished. Bootstrapping is bounded; waiting on that signal is not.
			name:        "an unknown mode bootstraps rather than waits",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}},
			mode:        block.StateSyncMode(99),
			wantOutcome: OutcomeSkipped,
			wantAdopted: 0,
		},
		{
			// Accept is the call that discards local history. If it fails we cannot say
			// what survived, so the caller hears about it instead of bootstrapping over it.
			name:        "a failed adoption is surfaced",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			voters:      []voter{{weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}, {weight: 10, holds: []int{0}}},
			acceptErr:   errWorld,
			wantOutcome: OutcomeInvalid,
			wantErr:     true,
			wantAdopted: 0,
		},
		{
			name:        "ratification cannot be asked",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			ballotsErr:  errWorld,
			wantOutcome: OutcomeInvalid,
			wantErr:     true,
			wantAdopted: none,
		},
		{
			name:        "the local tip cannot be read",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			tipErr:      errWorld,
			wantOutcome: OutcomeInvalid,
			wantErr:     true,
			wantAdopted: none,
			wantNoVote:  true,
		},
		{
			// Having no interrupted sync is ordinary; failing to answer whether there is
			// one is not, and the round refuses to guess.
			name:        "the interrupted sync cannot be read",
			heights:     []uint64{100},
			offers:      []int{0},
			resume:      none,
			resumeErr:   errWorld,
			wantOutcome: OutcomeInvalid,
			wantErr:     true,
			wantAdopted: none,
			wantNoVote:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			summaries := make([]*testSummary, len(tt.heights))
			vm := &testVM{
				tip:       tt.tip,
				tipErr:    tt.tipErr,
				resumeErr: tt.resumeErr,
				known:     map[string]*testSummary{},
			}
			for i, h := range tt.heights {
				id := ids.GenerateTestID()
				s := &testSummary{
					id:        id,
					height:    h,
					bytes:     []byte("summary@" + strconv.FormatUint(h, 10) + ":" + id.String()),
					mode:      tt.mode,
					acceptErr: tt.acceptErr,
				}
				summaries[i] = s
				vm.known[string(s.bytes)] = s
			}
			if tt.resume != none {
				vm.resume = summaries[tt.resume]
			}

			net := &testNet{offersErr: tt.offersErr, ballotsErr: tt.ballotsErr, total: tt.total}
			for _, i := range tt.offers {
				offer := Offer{NodeID: ids.GenerateTestNodeID()}
				if i == junk {
					offer.Bytes = []byte("bytes no VM can read")
				} else {
					offer.Bytes = summaries[i].bytes
				}
				net.offers = append(net.offers, offer)
			}
			for _, v := range tt.voters {
				ballot := Ballot{NodeID: ids.GenerateTestNodeID(), Weight: v.weight}
				for _, i := range v.holds {
					ballot.Held = append(ballot.Held, summaries[i].id)
				}
				net.ballots = append(net.ballots, ballot)
				if v.twice {
					net.ballots = append(net.ballots, ballot)
				}
				if tt.total == 0 {
					net.total += v.weight
				}
			}

			outcome, err := New(Config[block.StateSummary]{Source: net, VM: vm}).Run(context.Background())

			if tt.wantErr && err == nil {
				t.Fatalf("want an error, got outcome %s", outcome)
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if outcome != tt.wantOutcome {
				t.Errorf("outcome = %s, want %s", outcome, tt.wantOutcome)
			}
			for i, s := range summaries {
				want := 0
				if i == tt.wantAdopted {
					want = 1
				}
				if s.accepts != want {
					t.Errorf("summary %d (height %d) was handed to the VM %d times, want %d", i, s.height, s.accepts, want)
				}
			}
			switch {
			case tt.wantNoVote && len(net.asked) != 0:
				t.Errorf("ratification ran over %v; it must not have been reached", net.asked)
			case tt.wantAsked != nil:
				if len(net.asked) != 1 {
					t.Fatalf("ratification ran %d times, want once", len(net.asked))
				}
				if got := net.asked[0]; !equalHeights(got, tt.wantAsked) {
					t.Errorf("ratification ran over %v, want %v", got, tt.wantAsked)
				}
			}
		})
	}
}

// TestRunRefusesEveryDenominatorBelowMajority walks the turnout floor across the whole
// range on a set that is otherwise unanimous, so the only thing deciding the outcome is
// how much of the beacon set spoke. Unanimity among a minority is what an eclipse
// produces; the floor is the line where it stops counting.
func TestRunRefusesEveryDenominatorBelowMajority(t *testing.T) {
	const total = 100
	for answered := uint64(0); answered <= total; answered += 10 {
		s := &testSummary{
			id:     ids.GenerateTestID(),
			height: 100,
			bytes:  []byte("summary@100"),
			mode:   block.StateSyncStatic,
		}
		vm := &testVM{known: map[string]*testSummary{string(s.bytes): s}}
		net := &testNet{
			offers: []Offer{{NodeID: ids.GenerateTestNodeID(), Bytes: s.bytes}},
			total:  total,
		}
		if answered > 0 {
			net.ballots = []Ballot{{NodeID: ids.GenerateTestNodeID(), Weight: answered, Held: []ids.ID{s.id}}}
		}

		outcome, err := New(Config[block.StateSummary]{Source: net, VM: vm}).Run(context.Background())
		if err != nil {
			t.Fatalf("answered=%d: unexpected error: %v", answered, err)
		}
		want := OutcomeSkipped
		if answered > total/2 {
			want = OutcomeAdopted
		}
		if outcome != want {
			t.Errorf("answered=%d of %d: outcome = %s, want %s", answered, total, outcome, want)
		}
	}
}

// TestOutcomeZeroValueIsNotAdopted pins the fail-safe: the caller reads this value to
// decide whether to wait for a VM that is fetching state. An uninitialized Outcome must
// never send it down that path.
func TestOutcomeZeroValueIsNotAdopted(t *testing.T) {
	var zero Outcome
	if zero == OutcomeAdopted {
		t.Fatal("the zero Outcome reads as adopted")
	}
	if zero != OutcomeInvalid {
		t.Fatalf("the zero Outcome is %s, want invalid", zero)
	}
}

func equalHeights(a, b []uint64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// The paths below are the ones no case above reached. Each is an inversion: it
// asserts what must NOT happen, because every one of them fails by looking like an
// ordinary skip.

// TestOutcomeStringsAreDistinct: an operator reads these out of a log, and two that
// print alike are indistinguishable exactly where it matters. The zero value must
// not borrow a real outcome's name.
func TestOutcomeStringsAreDistinct(t *testing.T) {
	for _, c := range []struct {
		o    Outcome
		want string
	}{
		{OutcomeAdopted, "adopted"},
		{OutcomeSkipped, "skipped"},
		{OutcomeInvalid, "invalid"},
		{Outcome(99), "invalid"},
	} {
		if got := c.o.String(); got != c.want {
			t.Fatalf("Outcome(%d) printed %q, want %q", c.o, got, c.want)
		}
	}
}

// TestTipBlockUnreadableRefusesTheRound: the local tip bounds which candidates are
// worth adopting. Failing to read the BLOCK — as distinct from its id — must refuse
// the round rather than proceed with a zero tip, which would make every candidate
// look taller than local state and adopt one on a node already ahead of it.
func TestTipBlockUnreadableRefusesTheRound(t *testing.T) {
	boom := errors.New("tip block missing from the store")
	vm := &testVM{tip: 10, blockErr: boom, known: map[string]*testSummary{}}
	net := &testNet{}

	out, err := New(Config[block.StateSummary]{Source: net, VM: vm}).Run(context.Background())
	if !errors.Is(err, boom) {
		t.Fatalf("error = %v, want the tip-block failure", err)
	}
	if out != OutcomeInvalid {
		t.Fatalf("outcome = %v, want invalid: a round that could not read the local tip reported a real outcome", out)
	}
	if len(net.asked) != 0 {
		t.Fatal("ratification ran without knowing the local tip")
	}
}

// TestDuplicateHeightsAreAskedOnce: two beacons holding different summaries at one
// height is ordinary — honest peers differ on content, not on height. The request
// must carry that height once, or a beacon votes on the same number twice.
func TestDuplicateHeightsAreAskedOnce(t *testing.T) {
	a := &testSummary{id: ids.GenerateTestID(), height: 40, bytes: []byte("a")}
	b := &testSummary{id: ids.GenerateTestID(), height: 40, bytes: []byte("b")}
	c := &testSummary{id: ids.GenerateTestID(), height: 50, bytes: []byte("c")}

	vm := &testVM{tip: 1, known: map[string]*testSummary{"a": a, "b": b, "c": c}}
	net := &testNet{offers: []Offer{
		{NodeID: ids.GenerateTestNodeID(), Bytes: a.bytes},
		{NodeID: ids.GenerateTestNodeID(), Bytes: b.bytes},
		{NodeID: ids.GenerateTestNodeID(), Bytes: c.bytes},
	}}

	if _, err := New(Config[block.StateSummary]{Source: net, VM: vm}).Run(context.Background()); err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(net.asked) != 1 {
		t.Fatalf("ratification ran %d times, want 1", len(net.asked))
	}
	got := net.asked[0]
	if len(got) != 2 || got[0] != 40 || got[1] != 50 {
		t.Fatalf("asked about %v, want [40 50]: a height offered twice was requested twice", got)
	}
}

// TestEqualHeightsElectTheSameWinnerEveryRun: two summaries that both ratify at one
// height must elect the same one on every run. Ordering on map iteration passes a
// hundred times and then diverges two nodes onto different state.
func TestEqualHeightsElectTheSameWinnerEveryRun(t *testing.T) {
	winner := ""
	for i := 0; i < 32; i++ {
		lo := &testSummary{id: ids.ID{0x01}, height: 77, bytes: []byte("lo"), mode: block.StateSyncStatic}
		hi := &testSummary{id: ids.ID{0xff}, height: 77, bytes: []byte("hi"), mode: block.StateSyncStatic}
		vm := &testVM{tip: 1, known: map[string]*testSummary{"lo": lo, "hi": hi}}

		one, two := ids.GenerateTestNodeID(), ids.GenerateTestNodeID()
		net := &testNet{
			offers: []Offer{
				{NodeID: one, Bytes: lo.bytes},
				{NodeID: two, Bytes: hi.bytes},
			},
			ballots: []Ballot{
				{NodeID: one, Weight: 60, Held: []ids.ID{lo.id, hi.id}},
				{NodeID: two, Weight: 40, Held: []ids.ID{lo.id, hi.id}},
			},
			total: 100,
		}

		out, err := New(Config[block.StateSummary]{Source: net, VM: vm}).Run(context.Background())
		if err != nil {
			t.Fatalf("run: %v", err)
		}
		if out != OutcomeAdopted {
			t.Fatalf("outcome = %v, want adopted", out)
		}

		got := "lo"
		if hi.accepts > 0 {
			got = "hi"
		}
		if winner == "" {
			winner = got
			continue
		}
		if got != winner {
			t.Fatalf("run %d elected %q after %q — the winner depends on map order, "+
				"so two nodes can adopt different state at the same height", i, got, winner)
		}
	}
}
