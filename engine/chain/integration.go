// Copyright (C) 2019-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"context"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
)

// ValidatorSampler provides access to the validator set for peer sampling.
// This is the minimal interface needed by consensus - avoids importing full validator package.
type ValidatorSampler interface {
	// Sample returns k random validator NodeIDs from the network.
	Sample(networkID ids.ID, k int) ([]ids.NodeID, error)
	// Count returns the number of validators in the network.
	Count(networkID ids.ID) int
}

// NetworkConfig holds parameters for integrating consensus with the node network.
type NetworkConfig struct {
	// ChainID is the ID of this chain (used for chain-scoped messages like Put, PullQuery)
	ChainID ids.ID
	// NetworkID is the ID of the network whose validators secure this chain
	// For primary network chains (P/X/C), this equals constants.PrimaryNetworkID
	// For L1 chains, this is the L1's validator set ID
	NetworkID ids.ID
	// NodeID is this node's identifier (for excluding self from samples)
	NodeID ids.NodeID
	// Validators provides access to the validator set for peer sampling.
	// If nil, the engine broadcasts to all peers (less efficient).
	Validators ValidatorSampler
	// Logger for consensus events
	Logger log.Logger
	// Gossiper broadcasts messages to validators
	Gossiper Gossiper
	// VM implements BlockBuilder for block creation
	VM BlockBuilder
	// Params are optional consensus parameters. If nil, DefaultParams() is used.
	// For small validator sets (e.g., 5 nodes), use LocalParams() which has
	// K=5, Beta=4 - appropriate thresholds for the validator count.
	Params *config.Parameters

	// Quorum-cert finality (multi-validator). The node supplies these to enable
	// α-of-K cert-witnessed finality:
	//   - VoteVerifier MUST be non-nil for any K>1 chain (the engine refuses to
	//     Start a multi-validator engine without it). Verifies vote + cert
	//     signatures against the chain's validator key set.
	//   - VoteSigner signs THIS node's accept votes (so its signature joins the
	//     cert when it votes). Required for this node to contribute to a cert.
	//   - CryptoWitnessSource (optional) upgrades engine certs to
	//     quasar.WeightedQuorumCert when the chain's PQ weighted validator set is
	//     plumbed. nil keeps the engine-level cert as the witness.
	//
	// For a single-validator chain (K==1, e.g. --dev) leave these nil.
	VoteVerifier VoteVerifier
	VoteSigner   VoteSigner

	// StakeSource (optional) makes finality a ⅔-by-STAKE supermajority instead
	// of a raw voter count (HIGH-3). REQUIRED for a PoS value chain with unequal
	// stake; nil keeps count-α finality (correct only under equal stake enforced
	// at admission). The node supplies a source backed by the chain's validator
	// set weights.
	StakeSource StakeSource

	// ValidatorSetRoot (optional) binds every vote/cert to the active weighted
	// validator set at the block's height (the MEDIUM fix), so the ⅔-by-stake
	// predicate is ENFORCED at the cert-position epoch: a cert gathered under one
	// epoch's set cannot be re-verified against another epoch's set (its
	// signatures were over the bound root). Supply it on a chain whose validator
	// set/stake changes across epochs (alongside StakeSource); nil binds Empty
	// (the correct no-op for a fixed validator set).
	ValidatorSetRoot ValidatorSetRootSource

	// Catchup (optional) is the node's ancestor-fetch transport, wired into the
	// engine's runtime auto-recovery (see Catchup / Runtime.requestCatchup). With
	// it, a follower that falls behind — missing a block's PARENT (out-of-order
	// gossip) or its BYTES (a vote outran the block) — self-heals by fetching the
	// missing block and rejoining the frontier. nil keeps the legacy no-self-heal
	// behaviour (a behind follower is stranded).
	Catchup Catchup
}

// Gossiper abstracts the network layer for consensus message broadcasting.
// This minimal interface avoids importing the full node/network package.
//
// Gossiper is the network-level interface. BlockProposer (in engine.go)
// is the consensus-level interface. The gossiperAdapter bridges them.
type Gossiper interface {
	// GossipPut broadcasts a Put message with block data to validators.
	// Returns the number of validators the message was sent to.
	GossipPut(chainID ids.ID, networkID ids.ID, blockData []byte) int
	// SendPullQuery sends a PullQuery to specific validators requesting votes.
	SendPullQuery(chainID ids.ID, networkID ids.ID, blockID ids.ID, validators []ids.NodeID) int
	// SendPushQuery sends a PushQuery (block data + vote request) to validators.
	// Unlike PullQuery (which only sends blockID), PushQuery includes the block bytes
	// so peers can immediately verify and respond with their vote.
	// Returns the number of validators the message was sent to.
	SendPushQuery(chainID ids.ID, networkID ids.ID, blockData []byte, validators []ids.NodeID) int
	// SendVote sends a vote response back to the proposer node.
	// Vote response back to the proposer
	// This is called after fast-follow acceptance to notify the proposer
	// that this node has accepted the block, enabling the proposer to
	// reach vote threshold and finalize its own copy.
	SendVote(chainID ids.ID, toNodeID ids.NodeID, blockID ids.ID) error
}

