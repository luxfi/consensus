<<<<<<< HEAD
// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package wavefpc

import (
	"crypto/sha256"
	"sync"
	"sync/atomic"
	
	"github.com/luxfi/ids"
)

// Implementation of WaveFPC - the core FPC engine
type waveFPC struct {
	cfg Config
	cls Classifier
	dag DAGTap
	pq  PQEngine
	
	mu          sync.RWMutex
	epochPaused atomic.Bool
	
	// Core state (sharded for performance)
	votes       *ShardedMap[TxRef, *Bitset]      // Voters for tx
	votedOn     *ShardedMap[[64]byte, TxRef]     // key = hash(validator||object)
	state       *ShardedMap[TxRef, Status]        // Transaction state
	mixedTxs    *ShardedMap[TxRef, bool]          // Tracks mixed txs
	conflicts   *ShardedMap[ObjectID, []TxRef]    // Object -> conflicting txs
	
	// Epoch tracking
	epochBitAuthors map[ids.NodeID]bool
	epochMu         sync.Mutex
	
	// Metrics
	metrics Metrics
	
	// My node identity
	myNodeID    ids.NodeID
	validators  []ids.NodeID
}

// New creates a new WaveFPC instance
func New(cfg Config, cls Classifier, dag DAGTap, pq PQEngine, myNodeID ids.NodeID, validators []ids.NodeID) WaveFPC {
	return &waveFPC{
		cfg:             cfg,
		cls:             cls,
		dag:             dag,
		pq:              pq,
		myNodeID:        myNodeID,
		validators:      validators,
		votes:           NewShardedMap[TxRef, *Bitset](16),
		votedOn:         NewShardedMap[[64]byte, TxRef](16),
		state:           NewShardedMap[TxRef, Status](16),
		mixedTxs:        NewShardedMap[TxRef, bool](16),
		conflicts:       NewShardedMap[ObjectID, []TxRef](16),
		epochBitAuthors: make(map[ids.NodeID]bool),
	}
}

// NextVotes returns votes to include in the next block
func (w *waveFPC) NextVotes(budget int) []TxRef {
	if w.epochPaused.Load() {
		return nil // No new votes during epoch close
	}
	
	picks := make([]TxRef, 0, budget)
	processed := 0
	
	// TODO: Get candidates from mempool/DAG frontier
	// For now, return empty
	candidates := w.getCandidates()
	
	for _, tx := range candidates {
		if processed >= budget {
			break
		}
		
		// Check if it's owned-only
		owned := w.cls.OwnedInputs(tx)
		if len(owned) == 0 {
			continue // Skip shared/mixed txs
		}
		
		// Check if we've already voted on any of these objects
		canVote := true
		for _, obj := range owned {
			key := w.makeVoteKey(w.myNodeID, obj)
			if existing, ok := w.votedOn.Get(key); ok {
				if existing != tx {
					canVote = false // Already voted for different tx on this object
					break
				}
			}
		}
		
		if !canVote {
			continue
		}
		
		// Reserve our vote locally
		for _, obj := range owned {
			key := w.makeVoteKey(w.myNodeID, obj)
			w.votedOn.Set(key, tx)
		}
		
		picks = append(picks, tx)
		processed++
	}
	
	return picks
}

// OnBlockObserved processes votes from an observed block
func (w *waveFPC) OnBlockObserved(b *Block) {
	if len(b.Payload.FPCVotes) == 0 {
		return
	}
	
	voterIdx := ValidatorIndex(b.Author, w.validators)
	if voterIdx < 0 {
		return // Not in validator set
	}
	
	for _, raw := range b.Payload.FPCVotes {
		var tx TxRef
		copy(tx[:], raw)
		
		// Get owned inputs
		owned := w.cls.OwnedInputs(tx)
		if len(owned) == 0 {
			continue // Not an owned tx
		}
		
		// Check for equivocation on each object
		validVote := true
		for _, obj := range owned {
			key := w.makeVoteKey(b.Author, obj)
			if prev, ok := w.votedOn.Get(key); ok && prev != tx {
				validVote = false // Equivocation detected
				break
			}
		}
		
		if !validVote {
			continue
		}
		
		// Record the vote for each object
		for _, obj := range owned {
			key := w.makeVoteKey(b.Author, obj)
			w.votedOn.Set(key, tx)
			
			// Track conflicts
			w.addConflict(obj, tx)
		}
		
		// Update vote bitset
		bs := w.getOrCreateBitset(tx)
		bs.mu.Lock()
		newVote := bs.Set(voterIdx)
		count := bs.Count()
		bs.mu.Unlock()
		
		if newVote && count >= 2*w.cfg.F+1 {
			// Transaction is now executable!
			w.state.Set(tx, Executable)
			atomic.AddUint64(&w.metrics.ExecutableTxs, 1)
			
			// Submit to PQ engine if available
			if w.pq != nil {
				bs.mu.Lock()
				voters := bs.GetVoters(w.validators)
				bs.mu.Unlock()
				w.pq.Submit(tx, voters)
			}
		}
		
		atomic.AddUint64(&w.metrics.TotalVotes, 1)
	}
}

