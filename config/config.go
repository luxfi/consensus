package config

import (
	"errors"
	"fmt"
	"time"
)

// Error variables for parameter validation
var (
	ErrParametersInvalid  = errors.New("invalid consensus parameters")
	ErrInvalidK           = errors.New("k must be >= 1")
	ErrInvalidAlpha       = errors.New("alpha must be between 0.66 and 1.0")
	ErrInvalidBeta        = errors.New("beta must be >= 1")
	ErrBlockTimeTooLow    = errors.New("block time must be >= 1ms")
	ErrRoundTimeoutTooLow = errors.New("round timeout must be >= block time")
	// ErrKTooLowForMainnet enforces the mainnet sample-size floor. K>=11 is the
	// floor that gives mainnet a meaningful Byzantine margin: it is NOT merely
	// "K>=11" as a magic number — it is the smallest K for which the BFT overlap
	// bound 2*AlphaPreference - K >= floor((K-1)/3)+1 (= f+1) can hold with
	// f=floor((K-1)/3)>=3, so two quorums always overlap in >f honest nodes and
	// a sub-1/3 coalition can neither finalize nor fork. A smaller K shrinks f
	// toward 0 and makes a cheap stake-fraction (51%-style) attack viable.
	ErrKTooLowForMainnet = errors.New("consensus: mainnet (networkID=1) requires K>=11 so the BFT overlap bound 2*AlphaPreference-K >= floor((K-1)/3)+1 holds with f=floor((K-1)/3)>=3; a smaller K drives f->0 and enables a cheap sub-1/3 (51%-style) attack")
	// ErrKTooLowForTestnet is the testnet analogue: K>=5 keeps f=floor((K-1)/3)>=1
	// so the same overlap bound (2*AlphaPreference-K >= f+1) is satisfiable with
	// at least single-fault Byzantine tolerance.
	ErrKTooLowForTestnet = errors.New("consensus: testnet (networkID=2) requires K>=5 so the BFT overlap bound 2*AlphaPreference-K >= floor((K-1)/3)+1 holds with f=floor((K-1)/3)>=1; a smaller K leaves f=0 and a single faulty validator can fork it")

	// ErrAlphaBelowBFTQuorum is returned by Valid() when the integer accept
	// quorum (AlphaPreference) is too small for the sample size K to guarantee
	// safety under Byzantine faults. Two α-quorums must overlap in MORE than f
	// honest nodes so they cannot certify conflicting blocks; that requires
	//
	//	2·AlphaPreference − K ≥ f + 1,   where f = ⌊(K-1)/3⌋.
	//
	// A config that fails this (e.g. K=3/α=2 treated as BFT) admits a single
	// faulty validator forking the chain — exactly the round-1 hole.
	ErrAlphaBelowBFTQuorum = errors.New("consensus: alpha quorum too small for K to be Byzantine-safe (need 2*AlphaPreference - K >= floor((K-1)/3)+1)")

	// ErrKTooLowForValue is returned by ValidateForValueNetwork when a value /
	// PoS chain is configured with K<4. K=3 tolerates f=⌊2/3⌋=0 Byzantine
	// validators — i.e. NO fault tolerance — so a single Byzantine validator can
	// fork it. Real-value chains require f≥1 ⟹ K≥4 (α≥3); mainnet requires K≥11.
	ErrKTooLowForValue = errors.New("consensus: value/PoS chain requires K>=4 (f>=1 Byzantine tolerance); K=3 has f=0 and a single faulty validator forks it")

	// ErrKBelowLiveFloor is returned by ValidateForLiveValueNetwork when the
	// committee is smaller than min(tierFloor, liveN) — i.e. it under-samples the
	// live validator set. With K sized to the live set this never fires; it guards
	// an operator who pins a K smaller than the validators that actually exist.
	ErrKBelowLiveFloor = errors.New("consensus: K under-samples the live validator set (need K >= min(tierFloor, liveValidators))")
)

