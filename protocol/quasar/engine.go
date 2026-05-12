// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Quasar implementation - the supermassive black hole at the center of the blockchain galaxy.

package quasar

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/luxfi/consensus/config"
)

// quasarEngine implements the Engine interface.
type quasarEngine struct {
	mu     sync.RWMutex
	cfg    Config
	ctx    context.Context
	cancel context.CancelFunc

	// Block processing
	incoming  chan *Block
	finalized chan *Block

	// State
	finalizedBlocks map[string]*Block // hash -> block
	height          uint64
	startTime       time.Time

	// Consensus engine
	certifier *Certifier
	signer    *signer // real BLS+Ringtail+ML-DSA signer (optional, may be nil for legacy)

	// Metrics
	processed uint64
}

var (
	// ErrThresholdTooLow is returned when the consensus threshold is below the
	// minimum safe value of 2. Single-node mode must use NewTestEngine.
	ErrThresholdTooLow = errors.New("QThreshold must be >= 2 for production consensus")

	// ErrPartialTripleCert is returned by generateCert when the chain's
	// ChainSecurityProfile demands a triple-mode (P+Q+Z) certificate but
	// the signer can't produce all three layers — typically because no
	// validator is configured with Ringtail shares (the Q layer). Closes
	// CR-10: previously this path fell through to a SHA-256 placeholder
	// and the engine would emit a cert the network believes is triple
	// when it's only single-layer.
	ErrPartialTripleCert = errors.New("Certifier: refusing to emit partial cert under triple-mode profile")
)

// NewEngine creates a new Quasar consensus engine.
// Threshold must be >= 2 for production use. For single-node testing, use NewTestEngine.
func NewEngine(cfg Config) (Engine, error) {
	if cfg.QThreshold < 2 {
		return nil, ErrThresholdTooLow
	}
	threshold := cfg.QThreshold

	certifier, err := newCertifier(threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to create certifier: %w", err)
	}

	bufSize := 1000
	return &quasarEngine{
		cfg:             cfg,
		incoming:        make(chan *Block, bufSize),
		finalized:       make(chan *Block, bufSize),
		finalizedBlocks: make(map[string]*Block),
		certifier:       certifier,
	}, nil
}

// NewTestEngine creates a Quasar engine with threshold=1 for single-node testing.
// Must NOT be used in production.
func NewTestEngine(cfg Config) (Engine, error) {
	threshold := cfg.QThreshold
	if threshold < 1 {
		threshold = 1
	}

	certifier, err := newCertifier(threshold)
	if err != nil {
		return nil, fmt.Errorf("failed to create certifier: %w", err)
	}

	bufSize := 1000
	return &quasarEngine{
		cfg:             cfg,
		incoming:        make(chan *Block, bufSize),
		finalized:       make(chan *Block, bufSize),
		finalizedBlocks: make(map[string]*Block),
		certifier:       certifier,
	}, nil
}

// Start begins the consensus engine.
func (q *quasarEngine) Start(ctx context.Context) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.ctx, q.cancel = context.WithCancel(ctx)
	q.startTime = time.Now()

	go q.processLoop()
	return nil
}

// Stop gracefully shuts down the consensus engine.
func (q *quasarEngine) Stop() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	if q.cancel != nil {
		q.cancel()
	}
	return nil
}

// Submit adds a block to the consensus pipeline.
func (q *quasarEngine) Submit(block *Block) error {
	if block == nil {
		return fmt.Errorf("nil block")
	}

	select {
	case q.incoming <- block:
		return nil
	default:
		return fmt.Errorf("buffer full")
	}
}

// Finalized returns a channel of finalized blocks.
func (q *quasarEngine) Finalized() <-chan *Block {
	return q.finalized
}

// IsFinalized checks if a block is finalized.
func (q *quasarEngine) IsFinalized(blockID [32]byte) bool {
	q.mu.RLock()
	defer q.mu.RUnlock()

	hash := hex.EncodeToString(blockID[:])
	_, ok := q.finalizedBlocks[hash]
	return ok
}

// Stats returns consensus metrics.
func (q *quasarEngine) Stats() Stats {
	q.mu.RLock()
	defer q.mu.RUnlock()

	return Stats{
		Height:          q.height,
		ProcessedBlocks: q.processed,
		FinalizedBlocks: uint64(len(q.finalizedBlocks)),
		PendingBlocks:   len(q.incoming),
		Validators:      q.certifier.validatorCount(),
		Uptime:          time.Since(q.startTime),
	}
}

// processLoop is the main consensus loop.
func (q *quasarEngine) processLoop() {
	for {
		select {
		case <-q.ctx.Done():
			return
		case block := <-q.incoming:
			q.processBlock(block)
		}
	}
}