// QuorumGossiper is the vote/cert distribution topology required for α-of-K
// finality. It is the STRUCTURAL fix for the proposer-freeze: under the old
// SendVote-to-proposer-only topology a follower's vote reached only the
// proposer, so if the proposer's own Chits dropped it pinned below alpha and
// froze (which is why self-finality was bolted on). Here followers broadcast
// their SIGNED votes to ALL validators and any node that collects alpha
// distinct signed votes assembles + gossips the cert — so finality no longer
// depends on one node's inbound Chits.
//
// A Gossiper that does not implement QuorumGossiper runs in legacy single-
// validator / no-cert mode (K==1). Multi-validator finality requires it.
type QuorumGossiper interface {
	// BroadcastVote sends this node's SIGNED accept vote for blockID to ALL
	// validators on the network (not just the proposer). voteBytes is the
	// encoded signed vote (node id + signature over the canonical vote
	// message). Returns the number of validators reached.
	BroadcastVote(chainID ids.ID, networkID ids.ID, blockID ids.ID, voteBytes []byte) int
	// GossipCert broadcasts an assembled finality cert (encoded
	// WeightedQuorumCert / engine QuorumCert) to ALL validators so they can
	// finalize blockID on a verifiable α-of-K proof. Returns validators reached.
	GossipCert(chainID ids.ID, networkID ids.ID, blockID ids.ID, certBytes []byte) int
}

// Runtime wraps Transitive with network integration and VM notification handling.
// Use NewRuntime to create - this is the "one right way" to set up consensus.
type Runtime struct {
	*Transitive
	config NetworkConfig

	// Validator sampling for k-peer polls
	validators ValidatorSampler
	nodeID     ids.NodeID

	// fastFollowMu serializes fast-follow block acceptance to prevent
	// duplicate gossip deliveries from racing on the accept path.
	fastFollowMu sync.Mutex
	// fastFollowHeight tracks the highest block height accepted via fast-follow.
	// We use height instead of parent ID matching because:
	// 1. VM.LastAccepted() is stale after Accept() calls
	// 2. Block-producing nodes don't update the tracker when they build blocks
	// Height-based acceptance is safe because blocks are already verified.
	fastFollowHeight uint64
}

// bftCommittee scales a preset sample size k down to the live validator count and
// returns the BFT-supermajority quorum α = ⌊2k/3⌋+1 for the resulting committee.
// It only ever shrinks an oversized committee (count < k) — a preset that already
// fits (k ≤ count) is reported unchanged (clamped=false) so its hand-tuned α is
// preserved. The α formula reproduces every preset exactly: K4→α3, K11→α8,
// K20→α14, K21→α15, guaranteeing the result clears the BFT α-floor (2α−k ≥ f+1)
// while staying reachable (α ≤ k). This is the one mechanism that keeps a network
// from booting with an unsatisfiable quorum (α > live validators), which wedges
// finality permanently: every block verifies but the α-of-K cert never assembles.
func bftCommittee(k, count int) (newK, alpha int, clamped bool) {
	if count <= 0 || k <= count {
		return k, 0, false
	}
	return count, 2*count/3 + 1, true
}

