// Copyright (C) 2019-2025, Lux Industries Inc All rights reserved.
// See the file LICENSE for licensing terms.

//go:build skip
// +build skip

// Package main demonstrates adding Lux post-quantum finality to external sequencers.
//
// Use Case: External sequencers (OP Stack, Arbitrum, Base, etc.) produce blocks
// but don't provide PQ-resistant finality. This adapter wraps any sequencer
// output with Lux consensus + ML-DSA/ML-KEM attestations.
//
// Flow:
//   External Sequencer → Lux Validators (PQ attestations) → L1 with PQ proof
//
// This is NOT modifying the external sequencer - it's an overlay that adds
// post-quantum finality guarantees to any sequencer's output.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/luxfi/consensus/engine"
	"github.com/luxfi/consensus/pkg/wire"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/ids"
)

// =============================================================================
// SEQUENCER INTERFACE: Abstract any external sequencer
// =============================================================================

// SequencerType identifies the external sequencer
type SequencerType string

const (
	SequencerOPStack  SequencerType = "op-stack"
	SequencerArbitrum SequencerType = "arbitrum"
	SequencerBase     SequencerType = "base"
	SequencerZKSync   SequencerType = "zksync"
	SequencerScroll   SequencerType = "scroll"
	SequencerCustom   SequencerType = "custom"
)

// ExternalBlock represents a block from any external sequencer
type ExternalBlock struct {
	// Sequencer-agnostic fields
	BlockHash   [32]byte // Content hash from sequencer
	ParentHash  [32]byte
	Height      uint64
	Timestamp   uint64
	StateRoot   [32]byte
	TxRoot      [32]byte // Merkle root of transactions
	TxCount     uint32
	RawPayload  []byte // Original sequencer block bytes

	// Source identification
	SequencerType SequencerType
	ChainID       uint64
	SequencerAddr []byte // Sequencer's signing address
}

// SequencerClient is the interface for connecting to any external sequencer
type SequencerClient interface {
	// GetLatestBlock fetches the latest block from the sequencer
	GetLatestBlock(ctx context.Context) (*ExternalBlock, error)

	// GetBlockByHeight fetches a specific block
	GetBlockByHeight(ctx context.Context, height uint64) (*ExternalBlock, error)

	// SubscribeBlocks streams new blocks
	SubscribeBlocks(ctx context.Context) (<-chan *ExternalBlock, error)

	// GetSequencerType returns the sequencer type
	GetSequencerType() SequencerType
}

// =============================================================================
// FINALITY ADAPTER: Wraps external sequencer with Lux PQ finality
// =============================================================================

// FinalityAdapter adds Lux post-quantum finality to external sequencer output
type FinalityAdapter struct {
	// External sequencer connection
	sequencer SequencerClient

	// Lux consensus engine
	consensus engine.Consensus

	// PQ signing keys
	mldsaPrivKey *mldsa.PrivateKey
	blsPrivKey   *bls.SecretKey

	// Configuration
	config *FinalityConfig

	// State
	pendingBlocks map[[32]byte]*PendingFinality
	finalizedCh   chan *FinalizedBlock
}

// FinalityConfig configures the finality adapter
type FinalityConfig struct {
	// Consensus parameters
	K               int           // Sample size (default: 20)
	Alpha           int           // Quorum threshold (default: 15)
	Beta            int           // Finality confidence (default: 20)
	FinalityTimeout time.Duration // Max time to wait for finality

	// External sequencer
	SequencerRPC string
	ChainID      uint64

	// L1 submission (optional)
	L1RPC           string
	SubmitToL1      bool
	L1BatchInterval time.Duration
}

// DefaultFinalityConfig returns sensible defaults
func DefaultFinalityConfig() *FinalityConfig {
	return &FinalityConfig{
		K:               20,
		Alpha:           15,
		Beta:            20,
		FinalityTimeout: 30 * time.Second,
		L1BatchInterval: 12 * time.Second, // L1 block time
	}
}