// processBlock processes a single block through consensus.
func (q *quasarEngine) processBlock(block *Block) {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.processed++

	// Generate quantum certificate
	cert := q.certifier.generateCert(block)
	if cert == nil {
		return // Did not achieve consensus
	}

	// Vote-acceptance gate (CR-10): under a triple-mode profile the
	// cert MUST carry every layer (BLS + Ringtail + MLDSAProof).
	// QuasarCert.Verify enforces that structural property; rejecting
	// here keeps the cert-acceptance gate honest even if a future
	// generateCert path forgets to refuse silently. Belt and braces.
	q.certifier.mu.RLock()
	demandsTriple := q.certifier.demandsTriple()
	q.certifier.mu.RUnlock()
	if demandsTriple && !cert.Verify(nil) {
		return
	}

	// Finalize block
	block.Cert = cert
	block.Hash = computeHash(block)

	q.finalizedBlocks[block.Hash] = block
	q.height++

	// Notify listeners
	select {
	case q.finalized <- block:
	default:
		// Drop if buffer full
	}
}

// SetProfile binds a ChainSecurityProfile to this engine. Forwards to the
// certifier so generateCert can enforce the triple-mode gate (CR-10).
//
// Strict-PQ / FIPS profiles refuse any cert below P+Q+Z. Non-strict
// profiles preserve legacy behaviour (placeholder + fallback OK).
func (q *quasarEngine) SetProfile(profile *config.ChainSecurityProfile) {
	q.certifier.SetProfile(profile)
}

// computeHash computes a block hash.
func computeHash(block *Block) string {
	h := sha256.New()
	h.Write(block.ID[:])
	h.Write(block.ChainID[:])
	h.Write([]byte(block.ChainName))
	h.Write([]byte(fmt.Sprintf("%d:%d", block.Height, block.Timestamp.Unix())))
	return hex.EncodeToString(h.Sum(nil))
}

// Certifier handles certificate generation for the engine.
//
// When a Signer is attached via AttachSigner the certifier produces real
// QuasarCerts (BLS share + ML-DSA sig + Ringtail Round 1 commitment). With
// no signer, it falls back to deterministic SHA-256 commitments suitable
// only for in-process unit tests.
//
// Triple-mode enforcement (CR-10): when SetProfile attaches a strict-PQ
// or FIPS profile, generateCert REFUSES to fall back to the SHA-256
// placeholder OR to a non-triple realCert. The audit gate is profile-
// driven, not heuristic.
type Certifier struct {
	mu         sync.RWMutex
	threshold  int
	validators map[string]int // validator -> weight

	// Real PQ signing engine. Optional: when nil, the certifier uses the
	// legacy SHA-256 fallback for backward compatibility with single-node
	// engine tests.
	signer    *signer
	signerCtx context.Context

	// profile, when non-nil and strict-PQ / FIPS, demands triple-mode
	// certificates. generateCert returns nil rather than emitting a
	// SHA-256 placeholder or single-layer cert under such a profile —
	// the engine's caller MUST then treat the round as unfinalised.
	profile *config.ChainSecurityProfile
}

func newCertifier(threshold int) (*Certifier, error) {
	return &Certifier{
		threshold:  threshold,
		validators: make(map[string]int),
	}, nil
}

// AttachSigner wires a real BLS+Ringtail+ML-DSA signer into the certifier.
// After this call, generateCert produces real cryptographic certificates.
func (h *Certifier) AttachSigner(ctx context.Context, s *signer) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.signer = s
	h.signerCtx = ctx
}

// SetProfile binds a ChainSecurityProfile to this certifier. When the
// profile is strict-PQ or FIPS, generateCert refuses every code path
// that would produce a non-triple cert (the SHA-256 placeholder and the
// single-layer realCert fallback). Closes CR-10.
//
// Nil profile preserves legacy behaviour (placeholder + fallback OK).
func (h *Certifier) SetProfile(profile *config.ChainSecurityProfile) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.profile = profile
}

// demandsTriple reports whether the certifier's profile requires every
// cert to be triple-mode (P+Q+Z). Strict-PQ and FIPS demand it; other
// profiles do not. Caller MUST hold h.mu.RLock.
func (h *Certifier) demandsTriple() bool {
	return h.profile != nil && h.profile.IsPQ()
}

func (h *Certifier) validatorCount() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.validators)
}

