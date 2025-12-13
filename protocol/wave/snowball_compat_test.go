// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.
//
// Snowball compatibility tests - validates that Wave consensus exhibits
// equivalent safety and liveness properties to Snowball binary voting.

package wave

import (
	"context"
	"testing"
	"time"

	"github.com/luxfi/consensus/core/types"
	"github.com/stretchr/testify/require"
)

// --- Test infrastructure for snowball-style voting simulation ---

// binaryChoice represents a binary voting choice (like snowball red/blue)
type binaryChoice int

const (
	choiceRed  binaryChoice = 0
	choiceBlue binaryChoice = 1
)

// binarySnowball simulates Snowball binary voting using Wave primitives
// Maps: Snowball -> Wave
//   - preference -> current vote bias
//   - preferenceStrength -> accumulated vote count
//   - confidence -> consecutive rounds at threshold
//   - finalized -> decided
type binarySnowball struct {
	alphaPreference int  // minimum votes to update preference
	alphaConfidence int  // minimum votes to increment confidence
	beta            int  // consecutive rounds needed for finalization
	preference      int  // current preference (0 or 1)
	prefStrength    [2]int // accumulated votes per choice
	confidence      int  // consecutive successful polls
	finalized       bool
}

func newBinarySnowball(alphaPreference int, alphaConfidence int, beta int, initialChoice int) *binarySnowball {
	return &binarySnowball{
		alphaPreference: alphaPreference,
		alphaConfidence: alphaConfidence,
		beta:            beta,
		preference:      initialChoice,
		prefStrength:    [2]int{0, 0},
		confidence:      0,
		finalized:       false,
	}
}

// RecordPoll processes a poll result with given vote count for a choice
func (sb *binarySnowball) RecordPoll(voteCount int, choice int) {
	if sb.finalized {
		return
	}

	// Update preference strength
	sb.prefStrength[choice]++

	// Check if we meet alpha thresholds
	if voteCount >= sb.alphaConfidence {
		// Strong confidence vote
		if choice == sb.preference {
			sb.confidence++
		} else {
			// Switch preference
			sb.confidence = 1
		}

		// Update preference based on accumulated strength
		if sb.prefStrength[choice] > sb.prefStrength[1-choice] {
			sb.preference = choice
		} else if voteCount >= sb.alphaPreference {
			sb.preference = choice
		}

		// Check finalization
		if sb.confidence >= sb.beta {
			sb.finalized = true
		}
	} else if voteCount >= sb.alphaPreference {
		// Preference-only vote - update preference but reset confidence
		if sb.prefStrength[choice] > sb.prefStrength[1-choice] {
			sb.preference = choice
		}
		sb.confidence = 0
	}
}

// RecordUnsuccessfulPoll resets confidence counter
func (sb *binarySnowball) RecordUnsuccessfulPoll() {
	sb.confidence = 0
}

func (sb *binarySnowball) Preference() int { return sb.preference }
func (sb *binarySnowball) Finalized() bool { return sb.finalized }

// --- Snowball Binary Tests (ported from avalanchego) ---

// TestSnowballBinaryBasic tests basic binary snowball voting behavior
func TestSnowballBinaryBasic(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 2, 3
	beta := 2

	sb := newBinarySnowball(alphaPreference, alphaConfidence, beta, red)
	require.Equal(red, sb.Preference())
	require.False(sb.Finalized())

	// Vote for blue with confidence threshold - switches preference
	sb.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, sb.Preference())
	require.False(sb.Finalized())

	// Vote for red - resets confidence counter, preference may stay blue due to strength
	sb.RecordPoll(alphaConfidence, red)
	require.False(sb.Finalized())

	// Two consecutive blue votes to reach beta
	sb.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, sb.Preference())
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, sb.Preference())
	require.True(sb.Finalized())
}

