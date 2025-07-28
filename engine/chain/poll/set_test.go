// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package poll

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/utils/bag"
	log "github.com/luxfi/log"
)

var (
	blkID1 = ids.ID{1}
	blkID2 = ids.ID{2}
	blkID3 = ids.ID{3}
	blkID4 = ids.ID{4}

	vdr1 = ids.BuildTestNodeID([]byte{0x01})
	vdr2 = ids.BuildTestNodeID([]byte{0x02})
	vdr3 = ids.BuildTestNodeID([]byte{0x03})
	vdr4 = ids.BuildTestNodeID([]byte{0x04})
	vdr5 = ids.BuildTestNodeID([]byte{0x05}) // k = 5
)

func TestNewSetErrorOnPollsMetrics(t *testing.T) {
	require := require.New(t)

	alpha := 1
	factory := newEarlyTermNoTraversalTestFactory(require, alpha)
	log := log.NewNoOpLogger()
	registerer := prometheus.NewRegistry()

	require.NoError(registerer.Register(prometheus.NewCounter(prometheus.CounterOpts{
		Name: "polls",
	})))

	_, err := NewSet(factory, log, registerer)
	require.ErrorIs(err, errFailedPollsMetric)
}

func TestNewSetErrorOnPollDurationMetrics(t *testing.T) {
	require := require.New(t)

	alpha := 1
	factory := newEarlyTermNoTraversalTestFactory(require, alpha)
	log := log.NewNoOpLogger()
	registerer := prometheus.NewRegistry()

	require.NoError(registerer.Register(prometheus.NewCounter(prometheus.CounterOpts{
		Name: "poll_duration_count",
	})))

	_, err := NewSet(factory, log, registerer)
	require.ErrorIs(err, errFailedPollDurationMetrics)
}

func TestCreateAndFinishPollOutOfOrder_NewerFinishesFirst(t *testing.T) {
	require := require.New(t)

	vdrs := []ids.NodeID{vdr1, vdr2, vdr3} // k = 3
	alpha := 3

	factory := newEarlyTermNoTraversalTestFactory(require, alpha)
	log := log.NewNoOpLogger()
	registerer := prometheus.NewRegistry()
	s, err := NewSet(factory, log, registerer)
	require.NoError(err)

	// create two polls for the two blocks
	vdrBag := bag.Of(vdrs...)
	require.True(s.Add(1, vdrBag))

	vdrBag = bag.Of(vdrs...)
	require.True(s.Add(2, vdrBag))
	require.Equal(2, s.Len())

	// vote out of order
	id1, ok1 := s.Vote(1, vdr1, blkID1)
	require.False(ok1)
	require.Equal(ids.Empty, id1)
	
	id2, ok2 := s.Vote(2, vdr2, blkID2)
	require.False(ok2)
	require.Equal(ids.Empty, id2)
	
	id3, ok3 := s.Vote(2, vdr3, blkID2)
	require.False(ok3)
	require.Equal(ids.Empty, id3)

	// poll 2 finished
	id, ok := s.Vote(2, vdr1, blkID2) // expect 2 to not have finished because 1 is still pending
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(1, vdr2, blkID1)
	require.False(ok)
	require.Equal(ids.Empty, id)

	// Debug: check all poll states before poll 1 finishes
	t.Logf("Before poll 1 finishes:")
	for i := uint32(1); i <= 2; i++ {
		if holder, exists := (s.(*set)).polls.Get(i); exists {
			if p, ok := holder.(*poll); ok {
				t.Logf("  Poll %d: tryCount=%d, wasBlocked=%v, finished=%v", i, p.tryCount, p.wasBlocked, p.Finished())
			}
		}
	}
	
	id, ok = s.Vote(1, vdr3, blkID1) // poll 1 finished
	require.True(ok)
	require.Equal(blkID1, id)

	// Vote on poll 2 to finish it
	// Debug: check poll 2 state
	require.Equal(1, s.Len(), "Expected only poll 2 to remain")
	if holder, exists := (s.(*set)).polls.Get(2); exists {
		if p, ok := holder.(*poll); ok {
			t.Logf("Poll 2 state before votes: tryCount=%d, wasBlocked=%v", p.tryCount, p.wasBlocked)
		}
	}
	
	id, ok = s.Vote(2, vdr2, blkID2)
	require.False(ok)
	id, ok = s.Vote(2, vdr3, blkID2)
	require.True(ok)
	require.Equal(blkID2, id)
}