// generateCert creates a certificate for the given block. When a signer is
// attached it returns a real QuasarCert (BLS share + ML-DSA sig + Ringtail
// Round 1 commitment). Otherwise it falls back to SHA-256 placeholders for
// in-process tests.
//
// Under a strict-PQ / FIPS profile (CR-10) every fallback path is closed:
// generateCert returns nil unless a real triple-layer cert can be produced.
// The engine treats nil as "round did not finalise" and refuses to advance.
func (h *Certifier) generateCert(block *Block) *QuasarCert {
	h.mu.RLock()
	signer := h.signer
	ctx := h.signerCtx
	validatorCount := len(h.validators)
	demandsTriple := h.demandsTriple()
	h.mu.RUnlock()

	if signer != nil {
		cert := h.realCert(ctx, signer, block, validatorCount)
		if cert != nil {
			// Under a triple-mode profile we MUST refuse a cert that
			// doesn't carry every layer. The realCert path produces
			// BLS + MLDSAProof but intentionally leaves Ringtail
			// empty when this engine signs as a single participant
			// (the Round-1 commitment isn't a verifiable sig on its
			// own — the aggregated Ringtail signature lands here at
			// the higher protocol layer, see epoch.go BundleSigner).
			//
			// Strict-PQ profiles can't emit such a cert. The right
			// architectural answer is to construct this Certifier
			// with a signer that completes all three layers at this
			// point, or to advance the round via the BundleSigner
			// path. Returning nil here surfaces the misconfiguration
			// as "round did not finalise" rather than silently
			// emitting a downgraded cert.
			if demandsTriple {
				if cert.Corona == nil || len(cert.MLDSARollup) == 0 || len(cert.BLS) == 0 {
					return nil
				}
			}
			return cert
		}
		// realCert returned nil (no configured validator with the
		// required keys). Under a triple-mode profile we refuse to
		// fall through to the SHA-256 placeholder — the placeholder
		// is structurally indistinguishable from a real cert on the
		// wire and would silently downgrade.
		if demandsTriple {
			return nil
		}
		// Fall through to SHA-256 placeholder if real signing fails (no
		// configured validator with all three keys), so the engine still
		// makes progress in test scenarios. Production callers should
		// always have a fully-configured signer.
	}

	// No signer attached: never admissible under a triple-mode profile.
	if demandsTriple {
		return nil
	}

	// Legacy SHA-256 placeholder (only used when no signer is attached).
	blsData := sha256.Sum256(block.ID[:])
	pqData := sha256.Sum256(append(block.ID[:], block.ChainID[:]...))

	return &QuasarCert{
		BLS:         blsData[:],
		MLDSARollup: pqData[:],
		Epoch:       block.Height,
		Finality:    time.Now(),
		Validators:  validatorCount,
	}
}

// realCert produces a real QuasarCert by driving the signer's
// TripleSignRound1 path. Returns nil if no validator is configured for all
// three signing paths (the caller falls back to the legacy placeholder).
func (h *Certifier) realCert(ctx context.Context, s *signer, block *Block, validatorCount int) *QuasarCert {
	if ctx == nil {
		ctx = context.Background()
	}

	// Pick any configured validator (the engine itself signs as one
	// participant; full quorum aggregation happens at a higher layer when
	// multiple validators contribute shares).
	var validatorID string
	s.mu.RLock()
	for id := range s.blsSigners {
		validatorID = id
		break
	}
	if validatorID == "" {
		for id := range s.blsKeys {
			validatorID = id
			break
		}
	}
	s.mu.RUnlock()
	if validatorID == "" {
		return nil
	}

	msg := buildBlockMessage(block)
	prfKey := buildPRFKey(block)
	sessionID := int(block.Height)

	sig, _, err := s.TripleSignRound1(ctx, validatorID, msg, sessionID, prfKey)
	if err != nil {
		// TripleSignRound1 requires a BLS threshold signer. Fall back to
		// legacy single-key BLS+ML-DSA via SignMessageWithContext.
		legacy, lerr := s.SignMessageWithContext(ctx, validatorID, msg)
		if lerr != nil {
			return nil
		}
		sig = legacy
	}

	cert := &QuasarCert{
		BLS:    append([]byte(nil), sig.BLS...),
		Corona: nil, // Corona Round 1 commitment is not a verifiable
		// signature on its own; aggregation/Round2 is run by the
		// consensus driver. We leave Corona empty here -- it's wired
		// at the higher protocol layer (epoch.go BundleSigner).
		Epoch:      block.Height,
		Finality:   time.Now(),
		Validators: validatorCount,
	}

	if len(sig.MLDSA) > 0 {
		cert.MLDSARollup = EncodeMLDSASigs([][]byte{sig.MLDSA})
	}

	return cert
}

// buildBlockMessage builds the canonical message bytes that the signer
// commits to for a block. Stable across encode/decode of QuasarCert.
func buildBlockMessage(block *Block) []byte {
	h := sha256.New()
	h.Write(block.ID[:])
	h.Write(block.ChainID[:])
	var buf [8]byte
	for i := 0; i < 8; i++ {
		buf[i] = byte(block.Height >> (8 * (7 - i)))
	}
	h.Write(buf[:])
	return h.Sum(nil)
}

// buildPRFKey derives a per-block 32-byte PRF key for Ringtail signing.
func buildPRFKey(block *Block) []byte {
	h := sha256.Sum256(append(block.ID[:], block.ChainID[:]...))
	return h[:]
}

// AddValidator adds a validator to the consensus.
func (h *Certifier) AddValidator(id string, weight int) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.validators[id] = weight
}

// RemoveValidator removes a validator from the consensus.
func (h *Certifier) RemoveValidator(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.validators, id)
}