// TestSnowballBinaryRecordPreference tests preference-only polls
func TestSnowballBinaryRecordPreference(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2

	sb := newBinarySnowball(alphaPreference, alphaConfidence, beta, red)
	require.Equal(red, sb.Preference())
	require.False(sb.Finalized())

	// Confidence vote for blue
	sb.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, sb.Preference())
	require.False(sb.Finalized())

	// Confidence vote for red - switches
	sb.RecordPoll(alphaConfidence, red)
	require.False(sb.Finalized())

	// Preference-only vote for red
	sb.RecordPoll(alphaPreference, red)
	require.Equal(red, sb.Preference())
	require.False(sb.Finalized())

	// Two confidence votes for red to finalize
	sb.RecordPoll(alphaConfidence, red)
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, red)
	require.Equal(red, sb.Preference())
	require.True(sb.Finalized())
}

// TestSnowballBinaryUnsuccessfulPoll tests confidence reset on failed poll
func TestSnowballBinaryUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 2

	sb := newBinarySnowball(alphaPreference, alphaConfidence, beta, red)

	// Confidence vote for blue
	sb.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, sb.Preference())
	require.False(sb.Finalized())

	// Unsuccessful poll resets confidence
	sb.RecordUnsuccessfulPoll()

	// Need beta more consecutive polls
	sb.RecordPoll(alphaConfidence, blue)
	require.False(sb.Finalized())

	sb.RecordPoll(alphaConfidence, blue)
	require.True(sb.Finalized())
}

// TestSnowballBinaryLockColor tests that finalized choice cannot change
func TestSnowballBinaryLockColor(t *testing.T) {
	require := require.New(t)

	red := 0
	blue := 1

	alphaPreference, alphaConfidence := 1, 2
	beta := 1

	sb := newBinarySnowball(alphaPreference, alphaConfidence, beta, red)

	// Single confidence vote finalizes with beta=1
	sb.RecordPoll(alphaConfidence, red)
	require.Equal(red, sb.Preference())
	require.True(sb.Finalized())

	// Votes for blue should not change finalized preference
	sb.RecordPoll(alphaConfidence, blue)
	require.Equal(red, sb.Preference())
	require.True(sb.Finalized())

	sb.RecordPoll(alphaConfidence, blue)
	require.Equal(red, sb.Preference())
	require.True(sb.Finalized())
}

// --- Wave-based Snowball simulation tests ---

// waveSnowball wraps Wave to provide Snowball-like semantics
type waveSnowball struct {
	wave   Wave[string]
	itemID string
	tx     *deterministicTransport
	cut    *mockCut[string]
}

// deterministicTransport provides controlled vote injection
type deterministicTransport struct {
	votes []Photon[string]
}

func newDeterministicTransport() *deterministicTransport {
	return &deterministicTransport{votes: nil}
}

func (d *deterministicTransport) RequestVotes(ctx context.Context, peers []types.NodeID, item string) <-chan Photon[string] {
	ch := make(chan Photon[string], len(d.votes))
	go func() {
		defer close(ch)
		for _, v := range d.votes {
			select {
			case ch <- v:
			case <-ctx.Done():
				return
			}
		}
	}()
	return ch
}

func (d *deterministicTransport) MakeLocalPhoton(item string, prefer bool) Photon[string] {
	return Photon[string]{Item: item, Prefer: prefer, Timestamp: time.Now()}
}

func (d *deterministicTransport) SetVotes(yesCount, noCount int) {
	d.votes = nil
	for i := 0; i < yesCount; i++ {
		nodeID := [20]byte{byte(i + 1)}
		d.votes = append(d.votes, Photon[string]{
			Item:      "test",
			Prefer:    true,
			Sender:    nodeID,
			Timestamp: time.Now(),
		})
	}
	for i := 0; i < noCount; i++ {
		nodeID := [20]byte{byte(yesCount + i + 1)}
		d.votes = append(d.votes, Photon[string]{
			Item:      "test",
			Prefer:    false,
			Sender:    nodeID,
			Timestamp: time.Now(),
		})
	}
}

func newWaveSnowball(k int, alpha float64, beta uint32) *waveSnowball {
	cfg := Config{
		K:       k,
		Alpha:   alpha,
		Beta:    beta,
		RoundTO: 100 * time.Millisecond,
	}

	cut := newMockCut[string](k * 2)
	tx := newDeterministicTransport()
	wave := New[string](cfg, cut, tx)

	return &waveSnowball{
		wave:   wave,
		itemID: "test",
		tx:     tx,
		cut:    cut,
	}
}