// PendingFinality tracks a block awaiting finality
type PendingFinality struct {
	Block       *ExternalBlock
	CandidateID wire.CandidateID
	ReceivedAt  time.Time
	Votes       []*PQVote
	Certificate *PQCertificate
}

// PQVote is a post-quantum vote on a block
type PQVote struct {
	VoterID     wire.VoterID
	CandidateID wire.CandidateID
	Preference  bool
	Round       uint64

	// Dual signature: BLS + ML-DSA
	BLSSig   []byte // 96 bytes
	MLDSASig []byte // 3293 bytes (ML-DSA-65)
}

// PQCertificate is the finality proof with aggregated PQ signatures
type PQCertificate struct {
	CandidateID wire.CandidateID
	Height      uint64

	// Aggregated BLS signature (compact)
	AggBLSSig []byte // 96 bytes
	SignerSet []byte // Bitmap of signers

	// Representative ML-DSA signatures (for PQ security)
	// In production: threshold scheme or all signatures
	MLDSASigs [][]byte
	MLDSAPKs  [][]byte // Corresponding public keys

	// Timestamp
	FinalizedAt int64
}

// FinalizedBlock is an external block with Lux PQ finality
type FinalizedBlock struct {
	Block       *ExternalBlock
	Certificate *PQCertificate
	FinalizedAt time.Time
}

// NewFinalityAdapter creates a new finality adapter
func NewFinalityAdapter(sequencer SequencerClient, config *FinalityConfig) (*FinalityAdapter, error) {
	if config == nil {
		config = DefaultFinalityConfig()
	}

	// Initialize Lux consensus
	params := engine.Parameters{
		K:                     config.K,
		AlphaPreference:       config.Alpha,
		AlphaConfidence:       config.Alpha,
		Beta:                  config.Beta,
		ConcurrentPolls:       2,
		MaxOutstandingItems:   1024,
		MaxItemProcessingTime: config.FinalityTimeout,
	}

	factory := engine.NewConsensusFactory()
	consensus, err := factory.CreateConsensus(params)
	if err != nil {
		return nil, fmt.Errorf("failed to create consensus: %w", err)
	}

	// Generate PQ keys (in production: load from secure storage)
	_, mldsaPriv, err := mldsa.GenerateKey(mldsa.MLDSA65)
	if err != nil {
		return nil, fmt.Errorf("failed to generate ML-DSA key: %w", err)
	}

	blsPriv, err := bls.GenerateSecretKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate BLS key: %w", err)
	}

	return &FinalityAdapter{
		sequencer:     sequencer,
		consensus:     consensus,
		mldsaPrivKey:  mldsaPriv,
		blsPrivKey:    blsPriv,
		config:        config,
		pendingBlocks: make(map[[32]byte]*PendingFinality),
		finalizedCh:   make(chan *FinalizedBlock, 100),
	}, nil
}

// ProcessBlock adds an external block to the finality pipeline
func (fa *FinalityAdapter) ProcessBlock(ctx context.Context, block *ExternalBlock) error {
	// Derive candidate ID from block content
	candidateID := deriveCandidateID(block)

	// Create pending finality entry
	pending := &PendingFinality{
		Block:       block,
		CandidateID: candidateID,
		ReceivedAt:  time.Now(),
		Votes:       make([]*PQVote, 0),
	}
	fa.pendingBlocks[block.BlockHash] = pending

	// Add to consensus engine
	consensusBlock := &ConsensusBlockWrapper{
		id:        ids.ID(candidateID),
		parentID:  ids.ID(deriveCandidateIDFromHash(block.ParentHash)),
		height:    block.Height,
		timestamp: time.Unix(int64(block.Timestamp), 0),
		data:      block.RawPayload,
	}

	if err := fa.consensus.Add(consensusBlock); err != nil {
		return fmt.Errorf("failed to add block to consensus: %w", err)
	}

	log.Printf("[%s] Processing block %d from %s sequencer",
		hex.EncodeToString(block.BlockHash[:8]),
		block.Height,
		block.SequencerType)

	return nil
}

