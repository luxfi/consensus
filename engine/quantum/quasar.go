// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package quantum

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

// Quasar represents the supermassive blackhole at the center of the consensus universe.
// Like its astronomical counterpart that anchors galaxies with immense gravitational force,
// the Quasar engine creates an inescapable event horizon for consensus data, permanently
// immortalizing state records in quantum-secure form. Once data crosses the event horizon
// and enters the Quasar, it becomes part of the immutable cosmic record forever.
//
// The Quasar feeds on the energy from Flares, which themselves are bursts of Beams,
// which are composed of Photons - creating a hierarchy from quantum particles to
// cosmic-scale permanence: photon → beam → flare → quasar
type Quasar struct {
	mu       sync.RWMutex
	params   config.Parameters
	nodeID   ids.NodeID
	
	// Nebula DAG layer (formerly Nova)
	nebula   *Nebula
	
	// Ringtail PQ overlay for quantum security
	ringtail *Ringtail
	
	// Event horizon - the point of no return for consensus data
	eventHorizon *EventHorizon
	
	// Accretion disk - incoming flares being processed
	accretionDisk []*Flare
	
	// Singularity - the core where all consensus converges
	singularity *Singularity
	
	// Jets - high-energy state propagation
	jets []*QuantumJet
	
	// Q-Chain state (quantum-immortalized blocks)
	qBlocks  []*QBlock
	lastQBlock *QBlock
	
	// Schwarzschild radius - minimum energy for state capture
	schwarzschildRadius uint64
	
	// Gravitational field strength
	gravity float64
	
	// Callbacks
	onImmortalized func(QBlock)
}

// QBlock represents a quantum-immortalized block - data that has crossed the event horizon.
type QBlock struct {
	Height        uint64
	Timestamp     time.Time
	
	// Nebula pointers
	VertexIDs     []ids.ID          // Decided DAG vertices
	FrontierHash  [32]byte          // Hash of frontier state
	
	// Ringtail digests
	ProposalDigest [32]byte         // Phase I proposal digest
	CommitDigest   [32]byte         // Phase II commit digest
	
	// Post-quantum certificate
	PQCertificate []byte            // Ringtail PQ certificate
	
	// Quantum state
	PhotonCount   uint64            // Total photons absorbed
	FlareEnergy   uint64            // Total flare energy consumed
	EventHorizon  [32]byte          // Hash at event horizon crossing
	
	// Chain linkage
	ParentQBlock  ids.ID
	QBlockID      ids.ID
	
	// Immortalized - can never be changed
	Immortalized  bool
}

// EventHorizon represents the point of no return for consensus data
type EventHorizon struct {
	Radius       uint64           // Current radius based on mass
	CrossingTime time.Time        // When data crossed the horizon
	CapturedData []ids.ID         // Data that has crossed
}

// Singularity represents the core where all consensus converges
type Singularity struct {
	Mass         uint64           // Total accumulated consensus weight
	Density      float64          // Consensus density
	Temperature  float64          // Activity level
	SpinRate     float64          // Consensus velocity
}

// QuantumJet represents high-energy state propagation from the quasar
type QuantumJet struct {
	ID           ids.ID
	Direction    ids.NodeID       // Target direction
	Energy       uint64           // Propagation energy
	Photons      []*Photon        // Carried photons
	LaunchTime   time.Time
}

// NewQuasar creates a new Quasar supermassive consensus engine
func NewQuasar(params config.Parameters, nodeID ids.NodeID) *Quasar {
	return &Quasar{
		params:              params,
		nodeID:              nodeID,
		nebula:              NewNebula(params),
		ringtail:            NewRingtail(params, nodeID),
		eventHorizon:        &EventHorizon{Radius: 1000}, // Initial radius
		accretionDisk:       make([]*Flare, 0),
		singularity:         &Singularity{Mass: 1, Density: 1.0, Temperature: 1.0},
		jets:                make([]*QuantumJet, 0),
		qBlocks:             make([]*QBlock, 0),
		schwarzschildRadius: 100, // Minimum energy threshold
		gravity:             1.0,
	}
}