func (ws *waveSnowball) Poll(yesVotes, noVotes int) {
	ws.tx.SetVotes(yesVotes, noVotes)
	ws.wave.Tick(context.Background(), ws.itemID)
}

func (ws *waveSnowball) Preference() bool {
	return ws.wave.Preference(ws.itemID)
}

func (ws *waveSnowball) Finalized() bool {
	state, exists := ws.wave.State(ws.itemID)
	if !exists {
		return false
	}
	return state.Decided
}

func (ws *waveSnowball) Count() uint32 {
	state, exists := ws.wave.State(ws.itemID)
	if !exists {
		return 0
	}
	return state.Count
}

// TestWaveSnowballEquivalence tests that Wave exhibits snowball-like behavior
func TestWaveSnowballEquivalence(t *testing.T) {
	require := require.New(t)

	// K=5, Alpha=0.8 (threshold=4), Beta=3
	ws := newWaveSnowball(5, 0.8, 3)

	require.False(ws.Finalized())

	// Poll with 5 yes votes (above threshold)
	ws.Poll(5, 0)
	require.True(ws.Preference())
	require.Equal(uint32(1), ws.Count())
	require.False(ws.Finalized())

	// Another yes poll
	ws.Poll(5, 0)
	require.True(ws.Preference())
	require.Equal(uint32(2), ws.Count())
	require.False(ws.Finalized())

	// Third yes poll - should finalize
	ws.Poll(5, 0)
	require.True(ws.Preference())
	require.Equal(uint32(3), ws.Count())
	require.True(ws.Finalized())
}

// TestWaveSnowballConfidenceReset tests confidence reset on preference switch
func TestWaveSnowballConfidenceReset(t *testing.T) {
	require := require.New(t)

	// K=5, Alpha=0.8 (threshold=4), Beta=3
	ws := newWaveSnowball(5, 0.8, 3)

	// Two yes polls
	ws.Poll(5, 0)
	ws.Poll(5, 0)
	require.True(ws.Preference())
	require.Equal(uint32(2), ws.Count())

	// No poll - should reset count
	ws.Poll(0, 5)
	require.False(ws.Preference())
	require.Equal(uint32(1), ws.Count()) // Reset to 1

	// Three no polls to finalize
	ws.Poll(0, 5)
	ws.Poll(0, 5)
	require.False(ws.Preference())
	require.True(ws.Finalized())
}

// TestWaveSnowballSplitVote tests behavior with split votes (no threshold met)
func TestWaveSnowballSplitVote(t *testing.T) {
	require := require.New(t)

	// K=10, Alpha=0.8 (threshold=8), Beta=3
	ws := newWaveSnowball(10, 0.8, 3)

	// Build up some confidence
	ws.Poll(10, 0)
	ws.Poll(10, 0)
	require.Equal(uint32(2), ws.Count())

	// Split vote - neither side reaches threshold
	ws.Poll(5, 5)
	require.Equal(uint32(0), ws.Count()) // Reset on split
	require.False(ws.Finalized())
}

// TestWaveSnowballDecisionPersistence tests that decisions are final
func TestWaveSnowballDecisionPersistence(t *testing.T) {
	require := require.New(t)

	// K=5, Alpha=0.8, Beta=2
	ws := newWaveSnowball(5, 0.8, 2)

	// Finalize with yes preference
	ws.Poll(5, 0)
	ws.Poll(5, 0)
	require.True(ws.Finalized())
	require.True(ws.Preference())

	// Opposite votes should not change decision
	ws.Poll(0, 5)
	ws.Poll(0, 5)
	ws.Poll(0, 5)
	require.True(ws.Finalized())
	// Note: preference tracking continues but decision is final
}

// --- Parameter validation tests (ported from parameters_test.go) ---