func TestCreateAndFinishPollOutOfOrder_OlderFinishesFirst(t *testing.T) {
	require := require.New(t)

	vdrs := []ids.NodeID{vdr1, vdr2, vdr3} // k = 3
	alpha := 3

	factory := newEarlyTermNoTraversalTestFactory(require, alpha)
	log := log.NewNoOpLogger()
	registerer := prometheus.NewRegistry()
	s, err := NewSet(factory, log, registerer)
	require.NoError(err)

	// create two polls for the two blocks
	vdrBag := bag.Of(vdrs...)
	require.True(s.Add(1, vdrBag))

	vdrBag = bag.Of(vdrs...)
	require.True(s.Add(2, vdrBag))
	require.Equal(2, s.Len())

	// vote out of order
	id, ok := s.Vote(1, vdr1, blkID1)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(2, vdr2, blkID2)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(2, vdr3, blkID2)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(1, vdr2, blkID1)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(1, vdr3, blkID1) // poll 1 finished, poll 2 still remaining
	require.True(ok)                  // because 1 is the oldest
	require.Equal(blkID1, id)

	id, ok = s.Vote(2, vdr1, blkID2) // poll 2 finished
	require.True(ok)                  // because 2 is the oldest now
	require.Equal(blkID2, id)
}

func TestCreateAndFinishPollOutOfOrder_UnfinishedPollsGaps(t *testing.T) {
	require := require.New(t)

	vdrs := []ids.NodeID{vdr1, vdr2, vdr3} // k = 3
	alpha := 3

	factory := newEarlyTermNoTraversalTestFactory(require, alpha)
	log := log.NewNoOpLogger()
	registerer := prometheus.NewRegistry()
	s, err := NewSet(factory, log, registerer)
	require.NoError(err)

	// create three polls for the two blocks
	vdrBag := bag.Of(vdrs...)
	require.True(s.Add(1, vdrBag))

	vdrBag = bag.Of(vdrs...)
	require.True(s.Add(2, vdrBag))

	vdrBag = bag.Of(vdrs...)
	require.True(s.Add(3, vdrBag))
	require.Equal(3, s.Len())

	// vote out of order
	// 2 finishes first to create a gap of finished poll between two unfinished polls 1 and 3
	id, ok := s.Vote(2, vdr3, blkID2)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(2, vdr2, blkID2)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(2, vdr1, blkID2)
	require.False(ok)
	require.Equal(ids.Empty, id)

	// 3 finishes now, 2 has already finished but 1 is not finished so we expect to receive no results still
	t.Logf("Before voting on poll 3")
	id, ok = s.Vote(3, vdr2, blkID3)
	require.False(ok)
	require.Equal(ids.Empty, id)
	
	id, ok = s.Vote(3, vdr3, blkID3)
	require.False(ok)
	require.Equal(ids.Empty, id)
	
	id, ok = s.Vote(3, vdr1, blkID3)
	require.False(ok)
	require.Equal(ids.Empty, id)
	t.Logf("After voting on poll 3 - poll 3 should be finished but blocked")

	// 1 finishes now, only poll 1 should return since polls process one at a time
	t.Logf("Before voting on poll 1 - remaining polls: %d", s.Len())
	id, ok = s.Vote(1, vdr1, blkID1)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(1, vdr2, blkID1)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(1, vdr3, blkID1)
	require.True(ok)
	require.Equal(blkID1, id)
	
	t.Logf("After poll 1 processed, polls remaining: %d", s.Len())

	// Poll 2 already finished, but returns nothing since it was already processed
	// Poll 3 needs more votes
	t.Logf("Polls remaining: %d", s.Len())
	t.Logf("Poll set state: %s", s.String())
	require.Equal(1, s.Len(), "Expected only poll 3 to remain")
	
	// Debug: check poll 3 state before final vote
	if holder, exists := (s.(*set)).polls.Get(3); exists {
		if p, ok := holder.(*poll); ok {
			t.Logf("Poll 3 state before final vote: tryCount=%d, wasBlocked=%v", p.tryCount, p.wasBlocked)
		}
	}
	
	id, ok = s.Vote(3, vdr1, blkID3)
	t.Logf("Vote on poll 3 result: ok=%v, id=%v", ok, id)
	
	// Check poll 3 state after vote
	if holder, exists := (s.(*set)).polls.Get(3); exists {
		if p, ok := holder.(*poll); ok {
			t.Logf("Poll 3 state after vote: tryCount=%d, wasBlocked=%v, finished=%v", p.tryCount, p.wasBlocked, p.Finished())
		}
	} else {
		t.Logf("Poll 3 no longer exists after vote")
	}
	
	require.True(ok)
	require.Equal(blkID3, id)
}

