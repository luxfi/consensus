package prism

import (
	"math/rand"
	"sort"

	"github.com/luxfi/ids"
)

// ============================================================================
// Sampler compatibility - kept for back-compat with older code.
// Use Splitter instead.
// ============================================================================

// Sampler is kept for back-compat with older code.
// Use Splitter instead.
type Sampler interface {
	// Sample returns up to k validators.
	Sample(validators []ids.NodeID, k int) ([]ids.NodeID, error)
}

// NewSampler returns a Sampler that delegates to the new Splitter.
func NewSampler(src rand.Source) Sampler {
	return NewSplitter(src)
}

// ---- Splitter default implementation ----

// Splitter selects a k-sized subset from validators.
type Splitter interface {
	Sample(validators []ids.NodeID, k int) ([]ids.NodeID, error)
}

type splitterImpl struct {
	rng *rand.Rand
}

// NewSplitter creates a Splitter backed by math/rand.Source.
func NewSplitter(src rand.Source) Splitter {
	if src == nil {
		src = rand.NewSource(1) // deterministic default for tests
	}
	return &splitterImpl{rng: rand.New(src)}
}

func (s *splitterImpl) Sample(validators []ids.NodeID, k int) ([]ids.NodeID, error) {
	if k <= 0 || len(validators) == 0 {
		return nil, nil
	}

	// Copy validators to avoid mutating input
	vals := make([]ids.NodeID, len(validators))
	copy(vals, validators)
	
	if len(vals) <= k {
		// Sort to keep deterministic order for small sets.
		sort.Slice(vals, func(i, j int) bool {
			return vals[i].String() < vals[j].String()
		})
		return vals, nil
	}

	// Fisher–Yates shuffle first k
	for i := 0; i < k; i++ {
		j := i + s.rng.Intn(len(vals)-i)
		vals[i], vals[j] = vals[j], vals[i]
	}
	chosen := vals[:k]

	// Stable order to avoid churn downstream
	sort.Slice(chosen, func(i, j int) bool {
		return chosen[i].String() < chosen[j].String()
	})
	return chosen, nil
}

// ============================================================================
// Traverser compatibility - kept for back-compat; prefer Refractor.
// ============================================================================

// Traverser is an alias of Refractor.
type Traverser = Refractor

// NewTraverser returns a new Refractor (alias).
func NewTraverser(cfg RefractConfig) *Refractor {
	return NewRefractor(cfg)
}

// ============================================================================
// Quorum compatibility - kept for back-compat; prefer Cut.
// ============================================================================

// Quorum is an alias of Cut.
type Quorum = Cut

// NewQuorum constructs a Cut (alias).
func NewQuorum(alphaPreference, alphaConfidence, beta int) *Cut {
	return NewCut(alphaPreference, alphaConfidence, beta)
}

// ============================================================================
// BinarySampler manages binary consensus sampling for light protocols
// ============================================================================

// BinarySampler manages binary consensus sampling for light protocols
type BinarySampler struct {
	preference int
	count      [2]int
}

// NewBinarySampler creates a new binary sampler with initial preference
func NewBinarySampler(initialPreference int) BinarySampler {
	return BinarySampler{
		preference: initialPreference,
	}
}

// Preference returns the current preference
func (bs *BinarySampler) Preference() int {
	return bs.preference
}

// RecordSuccessfulPoll records a successful poll result
func (bs *BinarySampler) RecordSuccessfulPoll(choice int) {
	bs.count[choice]++
	// Update preference to choice with more successful polls
	if bs.count[choice] > bs.count[1-choice] {
		bs.preference = choice
	}
}

// ============================================================================
// UnarySampler for single-value consensus
// ============================================================================

// UnarySampler tracks consensus on a single value
type UnarySampler struct {
	polled bool
}

// NewUnarySampler creates a new unary sampler
func NewUnarySampler() *UnarySampler {
	return &UnarySampler{}
}

// RecordPoll records that polling has occurred
func (u *UnarySampler) RecordPoll() {
	u.polled = true
}

// Finalized returns whether consensus has been reached
func (u *UnarySampler) Finalized() bool {
	return u.polled
}

// ============================================================================
// NArySampler for multi-value consensus
// ============================================================================

// NArySampler tracks consensus among n choices
type NArySampler struct {
	n          int
	preference int
	counts     []int
}

// NewNArySampler creates a new n-ary sampler
func NewNArySampler(n int) *NArySampler {
	return &NArySampler{
		n:      n,
		counts: make([]int, n),
	}
}

// RecordPoll records poll results for a choice
func (n *NArySampler) RecordPoll(choice int) {
	if choice >= 0 && choice < n.n {
		n.counts[choice]++
		// Update preference to choice with most polls
		maxCount := n.counts[n.preference]
		if n.counts[choice] > maxCount {
			n.preference = choice
		}
	}
}

// Preference returns the current preferred choice
func (n *NArySampler) Preference() int {
	return n.preference
}

// Count returns the poll count for a specific choice
func (n *NArySampler) Count(choice int) int {
	if choice >= 0 && choice < n.n {
		return n.counts[choice]
	}
	return 0
}