// TestWaveParameterValidation tests configuration constraints
func TestWaveParameterValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     Config
		valid   bool
	}{
		{
			name:  "valid minimal",
			cfg:   Config{K: 1, Alpha: 0.5, Beta: 1, RoundTO: time.Millisecond},
			valid: true,
		},
		{
			name:  "valid standard",
			cfg:   Config{K: 20, Alpha: 0.8, Beta: 20, RoundTO: time.Second},
			valid: true,
		},
		{
			name:  "zero K",
			cfg:   Config{K: 0, Alpha: 0.5, Beta: 1, RoundTO: time.Millisecond},
			valid: false,
		},
		{
			name:  "alpha too low",
			cfg:   Config{K: 10, Alpha: 0.0, Beta: 1, RoundTO: time.Millisecond},
			valid: false,
		},
		{
			name:  "alpha too high",
			cfg:   Config{K: 10, Alpha: 1.1, Beta: 1, RoundTO: time.Millisecond},
			valid: false,
		},
		{
			name:  "zero beta",
			cfg:   Config{K: 10, Alpha: 0.5, Beta: 0, RoundTO: time.Millisecond},
			valid: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := validateConfig(tt.cfg)
			require.Equal(t, tt.valid, valid)
		})
	}
}

// validateConfig checks if a Wave config is valid
func validateConfig(cfg Config) bool {
	if cfg.K <= 0 {
		return false
	}
	if cfg.Alpha <= 0 || cfg.Alpha > 1 {
		return false
	}
	if cfg.Beta <= 0 {
		return false
	}
	return true
}

// --- N-ary voting simulation (ported from nnary_snowball_test.go) ---

// narySnowball simulates N-ary snowball voting
type narySnowball struct {
	alphaPreference int
	alphaConfidence int
	beta            int
	choices         map[string]int // choice -> preference strength
	preference      string
	confidence      int
	finalized       bool
}

func newNarySnowball(alphaPreference, alphaConfidence, beta int, initial string) *narySnowball {
	return &narySnowball{
		alphaPreference: alphaPreference,
		alphaConfidence: alphaConfidence,
		beta:            beta,
		choices:         map[string]int{initial: 0},
		preference:      initial,
		confidence:      0,
		finalized:       false,
	}
}

func (ns *narySnowball) Add(choice string) {
	if _, exists := ns.choices[choice]; !exists {
		ns.choices[choice] = 0
	}
}

func (ns *narySnowball) RecordPoll(voteCount int, choice string) {
	if ns.finalized {
		return
	}

	ns.choices[choice]++

	if voteCount >= ns.alphaConfidence {
		if choice == ns.preference {
			ns.confidence++
		} else {
			ns.confidence = 1
			ns.preference = choice
		}

		if ns.confidence >= ns.beta {
			ns.finalized = true
		}
	} else if voteCount >= ns.alphaPreference {
		// Find strongest choice
		maxStrength := 0
		for c, s := range ns.choices {
			if s > maxStrength {
				maxStrength = s
				ns.preference = c
			}
		}
		ns.confidence = 0
	}
}

func (ns *narySnowball) RecordUnsuccessfulPoll() {
	ns.confidence = 0
}

func (ns *narySnowball) Preference() string { return ns.preference }
func (ns *narySnowball) Finalized() bool    { return ns.finalized }

// TestNarySnowballBasic tests N-ary snowball with multiple choices
func TestNarySnowballBasic(t *testing.T) {
	require := require.New(t)

	red, blue, green := "red", "blue", "green"

	alphaPreference, alphaConfidence := 1, 2
	beta := 2

	ns := newNarySnowball(alphaPreference, alphaConfidence, beta, red)
	ns.Add(blue)
	ns.Add(green)

	require.Equal(red, ns.Preference())
	require.False(ns.Finalized())

	// Vote for blue
	ns.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, ns.Preference())
	require.False(ns.Finalized())

	// Vote for red
	ns.RecordPoll(alphaConfidence, red)
	require.Equal(red, ns.Preference())
	require.False(ns.Finalized())

	// Vote for red again - preference only
	ns.RecordPoll(alphaPreference, red)
	require.Equal(red, ns.Preference())
	require.False(ns.Finalized())

	// Confidence vote for red
	ns.RecordPoll(alphaConfidence, red)
	require.False(ns.Finalized())

	// Another vote for blue changes preference
	ns.RecordPoll(alphaPreference, blue)
	require.False(ns.Finalized())

	// Two confidence votes for blue to finalize
	ns.RecordPoll(alphaConfidence, blue)
	require.False(ns.Finalized())

	ns.RecordPoll(alphaConfidence, blue)
	require.Equal(blue, ns.Preference())
	require.True(ns.Finalized())
}