func TestCreateAndFinishSuccessfulPoll(t *testing.T) {
	require := require.New(t)

	vdrs := bag.Of(vdr1, vdr2) // k = 2
	alpha := 2

	factory := newEarlyTermNoTraversalTestFactory(require, alpha)
	log := log.NewNoOpLogger()
	registerer := prometheus.NewRegistry()
	s, err := NewSet(factory, log, registerer)
	require.NoError(err)

	require.Zero(s.Len())

	require.True(s.Add(0, vdrs))
	require.Equal(1, s.Len())

	require.False(s.Add(0, vdrs))
	require.Equal(1, s.Len())

	id, ok := s.Vote(1, vdr1, blkID1) // unknown poll
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(0, vdr1, blkID1)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(0, vdr1, blkID1) // duplicate vote
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Vote(0, vdr2, blkID1)
	require.True(ok)
	require.Equal(blkID1, id)
}

func TestCreateAndFinishFailedPoll(t *testing.T) {
	require := require.New(t)

	vdrs := bag.Of(vdr1, vdr2) // k = 2
	alpha := 1

	factory := newEarlyTermNoTraversalTestFactory(require, alpha)
	log := log.NewNoOpLogger()
	registerer := prometheus.NewRegistry()
	s, err := NewSet(factory, log, registerer)
	require.NoError(err)

	require.Zero(s.Len())

	require.True(s.Add(0, vdrs))
	require.Equal(1, s.Len())

	require.False(s.Add(0, vdrs))
	require.Equal(1, s.Len())

	id, ok := s.Drop(1, vdr1) // unknown poll
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Drop(0, vdr1)
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Drop(0, vdr1) // duplicate drop
	require.False(ok)
	require.Equal(ids.Empty, id)

	id, ok = s.Drop(0, vdr2)
	require.True(ok)
	require.Equal(ids.Empty, id) // No preference since all dropped
}

func TestSetString(t *testing.T) {
	require := require.New(t)

	vdrs := bag.Of(vdr1) // k = 1
	alpha := 1

	factory := newEarlyTermNoTraversalTestFactory(require, alpha)
	log := log.NewNoOpLogger()
	registerer := prometheus.NewRegistry()
	s, err := NewSet(factory, log, registerer)
	require.NoError(err)

	expected := `current polls: (Size = 1)
    RequestID 0:
        waiting on Bag[ids.NodeID]: (Size = 1)
            NodeID-6HgC8KRBEhXYbF4riJyJFLSHt37UNuRt: 1
        received Bag[ids.ID]: (Size = 0)`
	require.True(s.Add(0, vdrs))
	require.Equal(expected, s.String())
}