// errKTooLowForValueLive wraps ErrKTooLowForValue with the live-aware context.
func errKTooLowForValueLive(p Parameters, networkID uint32) error {
	return fmt.Errorf("%w: K=%d f=%d networkID=%d", ErrKTooLowForValue, p.K, p.ByzantineFaultTolerance(), networkID)
}

// errKBelowLiveFloor wraps ErrKBelowLiveFloor with the resolved live floor.
func errKBelowLiveFloor(p Parameters, networkID uint32, liveN, effective int) error {
	return fmt.Errorf("%w: K=%d < %d (networkID=%d liveValidators=%d)", ErrKBelowLiveFloor, p.K, effective, networkID, liveN)
}

// Parameters defines consensus parameters
type Parameters struct {
	K                     int
	Alpha                 float64 // For compatibility with Quasar
	Beta                  uint32  // For compatibility with Quasar
	AlphaPreference       int
	AlphaConfidence       int
	BetaVirtuous          int
	BetaRogue             int
	ConcurrentPolls       int
	ConcurrentRepolls     int
	OptimalProcessing     int
	MaxOutstandingItems   int
	MaxItemProcessingTime time.Duration
	Parents               int
	BatchSize             int
	BlockTime             time.Duration // For compatibility
	RoundTO               time.Duration // For compatibility
	GasLimit              uint64        // Per-block gas limit (0 = use chain default)

	// ConvergenceSettleWindow is how long a contested fork slot is observed (since the
	// last competing sibling was tracked) before this node casts its ONE per-height
	// accept signature for the converged (lowest-canonical) winner. It MUST exceed the
	// sibling-gossip latency of the deployment: a settle shorter than gossip lets a node
	// bind its vote to an incomplete sibling set (a non-global-minimum block), which —
	// under the height-only vote-once rule — is an UNRECOVERABLE vote split. It is
	// decoupled from RoundTO on purpose so an operator can lengthen it for high-latency
	// (WAN) validator sets WITHOUT slowing the round cadence. Zero ⇒ auto (RoundTO/2,
	// clamped [150ms, 2s]); see Transitive.convergenceSettleWindow.
	ConvergenceSettleWindow time.Duration

	// PQMode selects which post-quantum signature paths the engine runs
	// alongside BLS. Zero value (PQModeBLSOnly) preserves the classical
	// fast path. See pq_mode.go for the full enum.
	PQMode PQMode
}

// WithPQMode returns a copy of Parameters with the given PQ mode set.
// Use config.PQModeFromEnv to honour the CONSENSUS_PQ_MODE override.
func (p Parameters) WithPQMode(m PQMode) Parameters {
	p.PQMode = m
	return p
}

// PostQuantum reports whether this Parameters carries any PQ witness set
// on top of the classical BLS aggregate (i.e. PQMode != BLSOnly).
// Mirrors the simple boolean knob exposed to operators who don't want to
// pick a witness set explicitly.
func (p Parameters) PostQuantum() bool {
	return p.PQMode.IsPostQuantum()
}

// WithPostQuantum collapses the five-way enum onto a boolean:
//
//	true  -> PQModeTripleQuantum   // strongest available
//	false -> PQModeBLSOnly         // classical fast path
//
// For middle-ground modes (BLSPlusMLDSA, BLSPlusCorona, BLSPlusGroth16),
// call WithPQMode directly with the desired constant.
func (p Parameters) WithPostQuantum(on bool) Parameters {
	p.PQMode = PQModeFromBool(on)
	return p
}

// DefaultParams returns default parameters with 69% threshold
func DefaultParams() Parameters {
	return Parameters{
		K:                     20,
		Alpha:                 0.69, // 69% threshold for BFT
		Beta:                  14,   // Adjusted for 69% (was 15)
		AlphaPreference:       14,   // 70% of K for 69% threshold
		AlphaConfidence:       14,   // Matches AlphaPreference for 69%
		BetaVirtuous:          14,   // Virtuous confidence for 69%
		BetaRogue:             20,   // Rogue confidence remains high
		ConcurrentPolls:       4,
		ConcurrentRepolls:     4,
		OptimalProcessing:     10,
		MaxOutstandingItems:   1024,
		MaxItemProcessingTime: 2 * time.Minute,
		Parents:               2, // Reduced for efficiency
		BatchSize:             30,
		BlockTime:             100 * time.Millisecond,
		RoundTO:               250 * time.Millisecond,
	}
}