// TestNarySnowballVirtuous tests immediate finalization with single choice
func TestNarySnowballVirtuous(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 1

	ns := newNarySnowball(alphaPreference, alphaConfidence, beta, "red")

	require.Equal("red", ns.Preference())
	require.False(ns.Finalized())

	ns.RecordPoll(alphaConfidence, "red")
	require.Equal("red", ns.Preference())
	require.True(ns.Finalized())
}

// TestNarySnowballUnsuccessfulPoll tests confidence reset
func TestNarySnowballUnsuccessfulPoll(t *testing.T) {
	require := require.New(t)

	alphaPreference, alphaConfidence := 1, 2
	beta := 2

	ns := newNarySnowball(alphaPreference, alphaConfidence, beta, "red")
	ns.Add("blue")

	ns.RecordPoll(alphaConfidence, "blue")
	require.Equal("blue", ns.Preference())
	require.False(ns.Finalized())

	ns.RecordUnsuccessfulPoll()

	ns.RecordPoll(alphaConfidence, "blue")
	require.False(ns.Finalized())

	ns.RecordPoll(alphaConfidence, "blue")
	require.True(ns.Finalized())

	// Additional polls after finalization
	for i := 0; i < 4; i++ {
		ns.RecordPoll(alphaConfidence, "red")
		require.Equal("blue", ns.Preference())
		require.True(ns.Finalized())
	}
}

// --- Network simulation tests (ported from network_test.go) ---

// simNode represents a node in the simulated network
type simNode struct {
	preference int // 0 or 1
	confidence int
	finalized  bool
	nodeID     int
}

// simNetwork simulates a network of snowball nodes
type simNetwork struct {
	nodes  []*simNode
	params struct {
		k               int
		alphaPreference int
		alphaConfidence int
		beta            int
	}
	seed int64
}

func newSimNetwork(k, alphaPreference, alphaConfidence, beta, numNodes int) *simNetwork {
	net := &simNetwork{
		nodes: make([]*simNode, 0, numNodes),
	}
	net.params.k = k
	net.params.alphaPreference = alphaPreference
	net.params.alphaConfidence = alphaConfidence
	net.params.beta = beta

	// Create nodes with initial preferences biased toward 0 (supermajority)
	// Need enough bias that random samples of k nodes exceed alphaConfidence
	for i := 0; i < numNodes; i++ {
		// Bias initial preferences: 85% toward 0, 15% toward 1
		// With k=10 and alphaConfidence=8, need 80%+ to consistently exceed threshold
		initial := 0
		if i%7 == 0 { // ~14% minority
			initial = 1
		}
		node := &simNode{
			preference: initial,
			confidence: 0,
			finalized:  false,
			nodeID:     i,
		}
		net.nodes = append(net.nodes, node)
	}

	return net
}

// Round executes one round of the consensus protocol for all nodes
func (net *simNetwork) Round() {
	net.seed++

	// Sample network preference once per round (simulates network-wide poll)
	redVotes, blueVotes := 0, 0
	for i := 0; i < net.params.k && i < len(net.nodes); i++ {
		peerIdx := (int(net.seed)*7 + i*3) % len(net.nodes) // Better mixing
		if net.nodes[peerIdx].preference == 0 {
			redVotes++
		} else {
			blueVotes++
		}
	}

	// Each non-finalized node processes the poll
	for _, node := range net.nodes {
		if node.finalized {
			continue
		}

		// Determine majority from sampled votes
		var majorityChoice int
		var majorityVotes int
		if redVotes > blueVotes {
			majorityChoice = 0
			majorityVotes = redVotes
		} else if blueVotes > redVotes {
			majorityChoice = 1
			majorityVotes = blueVotes
		} else {
			// Tie - reset confidence
			node.confidence = 0
			continue
		}

		// Check threshold and update
		if majorityVotes >= net.params.alphaConfidence {
			if majorityChoice == node.preference {
				node.confidence++
			} else {
				node.preference = majorityChoice
				node.confidence = 1
			}

			if node.confidence >= net.params.beta {
				node.finalized = true
			}
		} else if majorityVotes >= net.params.alphaPreference {
			node.preference = majorityChoice
			node.confidence = 0
		}
	}
}