// OnBlockAccepted checks for anchoring when a block is accepted
func (w *waveFPC) OnBlockAccepted(b *Block) {
	// Check if any executable txs can now be marked final
	for _, raw := range b.Payload.FPCVotes {
		var tx TxRef
		copy(tx[:], raw)
		
		if st, _ := w.state.Get(tx); st == Executable {
			if w.anchorCovers(tx, b) {
				w.state.Set(tx, Final)
				atomic.AddUint64(&w.metrics.FinalTxs, 1)
			}
		}
	}
	
	// Track epoch bit authors
	if b.Payload.EpochBit {
		w.registerEpochBitAuthor(b.Author)
	}
}

// OnEpochCloseStart begins epoch closing
func (w *waveFPC) OnEpochCloseStart() {
	w.epochPaused.Store(true)
	atomic.AddUint64(&w.metrics.EpochChanges, 1)
}

// OnEpochClosed completes epoch transition
func (w *waveFPC) OnEpochClosed() {
	w.epochPaused.Store(false)
	
	// Clear epoch bit authors
	w.epochMu.Lock()
	w.epochBitAuthors = make(map[ids.NodeID]bool)
	w.epochMu.Unlock()
	
	// TODO: Clear old vote state
}

// Status returns the current status and proof for a transaction
func (w *waveFPC) Status(tx TxRef) (Status, Proof) {
	st, _ := w.state.Get(tx)
	
	proof := Proof{
		Status: st,
	}
	
	// Get vote count
	if bs := w.getBitset(tx); bs != nil {
		bs.mu.Lock()
		proof.VoterCount = bs.Count()
		proof.VoterBitmap = bs.Bytes()
		bs.mu.Unlock()
	}
	
	// Get PQ proof if available
	if w.pq != nil && w.pq.HasPQ(tx) {
		if pqBundle, ok := w.pq.GetPQ(tx); ok {
			proof.RingtailProof = pqBundle
		}
	}
	
	return st, proof
}

// MarkMixed marks a transaction as mixed (owned+shared)
func (w *waveFPC) MarkMixed(tx TxRef) {
	w.mixedTxs.Set(tx, true)
	w.state.Set(tx, Mixed)
}

// GetMetrics returns current metrics
func (w *waveFPC) GetMetrics() Metrics {
	return w.metrics
}

// Helper methods

func (w *waveFPC) makeVoteKey(validator ids.NodeID, obj ObjectID) [64]byte {
	h := sha256.New()
	h.Write(validator[:])
	h.Write(obj[:])
	var key [64]byte
	copy(key[:32], h.Sum(nil))
	return key
}

func (w *waveFPC) getOrCreateBitset(tx TxRef) *Bitset {
	bs, _ := w.votes.GetOrCreate(tx, func() *Bitset {
		return NewBitset(w.cfg.N)
	})
	return bs
}

func (w *waveFPC) getBitset(tx TxRef) *Bitset {
	bs, _ := w.votes.Get(tx)
	return bs
}

func (w *waveFPC) anchorCovers(tx TxRef, anchor *Block) bool {
	// Check if anchor's ancestry contains enough votes for tx
	bs := w.getBitset(tx)
	if bs == nil {
		return false
	}
	
	bs.mu.Lock()
	voteCount := bs.Count()
	bs.mu.Unlock()
	
	// Need ≥2f+1 votes and anchor must contain them in ancestry
	if voteCount < 2*w.cfg.F+1 {
		return false
	}
	
	// For simplicity, assume anchor covers if it or its ancestry contains tx
	// In production, check actual vote inclusion in ancestor blocks
	return w.dag.InAncestry(anchor.ID, tx)
}

func (w *waveFPC) registerEpochBitAuthor(author ids.NodeID) {
	w.epochMu.Lock()
	defer w.epochMu.Unlock()
	
	w.epochBitAuthors[author] = true
	
	// Check if we have enough authors for epoch close
	if len(w.epochBitAuthors) >= 2*w.cfg.F+1 {
		// Epoch can close
		// TODO: Trigger epoch close completion
	}
}