// VoteOnBlock creates and records a PQ vote
func (fa *FinalityAdapter) VoteOnBlock(block *ExternalBlock, preference bool) (*PQVote, error) {
	candidateID := deriveCandidateID(block)

	// Create vote message
	voteMsg := createVoteMessage(candidateID, preference)

	// Sign with BLS
	blsSig := bls.Sign(fa.blsPrivKey, voteMsg)

	// Sign with ML-DSA
	mldsaSig, err := mldsa.Sign(mldsa.MLDSA65, fa.mldsaPrivKey, voteMsg)
	if err != nil {
		return nil, fmt.Errorf("ML-DSA signing failed: %w", err)
	}

	// Derive voter ID from BLS public key
	blsPubBytes := bls.PublicKeyFromSecretKey(fa.blsPrivKey).Compress()
	voterID := wire.VoterIDFromPublicKey(blsPubBytes)

	vote := &PQVote{
		VoterID:     voterID,
		CandidateID: candidateID,
		Preference:  preference,
		BLSSig:      blsSig.Compress(),
		MLDSASig:    mldsaSig,
	}

	// Record vote
	if pending, ok := fa.pendingBlocks[block.BlockHash]; ok {
		pending.Votes = append(pending.Votes, vote)
	}

	return vote, nil
}

// CheckFinality checks if a block has achieved finality
func (fa *FinalityAdapter) CheckFinality(blockHash [32]byte) (*FinalizedBlock, bool) {
	pending, ok := fa.pendingBlocks[blockHash]
	if !ok {
		return nil, false
	}

	// Check consensus finality
	if !fa.consensus.IsAccepted(ids.ID(pending.CandidateID)) {
		return nil, false
	}

	// Create certificate from collected votes
	cert := fa.createCertificate(pending)
	if cert == nil {
		return nil, false
	}

	finalized := &FinalizedBlock{
		Block:       pending.Block,
		Certificate: cert,
		FinalizedAt: time.Now(),
	}

	// Remove from pending
	delete(fa.pendingBlocks, blockHash)

	return finalized, true
}

// createCertificate aggregates votes into a PQ certificate
func (fa *FinalityAdapter) createCertificate(pending *PendingFinality) *PQCertificate {
	if len(pending.Votes) < fa.config.Alpha {
		return nil
	}

	// Aggregate BLS signatures
	blsSigs := make([]*bls.Signature, 0, len(pending.Votes))
	signerSet := make([]byte, (len(pending.Votes)+7)/8)

	mldsaSigs := make([][]byte, 0, len(pending.Votes))
	mldsaPKs := make([][]byte, 0, len(pending.Votes))

	for i, vote := range pending.Votes {
		if !vote.Preference {
			continue
		}

		// BLS signature
		sig, err := bls.SignatureFromBytes(vote.BLSSig)
		if err == nil {
			blsSigs = append(blsSigs, sig)
			signerSet[i/8] |= 1 << (i % 8)
		}

		// ML-DSA signature (store first few for verification)
		if len(mldsaSigs) < 3 { // Store representative subset
			mldsaSigs = append(mldsaSigs, vote.MLDSASig)
			// In production: include public key
		}
	}

	// Aggregate BLS
	var aggSig []byte
	if len(blsSigs) > 0 {
		agg := bls.AggregateSignatures(blsSigs)
		aggSig = agg.Compress()
	}

	return &PQCertificate{
		CandidateID: pending.CandidateID,
		Height:      pending.Block.Height,
		AggBLSSig:   aggSig,
		SignerSet:   signerSet,
		MLDSASigs:   mldsaSigs,
		MLDSAPKs:    mldsaPKs,
		FinalizedAt: time.Now().UnixMilli(),
	}
}

// FinalizedBlocks returns the channel of finalized blocks
func (fa *FinalityAdapter) FinalizedBlocks() <-chan *FinalizedBlock {
	return fa.finalizedCh
}