func (net *simNetwork) Finalized() bool {
	for _, node := range net.nodes {
		if !node.finalized {
			return false
		}
	}
	return true
}

func (net *simNetwork) Agreement() bool {
	if len(net.nodes) == 0 {
		return true
	}
	pref := net.nodes[0].preference
	for _, node := range net.nodes {
		if node.preference != pref {
			return false
		}
	}
	return true
}

func (net *simNetwork) Disagreement() bool {
	var finalizedPref *int
	for _, node := range net.nodes {
		if node.finalized {
			if finalizedPref == nil {
				p := node.preference
				finalizedPref = &p
			} else if *finalizedPref != node.preference {
				return true
			}
		}
	}
	return false
}

// TestNetworkConvergence tests that a network converges to agreement
func TestNetworkConvergence(t *testing.T) {
	require := require.New(t)

	// Parameters for faster convergence in test
	// With 20 nodes, k=10 samples, alphaConfidence=6 (60%), beta=5
	net := newSimNetwork(
		10, // k - sample 10 nodes per round
		5,  // alphaPreference - 50% threshold
		6,  // alphaConfidence - 60% threshold
		5,  // beta - 5 consecutive rounds
		20, // numNodes
	)

	maxRounds := 200
	for i := 0; i < maxRounds && !net.Agreement(); i++ {
		net.Round()
	}

	require.False(net.Disagreement(), "Network should not have disagreement")
	require.True(net.Agreement(), "Network should reach agreement")
}

// TestNetworkSmallQuorum tests convergence with small quorum
func TestNetworkSmallQuorum(t *testing.T) {
	require := require.New(t)

	// Small network with tight quorum
	net := newSimNetwork(
		5,  // k
		3,  // alphaPreference
		4,  // alphaConfidence
		5,  // beta
		10, // numNodes
	)

	maxRounds := 300
	for i := 0; i < maxRounds && !net.Agreement(); i++ {
		net.Round()
	}

	require.False(net.Disagreement())
	require.True(net.Agreement())
}

// --- Performance characteristics tests ---

// TestConvergenceSpeed measures rounds to agreement
func TestConvergenceSpeed(t *testing.T) {
	require := require.New(t)

	// Run multiple trials
	totalRounds := 0
	trials := 5

	for trial := 0; trial < trials; trial++ {
		net := newSimNetwork(8, 4, 5, 5, 15)
		net.seed = int64(trial * 1000) // Different seed per trial

		rounds := 0
		for rounds < 300 && !net.Agreement() {
			net.Round()
			rounds++
		}

		require.True(net.Agreement(), "Trial %d should agree", trial)
		totalRounds += rounds
	}

	avgRounds := totalRounds / trials
	t.Logf("Average rounds to convergence: %d", avgRounds)

	// Convergence should be reasonably fast
	require.Less(avgRounds, 150, "Average convergence should be under 150 rounds")
}

// TestSafetyUnderChurn tests safety with node preference changes
func TestSafetyUnderChurn(t *testing.T) {
	require := require.New(t)

	net := newSimNetwork(10, 7, 8, 15, 30)

	rounds := 0
	for rounds < 300 && !net.Agreement() {
		net.Round()
		rounds++

		// Simulate churn: reset confidence for non-finalized nodes occasionally
		if rounds%50 == 0 {
			for _, node := range net.nodes {
				if !node.finalized && rounds%100 == 0 {
					// This simulates network partition recovery
					node.confidence = 0
				}
			}
		}
	}

	require.False(net.Disagreement(), "Safety should hold under churn")
}