// MainnetParams returns mainnet parameters with 69% threshold
func MainnetParams() Parameters {
	p := DefaultParams()
	p.K = 21
	p.AlphaPreference = 15 // ~71% of K for 69% threshold
	p.AlphaConfidence = 15 // Matches AlphaPreference
	p.BetaVirtuous = 15    // Virtuous confidence for 69%
	p.BlockTime = 200 * time.Millisecond
	p.RoundTO = 400 * time.Millisecond
	return p
}

// TestnetParams returns testnet parameters with 69% threshold
func TestnetParams() Parameters {
	p := DefaultParams()
	p.K = 11
	p.Alpha = 0.69        // 69% threshold
	p.Beta = 8            // Adjusted for 69%
	p.AlphaPreference = 8 // ~73% of K for 69% threshold
	p.AlphaConfidence = 8 // Matches AlphaPreference
	p.BetaVirtuous = 8    // Virtuous confidence for 69%
	p.BlockTime = 100 * time.Millisecond
	p.RoundTO = 225 * time.Millisecond
	return p
}

// LocalParams returns local parameters with 2/3 threshold for 3-node networks.
// Uses 1ms block time for maximum throughput on localhost (zero network latency).
//
// SECURITY: K=3/α=2 is f=0 (CFT) — a single Byzantine validator forks it. This
// is the documented CRITICAL-2 fork config and is FORBIDDEN on value networks
// (ValidateForValueNetwork rejects it). Use it ONLY for a single-operator local
// network where every node is trusted (no value across independent parties). For
// a multi-node local network that must be genuinely BFT, use LocalBFTParams.
func LocalParams() Parameters {
	p := DefaultParams()
	p.K = 3
	p.Alpha = 0.67        // 2/3 threshold
	p.Beta = 2            // Adjusted for 2/3
	p.AlphaPreference = 2 // 2 of 3 for preference
	p.AlphaConfidence = 2 // 2 of 3 for confidence
	p.BetaVirtuous = 2    // Virtuous confidence for 2/3
	p.BlockTime = 1 * time.Millisecond
	p.RoundTO = 5 * time.Millisecond
	return p
}

// LocalBFTParams returns the MINIMAL Byzantine-fault-tolerant parameter set for a
// small local network: K=4, α=3 (AlphaPreference = AlphaConfidence = 3). This is
// f=1 (3·1+1 = 4 the minimal BFT committee), the smallest K that
// ValidateForValueNetwork admits (K≥4). It clears the BFT α-floor
// (2·AlphaPreference − K = 2·3 − 4 = 2 ≥ f+1 = 2), so a single faulty validator
// can neither forge a quorum nor stall finality.
//
// Why this exists: DefaultParams (K=20, α=14) is unsatisfiable on a small local
// network — α=14 affirmative votes are unreachable with only 3-4 validators, so
// NO block ever finalizes (the P-Chain freezes at height 0 and no chain is
// created). A local 4-validator devnet must use K=4 to make quorum reachable
// while staying genuinely BFT. Unlike LocalParams (K=3, f=0), this passes
// ValidateForValueNetwork and the CRITICAL-2 multi-node-is-BFT regression — it
// is a REAL BFT set, just sized for a minimal committee. Requires ≥4 validators
// to provide f=1 in practice; with fewer, quorum needs near-unanimity and runtime
// fault tolerance degrades to f=0 (still safe, just not live under one fault).
//
// Keeps LocalParams' localhost timing (1ms blocks / 5ms rounds).
func LocalBFTParams() Parameters {
	p := DefaultParams()
	p.K = 4
	p.Alpha = 0.67        // ⌈3/4⌉ ≈ 2/3-style threshold for a 4-committee
	p.Beta = 2            // fast confirmation on localhost
	p.AlphaPreference = 3 // 3 of 4 for preference (f=1)
	p.AlphaConfidence = 3 // 3 of 4 for confidence (f=1)
	p.BetaVirtuous = 2    // virtuous confidence
	p.BetaRogue = 4       // rogue confidence stays above the committee size
	p.BlockTime = 1 * time.Millisecond
	p.RoundTO = 5 * time.Millisecond
	return p
}