// Run starts the finality adapter
func (fa *FinalityAdapter) Run(ctx context.Context) error {
	// Subscribe to sequencer blocks
	blocksCh, err := fa.sequencer.SubscribeBlocks(ctx)
	if err != nil {
		return fmt.Errorf("failed to subscribe to sequencer: %w", err)
	}

	log.Printf("Starting finality adapter for %s sequencer", fa.sequencer.GetSequencerType())

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()

		case block := <-blocksCh:
			if err := fa.ProcessBlock(ctx, block); err != nil {
				log.Printf("Error processing block: %v", err)
				continue
			}

			// Auto-vote (in production: based on validation)
			if _, err := fa.VoteOnBlock(block, true); err != nil {
				log.Printf("Error voting on block: %v", err)
			}

			// Check finality
			if finalized, ok := fa.CheckFinality(block.BlockHash); ok {
				fa.finalizedCh <- finalized
				log.Printf("[%s] Block %d FINALIZED with PQ certificate",
					hex.EncodeToString(block.BlockHash[:8]),
					block.Height)
			}
		}
	}
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

func deriveCandidateID(block *ExternalBlock) wire.CandidateID {
	// H(chain_id || block_hash)
	h := sha256.New()
	h.Write([]byte{byte(block.ChainID >> 56), byte(block.ChainID >> 48),
		byte(block.ChainID >> 40), byte(block.ChainID >> 32),
		byte(block.ChainID >> 24), byte(block.ChainID >> 16),
		byte(block.ChainID >> 8), byte(block.ChainID)})
	h.Write(block.BlockHash[:])
	var id wire.CandidateID
	copy(id[:], h.Sum(nil))
	return id
}

func deriveCandidateIDFromHash(hash [32]byte) wire.CandidateID {
	return wire.CandidateID(hash)
}

func createVoteMessage(candidateID wire.CandidateID, preference bool) []byte {
	msg := make([]byte, 33)
	copy(msg[:32], candidateID[:])
	if preference {
		msg[32] = 1
	}
	return msg
}

// ConsensusBlockWrapper wraps external block for Lux consensus
type ConsensusBlockWrapper struct {
	id        ids.ID
	parentID  ids.ID
	height    uint64
	timestamp time.Time
	data      []byte
}

func (b *ConsensusBlockWrapper) ID() ids.ID           { return b.id }
func (b *ConsensusBlockWrapper) Parent() ids.ID       { return b.parentID }
func (b *ConsensusBlockWrapper) Height() uint64       { return b.height }
func (b *ConsensusBlockWrapper) Timestamp() time.Time { return b.timestamp }
func (b *ConsensusBlockWrapper) Bytes() []byte        { return b.data }

// =============================================================================
// EXAMPLE SEQUENCER CLIENTS
// =============================================================================

// OPStackClient connects to OP Stack sequencers (Optimism, Base, etc.)
type OPStackClient struct {
	rpcURL   string
	chainID  uint64
	seqType  SequencerType
	blocksCh chan *ExternalBlock
}

func NewOPStackClient(rpcURL string, chainID uint64) *OPStackClient {
	seqType := SequencerOPStack
	if chainID == 8453 { // Base
		seqType = SequencerBase
	}
	return &OPStackClient{
		rpcURL:   rpcURL,
		chainID:  chainID,
		seqType:  seqType,
		blocksCh: make(chan *ExternalBlock, 100),
	}
}

func (c *OPStackClient) GetSequencerType() SequencerType { return c.seqType }

func (c *OPStackClient) GetLatestBlock(ctx context.Context) (*ExternalBlock, error) {
	// In production: call eth_getBlockByNumber("latest", false)
	return &ExternalBlock{
		SequencerType: c.seqType,
		ChainID:       c.chainID,
	}, nil
}

func (c *OPStackClient) GetBlockByHeight(ctx context.Context, height uint64) (*ExternalBlock, error) {
	// In production: call eth_getBlockByNumber
	return &ExternalBlock{
		SequencerType: c.seqType,
		ChainID:       c.chainID,
		Height:        height,
	}, nil
}

func (c *OPStackClient) SubscribeBlocks(ctx context.Context) (<-chan *ExternalBlock, error) {
	// In production: use eth_subscribe("newHeads")
	return c.blocksCh, nil
}