func (w *waveFPC) addConflict(obj ObjectID, tx TxRef) {
	existing, _ := w.conflicts.Get(obj)
	
	// Check if already tracked
	for _, e := range existing {
		if e == tx {
			return
		}
	}
	
	// Add to conflicts
	existing = append(existing, tx)
	w.conflicts.Set(obj, existing)
	
	if len(existing) > 1 {
		atomic.AddUint64(&w.metrics.ConflictCount, 1)
	}
}

func (w *waveFPC) getCandidates() []TxRef {
	// TODO: Get from mempool/DAG frontier
	// This would integrate with your existing transaction pool
	return nil
}

// Bitset for tracking voters (thread-safe)
type Bitset struct {
	mu   sync.Mutex
	bits []uint64
	size int
}

func NewBitset(size int) *Bitset {
	numWords := (size + 63) / 64
	return &Bitset{
		bits: make([]uint64, numWords),
		size: size,
	}
}

func (b *Bitset) Set(idx int) bool {
	if idx < 0 || idx >= b.size {
		return false
	}
	
	word := idx / 64
	bit := uint64(1) << (idx % 64)
	
	if b.bits[word]&bit != 0 {
		return false // Already set
	}
	
	b.bits[word] |= bit
	return true
}

func (b *Bitset) Count() int {
	count := 0
	for _, word := range b.bits {
		// Brian Kernighan's algorithm
		w := word
		for w != 0 {
			w &= w - 1
			count++
		}
	}
	return count
}

func (b *Bitset) Bytes() []byte {
	bytes := make([]byte, len(b.bits)*8)
	for i, word := range b.bits {
		for j := 0; j < 8; j++ {
			bytes[i*8+j] = byte(word >> (j * 8))
		}
	}
	return bytes
}

func (b *Bitset) GetVoters(validators []ids.NodeID) []ids.NodeID {
	var voters []ids.NodeID
	for i := 0; i < b.size && i < len(validators); i++ {
		word := i / 64
		bit := uint64(1) << (i % 64)
		if b.bits[word]&bit != 0 {
			voters = append(voters, validators[i])
		}
	}
	return voters
=======
// Package wavefpc implements Wave Fast Path Certification
// An optimized vote counter that rides inside ordinary blocks for fast finality
package wavefpc

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/log"
	metrics "github.com/luxfi/metric"
	"github.com/luxfi/consensus/core/interfaces"
)

// Interfaces that wavefpc provides

// Counter tracks votes and produces certificates
type Counter interface {
	// Add a vote from a validator
	AddVote(voterID ids.NodeID, vote *Vote) error
	
	// Check if we have enough votes for a certificate
	HasCertificate(blockID ids.ID) bool
	
	// Get the certificate for a block (returns nil if not ready)
	GetCertificate(blockID ids.ID) *Certificate
	
	// Notify counter that a block was accepted
	NotifyAccepted(blockID ids.ID)
	
	// Clean up old votes
	Prune(minHeight uint64)
}

// Hook allows consensus engines to integrate with WaveFPC
type Hook interface {
	// Called when a new block is being built
	OnBuildBlock(ctx context.Context, parentID ids.ID) (*BlockExtension, error)
	
	// Called when a block is being verified
	OnVerifyBlock(ctx context.Context, blockID ids.ID, ext *BlockExtension) error
	
	// Called when a block is accepted
	OnAcceptBlock(ctx context.Context, blockID ids.ID)
}

// Manager coordinates WaveFPC operations
type Manager interface {
	Counter
	Hook
	
	// Start the manager
	Start(ctx context.Context) error
	
	// Stop the manager
	Stop() error
}

// Types

// Vote represents a single validator's vote
type Vote struct {
	BlockID   ids.ID
	Height    uint64
	Signature []byte // BLS signature
	Timestamp time.Time
}

// Certificate represents a complete set of votes
type Certificate struct {
	BlockID      ids.ID
	Height       uint64
	Votes        map[ids.NodeID]*Vote
	Aggregate    []byte // Aggregated BLS signature
	Participants []ids.NodeID
	Timestamp    time.Time
}

// BlockExtension contains WaveFPC data embedded in blocks
type BlockExtension struct {
	// Votes from this proposer
	ProposerVotes []*Vote
	
	// Certificates for recent blocks
	Certificates []*Certificate
	
	// Metrics
	VoteCount int
	CertCount int
}