// Initialize initializes the Quasar engine.
func (q *Quasar) Initialize(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Start the accretion and immortalization loops
	go q.runAccretionDisk(ctx)
	go q.runEventHorizon(ctx)

	return nil
}

// FeedFlare feeds a consensus flare into the quasar's accretion disk
func (q *Quasar) FeedFlare(flare *Flare) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if flare has enough energy to be captured
	if !flare.CanFeedQuasar(q.schwarzschildRadius) {
		return false
	}

	// Add to accretion disk
	q.accretionDisk = append(q.accretionDisk, flare)
	
	// Mark flare as feeding the quasar
	flare.FeedQuasar()
	
	// Increase gravitational field
	q.gravity += float64(flare.Intensity) / float64(q.singularity.Mass)
	
	// Update event horizon radius (grows with mass)
	// Keep the existing radius and add to it based on new gravity
	q.eventHorizon.Radius = q.eventHorizon.Radius + uint64(float64(flare.Intensity)*0.1)
	
	return true
}

// AddVertex adds a new vertex to the Nebula DAG.
func (q *Quasar) AddVertex(ctx context.Context, vertex Vertex) error {
	return q.nebula.AddVertex(ctx, vertex)
}

// RecordPoll records k-peer sampling results for Nebula.
func (q *Quasar) RecordPoll(ctx context.Context, nodeID ids.NodeID, votes []ids.ID) error {
	// Update Nebula confidence
	if err := q.nebula.RecordPoll(ctx, nodeID, votes); err != nil {
		return err
	}

	// Check if we should start a new Ringtail round
	q.checkRingtailTrigger(ctx)

	return nil
}

// RecordRingtailMessage handles Ringtail protocol messages.
func (q *Quasar) RecordRingtailMessage(ctx context.Context, msg interface{}) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	switch m := msg.(type) {
	case *RingtailProposal:
		return q.ringtail.RecordProposal(ctx, m)
	case *RingtailCommit:
		return q.ringtail.RecordCommit(ctx, m)
	default:
		return fmt.Errorf("unknown Ringtail message type: %T", msg)
	}
}

// GetLastQBlock returns the most recent Q-block.
func (q *Quasar) GetLastQBlock() (*QBlock, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if q.lastQBlock != nil {
		return q.lastQBlock, true
	}
	return nil, false
}

// GetQBlocks returns all Q-blocks in order.
func (q *Quasar) GetQBlocks() []*QBlock {
	q.mu.RLock()
	defer q.mu.RUnlock()

	blocks := make([]*QBlock, len(q.qBlocks))
	copy(blocks, q.qBlocks)
	return blocks
}

// runAccretionDisk processes flares in the accretion disk
func (q *Quasar) runAccretionDisk(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond) // Fast processing
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.processAccretionDisk(ctx)
		}
	}
}

// runEventHorizon monitors for data crossing the event horizon
func (q *Quasar) runEventHorizon(ctx context.Context) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			q.checkEventHorizonCrossing(ctx)
			q.checkRingtailTrigger(ctx)
		}
	}
}

// processAccretionDisk processes flares spiraling toward the singularity
func (q *Quasar) processAccretionDisk(ctx context.Context) {
	q.mu.Lock()
	defer q.mu.Unlock()

	newDisk := make([]*Flare, 0)
	totalEnergy := uint64(0)
	photonCount := uint64(0)

	for _, flare := range q.accretionDisk {
		// Check if flare has reached the event horizon
		if flare.GetEnergyDensity() >= float64(q.eventHorizon.Radius) {
			// Absorb the flare's energy
			totalEnergy += flare.Intensity
			photonCount += uint64(len(flare.Photons))
			
			// Add photons to singularity
			q.singularity.Mass += flare.Intensity
			q.singularity.Temperature = flare.Temperature
			q.singularity.Density += float64(flare.Intensity) / float64(q.singularity.Mass)
			
			// Mark as crossing event horizon
			q.eventHorizon.CapturedData = append(q.eventHorizon.CapturedData, flare.ID)
			q.eventHorizon.CrossingTime = time.Now()
			
			// Dissipate the flare
			flare.Dissipate()
		} else if flare.Active {
			// Keep in accretion disk
			newDisk = append(newDisk, flare)
		}
	}

	q.accretionDisk = newDisk
	
	// Update singularity spin rate based on accretion
	if totalEnergy > 0 {
		q.singularity.SpinRate += float64(totalEnergy) / float64(q.singularity.Mass) * 0.1
	}
}