// NewRuntime creates a fully wired consensus runtime ready for production use.
//
// This is the single, canonical way to create a chain consensus runtime for node integration.
// It:
//  1. Creates the Transitive engine with default parameters
//  2. Wires the network gossiper as the BlockProposer
//  3. Registers the VM for block building
//  4. Returns a ready-to-start runtime
//
// Usage in manager.go:
//
//	runtime := chain.NewRuntime(chain.NetworkConfig{
//	    ChainID:   chainParams.ID,
//	    NetworkID: chainParams.ChainID,  // PrimaryNetworkID for P/X/C
//	    Logger:    m.Log,
//	    Gossiper:  &networkGossiper{net: m.Net, msgCreator: m.MsgCreator},
//	    VM:        vm.(chain.BlockBuilder),
//	})
//	if err := runtime.Start(ctx, true); err != nil { return err }
//	go runtime.ForwardVMNotifications(toEngine)
func NewRuntime(cfg NetworkConfig) *Runtime {
	// Use provided params or default
	params := config.DefaultParams()
	if cfg.Params != nil {
		params = *cfg.Params
	}

	// Dynamic committee sizing (liveness, one mechanism): a static preset K can
	// exceed the live validator count. TestnetParams (K=11, α=8) on a 5-validator
	// network demands 8 affirmative votes from 5 nodes — unreachable — so NO block
	// ever finalizes: each block verifies but the α-of-K quorum cert never
	// assembles and the chain wedges. Scale K down to the live set and α to the BFT
	// supermajority ⌊2K/3⌋+1 — the exact relation every preset already encodes
	// (K4→α3, K11→α8, K20→α14, K21→α15) — so any validator count yields a
	// satisfiable, BFT-valid (K, α) with no per-network preset tuning to drift. The
	// clamp only ever shrinks an oversized committee (params.K > count); a preset
	// that already fits (K ≤ count) is untouched. The ⅔-STAKE cert
	// (WithStakeWeighting, below) still layers weighted BFT safety on top of this
	// count floor, so a smaller committee never weakens the supermajority guarantee.
	validatorCount := -1
	if cfg.Validators != nil {
		validatorCount = cfg.Validators.Count(cfg.NetworkID)
		if k, alpha, clamped := bftCommittee(params.K, validatorCount); clamped {
			params.K = k
			params.AlphaPreference = alpha
			params.AlphaConfidence = alpha
		}
	}

	engine := NewWithParams(params)

	// Wire α-of-K quorum-cert finality for multi-validator chains. The engine
	// refuses to Start a K>1 engine without a verifier (fail-closed), so a
	// production multi-validator chain MUST supply cfg.VoteVerifier. The cert
	// gossiper bridges the network Gossiper's cert-distribution to the engine.
	if cfg.VoteVerifier != nil {
		var certGossiper CertGossiper
		if cfg.Gossiper != nil {
			certGossiper = &gossiperCertBridge{
				gossiper:  cfg.Gossiper,
				chainID:   cfg.ChainID,
				networkID: cfg.NetworkID,
			}
		}
		WithQuorumCert(cfg.ChainID, cfg.NodeID, cfg.VoteVerifier, certGossiper, cfg.VoteSigner)(engine)
		// Stake-weighted finality (HIGH-3): when the node supplies validator
		// weights, a cert must clear a ⅔-of-stake supermajority, not just the
		// α-of-K count.
		if cfg.StakeSource != nil {
			WithStakeWeighting(cfg.StakeSource)(engine)
		}
		// Epoch binding (MEDIUM): pin every vote/cert to the active weighted set
		// at the block's height so the stake predicate is enforced at the
		// cert-position epoch (a cross-epoch cert fails verification).
		if cfg.ValidatorSetRoot != nil {
			WithValidatorSetRoot(cfg.ValidatorSetRoot)(engine)
		}
	}

	rt := &Runtime{
		Transitive: engine,
		config:     cfg,
		validators: cfg.Validators,
		nodeID:     cfg.NodeID,
	}

	// Wire runtime auto-recovery (ONE mechanism, two triggers): the engine holds
	// the fetch GATE (claimCatchupLocked) and the missing-PARENT trigger
	// (followVerifiedBlock → requestCatchup); the engine's missing-BYTES trigger
	// (handleVote buffering a vote for an untracked block) signals through
	// requestMissing into the SAME requestCatchup transport. WithCatchup gives the
	// engine the transport handle for its gate's nil-check; requestMissing supplies
	// the networkID-bearing round-trip the engine itself cannot make.
	WithCatchup(cfg.Catchup)(engine)
	engine.requestMissing = rt.requestCatchup

	// Log validator set status for debugging
	hasLogger := cfg.Logger != nil && !cfg.Logger.IsZero()
	if validatorCount >= 0 {
		if hasLogger {
			cfg.Logger.Info("consensus engine initialized with validator set",
				log.Stringer("networkID", cfg.NetworkID),
				log.Int("validatorCount", validatorCount),
				log.Int("k", params.K),
				log.Int("alpha", params.AlphaPreference))
		}
	} else {
		if hasLogger {
			cfg.Logger.Warn("consensus engine initialized WITHOUT validator set - will broadcast to all peers",
				log.Stringer("networkID", cfg.NetworkID))
		}
	}

	// Wire the proposer (adapts Gossiper to BlockProposer interface).
	// In single-node mode (K=1, e.g. --dev), provide a self-voter callback
	// so the proposer can accept its own blocks without network round-trips.
	var selfVoter func(ids.ID)
	if params.K == 1 {
		selfVoter = func(blockID ids.ID) {
			engine.ReceiveVote(Vote{
				BlockID:  blockID,
				NodeID:   cfg.NodeID,
				Accept:   true,
				SignedAt: time.Now(),
			})
		}
	}
	engine.SetProposer(&gossiperProposer{
		gossiper:   cfg.Gossiper,
		chainID:    cfg.ChainID,
		networkID:  cfg.NetworkID,
		logger:     cfg.Logger,
		validators: cfg.Validators,
		nodeID:     cfg.NodeID,
		k:          params.K,
		selfVoter:  selfVoter,
	})

	// Set the VM for block building
	if cfg.VM != nil {
		engine.SetVM(cfg.VM)
	}

	return rt
}

// SampleValidators returns k random validator NodeIDs for the network.
// This is used by the consensus engine for k-sampling polls.
// Returns nil if no validator sampler is configured (falls back to broadcast).
func (rt *Runtime) SampleValidators(k int) ([]ids.NodeID, error) {
	if rt.validators == nil {
		return nil, nil // Will broadcast to all
	}
	return rt.validators.Sample(rt.config.NetworkID, k)
}

// ValidatorCount returns the number of validators in the network.
// Returns 0 if no validator sampler is configured.
func (rt *Runtime) ValidatorCount() int {
	if rt.validators == nil {
		return 0
	}
	return rt.validators.Count(rt.config.NetworkID)
}