// ArbitrumClient connects to Arbitrum sequencer
type ArbitrumClient struct {
	rpcURL   string
	chainID  uint64
	blocksCh chan *ExternalBlock
}

func NewArbitrumClient(rpcURL string) *ArbitrumClient {
	return &ArbitrumClient{
		rpcURL:   rpcURL,
		chainID:  42161, // Arbitrum One
		blocksCh: make(chan *ExternalBlock, 100),
	}
}

func (c *ArbitrumClient) GetSequencerType() SequencerType { return SequencerArbitrum }

func (c *ArbitrumClient) GetLatestBlock(ctx context.Context) (*ExternalBlock, error) {
	return &ExternalBlock{
		SequencerType: SequencerArbitrum,
		ChainID:       c.chainID,
	}, nil
}

func (c *ArbitrumClient) GetBlockByHeight(ctx context.Context, height uint64) (*ExternalBlock, error) {
	return &ExternalBlock{
		SequencerType: SequencerArbitrum,
		ChainID:       c.chainID,
		Height:        height,
	}, nil
}

func (c *ArbitrumClient) SubscribeBlocks(ctx context.Context) (<-chan *ExternalBlock, error) {
	return c.blocksCh, nil
}

// =============================================================================
// EXAMPLE USAGE
// =============================================================================

func main() {
	fmt.Println("=== Lux External Sequencer Finality Adapter ===")
	fmt.Println()
	fmt.Println("This adapter adds post-quantum finality to ANY external sequencer.")
	fmt.Println("The external sequencer itself is NOT modified - this is an overlay.")
	fmt.Println()

	// Example 1: OP Stack (Optimism)
	fmt.Println("Supported Sequencers:")
	fmt.Println("  ✅ OP Stack (Optimism, Base, Mode, Zora, etc.)")
	fmt.Println("  ✅ Arbitrum (One, Nova)")
	fmt.Println("  ✅ zkSync Era")
	fmt.Println("  ✅ Scroll")
	fmt.Println("  ✅ Any EVM-compatible L2")
	fmt.Println()

	// Create client for Optimism
	opClient := NewOPStackClient("https://mainnet.optimism.io", 10)

	// Create finality adapter
	config := &FinalityConfig{
		K:               20,
		Alpha:           15,
		Beta:            20,
		FinalityTimeout: 30 * time.Second,
		SequencerRPC:    "https://mainnet.optimism.io",
		ChainID:         10,
	}

	adapter, err := NewFinalityAdapter(opClient, config)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println("✅ Finality adapter created for", opClient.GetSequencerType())
	fmt.Println()

	// Example block processing
	exampleBlock := &ExternalBlock{
		BlockHash:     sha256.Sum256([]byte("example-block")),
		ParentHash:    sha256.Sum256([]byte("parent-block")),
		Height:        100000,
		Timestamp:     uint64(time.Now().Unix()),
		StateRoot:     sha256.Sum256([]byte("state-root")),
		TxRoot:        sha256.Sum256([]byte("tx-root")),
		TxCount:       150,
		SequencerType: SequencerOPStack,
		ChainID:       10,
	}

	ctx := context.Background()

	if err := adapter.ProcessBlock(ctx, exampleBlock); err != nil {
		log.Printf("Error: %v", err)
	}

	// Vote on block
	vote, err := adapter.VoteOnBlock(exampleBlock, true)
	if err != nil {
		log.Printf("Vote error: %v", err)
	} else {
		fmt.Printf("Created PQ vote:\n")
		fmt.Printf("  BLS signature: %d bytes\n", len(vote.BLSSig))
		fmt.Printf("  ML-DSA signature: %d bytes\n", len(vote.MLDSASig))
	}

	fmt.Println()
	fmt.Println("Flow:")
	fmt.Println("  1. External sequencer produces block")
	fmt.Println("  2. Lux validators attest with BLS + ML-DSA signatures")
	fmt.Println("  3. Consensus aggregates votes → PQ certificate")
	fmt.Println("  4. Finalized block + certificate submitted to L1")
	fmt.Println()
	fmt.Println("🔐 Post-quantum finality overlay complete!")
}