// BurstParams returns parameters for maximum throughput burst mode.
// Designed for GPU EVM + Block-STM on high-bandwidth networks (800Gbps+).
// 1ms blocks × 100K txs/block (2.1B gas) = 100M TPS theoretical ceiling.
// Actual throughput bounded by GPU execution speed (Block-STM parallel).
func BurstParams() Parameters {
	return Parameters{
		K:                     3,
		Alpha:                 0.67,
		Beta:                  2,
		AlphaPreference:       2,
		AlphaConfidence:       2,
		BetaVirtuous:          2,
		BetaRogue:             3,
		ConcurrentPolls:       8,
		ConcurrentRepolls:     8,
		OptimalProcessing:     50,
		MaxOutstandingItems:   8192,
		MaxItemProcessingTime: 10 * time.Second,
		Parents:               2,
		BatchSize:             100,
		BlockTime:             1 * time.Millisecond,
		RoundTO:               5 * time.Millisecond,
		GasLimit:              2_100_000_000, // 2.1B gas → 100K simple txs/block
	}
}

// SoloGPUParams returns parameters for a single-node GPU-accelerated validator.
// Tuned for Apple Silicon (M1/M2/M3/M4) with unified memory:
//   - 1ms blocks, K=1 self-vote (no network latency)
//   - 1B gas limit — GPU EVM fills ~47K txs/block at 21K gas each
//   - With C++ GPU EVM (1M TPS): ~1M TPS sustained
//   - With Go EVM (188K TPS): ~188K TPS sustained
//   - Consensus overhead: <13μs per block (measured 76K blocks/sec)
func SoloGPUParams() Parameters {
	return Parameters{
		K:                     1,
		Alpha:                 1.0,
		Beta:                  1,
		AlphaPreference:       1,
		AlphaConfidence:       1,
		BetaVirtuous:          1,
		BetaRogue:             1,
		ConcurrentPolls:       4,
		ConcurrentRepolls:     4,
		OptimalProcessing:     20,
		MaxOutstandingItems:   4096,
		MaxItemProcessingTime: 5 * time.Second,
		Parents:               1,
		BatchSize:             50,
		BlockTime:             1 * time.Millisecond,
		RoundTO:               5 * time.Millisecond,
		GasLimit:              1_000_000_000, // 1B gas → 47K txs/block
	}
}

// XChainParams returns X-Chain parameters with 2/3 threshold for 3-node networks
func XChainParams() Parameters {
	p := DefaultParams()
	p.K = 3
	p.Alpha = 0.67        // 2/3 threshold
	p.Beta = 2            // Adjusted for 2/3
	p.AlphaPreference = 2 // 2 of 3 for preference
	p.AlphaConfidence = 2 // 2 of 3 for confidence
	p.BetaVirtuous = 2    // Virtuous confidence for 2/3
	p.BlockTime = 1 * time.Millisecond
	p.RoundTO = 5 * time.Millisecond
	return p
}