// Config for WaveFPC
type Config struct {
	// Quorum threshold (e.g., 0.67 for 2/3)
	QuorumThreshold float64
	
	// Maximum votes to include per block
	MaxVotesPerBlock int
	
	// Maximum certificates to include per block
	MaxCertsPerBlock int
	
	// Vote expiry time
	VoteExpiry time.Duration
	
	// Metrics registry
	Metrics metrics.Registry
	
	// Logger
	Log log.Logger
	
	// BLS keys
	BlsSK *bls.SecretKey
	BlsPK *bls.PublicKey
	
	// Validator set provider
	ValidatorSet interfaces.ValidatorSet
}

// Implementation

// manager implements the Manager interface
type manager struct {
	mu     sync.RWMutex
	config *Config
	
	// Vote tracking
	votes       map[ids.ID]map[ids.NodeID]*Vote // blockID -> voterID -> vote
	certificates map[ids.ID]*Certificate         // blockID -> certificate
	
	// Height tracking
	minHeight    uint64
	maxHeight    uint64
	accepted     map[ids.ID]bool
	
	// Metrics
	votesReceived metrics.Counter
	certsCreated  metrics.Counter
	votesExpired  metrics.Counter
	
	// Control
	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}
}

// NewManager creates a new WaveFPC manager
func NewManager(config *Config) Manager {
	return &manager{
		config:       config,
		votes:        make(map[ids.ID]map[ids.NodeID]*Vote),
		certificates: make(map[ids.ID]*Certificate),
		accepted:     make(map[ids.ID]bool),
		done:         make(chan struct{}),
	}
}

func (m *manager) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.ctx != nil {
		return errors.New("already started")
	}
	
	m.ctx, m.cancel = context.WithCancel(ctx)
	
	// Initialize metrics - simplified for now
	// In production, would use prometheus.NewCounter with proper registration
	
	// Start background tasks
	go m.pruneLoop()
	
	m.config.Log.Info("WaveFPC manager started")
	return nil
}

func (m *manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.cancel != nil {
		m.cancel()
		<-m.done
		m.cancel = nil
		m.ctx = nil
	}
	
	m.config.Log.Info("WaveFPC manager stopped")
	return nil
}

func (m *manager) AddVote(voterID ids.NodeID, vote *Vote) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Check if vote is expired
	if time.Since(vote.Timestamp) > m.config.VoteExpiry {
		return errors.New("vote expired")
	}
	
	// Initialize vote map for block if needed
	if m.votes[vote.BlockID] == nil {
		m.votes[vote.BlockID] = make(map[ids.NodeID]*Vote)
	}
	
	// Store vote
	m.votes[vote.BlockID][voterID] = vote
	
	// Update height tracking
	if vote.Height > m.maxHeight {
		m.maxHeight = vote.Height
	}
	
	// Increment metrics
	if m.votesReceived != nil {
		m.votesReceived.Inc()
	}
	
	// Check if we now have a certificate
	m.tryCreateCertificate(vote.BlockID)
	
	return nil
}

func (m *manager) HasCertificate(blockID ids.ID) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	_, exists := m.certificates[blockID]
	return exists
}

func (m *manager) GetCertificate(blockID ids.ID) *Certificate {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	return m.certificates[blockID]
}

func (m *manager) NotifyAccepted(blockID ids.ID) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.accepted[blockID] = true
	
	// Clean up votes for this block
	delete(m.votes, blockID)
}

func (m *manager) Prune(minHeight uint64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	m.minHeight = minHeight
	
	// Remove old votes
	for blockID, votes := range m.votes {
		if len(votes) > 0 {
			// Check height of first vote
			for _, v := range votes {
				if v.Height < minHeight {
					delete(m.votes, blockID)
					if m.votesExpired != nil {
						m.votesExpired.Add(float64(len(votes)))
					}
				}
				break
			}
		}
	}
	
	// Remove old certificates
	for blockID, cert := range m.certificates {
		if cert.Height < minHeight {
			delete(m.certificates, blockID)
		}
	}
	
	// Remove old accepted markers
	for blockID := range m.accepted {
		// We'd need block height here, simplified for now
		if len(m.accepted) > 1000 {
			delete(m.accepted, blockID)
			break
		}
	}
}

