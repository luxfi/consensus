// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"context"
	"errors"
	"sync"
	"time"
	
	"github.com/luxfi/ids"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/bls/signer/localsigner"
	"github.com/luxfi/crypto/ringtail"
)

var (
	ErrMissingRTCert = errors.New("missing Ringtail certificate")
	ErrQuasarTimeout = errors.New("quasar timeout - proposer will be slashed")
)

// Engine implements the Beam consensus engine with Quasar
type Engine struct {
	mu sync.RWMutex
	
	// Node info
	nodeID    ids.NodeID
	blsSigner bls.Signer
	rtSK      []byte
	
	// Consensus state
	height    uint64
	lastBlock *Block
	
	// Quasar integration
	quasar *quasar
	
	// Channels
	blockCh   chan *Block
	rtCertCh  map[uint64]chan []byte // height -> cert channel
	slashCh   chan SlashEvent
	
	// Configuration
	cfg Config
}

// SlashEvent represents a slashing event
type SlashEvent struct {
	Height     uint64
	ProposerID ids.NodeID
	Reason     string
}

// NewEngine creates a new Beam engine with Quasar
func NewEngine(nodeID ids.NodeID, blsSK, rtSK []byte, cfg Config) (*Engine, error) {
	q, err := newQuasar(rtSK, cfg)
	if err != nil {
		return nil, err
	}
	
	// Create BLS signer from secret key bytes
	blsSigner, err := localsigner.FromBytes(blsSK)
	if err != nil {
		return nil, err
	}
	
	return &Engine{
		nodeID:    nodeID,
		blsSigner: blsSigner,
		rtSK:      rtSK,
		quasar:    q,
		blockCh:   make(chan *Block, 100),
		rtCertCh:  make(map[uint64]chan []byte),
		slashCh:   make(chan SlashEvent, 10),
		cfg:       cfg,
	}, nil
}

// Propose creates a new block proposal with dual certificates
func (e *Engine) Propose(ctx context.Context, height uint64, parentID ids.ID, txs [][]byte) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	
	// Build block header
	header := Header{
		ChainID:    e.cfg.ChainID,
		Height:     height,
		ParentID:   parentID,
		Timestamp:  time.Now(),
		ProposerID: e.nodeID,
		// TODO: compute state roots
	}
	
	// Create block
	block := &Block{
		Header:       header,
		Transactions: txs,
	}
	
	// Get block hash for signing
	blockHash := block.Header.Hash()
	
	// Sign with BLS
	blsSig, err := e.blsSigner.Sign(blockHash[:])
	if err != nil {
		return err
	}
	// Serialize the signature to bytes
	sigBytes := blsSig.Serialize()
	copy(block.Certs.BLSAgg[:], sigBytes)
	
	// Sign with Ringtail
	share, err := e.quasar.sign(height, blockHash[:])
	if err != nil {
		return err
	}
	
	// Create certificate channel for this height
	certCh := make(chan []byte, 1)
	e.rtCertCh[height] = certCh
	
	// Broadcast Ringtail share
	// In production: gossip "RTSH|height|shareBytes"
	go e.broadcastRTShare(height, share)
	
	// Wait for Ringtail certificate with timeout
	timer := time.NewTimer(e.cfg.QuasarTimeout)
	defer timer.Stop()
	
	select {
	case cert := <-certCh:
		// Got certificate in time
		block.Certs.RTCert = cert
		
		// Verify our own block
		// TODO: Get validators public keys from consensus state
		// For now, skip verification of our own block since we just created it
		// if err := VerifyBlock(block, validators, e.quasar); err != nil {
		// 	return err
		// }
		
		// Broadcast complete block
		e.blockCh <- block
		return nil
		
	case <-timer.C:
		// Timeout - couldn't gather RT threshold fast enough
		// This block is invalid and proposer will be slashed
		e.slashCh <- SlashEvent{
			Height:     height,
			ProposerID: e.nodeID,
			Reason:     "Quasar timeout - failed to collect Ringtail certificate",
		}
		return ErrQuasarTimeout
		
	case <-ctx.Done():
		return ctx.Err()
	}
}

// OnRTShare processes incoming Ringtail shares
func (e *Engine) OnRTShare(height uint64, share []byte) {
	ready, cert := e.quasar.onShare(height, share)
	if ready {
		e.mu.RLock()
		ch, exists := e.rtCertCh[height]
		e.mu.RUnlock()
		
		if exists {
			select {
			case ch <- cert:
			default:
				// Channel full, certificate already delivered
			}
		}
	}
}

// VerifyAndAccept verifies and accepts a block
func (e *Engine) VerifyAndAccept(block *Block) error {
	// Verify dual certificates
	if err := VerifyBlock(block, e.quasar); err != nil {
		// If BLS passes but Ringtail fails, this could be a quantum attack
		if err == ErrRingtail {
			// Slash the proposer for invalid Ringtail cert
			e.slashCh <- SlashEvent{
				Height:     block.Height,
				ProposerID: block.ProposerID,
				Reason:     "Invalid Ringtail certificate - possible quantum attack",
			}
		}
		return err
	}
	
	// Update state
	e.mu.Lock()
	e.height = block.Height
	e.lastBlock = block
	e.mu.Unlock()
	
	return nil
}

// GetSlashEvents returns the slash event channel
func (e *Engine) GetSlashEvents() <-chan SlashEvent {
	return e.slashCh
}

// broadcastRTShare broadcasts a Ringtail share
func (e *Engine) broadcastRTShare(height uint64, share ringtail.Share) {
	// In production: implement P2P gossip
	// For testing: self-deliver
	e.OnRTShare(height, share)
}

// MinRoundInterval returns the minimum round interval
func (e *Engine) MinRoundInterval() time.Duration {
	return 50 * time.Millisecond
}

// Height returns current height
func (e *Engine) Height() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.height
}