// SingleValidatorParams returns parameters for single validator mainnet (K=1)
// This configuration is used for POA mode with a single staking validator
func SingleValidatorParams() Parameters {
	return Parameters{
		K:                     1,                      // Single validator
		Alpha:                 1.0,                    // 100% threshold (only one validator)
		Beta:                  1,                      // Immediate finalization
		AlphaPreference:       1,                      // Single validator preference
		AlphaConfidence:       1,                      // Single validator confidence
		BetaVirtuous:          1,                      // Immediate virtuous confidence
		BetaRogue:             1,                      // No rogue behavior possible
		ConcurrentPolls:       1,                      // Single poll at a time
		ConcurrentRepolls:     1,                      // Single repoll if needed
		OptimalProcessing:     1,                      // Process one at a time
		MaxOutstandingItems:   256,                    // Reduced for single validator
		MaxItemProcessingTime: 30 * time.Second,       // Faster timeout for single validator
		Parents:               1,                      // Single parent for linear chain
		BatchSize:             10,                     // Smaller batches for single validator
		BlockTime:             100 * time.Millisecond, // Fast block time
		RoundTO:               200 * time.Millisecond, // Quick round timeout
	}
}

// WithBlockTime returns a copy of Parameters with updated block time.
// Round timeout auto-scales: 5x for ultra-fast (<=1ms), 3x for fast (<10ms),
// 2.5x default. On localhost with GPU BLS, 1ms blocks + 5ms rounds is achievable.
func (p Parameters) WithBlockTime(blockTime time.Duration) Parameters {
	p.BlockTime = blockTime
	switch {
	case blockTime <= time.Millisecond:
		p.RoundTO = 5 * blockTime // 5ms for 1ms blocks
	case blockTime < 10*time.Millisecond:
		p.RoundTO = 3 * blockTime // 15ms for 5ms blocks
	default:
		p.RoundTO = blockTime*5/2 + time.Millisecond // 2.5x + 1ms
	}
	return p
}

// ByzantineFaultTolerance returns f, the maximum number of Byzantine validators
// this sample size can tolerate under classic BFT (n=K, f<n/3):
//
//	f = ⌊(K-1)/3⌋
//
// K=1→0, K=3→0, K=4→1, K=7→2, K=10→3, K=11→3, K=21→6. A value chain needs f≥1,
// hence K≥4 (see ValidateForValueNetwork).
func (p Parameters) ByzantineFaultTolerance() int {
	if p.K < 1 {
		return 0
	}
	return (p.K - 1) / 3
}

// bftQuorumFloor returns the minimum integer accept quorum α for safety at this
// K: the smallest α with 2α − K ≥ f + 1, i.e. α ≥ ⌈(K + f + 1)/2⌉.
func (p Parameters) bftQuorumFloor() int {
	f := p.ByzantineFaultTolerance()
	return (p.K + f + 1 + 1) / 2 // ceil((K+f+1)/2)
}

// Validate validates parameters (compatibility method)
func (p Parameters) Validate() error {
	return p.Valid()
}