// FinalizedLedger returns the in-process consensus finalized tip id and height, and
// whether ANY block has been finalized yet (set=false on an un-seeded / empty-genesis
// node before its first finalize).
//
// This is the SINGLE advancing source of truth for "where the node's accepted chain
// actually is" — the SAME per-height ledger the bootstrap contiguity check trusts
// (bootstrap_accept.go reads consensus.GetFinalizedHeight directly). It ADVANCES as
// blocks finalize via FinalizeBranch. It must NOT be confused with the VM's
// LastAccepted, which a fire-and-forget Accept can leave FROZEN at the boot snapshot —
// the staleness this engine already designs around (see fastFollowHeight above: "1.
// VM.LastAccepted() is stale after Accept() calls"). The node's bootstrap caught-up /
// height signals read THIS, not the VM cache, so a frozen VM.LastAccepted can never make
// a converging node re-descend forever and false-stall.
func (rt *Runtime) FinalizedLedger() (tip ids.ID, height uint64, set bool) {
	if rt.Transitive == nil || rt.Transitive.consensus == nil {
		return ids.Empty, 0, false
	}
	h, ok := rt.Transitive.consensus.GetFinalizedHeight()
	if !ok {
		return ids.Empty, 0, false
	}
	return rt.Transitive.consensus.GetFinalizedTip(), h, true
}

// FinalizedBlockAtHeight returns the block finalized at height h in the in-process
// per-height ledger, with ok=false when the node has not finalized that height THIS
// session (a height below the boot seed is never re-seeded, so it reads absent).
//
// It is the authoritative IN-PROCESS fork-sibling oracle — it replaces the dead coreth
// height index the node's acceptance check used to call (block.ChainVM.GetBlockIDAtHeight
// is unhandled over ZAP, so it returned nothing on the real C-Chain). When the ledger
// knows h it rejects a stored-but-losing sibling (canonical != id); when it does not
// (a height below the boot seed), the node degrades to the height-bound anchor
// (h > finalizedHeight ⇒ not accepted), which the ⅔-by-stake frontier naming (C1) backs.
func (rt *Runtime) FinalizedBlockAtHeight(h uint64) (ids.ID, bool) {
	if rt.Transitive == nil || rt.Transitive.consensus == nil {
		return ids.Empty, false
	}
	return rt.Transitive.consensus.FinalizedBlockAtHeight(h)
}

// ForwardVMNotifications reads from the VM's toEngine channel and forwards
// PendingTxs notifications to trigger block building through consensus.
//
// Call this as a goroutine after Start():
//
//	go runtime.ForwardVMNotifications(toEngine)
//
// The goroutine exits when the channel is closed.
func (rt *Runtime) ForwardVMNotifications(toEngine <-chan block.Message) {
	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("starting VM notification forwarder for Lux consensus",
			log.Stringer("chainID", rt.config.ChainID))
	}

	for msg := range toEngine {
		// Translate block.MessageType → engine.MessageType
		// block.PendingTxs = 1 (iota+1), core.PendingTxs = 0
		// block.StateSyncDone = 2, core.StateSyncDone = 1
		var engineMsgType MessageType
		switch msg.Type {
		case block.PendingTxs:
			engineMsgType = PendingTxs
		case block.StateSyncDone:
			engineMsgType = StateSyncDone
		default:
			if !rt.config.Logger.IsZero() {
				rt.config.Logger.Warn("unknown VM message type, dropping",
					log.Uint32("type", uint32(msg.Type)))
			}
			continue
		}

		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("received VM notification, forwarding to consensus engine",
				log.Uint32("vmType", uint32(msg.Type)),
				log.Uint32("engineType", uint32(engineMsgType)))
		}

		ctx := context.Background()
		if err := rt.Notify(ctx, Message{Type: engineMsgType}); err != nil {
			if rt.config.Logger != nil && !rt.config.Logger.IsZero() {
				rt.config.Logger.Warn("failed to notify consensus engine",
					log.Uint32("type", uint32(engineMsgType)),
					log.Err(err))
			}
		}

		// In single-node mode, drain any accepted blocks and call VM.Accept().
		// Notify → buildBlocksLocked → AddBlock + ProcessVote + Poll may have
		// accepted blocks synchronously. The engine tracks them in pendingBlocks
		// with Decided=false until we finalize here.
		rt.drainAcceptedBlocks(ctx)
	}

	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("VM notification forwarder stopped")
	}
}

// drainAcceptedBlocks finalizes any blocks that consensus has accepted.
// This is called after each VM notification to ensure accepted blocks have
// their VM.Accept() called promptly (especially important in single-node mode
// where blocks are accepted synchronously during buildBlocksLocked).
func (rt *Runtime) drainAcceptedBlocks(ctx context.Context) {
	rt.Transitive.DrainAccepted(ctx)
}

// gossiperCertBridge adapts the engine's CertGossiper (GossipCert(chainID,
// blockID, bytes)) to the network Gossiper's QuorumGossiper.GossipCert
// (chainID, networkID, blockID, bytes). If the configured Gossiper does not
// implement QuorumGossiper, cert gossip is a no-op (the engine still finalizes
// locally on the verified cert; followers reach finality via their own votes).
type gossiperCertBridge struct {
	gossiper  Gossiper
	chainID   ids.ID
	networkID ids.ID
}

var _ CertGossiper = (*gossiperCertBridge)(nil)

func (b *gossiperCertBridge) GossipCert(_ ids.ID, blockID ids.ID, certBytes []byte) error {
	if qg, ok := b.gossiper.(QuorumGossiper); ok {
		qg.GossipCert(b.chainID, b.networkID, blockID, certBytes)
	}
	return nil
}