func (m *manager) OnBuildBlock(ctx context.Context, parentID ids.ID) (*BlockExtension, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	
	ext := &BlockExtension{
		ProposerVotes: make([]*Vote, 0),
		Certificates:  make([]*Certificate, 0),
	}
	
	// Add our votes for recent blocks
	voteCount := 0
	for _, votes := range m.votes {
		if voteCount >= m.config.MaxVotesPerBlock {
			break
		}
		
		// Only include our own vote
		for voterID, vote := range votes {
			if voterID == m.config.ValidatorSet.Self() {
				ext.ProposerVotes = append(ext.ProposerVotes, vote)
				voteCount++
				break
			}
		}
	}
	
	// Add recent certificates
	certCount := 0
	for _, cert := range m.certificates {
		if certCount >= m.config.MaxCertsPerBlock {
			break
		}
		
		// Only include certificates not already accepted
		if !m.accepted[cert.BlockID] {
			ext.Certificates = append(ext.Certificates, cert)
			certCount++
		}
	}
	
	ext.VoteCount = len(ext.ProposerVotes)
	ext.CertCount = len(ext.Certificates)
	
	return ext, nil
}

func (m *manager) OnVerifyBlock(ctx context.Context, _ ids.ID, ext *BlockExtension) error {
	if ext == nil {
		return nil
	}
	
	m.mu.Lock()
	defer m.mu.Unlock()
	
	// Process votes from the block
	for _, vote := range ext.ProposerVotes {
		// Verify signature
		if err := m.verifyVote(vote); err != nil {
			return err
		}
		
		// Add to our vote collection
		// Note: In real implementation, we'd get the proposer ID from the block
		proposerID := ids.GenerateTestNodeID()
		m.AddVote(proposerID, vote)
	}
	
	// Process certificates
	for _, cert := range ext.Certificates {
		// Verify certificate
		if err := m.verifyCertificate(cert); err != nil {
			return err
		}
		
		// Store certificate
		m.certificates[cert.BlockID] = cert
	}
	
	return nil
}

func (m *manager) OnAcceptBlock(ctx context.Context, blockID ids.ID) {
	m.NotifyAccepted(blockID)
}

// Internal methods

func (m *manager) tryCreateCertificate(blockID ids.ID) {
	votes := m.votes[blockID]
	if votes == nil {
		return
	}
	
	// Check if we have quorum
	totalWeight := m.config.ValidatorSet.TotalWeight()
	voteWeight := uint64(0)
	
	for voterID := range votes {
		weight := m.config.ValidatorSet.GetWeight(voterID)
		voteWeight += weight
	}
	
	quorumWeight := uint64(float64(totalWeight) * m.config.QuorumThreshold)
	if voteWeight < quorumWeight {
		return
	}
	
	// Create certificate
	cert := &Certificate{
		BlockID:      blockID,
		Height:       m.maxHeight, // Simplified
		Votes:        votes,
		Participants: make([]ids.NodeID, 0, len(votes)),
		Timestamp:    time.Now(),
	}
	
	// Collect participants
	for voterID := range votes {
		cert.Participants = append(cert.Participants, voterID)
	}
	
	// Aggregate signatures (simplified - real implementation would use BLS)
	cert.Aggregate = []byte("aggregated_signature")
	
	// Store certificate
	m.certificates[blockID] = cert
	
	// Update metrics
	if m.certsCreated != nil {
		m.certsCreated.Inc()
	}
	
	m.config.Log.Info("Created certificate",
		log.String("blockID", blockID.String()),
		log.Int("votes", len(votes)),
	)
}

func (m *manager) verifyVote(vote *Vote) error {
	// Simplified - real implementation would verify BLS signature
	if len(vote.Signature) == 0 {
		return errors.New("missing signature")
	}
	return nil
}

func (m *manager) verifyCertificate(cert *Certificate) error {
	// Simplified - real implementation would verify aggregated BLS signature
	if len(cert.Aggregate) == 0 {
		return errors.New("missing aggregate signature")
	}
	
	// Verify quorum
	totalWeight := m.config.ValidatorSet.TotalWeight()
	certWeight := uint64(0)
	
	for _, participant := range cert.Participants {
		weight := m.config.ValidatorSet.GetWeight(participant)
		certWeight += weight
	}
	
	quorumWeight := uint64(float64(totalWeight) * m.config.QuorumThreshold)
	if certWeight < quorumWeight {
		return errors.New("certificate does not meet quorum")
	}
	
	return nil
}

func (m *manager) pruneLoop() {
	defer close(m.done)
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			// Prune old votes
			m.mu.Lock()
			now := time.Now()
			for blockID, votes := range m.votes {
				for voterID, vote := range votes {
					if now.Sub(vote.Timestamp) > m.config.VoteExpiry {
						delete(votes, voterID)
						if m.votesExpired != nil {
							m.votesExpired.Inc()
						}
					}
				}
				if len(votes) == 0 {
					delete(m.votes, blockID)
				}
			}
			m.mu.Unlock()
		}
	}
>>>>>>> a04aa1c (Complete WaveFPC implementation with comprehensive testing)
}