// Valid validates parameters with threshold enforcement
func (p Parameters) Valid() error {
	// Check K, Alpha, Beta first - these are always required
	if p.K < 1 {
		return ErrInvalidK
	}
	// Enforce minimum 2/3 threshold (with small tolerance for rounding)
	if p.Alpha < 0.66 || p.Alpha > 1.0 {
		return ErrInvalidAlpha
	}
	if p.Beta < 1 {
		return ErrInvalidBeta
	}
	if p.BlockTime > 0 && p.BlockTime < time.Millisecond {
		return ErrBlockTimeTooLow
	}
	if p.BlockTime > 0 && p.RoundTO > 0 && p.RoundTO < p.BlockTime {
		return ErrRoundTimeoutTooLow
	}

	// Only validate other fields if they are set (non-zero)
	if p.AlphaPreference != 0 && (p.AlphaPreference < 0 || p.AlphaPreference > p.K) {
		return ErrParametersInvalid
	}
	if p.AlphaConfidence != 0 && (p.AlphaConfidence < 0 || p.AlphaConfidence > p.K) {
		return ErrParametersInvalid
	}

	// BFT QUORUM FLOOR — the integer accept quorum (AlphaPreference, the α that
	// the chain engine actually counts toward finality) must be large enough
	// that two α-quorums overlap in more than f honest validators:
	//
	//	2·AlphaPreference − K ≥ ⌊(K-1)/3⌋ + 1
	//
	// Without this a config can finalize on a quorum that two conflicting
	// certs/decisions can both reach (the K=3/α=2 family with f silently 0 is
	// the boundary; anything weaker forks outright). Checked only when an
	// integer α is set (AlphaPreference>0); a chain that runs the float α path
	// derives AlphaPreference from K before reaching the engine.
	if p.AlphaPreference > 0 {
		f := p.ByzantineFaultTolerance()
		if 2*p.AlphaPreference-p.K < f+1 {
			return fmt.Errorf("%w: K=%d AlphaPreference=%d f=%d (2*%d-%d=%d < %d)",
				ErrAlphaBelowBFTQuorum, p.K, p.AlphaPreference, f,
				p.AlphaPreference, p.K, 2*p.AlphaPreference-p.K, f+1)
		}
	}
	if p.BetaVirtuous < 0 {
		return ErrParametersInvalid
	}
	if p.BetaRogue != 0 && p.BetaRogue < p.BetaVirtuous {
		return ErrParametersInvalid
	}
	if p.ConcurrentPolls != 0 && p.ConcurrentPolls < 1 {
		return ErrParametersInvalid
	}
	if p.OptimalProcessing != 0 && p.OptimalProcessing < 1 {
		return ErrParametersInvalid
	}
	if p.MaxOutstandingItems != 0 && p.MaxOutstandingItems < 1 {
		return ErrParametersInvalid
	}
	if p.MaxItemProcessingTime != 0 && p.MaxItemProcessingTime <= 0 {
		return ErrParametersInvalid
	}

	return nil
}

// ValidateForNetwork validates parameters are safe for the given network.
// Mainnet (networkID 1) requires K >= 11, testnet (networkID 2) requires K >= 5.
// Local/devnet (3 / >= 1337) allows any K.
//
// The Lux primary-network IDs are convention-fixed: 1 = mainnet, 2 = testnet,
// 3 = devnet, 1337 = localnet (see luxfi/constants). The testnet floor was
// previously keyed to networkID 5 (a legacy Avalanche-Fuji id) and therefore
// NEVER fired for the real Lux testnet (id 2) — a safety floor that did not
// trigger for the network it protects. It is now correctly keyed to 2.
func (p Parameters) ValidateForNetwork(networkID uint32) error {
	if err := p.Valid(); err != nil {
		return err
	}
	switch networkID {
	case 1: // mainnet
		if p.K < 11 {
			return ErrKTooLowForMainnet
		}
	case 2: // testnet
		if p.K < 5 {
			return ErrKTooLowForTestnet
		}
	}
	return nil
}

// ValidateForValueNetwork validates parameters are safe for a chain that
// finalizes REAL VALUE across independent parties (a PoS / value chain — C, D,
// any L1 that custodies funds). It is STRICTER than ValidateForNetwork: a value
// chain must tolerate at least one Byzantine validator (f≥1), so K=3 (f=0) is
// FORBIDDEN regardless of networkID. Single-validator (K==1) value chains are a
// separate, explicitly-chosen regime (POA --dev) and are NOT admitted here —
// value across independent parties requires a real quorum.
//
//	K==1            → rejected (use the single-validator regime knowingly, not here)
//	K==2,3          → rejected (f=0, no Byzantine tolerance — a single fault forks)
//	K>=4            → f≥1, admitted (subject to the mainnet/testnet floors below)
//	mainnet(1)      → K>=11
//	testnet(2)      → K>=5
//
// Use this (not ValidateForNetwork) when selecting params for a multi-node value
// chain; the node's value-DEX activation also fails closed on the engine Mode.
func (p Parameters) ValidateForValueNetwork(networkID uint32) error {
	if err := p.ValidateForNetwork(networkID); err != nil {
		return err
	}
	if p.ByzantineFaultTolerance() < 1 {
		return fmt.Errorf("%w: K=%d f=%d networkID=%d", ErrKTooLowForValue, p.K, p.ByzantineFaultTolerance(), networkID)
	}
	return nil
}