// gossiperProposer adapts a Gossiper to the BlockProposer interface.
// This bridges the network layer (Gossiper) to the consensus layer (BlockProposer).
type gossiperProposer struct {
	gossiper   Gossiper
	chainID    ids.ID
	networkID  ids.ID
	logger     log.Logger
	validators ValidatorSampler // For k-peer sampling
	nodeID     ids.NodeID       // This node's ID (to exclude from samples)
	k          int              // Sample size from consensus params
	selfVoter  func(ids.ID)     // Callback for single-node self-voting (--dev mode)
}

var _ BlockProposer = (*gossiperProposer)(nil)

// Propose broadcasts a block proposal to validators via the network gossiper.
func (p *gossiperProposer) Propose(ctx context.Context, proposal BlockProposal) error {
	if p.gossiper == nil {
		if p.logger != nil && !p.logger.IsZero() {
			p.logger.Warn("cannot propose block - gossiper is nil",
				log.Stringer("blockID", proposal.BlockID))
		}
		return nil // Not fatal - local acceptance still works
	}

	sentTo := p.gossiper.GossipPut(p.chainID, p.networkID, proposal.BlockData)
	if p.logger != nil && !p.logger.IsZero() {
		p.logger.Info("proposed block to validators",
			log.Stringer("blockID", proposal.BlockID),
			log.Uint64("height", proposal.Height),
			log.Int("sentTo", sentTo))
	}
	return nil
}

// RequestVotes asks specific validators to vote on a block.
// If req.Validators is nil and we have a ValidatorSampler, sample k validators.
// This implements Lux k-sampling: select k validators and request votes.
//
// Single-node mode (K=1, only validator is self): the proposer delivers a
// self-vote via the SelfVoter callback instead of polling the network.
// This is the standard path for --dev mode (local single-validator networks).
func (p *gossiperProposer) RequestVotes(ctx context.Context, req VoteRequest) error {
	if p.gossiper == nil {
		if p.logger != nil && !p.logger.IsZero() {
			p.logger.Warn("cannot request votes - gossiper is nil",
				log.Stringer("blockID", req.BlockID))
		}
		return nil
	}

	// Determine which validators to query
	validators := req.Validators
	if validators == nil && p.validators != nil && p.k > 0 {
		// Sample k validators from the validator set (excluding self)
		sampled, err := p.validators.Sample(p.networkID, p.k)
		if err != nil {
			if p.logger != nil && !p.logger.IsZero() {
				p.logger.Warn("failed to sample validators, falling back to broadcast",
					log.Stringer("blockID", req.BlockID),
					log.Int("k", p.k),
					log.Err(err))
			}
			// Fall back to broadcast (nil validators)
		} else {
			// Filter out self from sample
			filtered := make([]ids.NodeID, 0, len(sampled))
			for _, nodeID := range sampled {
				if nodeID != p.nodeID {
					filtered = append(filtered, nodeID)
				}
			}
			validators = filtered

			// Single-node mode: all sampled validators were self.
			// Deliver a self-vote directly — no network round-trip needed.
			if len(filtered) == 0 && p.k == 1 && p.selfVoter != nil {
				if p.logger != nil && !p.logger.IsZero() {
					p.logger.Info("single-node mode: self-voting for proposed block",
						log.Stringer("blockID", req.BlockID))
				}
				p.selfVoter(req.BlockID)
				return nil
			}

			if p.logger != nil && !p.logger.IsZero() {
				p.logger.Debug("sampled k validators for poll",
					log.Stringer("blockID", req.BlockID),
					log.Int("k", p.k),
					log.Int("sampled", len(validators)),
					log.Int("totalValidators", p.validators.Count(p.networkID)))
			}
		}
	}

	var sentTo int
	if len(req.BlockData) > 0 {
		// PushQuery: send block bytes + vote request in one message.
		// Peers can immediately verify and respond with their vote.
		sentTo = p.gossiper.SendPushQuery(p.chainID, p.networkID, req.BlockData, validators)
	} else {
		// Fallback to PullQuery (blockID only) if no block data available.
		sentTo = p.gossiper.SendPullQuery(p.chainID, p.networkID, req.BlockID, validators)
	}
	if p.logger != nil && !p.logger.IsZero() {
		p.logger.Debug("requested votes from validators",
			log.Stringer("blockID", req.BlockID),
			log.Int("requested", len(validators)),
			log.Int("sentTo", sentTo),
			log.Bool("pushQuery", len(req.BlockData) > 0))
	}
	return nil
}