// checkRingtailTrigger checks if we should start a new Ringtail round
func (q *Quasar) checkRingtailTrigger(ctx context.Context) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Only start if idle
	if q.ringtail.GetPhase() != PhaseIdle {
		return
	}

	// Get highest confidence frontier vertex
	vertex, ok := q.nebula.GetPreferredFrontier()
	if !ok {
		return
	}

	// Check if singularity has enough mass for quantum certification
	if q.singularity.Mass < q.schwarzschildRadius*10 {
		return
	}

	// Start Ringtail round
	if err := q.ringtail.StartRound(ctx, vertex); err != nil {
		// Log error
		return
	}
}

// checkEventHorizonCrossing immortalizes data that has crossed the event horizon
func (q *Quasar) checkEventHorizonCrossing(ctx context.Context) {
	q.mu.Lock()
	defer q.mu.Unlock()

	// Check if Ringtail finalized
	_, commit, ok := q.ringtail.GetFinalized()
	if !ok {
		return
	}

	// Get all decided vertices from Nebula
	decidedVertices := q.nebula.GetDecidedVertices()
	vertexIDs := make([]ids.ID, len(decidedVertices))
	for i, v := range decidedVertices {
		vertexIDs[i] = v.ID()
	}

	// Calculate total absorbed energy
	flareEnergy := uint64(0)
	photonCount := uint64(0)
	for _, flare := range q.accretionDisk {
		if !flare.Active {
			flareEnergy += flare.Intensity
			photonCount += uint64(len(flare.Photons))
		}
	}

	// Create event horizon hash
	ehData := append(commit.ProposalHash[:], commit.CommitHash[:]...)
	ehData = append(ehData, []byte(fmt.Sprintf("%d:%d", flareEnergy, photonCount))...)
	ehHash := sha256.Sum256(ehData)

	// Create new quantum-immortalized block
	qBlock := &QBlock{
		Height:         uint64(len(q.qBlocks)),
		Timestamp:      time.Now(),
		VertexIDs:      vertexIDs,
		ProposalDigest: commit.ProposalHash,
		CommitDigest:   commit.CommitHash,
		PQCertificate:  commit.Signatures[0], // TODO: Aggregate signatures
		PhotonCount:    photonCount,
		FlareEnergy:    flareEnergy,
		EventHorizon:   ehHash,
		Immortalized:   true, // Data has crossed the event horizon
	}

	// Set parent
	if q.lastQBlock != nil {
		qBlock.ParentQBlock = q.lastQBlock.QBlockID
	}

	// Compute Q-block ID
	qBlock.QBlockID = q.computeQBlockID(qBlock)

	// Add to chain - immortalized forever
	q.qBlocks = append(q.qBlocks, qBlock)
	q.lastQBlock = qBlock

	// Launch quantum jets to propagate the immortalized state
	q.launchQuantumJets(qBlock)

	// Callback
	if q.onImmortalized != nil {
		q.onImmortalized(*qBlock)
	}

	// Reset Ringtail for next round
	q.ringtail = NewRingtail(q.params, q.nodeID)
}