// HandleIncomingBlock processes a block received from network gossip.
// For follower nodes receiving blocks from the proposer, this uses a "fast-follow"
// pattern where verified blocks extending the accepted chain are accepted immediately.
//
// This is necessary because in the current architecture, votes are only sent back
// to the proposer (not gossiped to all validators). So followers would never reach
// the vote threshold on their own. Instead, followers trust that:
// 1. The proposer collected enough votes before gossiping the block
// 2. The block verifies correctly against their state
// 3. The block extends their current chain tip
//
// Returns the parsed block if successful, nil otherwise.
func (rt *Runtime) HandleIncomingBlock(ctx context.Context, blockData []byte, fromNodeID ids.NodeID) (block.Block, error) {
	if rt.config.VM == nil {
		return nil, nil
	}

	// Parse the block
	blk, err := rt.config.VM.ParseBlock(ctx, blockData)
	if err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("failed to parse incoming block",
				log.Stringer("from", fromNodeID),
				log.Err(err))
		}
		return nil, err
	}

	// Verify the block
	if err := blk.Verify(ctx); err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("incoming block failed verification",
				log.Stringer("blockID", blk.ID()),
				log.Stringer("from", fromNodeID),
				log.Err(err))
		}
		return blk, err
	}

	if !rt.config.Logger.IsZero() {
		rt.config.Logger.Info("received and verified block from gossip",
			log.Stringer("blockID", blk.ID()),
			log.Uint64("height", blk.Height()),
			log.Stringer("from", fromNodeID))
	}

	// QUORUM-GATED FOLLOW (replaces the old unverified fast-follow Accept):
	//
	// A follower MUST NOT Accept a gossiped block on mere arrival — that was the
	// follower-side half of the self-finality hole (an equivocating proposer
	// could get followers to commit two different blocks at one height). Instead
	// the follower:
	//   1. has already VERIFIED the block (above),
	//   2. tracks it for consensus and casts its OWN signed accept vote,
	//   3. BROADCASTS that signed vote to ALL validators (topology fix), and
	//   4. Accepts ONLY when it holds a verified α-of-K QuorumCert
	//      (handleIncomingCert, or a cert it assembles from gossiped votes).
	//
	// This both closes the fork (no commit without α-of-K witness) AND keeps
	// liveness (votes reach every validator, so finality does not hinge on one
	// node's inbound Chits; the proposer-freeze cannot recur).
	blockID := blk.ID()
	rt.fastFollowMu.Lock()
	defer rt.fastFollowMu.Unlock()

	incomingHeight := blk.Height()

	// Dedup: skip blocks at or below our tracked finalized height.
	if incomingHeight <= rt.fastFollowHeight {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Debug("follow: block at/below tracked height, skipping",
				log.Stringer("blockID", blockID),
				log.Uint64("incomingHeight", incomingHeight),
				log.Uint64("trackedHeight", rt.fastFollowHeight))
		}
		return blk, nil
	}

	// Track the verified block in consensus + pending and record OUR signed
	// accept vote toward its cert. castAndBroadcastVote returns after the vote
	// is queued for all validators. The block is NOT accepted here — finality
	// awaits the cert.
	rt.followVerifiedBlock(ctx, blk, fromNodeID)
	return blk, nil
}

// followVerifiedBlock tracks a verified gossip block and casts+broadcasts this
// node's signed accept vote to ALL validators. It does NOT Accept the block:
// acceptance is gated on a verified α-of-K QuorumCert (handleIncomingCert).
//
// The caller holds rt.fastFollowMu.
func (rt *Runtime) followVerifiedBlock(ctx context.Context, blk block.Block, fromNodeID ids.NodeID) {
	blockID := blk.ID()
	childEpoch := pChainHeightOf(blk) // epoch for the weighted set (MEDIUM-1)

	// RECEIVE-SIDE EPOCH GATE (HIGH-1, predicate a — monotonicity): refuse to
	// track or vote for a gossiped block whose stamped P-chain epoch height
	// REGRESSES below its parent's recorded epoch. The proposer's build-side
	// max(currentH, parentH) is proposer-only; a Byzantine proposer skips it and
	// stamps a stale H_old (a past epoch where its departed coalition held ≥⅔,
	// signed with leaked old keys) to finalize a fresh block against a validator
	// set the current set never approved (safety break). The parent's epoch is read
	// from the engine's tracked-block ledger (EpochHeightOf) — the authoritative
	// "epoch we recorded for the parent". Enforcing childEpoch ≥ parentEpoch here,
	// against the block's REAL parent (parentID is the inner block's own parent,
	// which the cert also binds — the attacker cannot decouple them), means a
	// chain's epoch can only move forward, so the far-past attack is closed and
	// safety reduces to current-set BFT. The recency UPPER bound (absurd-future H)
	// is enforced at the node boundary that holds the live P-chain height
	// (pChainHeightVM.ParseBlock), so the two predicates jointly bound the epoch to
	// [parentEpoch, localCurrentH+slack]. A missing parent (not yet tracked) leaves
	// nothing to regress against and is admitted: the far-past attack needs a stale
	// epoch BELOW the parent's, which is only meaningful once the parent is tracked;
	// an orphan with no tracked parent cannot extend finalized history anyway.
	if parentEpoch, ok := rt.Transitive.consensus.EpochHeightOf(blk.ParentID()); ok && childEpoch < parentEpoch {
		if rt.config.Logger != nil && !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("follow: REFUSED block — P-chain epoch regresses below parent (far-past epoch attack)",
				log.Stringer("blockID", blockID),
				log.Stringer("parentID", blk.ParentID()),
				log.Uint64("childEpoch", childEpoch),
				log.Uint64("parentEpoch", parentEpoch),
				log.Stringer("from", fromNodeID))
		}
		return
	}

	// AUTO-RECOVERY (the behind-follower self-heal): if this block's PARENT is one
	// we do not have — not Empty, not our finalized tip, not tracked/known — then
	// we are BEHIND. The child is an orphan we cannot finalize (the per-height
	// guard requires parent==finalizedTip) and re-polling it would be pure spam to
	// peers who are ahead. Instead fetch the missing chain via the catch-up seam;
	// the fetched ancestors arrive back through HandleIncomingBlock, fill the gap,
	// and the frontier reconciles — no manual snapshot reset. The fetch is
	// idempotent + throttled in the engine (claimCatchupLocked). We still track the
	// orphan below so it finalizes the moment its parent lands.
	rt.requestCatchup(blk.ParentID(), fromNodeID)

	consensusBlock := &Block{
		id:           blockID,
		parentID:     blk.ParentID(),
		height:       blk.Height(),
		timestamp:    blk.Timestamp().Unix(),
		data:         blk.Bytes(),
		pChainHeight: childEpoch,
	}
	setCanonicalFromVM(consensusBlock, blk) // stamp the inner execution commitment

	// Add to consensus tracking (idempotent: AddBlock errors if already present).
	_ = rt.Transitive.consensus.AddBlock(ctx, consensusBlock)

	rt.Transitive.mu.Lock()
	pending, exists := rt.Transitive.pendingBlocks[blockID]
	if !exists {
		pending = &PendingBlock{
			ConsensusBlock: consensusBlock,
			VMBlock:        blk,
			ProposedAt:     time.Now(),
			VoteCount:      0,
			Decided:        false,
			IsOwnProposal:  false,
		}
		rt.Transitive.pendingBlocks[blockID] = pending
	}
	chainID := rt.Transitive.chainID
	nodeID := rt.Transitive.nodeID
	signer := rt.Transitive.voteSigner
	verifier := rt.Transitive.voteVerifier
	pos := rt.Transitive.blockPositionLocked(pending, blockID)
	rt.Transitive.mu.Unlock()

	// THE SELF-HEAL DRAIN: this block is now tracked, so replay any votes a peer
	// parked for it before its bytes arrived (the gossip race handleVote buffered).
	// Each parked vote re-enters handleVote via the channel — now with the block
	// tracked — and is signature-verified exactly as a live vote. This is what
	// turns the former wedge (vote-before-block dropped forever) into finality:
	// the fetched/late block lands here and its buffered α-of-K votes complete the
	// quorum. Drain after unlock — drainBufferedVotes takes the engine lock.
	rt.Transitive.drainBufferedVotes(blockID)

	// CONVERGENCE (avalanchego snowman voter.go: SetPreference(Consensus.Preference())
	// after every poll): steer the inner VM to build on the engine's preferred BUILD
	// tip — the deepest verified block — now that this gossiped block is tracked. Without
	// it the VM keeps building on the last FINALIZED block, so when a proposer is down
	// every validator builds its own competing sibling at the same height, the α-of-K
	// votes split across the siblings, no cert assembles, and the chain HALTS. Steering
	// to the verified tip makes validators build H+1 on top of one verified block at H
	// and converge. Build hint only (Preference is not a finality decision); best effort.
	if rt.config.VM != nil {
		if tip := rt.Transitive.PreferredBuildTip(); tip != ids.Empty {
			_ = rt.config.VM.SetPreference(ctx, tip)
		}
	}

	// Single-validator / no-signer engines do not gossip votes; nothing to do.
	if signer == nil || verifier == nil {
		return
	}

	// NON-EQUIVOCATION (fork guard): a follower must NOT sign a conflicting sibling
	// at a height it has already committed to — that is the cross-node fork (two
	// valid siblings each gathering a ⅔-stake cert). Same canonical ⇒ idempotent.
	if !rt.Transitive.reserveSlotForSign(pos.Height, slotCanonical(pos)) {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("vote-once: follower refusing conflicting sibling at an already-committed height",
				log.Uint64("height", pos.Height), log.Stringer("blockID", blockID))
		}
		return
	}

	// Sign our accept vote over the canonical position message.
	message := CanonicalVoteMessage(pos)
	sig, err := signer.SignVote(message)
	if err != nil {
		if !rt.config.Logger.IsZero() {
			rt.config.Logger.Warn("follow: failed to sign accept vote",
				log.Stringer("blockID", blockID), log.Err(err))
		}
		return
	}

	// Record our own vote locally toward the cert (so a node that is itself
	// near-quorum can assemble), then broadcast it to ALL validators.
	ownVote := Vote{
		BlockID:   blockID,
		NodeID:    nodeID,
		Accept:    true,
		SignedAt:  time.Now(),
		Signature: sig,
		ParentID:  pos.ParentID,
		Round:     pos.Round,
	}
	rt.Transitive.ReceiveVote(ownVote)

	if qg, ok := rt.config.Gossiper.(QuorumGossiper); ok {
		if voteBytes, encErr := encodeSignedVote(nodeID, sig); encErr == nil {
			qg.BroadcastVote(chainID, rt.config.NetworkID, blockID, voteBytes)
		}
	} else if rt.config.Gossiper != nil {
		// Legacy gossiper without quorum support: at least notify the proposer
		// (keeps a degraded path working) — but finality still requires a cert.
		_ = rt.config.Gossiper.SendVote(rt.config.ChainID, fromNodeID, blockID)
	}
}