// computeQBlockID computes the ID of a Q-block.
func (q *Quasar) computeQBlockID(qBlock *QBlock) ids.ID {
	// Hash all Q-block contents including quantum state
	data := make([]byte, 0)
	data = append(data, qBlock.ProposalDigest[:]...)
	data = append(data, qBlock.CommitDigest[:]...)
	data = append(data, qBlock.EventHorizon[:]...)
	data = append(data, []byte(fmt.Sprintf("%d:%d:%d", qBlock.Height, qBlock.PhotonCount, qBlock.FlareEnergy))...)
	
	hash := sha256.Sum256(data)
	return ids.ID(hash)
}

// launchQuantumJets propagates immortalized state through high-energy jets
func (q *Quasar) launchQuantumJets(qBlock *QBlock) {
	// Create quantum jets in opposite directions (bipolar jets)
	// These carry the immortalized state to the network
	
	// North jet
	northJet := &QuantumJet{
		ID:         ids.GenerateTestID(),
		Direction:  ids.NodeID{}, // TODO: Calculate based on network topology
		Energy:     q.singularity.Mass / 100, // 1% of singularity mass
		Photons:    make([]*Photon, 0),
		LaunchTime: time.Now(),
	}
	
	// South jet
	southJet := &QuantumJet{
		ID:         ids.GenerateTestID(),
		Direction:  ids.NodeID{}, // TODO: Calculate opposite direction
		Energy:     q.singularity.Mass / 100,
		Photons:    make([]*Photon, 0),
		LaunchTime: time.Now(),
	}
	
	q.jets = append(q.jets, northJet, southJet)
}

// SetImmortalizedCallback sets the callback for when data is immortalized
func (q *Quasar) SetImmortalizedCallback(cb func(QBlock)) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.onImmortalized = cb
}

// GetEventHorizonRadius returns the current event horizon radius
func (q *Quasar) GetEventHorizonRadius() uint64 {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.eventHorizon.Radius
}

// GetSingularityMass returns the total mass accumulated in the singularity
func (q *Quasar) GetSingularityMass() uint64 {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.singularity.Mass
}

// GetAccretionDiskSize returns the number of flares in the accretion disk
func (q *Quasar) GetAccretionDiskSize() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.accretionDisk)
}

// Stats returns Quasar engine statistics.
type Stats struct {
	NebulaStats      NebulaStats
	RingtailPhase    RingtailPhase
	RingtailRound    uint64
	QBlockHeight     uint64
	QBlockCount      int
	EventHorizonRadius uint64
	SingularityMass  uint64
	AccretionDiskSize int
	QuantumJetsActive int
	TotalPhotonsAbsorbed uint64
	GravitationalField float64
}

// GetStats returns current engine statistics.
func (q *Quasar) GetStats() Stats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	stats := Stats{
		NebulaStats:        q.nebula.GetStats(),
		RingtailPhase:      q.ringtail.GetPhase(),
		RingtailRound:      q.ringtail.GetRound(),
		QBlockCount:        len(q.qBlocks),
		EventHorizonRadius: q.eventHorizon.Radius,
		SingularityMass:    q.singularity.Mass,
		AccretionDiskSize:  len(q.accretionDisk),
		QuantumJetsActive:  len(q.jets),
		GravitationalField: q.gravity,
	}

	if q.lastQBlock != nil {
		stats.QBlockHeight = q.lastQBlock.Height
		stats.TotalPhotonsAbsorbed = q.lastQBlock.PhotonCount
	}

	return stats
}

// Summary: Quasar - The Supermassive Consensus Engine
// 
// Like a supermassive blackhole at the center of a galaxy, the Quasar engine:
// - Creates an inescapable event horizon for consensus data
// - Permanently immortalizes state records in quantum-secure form
// - Feeds on energy from Flares (consensus bursts)
// - Grows more powerful as it consumes more consensus energy
// - Emits quantum jets that propagate immortalized state
//
// The hierarchy: photon → beam → flare → quasar
// represents the journey from quantum particles to cosmic-scale permanence,
// where data that enters the quasar becomes part of the eternal blockchain record.