// requestCatchup is the ONE catch-up TRANSPORT: "I am missing block `missingID`
// — fetch it from `from`." It is the single mechanism for runtime auto-recovery,
// shared by BOTH self-heal triggers:
//   - a gossiped child whose PARENT we lack (followVerifiedBlock), and
//   - a vote for a block whose BYTES we lack (the engine's requestMissing hook,
//     wired to this in NewRuntime).
//
// The engine owns idempotency + rate-limiting (claimCatchupLocked: one fetch per
// missing ID per catchupCooldown, suppressed once the block is tracked/known), so
// this method only supplies the networkID the engine does not hold and performs
// the RequestAncestors round-trip. The fetched block arrives back through
// HandleIncomingBlock, where it is tracked and its buffered votes drained. nil
// Catchup / Empty missingID ⇒ no-op (claimCatchupLocked returns false). The gate
// is claimed UNDER the engine lock; the transport call is made WITHOUT it (it may
// touch the network).
func (rt *Runtime) requestCatchup(missingID ids.ID, from ids.NodeID) {
	rt.Transitive.mu.Lock()
	claim := rt.Transitive.claimCatchupLocked(missingID)
	rt.Transitive.mu.Unlock()
	if !claim {
		return
	}
	_ = rt.config.Catchup.RequestAncestors(rt.config.ChainID, rt.config.NetworkID, missingID, from)
}

// OnImportComplete must be called after admin_importChain (RLP import) completes.
// This reconciles the consensus engine's state with the VM's actual state after import.
//
// The problem this solves:
//   - RLP import updates the EVM state database directly
//   - But the consensus engine still thinks lastAccepted is the old block
//   - This causes transactions to timeout (engine builds on wrong parent)
//   - And causes "chains not bootstrapped" errors on node restart
//
// This method:
//  1. Queries VM.LastAccepted() to get the current chain tip after import
//  2. Updates consensus.finalizedTip to match
//  3. Updates VM preference to build on the new tip
//  4. Marks consensus as bootstrapped
//
// Usage in EVM admin API after successful import:
//
//	if err := rt.OnImportComplete(ctx); err != nil {
//	    log.Warn("failed to sync consensus after import", "error", err)
//	}
//
// This is idempotent - safe to call even if import didn't change state.
func (rt *Runtime) OnImportComplete(ctx context.Context) error {
	logger := rt.config.Logger
	hasLogger := logger != nil && !logger.IsZero()

	if rt.config.VM == nil {
		if hasLogger {
			logger.Warn("OnImportComplete: VM is nil, cannot sync state")
		}
		return nil
	}

	// Step 1: Query VM for current last accepted block
	lastAcceptedID, err := rt.config.VM.LastAccepted(ctx)
	if err != nil {
		if hasLogger {
			logger.Warn("OnImportComplete: failed to get last accepted from VM",
				log.Err(err))
		}
		return err
	}

	// Step 2: Get block details (height) for consensus state
	var height uint64
	if lastAcceptedID != ids.Empty {
		blk, err := rt.config.VM.GetBlock(ctx, lastAcceptedID)
		if err != nil {
			if hasLogger {
				logger.Warn("OnImportComplete: failed to get block details",
					log.Stringer("blockID", lastAcceptedID),
					log.Err(err))
			}
			// Continue with height 0 - consensus can recover
		} else {
			height = blk.Height()
		}
	}

	// Step 3: Update VM preference to build on current tip
	// This is critical: without this, the VM's Preferred() returns old block
	// while GetLastAccepted() returns the imported block, causing state mismatch
	if err := rt.config.VM.SetPreference(ctx, lastAcceptedID); err != nil {
		if hasLogger {
			logger.Warn("OnImportComplete: failed to set VM preference",
				log.Stringer("blockID", lastAcceptedID),
				log.Err(err))
		}
		// Non-fatal: continue with consensus sync
	}

	// Step 4: Sync consensus engine state
	if err := rt.Transitive.SyncState(ctx, lastAcceptedID, height); err != nil {
		if hasLogger {
			logger.Warn("OnImportComplete: failed to sync consensus state",
				log.Stringer("blockID", lastAcceptedID),
				log.Uint64("height", height),
				log.Err(err))
		}
		return err
	}

	if hasLogger {
		logger.Info("OnImportComplete: consensus state synced with VM",
			log.Stringer("lastAcceptedID", lastAcceptedID),
			log.Uint64("height", height))
	}

	return nil
}

// SyncStateFromVM queries the VM for its current state and syncs the consensus
// engine to match. This is a lower-level version of OnImportComplete that can
// be called without a full Runtime (e.g., from standalone syncer usage).
//
// Returns the synced block ID and height, or error.
func SyncStateFromVM(ctx context.Context, vm BlockBuilder, consensus *Transitive) (ids.ID, uint64, error) {
	if vm == nil {
		return ids.Empty, 0, nil
	}

	// Get last accepted from VM
	lastAcceptedID, err := vm.LastAccepted(ctx)
	if err != nil {
		return ids.Empty, 0, err
	}

	// Get height
	var height uint64
	if lastAcceptedID != ids.Empty {
		blk, err := vm.GetBlock(ctx, lastAcceptedID)
		if err == nil {
			height = blk.Height()
		}
	}

	// Set preference (non-fatal if this fails)
	_ = vm.SetPreference(ctx, lastAcceptedID)

	// Sync consensus
	if consensus != nil {
		if err := consensus.SyncState(ctx, lastAcceptedID, height); err != nil {
			return lastAcceptedID, height, err
		}
	}

	return lastAcceptedID, height, nil
}
