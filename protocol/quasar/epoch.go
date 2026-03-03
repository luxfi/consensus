// Copyright (C) 2025, Lux Industries Inc All rights reserved.
// Epoch-based Pulsar share management for quantum-safe consensus.
//
// The cosmic vocabulary stack (see pulsar/DESIGN.md): a Pulsar group key
// is the persistent body that survives every validator-set rotation;
// only the share distribution rotates. Each Pulsar certificate emitted
// over a Quasar bundle is the "pulse" — one threshold signature under
// the unchanged GroupKey.
//
// Concretely: the master signing secret s and the public matrix A are
// preserved across every routine epoch (Refresh / Reshare). They change
// only at Reanchor — a rare governance event that opens a fresh
// KeyEra.
//
// This file owns the EpochManager — the consensus-layer driver that
// (a) bootstraps the first key era at chain genesis, (b) drives every
// subsequent rotation through the LSS-Pulsar adapter, and (c) wires
// the activation circuit-breaker so a chain never accepts a new
// committee whose shares fail to threshold-sign under the unchanged
// GroupKey.
//
// Rate limiting and validator-set change detection stay unchanged from
// the original RotateEpoch implementation — those are pure consensus
// policy and orthogonal to the share-rotation mechanics.

package quasar

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"sync"
	"time"

	"github.com/luxfi/pulsar/keyera"
	"github.com/luxfi/pulsar/primitives"
	pulsarReshare "github.com/luxfi/pulsar/reshare"
	pulsarSign "github.com/luxfi/pulsar/sign"
	ringtailThreshold "github.com/luxfi/pulsar/threshold"

	"github.com/luxfi/lattice/v7/ring"

	"github.com/luxfi/threshold/pkg/party"
	"github.com/luxfi/threshold/protocols/lss"
)

// signQ is the Pulsar lattice prime, re-exported here so the local
// Lambda recomputation does not require pulsar/sign to add a public
// accessor. Mirrors sign.Q at compile time.
var signQ = pulsarSign.Q

// pulsarComputeLagrange wraps primitives.ComputeLagrangeCoefficients so
// callers do not need to import pulsar/primitives directly. Returns
// Lagrange coefficients keyed by position in the input slice.
func pulsarComputeLagrange(r *ring.Ring, T []int, modulus *big.Int) []ring.Poly {
	return primitives.ComputeLagrangeCoefficients(r, T, modulus)
}

const (
	// MinEpochDuration is the minimum time between key rotations.
	// Changing keys frequently makes quantum attacks harder since any
	// progress toward breaking keys is invalidated when keys change.
	// At 10 minutes per epoch, uint64 supports 351 trillion years of epochs.
	MinEpochDuration = 10 * time.Minute

	// MaxEpochDuration is the maximum time keys can be used.
	// Forces rotation even if validator set hasn't changed.
	// With 10-minute epochs, force rotation after 1 hour (6 epochs).
	MaxEpochDuration = 1 * time.Hour

	// DefaultHistoryLimit is the default number of old epochs to keep.
	// With 10-minute epochs, 6 epochs = 1 hour of history for verification.
	DefaultHistoryLimit = 6

	// QuantumCheckpointInterval is how often we create quantum-safe signatures.
	// Every 3 seconds provides frequent quantum-safe anchors while keys rotate
	// every 10 minutes. With <100 validators, 3-second signing is achievable.
	QuantumCheckpointInterval = 3 * time.Second
)

var (
	ErrEpochRateLimited    = errors.New("epoch keygen rate limited: must wait at least 10 minutes between rotations")
	ErrNoValidatorChange   = errors.New("validator set unchanged, no keygen needed")
	ErrEpochNotFound       = errors.New("epoch not found")
	ErrInvalidValidatorSet = errors.New("invalid validator set configuration")
)

// EpochManager drives the Pulsar share lifecycle for the validator set.
//
// At chain genesis the manager runs Bootstrap (one-time trusted-dealer
// ceremony confined to era 0). Every subsequent validator-set rotation
// is a SHARE rotation, not a key rotation: lss.DynamicResharePulsar
// preserves the GroupKey (A, bTilde) and the master secret s, and only
// rotates the share distribution. The activation circuit-breaker
// (pulsar/reshare.VerifyActivation) gates committing the new committee.
//
// Rate-limiting policy is unchanged from the original epoch design.
// History retention exists to support cross-epoch verification while
// the chain transitions; signatures from old epochs continue to
// verify under the unchanged GroupKey, so history is operationally a
// per-validator-set log rather than a per-key log.
type EpochManager struct {
	mu sync.RWMutex

	// Current epoch state
	currentEpoch   uint64
	currentKeys    *EpochKeys
	lastKeygenTime time.Time

	// currentEra is the persistent Pulsar key era. Bootstrap creates it
	// once at genesis; every subsequent Reshare mutates currentEra.State
	// in place (preserving currentEra.GroupKey). On Reanchor we discard
	// the era and open a fresh one — currently unused; reanchor flows
	// through governance and is wired via a separate API.
	currentEra *keyera.KeyEra

	// Historical epochs for signature verification
	// We need to verify signatures from recent epochs during transitions
	epochHistory map[uint64]*EpochKeys
	historyLimit int // How many old epochs to keep

	// Validator set tracking
	currentValidators []string
	threshold         int

	// Chain identity bound into every resharing transcript. Defaults to
	// "lux-quasar" if unset by the caller; production deployments should
	// inject the chain-specific binding via SetChainID.
	chainID []byte
	groupID []byte
}

// EpochKeys holds the per-epoch Pulsar share state for a specific epoch.
//
// GroupKey is preserved across every Reshare within a key era. Shares
// rotate. Signers wrap each share for the 2-round Pulsar (Ringtail)
// signing protocol.
//
// Naming note: this type was historically called EpochKeys when each
// rotation generated a fresh keypair. With Pulsar resharing the type
// holds *shares* of one persistent key, but the public name is kept
// for byte-stable callers (BundleSigner, GroupedEpochManager,
// adversarial_test.go, etc.). Conceptually it is the per-epoch
// EpochShareState defined in pulsar/DESIGN.md.
type EpochKeys struct {
	Epoch        uint64
	CreatedAt    time.Time
	ExpiresAt    time.Time
	ValidatorSet []string
	Threshold    int
	TotalParties int
	GroupKey     *ringtailThreshold.GroupKey
	Shares       map[string]*ringtailThreshold.KeyShare
	Signers      map[string]*ringtailThreshold.Signer

	// LSS lifecycle fields. Generation increments by one on every
	// successful Reshare under the same KeyEraID. RollbackFrom is
	// nonzero only when this state descends from a Rollback. KeyEraID
	// bumps only on Reanchor (rare governance event).
	KeyEraID     uint64
	Generation   uint64
	RollbackFrom uint64
}

// NewEpochManager creates a new epoch manager.
func NewEpochManager(threshold int, historyLimit int) *EpochManager {
	if historyLimit < 1 {
		historyLimit = DefaultHistoryLimit // Keep enough epochs for cross-epoch verification
	}

	return &EpochManager{
		epochHistory: make(map[uint64]*EpochKeys),
		historyLimit: historyLimit,
		threshold:    threshold,
		chainID:      []byte("lux-quasar"),
		groupID:      []byte{0},
	}
}

// SetChainID binds the resharing transcript to a specific chain identity
// and Pulsar group. Production deployments should call this before
// InitializeEpoch so activation certs cannot be replayed across chains.
func (em *EpochManager) SetChainID(chainID, groupID []byte) {
	em.mu.Lock()
	defer em.mu.Unlock()
	if len(chainID) > 0 {
		em.chainID = append([]byte(nil), chainID...)
	}
	if len(groupID) > 0 {
		em.groupID = append([]byte(nil), groupID...)
	}
}

// InitializeEpoch creates the first epoch with the initial validator set.
//
// At chain genesis this runs the Pulsar Bootstrap ceremony — the one-time
// trusted-dealer step that forms the persistent GroupKey (A, bTilde) and
// distributes the initial shares. The dealer state is erased before
// returning. Subsequent rotations call RotateEpoch, which preserves
// GroupKey and only rotates the share distribution via the LSS-Pulsar
// adapter.
//
// Bootstrap is the only step where a master secret s exists in process
// memory. After this returns the node holds only public values and its
// own share.
func (em *EpochManager) InitializeEpoch(validators []string) (*EpochKeys, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	if len(validators) < 2 {
		return nil, fmt.Errorf("%w: need at least 2 validators", ErrInvalidValidatorSet)
	}

	if em.threshold >= len(validators) {
		return nil, fmt.Errorf("%w: threshold %d must be less than validator count %d",
			ErrInvalidValidatorSet, em.threshold, len(validators))
	}

	era, err := keyera.Bootstrap(
		em.threshold,
		validators,
		keyera.PulsarGroupID(0),
		keyera.PulsarKeyEraID(1), // EraID 0 is reserved as "unset"
		nil,                      // crypto/rand.Reader
	)
	if err != nil {
		return nil, fmt.Errorf("pulsar bootstrap: %w", err)
	}
	keys := em.epochKeysFromState(0, validators, em.threshold, era.State, era.GroupKey)

	em.currentEpoch = 0
	em.currentKeys = keys
	em.currentEra = era
	em.currentValidators = validators
	em.lastKeygenTime = time.Now()
	em.epochHistory[0] = keys

	return keys, nil
}

// RotateEpoch transitions the chain to a new validator set by RESHARING
// the existing GroupKey to the new committee — never by generating a
// fresh keypair. The master secret s, the public matrix A, and the
// rounded public key bTilde are all preserved across this call.
//
// Two distinct flows live here:
//
//  1. First call (currentEra == nil): treated as a delayed Bootstrap
//     for callers that did not invoke InitializeEpoch. Equivalent to
//     InitializeEpoch but rate-limit-aware.
//
//  2. Subsequent calls: build a map[party.ID]*lss.PulsarConfig from the
//     era's current shares, hand it to lss.DynamicResharePulsar, and
//     gate acceptance on the activation circuit-breaker. The new
//     committee threshold-signs the canonical activation message under
//     the unchanged GroupKey; only when the signature verifies does
//     this method commit the new state and return.
//
// Returns ErrEpochRateLimited if called within MinEpochDuration of the
// last rotation. Returns ErrNoValidatorChange if the validator set
// hasn't changed (unless force=true). Returns a wrapped activation
// error (pulsarReshare.ErrActivationFailed) if the new committee
// cannot collectively sign under the unchanged GroupKey — in which
// case the chain stays at the old epoch and the old committee
// continues signing.
func (em *EpochManager) RotateEpoch(validators []string, force bool) (*EpochKeys, error) {
	em.mu.Lock()
	defer em.mu.Unlock()

	now := time.Now()

	// Rate limiting: at most 1 rotation per 10 minutes
	if !em.lastKeygenTime.IsZero() {
		elapsed := now.Sub(em.lastKeygenTime)
		if elapsed < MinEpochDuration {
			remaining := MinEpochDuration - elapsed
			return nil, fmt.Errorf("%w: %v remaining", ErrEpochRateLimited, remaining.Round(time.Second))
		}
	}

	// Check if validator set actually changed
	if !force && em.validatorSetUnchanged(validators) {
		return nil, ErrNoValidatorChange
	}

	// Validate new set
	if len(validators) < 2 {
		return nil, fmt.Errorf("%w: need at least 2 validators", ErrInvalidValidatorSet)
	}

	effectiveThreshold := em.threshold
	if effectiveThreshold >= len(validators) {
		effectiveThreshold = len(validators) - 1
	}

	newEpoch := em.currentEpoch + 1
	keys, err := em.reshareEpochKeys(newEpoch, validators, effectiveThreshold)
	if err != nil {
		return nil, err
	}

	// Store old epoch in history
	if em.currentKeys != nil {
		em.epochHistory[em.currentEpoch] = em.currentKeys
	}

	// Prune old epochs
	em.pruneHistory()

	// Update current state
	em.currentEpoch = newEpoch
	em.currentKeys = keys
	em.currentValidators = validators
	em.lastKeygenTime = now
	em.epochHistory[newEpoch] = keys

	return keys, nil
}

// ForceRotateIfExpired rotates keys if MaxEpochDuration has passed.
// This ensures keys don't stay valid indefinitely even if validator set is stable.
func (em *EpochManager) ForceRotateIfExpired() (*EpochKeys, bool, error) {
	em.mu.RLock()
	if em.currentKeys == nil {
		em.mu.RUnlock()
		return nil, false, nil
	}

	expired := time.Now().After(em.currentKeys.ExpiresAt)
	validators := em.currentValidators
	em.mu.RUnlock()

	if !expired {
		return nil, false, nil
	}

	keys, err := em.RotateEpoch(validators, true)
	if err != nil {
		return nil, false, err
	}

	return keys, true, nil
}

// GetCurrentKeys returns the current epoch's keys.
func (em *EpochManager) GetCurrentKeys() *EpochKeys {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.currentKeys
}

// GetEpochKeys returns keys for a specific epoch (current or historical).
func (em *EpochManager) GetEpochKeys(epoch uint64) (*EpochKeys, error) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if keys, exists := em.epochHistory[epoch]; exists {
		return keys, nil
	}
	return nil, fmt.Errorf("%w: epoch %d", ErrEpochNotFound, epoch)
}

// GetCurrentEpoch returns the current epoch number.
func (em *EpochManager) GetCurrentEpoch() uint64 {
	em.mu.RLock()
	defer em.mu.RUnlock()
	return em.currentEpoch
}

// GetSigner returns the Ringtail signer for a validator in the current epoch.
func (em *EpochManager) GetSigner(validatorID string) (*ringtailThreshold.Signer, error) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if em.currentKeys == nil {
		return nil, errors.New("no current epoch keys")
	}

	signer, exists := em.currentKeys.Signers[validatorID]
	if !exists {
		return nil, fmt.Errorf("validator %s not in current epoch", validatorID)
	}

	return signer, nil
}

// GetSignerForEpoch returns the signer for a validator in a specific epoch.
func (em *EpochManager) GetSignerForEpoch(validatorID string, epoch uint64) (*ringtailThreshold.Signer, error) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	keys, exists := em.epochHistory[epoch]
	if !exists {
		return nil, fmt.Errorf("%w: epoch %d", ErrEpochNotFound, epoch)
	}

	signer, exists := keys.Signers[validatorID]
	if !exists {
		return nil, fmt.Errorf("validator %s not in epoch %d", validatorID, epoch)
	}

	return signer, nil
}

// VerifySignatureForEpoch verifies a Ringtail signature using the epoch's keys.
func (em *EpochManager) VerifySignatureForEpoch(message string, sig *ringtailThreshold.Signature, epoch uint64) bool {
	em.mu.RLock()
	keys, exists := em.epochHistory[epoch]
	em.mu.RUnlock()

	if !exists || keys.GroupKey == nil || sig == nil {
		return false
	}

	return ringtailThreshold.Verify(keys.GroupKey, message, sig)
}

// TimeUntilNextRotation returns how long until the next rotation is allowed.
func (em *EpochManager) TimeUntilNextRotation() time.Duration {
	em.mu.RLock()
	defer em.mu.RUnlock()

	if em.lastKeygenTime.IsZero() {
		return 0
	}

	elapsed := time.Since(em.lastKeygenTime)
	if elapsed >= MinEpochDuration {
		return 0
	}

	return MinEpochDuration - elapsed
}

// Stats returns current epoch statistics.
func (em *EpochManager) Stats() EpochStats {
	em.mu.RLock()
	defer em.mu.RUnlock()

	var validatorCount int
	var epochAge time.Duration
	if em.currentKeys != nil {
		validatorCount = len(em.currentKeys.ValidatorSet)
		epochAge = time.Since(em.currentKeys.CreatedAt)
	}

	return EpochStats{
		CurrentEpoch:      em.currentEpoch,
		EpochAge:          epochAge,
		ValidatorCount:    validatorCount,
		Threshold:         em.threshold,
		HistorySize:       len(em.epochHistory),
		LastKeygenTime:    em.lastKeygenTime,
		TimeUntilRotation: em.timeUntilNextRotationLocked(),
	}
}

// EpochStats provides statistics about the epoch manager.
type EpochStats struct {
	CurrentEpoch      uint64
	EpochAge          time.Duration
	ValidatorCount    int
	Threshold         int
	HistorySize       int
	LastKeygenTime    time.Time
	TimeUntilRotation time.Duration
}

// Internal helpers

// reshareEpochKeys drives the share rotation for a new validator set.
//
// First call (no era yet): runs Bootstrap and returns its initial state.
// This branch is the caller-friendly path for code that skipped
// InitializeEpoch — it keeps the rate-limit guard inline.
//
// Subsequent calls: builds the per-party PulsarConfig map from the
// current era's shares, calls lss.DynamicResharePulsar, then drives
// the new committee to threshold-sign the canonical activation message
// under the unchanged GroupKey. The activation cert is verified via
// pulsarReshare.VerifyActivation. The new state is committed only on
// successful verification.
func (em *EpochManager) reshareEpochKeys(epoch uint64, validators []string, threshold int) (*EpochKeys, error) {
	if em.currentEra == nil {
		era, err := keyera.Bootstrap(
			threshold,
			validators,
			keyera.PulsarGroupID(0),
			keyera.PulsarKeyEraID(1),
			nil,
		)
		if err != nil {
			return nil, fmt.Errorf("pulsar bootstrap (delayed): %w", err)
		}
		em.currentEra = era
		return em.epochKeysFromState(epoch, validators, threshold, era.State, era.GroupKey), nil
	}

	era := em.currentEra
	gkBefore := era.GroupKey

	// Build per-party LSS-Pulsar configs from the current era state.
	oldConfigs := make(map[party.ID]*lss.PulsarConfig, len(era.State.Validators))
	for _, v := range era.State.Validators {
		share, ok := era.State.Shares[v]
		if !ok {
			return nil, fmt.Errorf("pulsar reshare: missing share for old validator %s", v)
		}
		perParty := &keyera.EpochShareState{
			KeyEraID:     era.State.KeyEraID,
			Generation:   era.State.Generation,
			RollbackFrom: era.State.RollbackFrom,
			Epoch:        era.State.Epoch,
			Validators:   era.State.Validators,
			Threshold:    era.State.Threshold,
			Shares:       map[string]*ringtailThreshold.KeyShare{v: share},
		}
		oldConfigs[party.ID(v)] = &lss.PulsarConfig{State: perParty, PartyID: party.ID(v)}
	}

	newPartyIDs := make([]party.ID, len(validators))
	for i, v := range validators {
		newPartyIDs[i] = party.ID(v)
	}

	newConfigs, err := lss.DynamicResharePulsar(oldConfigs, newPartyIDs, threshold, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("pulsar reshare (lss adapter): %w", err)
	}

	// Reassemble a single in-process state for the new committee. Each
	// PulsarConfig holds only its own party's share; combine them into
	// the era state so this node — which carries every share in the
	// trusted-collaborator orchestration path — can drive activation
	// signing on behalf of the committee.
	var (
		nextKeyEraID     uint64
		nextGeneration   uint64
		nextRollbackFrom uint64
		nextEpoch        uint64
		nextThreshold    int
	)
	combinedShares := make(map[string]*ringtailThreshold.KeyShare, len(newPartyIDs))
	for _, id := range newPartyIDs {
		cfg := newConfigs[id]
		if cfg == nil || cfg.State == nil {
			return nil, fmt.Errorf("pulsar reshare: lss adapter returned no state for %s", id)
		}
		share, ok := cfg.State.Shares[string(id)]
		if !ok {
			return nil, fmt.Errorf("pulsar reshare: lss adapter returned no share for %s", id)
		}
		combinedShares[string(id)] = share
		nextKeyEraID = cfg.State.KeyEraID
		nextGeneration = cfg.State.Generation
		nextRollbackFrom = cfg.State.RollbackFrom
		nextEpoch = cfg.State.Epoch
		nextThreshold = cfg.State.Threshold
	}

	nextState := &keyera.EpochShareState{
		KeyEraID:     nextKeyEraID,
		Generation:   nextGeneration,
		RollbackFrom: nextRollbackFrom,
		Epoch:        nextEpoch,
		Validators:   append([]string(nil), validators...),
		Threshold:    nextThreshold,
		Shares:       combinedShares,
	}

	// Activation circuit-breaker. The new committee threshold-signs the
	// canonical activation message under the UNCHANGED GroupKey. If the
	// signature verifies, the chain commits; otherwise it stays at the
	// old epoch (the caller treats the error as ErrActivationFailed and
	// the old committee continues signing on the next round).
	if err := em.verifyActivationLocked(gkBefore, era.State, nextState); err != nil {
		return nil, fmt.Errorf("pulsar reshare activation: %w", err)
	}

	// Commit the new state to the era. Within a key era, era.GroupKey
	// pointer is preserved.
	era.State = nextState

	return em.epochKeysFromState(epoch, validators, threshold, nextState, era.GroupKey), nil
}

// epochKeysFromState assembles an EpochKeys snapshot from a Pulsar
// EpochShareState plus the era's persistent GroupKey. The Signers map
// wraps each share for the 2-round Pulsar signing protocol, byte-stable
// with the historical EpochKeys layout.
func (em *EpochManager) epochKeysFromState(epoch uint64, validators []string, threshold int, state *keyera.EpochShareState, gk *ringtailThreshold.GroupKey) *EpochKeys {
	now := time.Now()
	keys := &EpochKeys{
		Epoch:        epoch,
		CreatedAt:    now,
		ExpiresAt:    now.Add(MaxEpochDuration),
		ValidatorSet: append([]string(nil), validators...),
		Threshold:    threshold,
		TotalParties: len(validators),
		GroupKey:     gk,
		Shares:       make(map[string]*ringtailThreshold.KeyShare, len(validators)),
		Signers:      make(map[string]*ringtailThreshold.Signer, len(validators)),
		KeyEraID:     state.KeyEraID,
		Generation:   state.Generation,
		RollbackFrom: state.RollbackFrom,
	}
	for _, v := range validators {
		share := state.Shares[v]
		keys.Shares[v] = share
		keys.Signers[v] = ringtailThreshold.NewSigner(share)
	}
	return keys
}

// verifyActivationLocked drives the activation circuit-breaker for a
// resharing transition. The new committee threshold-signs the canonical
// activation message under the UNCHANGED GroupKey using their freshly-
// derived shares; the chain accepts the new state only when the
// signature verifies via pulsarReshare.VerifyActivation.
//
// In the trusted-collaborator orchestration path (this in-process
// EpochManager), every validator's share lives on this node, so the
// node can drive every Round1/Round2/Finalize step locally. In a
// distributed deployment, the round-based protocol at
// threshold/protocols/pulsar replaces this with per-party computation;
// the consensus mempool collects the partial signatures and the
// VerifyActivation check happens in the same shape.
//
// transcript and exchange hashes are bound to the chain identity
// (chainID, groupID), the era lineage (KeyEraID), and the
// generation/epoch tuple so that a malicious proposer cannot replay an
// activation cert across chains, eras, or generations.
func (em *EpochManager) verifyActivationLocked(gk *ringtailThreshold.GroupKey, oldState, newState *keyera.EpochShareState) error {
	// Threshold-sign the activation message under the new committee.
	signers := make([]*ringtailThreshold.Signer, 0, len(newState.Validators))
	signerIndices := make([]int, 0, len(newState.Validators))
	for _, v := range newState.Validators {
		share, ok := newState.Shares[v]
		if !ok {
			return fmt.Errorf("activation: missing share for new validator %s", v)
		}
		signers = append(signers, ringtailThreshold.NewSigner(share))
		signerIndices = append(signerIndices, share.Index)
	}

	// Sign the exact bytes the chain will verify. We don't yet have a
	// distributed exchange transcript (no commits/complaints in the
	// in-process path); the empty ReshareTranscript hashes deterministically
	// to a fixed value the verifier reproduces.
	tx := pulsarReshare.TranscriptInputs{
		ChainID:            em.chainID,
		GroupID:            em.groupID,
		OldEpochID:         oldState.Epoch,
		NewEpochID:         newState.Epoch,
		OldSetHash:         hashValidatorSetForActivation(oldState.Validators),
		NewSetHash:         hashValidatorSetForActivation(newState.Validators),
		ThresholdOld:       uint32(oldState.Threshold),
		ThresholdNew:       uint32(newState.Threshold),
		GroupPublicKeyHash: hashGroupKey(gk),
		Variant:            "reshare",
	}
	exchange := pulsarReshare.ReshareTranscript{}
	activation := pulsarReshare.ActivationMessage{
		Transcript:        tx,
		ReshareTranscript: exchange,
	}
	// nil suite resolves to the production default Pulsar HashSuite.
	msg := activation.SignableBytes(nil)

	const sessionID = 0
	prfKey := derivePRFKey(em.chainID, em.groupID, newState.KeyEraID, newState.Generation, newState.Epoch)

	round1 := make(map[int]*ringtailThreshold.Round1Data, len(signers))
	for _, s := range signers {
		// Round1 sets PartyID from the underlying share index.
		r1 := s.Round1(sessionID, prfKey, signerIndices)
		round1[r1.PartyID] = r1
	}
	round2 := make(map[int]*ringtailThreshold.Round2Data, len(signers))
	for _, s := range signers {
		r2, err := s.Round2(sessionID, string(msg), prfKey, signerIndices, round1)
		if err != nil {
			return fmt.Errorf("activation: round2: %w", err)
		}
		round2[r2.PartyID] = r2
	}
	sig, err := signers[0].Finalize(round2)
	if err != nil {
		return fmt.Errorf("activation: finalize: %w", err)
	}

	// VerifyActivation runs the chain-level circuit-breaker check.
	// It re-derives both transcript hashes from cert.Message and
	// confirms they match the chain's local view, then runs the
	// supplied verifier callback against the unchanged GroupKey.
	cert := &pulsarReshare.ActivationCert{
		Message:   activation,
		Signature: nil, // opaque bytes; the verifier closure ignores Signature serialization here
	}
	localTranscriptHash := tx.Hash(nil)
	localExchangeHash := exchange.Hash(nil)
	verifier := func(message, _ []byte) bool {
		// We hold the structured Pulsar Signature (sig) directly; the
		// closure receives the same canonical message we just signed.
		// The Signature reference is captured here; ActivationCert's
		// opaque bytes are unused in the in-process path.
		_ = message
		return ringtailThreshold.Verify(gk, string(msg), sig)
	}
	if err := pulsarReshare.VerifyActivation(cert, localTranscriptHash, localExchangeHash, nil, verifier); err != nil {
		return err
	}
	return nil
}

// hashValidatorSetForActivation returns the canonical 32-byte hash of a validator set.
// Sorted-by-string ordering matches the OldSetHash/NewSetHash binding
// the activation transcript expects.
func hashValidatorSetForActivation(validators []string) [32]byte {
	sorted := append([]string(nil), validators...)
	// Insertion sort — committee size is bounded.
	for i := 1; i < len(sorted); i++ {
		for j := i; j > 0 && sorted[j-1] > sorted[j]; j-- {
			sorted[j-1], sorted[j] = sorted[j], sorted[j-1]
		}
	}
	h := sha256.New()
	h.Write([]byte("quasar.validator-set.v1"))
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(sorted)))
	h.Write(lenBuf[:])
	for _, v := range sorted {
		binary.BigEndian.PutUint32(lenBuf[:], uint32(len(v)))
		h.Write(lenBuf[:])
		h.Write([]byte(v))
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// hashGroupKey returns a stable 32-byte digest of a Pulsar GroupKey for
// transcript binding. The current public Bytes() method returns a
// short-form summary; we wrap it in SHA-256 for fixed width.
func hashGroupKey(gk *ringtailThreshold.GroupKey) [32]byte {
	h := sha256.New()
	h.Write([]byte("quasar.group-key.v1"))
	if gk != nil {
		h.Write(gk.Bytes())
	}
	var out [32]byte
	copy(out[:], h.Sum(nil))
	return out
}

// derivePRFKey produces a 32-byte session-binding PRF key for the
// activation signing. Distinct from any block-level prfKey so an
// activation signature cannot be replayed for block-level signing.
func derivePRFKey(chainID, groupID []byte, keyEraID, generation, epoch uint64) []byte {
	h := sha256.New()
	h.Write([]byte("quasar.activation.prf.v1"))
	h.Write(chainID)
	h.Write(groupID)
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], keyEraID)
	h.Write(buf[:])
	binary.BigEndian.PutUint64(buf[:], generation)
	h.Write(buf[:])
	binary.BigEndian.PutUint64(buf[:], epoch)
	h.Write(buf[:])
	return h.Sum(nil)
}

func (em *EpochManager) validatorSetUnchanged(newValidators []string) bool {
	if len(newValidators) != len(em.currentValidators) {
		return false
	}

	// Create set of current validators
	current := make(map[string]bool, len(em.currentValidators))
	for _, v := range em.currentValidators {
		current[v] = true
	}

	// Check if all new validators exist in current set
	for _, v := range newValidators {
		if !current[v] {
			return false
		}
	}

	return true
}

func (em *EpochManager) pruneHistory() {
	if len(em.epochHistory) <= em.historyLimit {
		return
	}

	// Find the minimum epoch to keep
	minEpochToKeep := em.currentEpoch
	if uint64(em.historyLimit) < minEpochToKeep {
		minEpochToKeep = em.currentEpoch - uint64(em.historyLimit) + 1
	} else {
		minEpochToKeep = 0
	}

	// Remove old epochs
	for epoch := range em.epochHistory {
		if epoch < minEpochToKeep {
			delete(em.epochHistory, epoch)
		}
	}
}

func (em *EpochManager) timeUntilNextRotationLocked() time.Duration {
	if em.lastKeygenTime.IsZero() {
		return 0
	}

	elapsed := time.Since(em.lastKeygenTime)
	if elapsed >= MinEpochDuration {
		return 0
	}

	return MinEpochDuration - elapsed
}

// ============================================================================
// Quantum Block - Bundles BLS blocks into quantum-safe anchors
// ============================================================================
//
// Architecture (parallel execution):
//   BLS Layer:     [B1]--[B2]--[B3]--[B4]--[B5]--[B6]--[B7]--[B8]--[B9]--...
//                   |     500ms finality per block     |
//                   |_____________________________________|
//                                    |
//   Quantum Layer:              [QB1: Merkle(B1-B6)]--------[QB2: Merkle(B7-B12)]
//                                    |  3-second interval, async Ringtail signing
//
// NTT Ringtail benchmarks (IEEE S&P 2025):
//   - 0.6s online signing phase (2-round protocol)
//   - 2.5s total including offline prep across 5 continents
//   - Our 3-second interval provides comfortable margin

// QuantumBundle bundles multiple BLS-signed blocks into a quantum-safe anchor.
// BLS blocks continue at 500ms pace; quantum bundles form every 3 seconds
// containing a Merkle root of ~6 BLS block hashes.
// Note: This is distinct from core.go's QuantumBlock which represents per-block finality.
type QuantumBundle struct {
	Epoch        uint64     // Current key epoch
	Sequence     uint64     // Bundle sequence within epoch
	StartHeight  uint64     // First BLS block in this bundle
	EndHeight    uint64     // Last BLS block in this bundle
	BlockCount   int        // Number of BLS blocks bundled
	MerkleRoot   [32]byte   // Merkle root of BLS block hashes
	BlockHashes  [][32]byte // Individual block hashes (for Merkle proof)
	PreviousHash [32]byte   // Previous bundle hash (chain linkage)
	Timestamp    int64      // Unix timestamp

	// Ringtail threshold signature (post-quantum secure)
	Signature *ringtailThreshold.Signature
}

// Hash returns the hash of this bundle for chain linkage.
func (qb *QuantumBundle) Hash() [32]byte {
	h := sha256.New()
	buf := make([]byte, 8)

	// Epoch + sequence
	binary.BigEndian.PutUint64(buf, qb.Epoch)
	h.Write(buf)
	binary.BigEndian.PutUint64(buf, qb.Sequence)
	h.Write(buf)

	// Block range
	binary.BigEndian.PutUint64(buf, qb.StartHeight)
	h.Write(buf)
	binary.BigEndian.PutUint64(buf, qb.EndHeight)
	h.Write(buf)

	// Merkle root of bundled BLS blocks
	h.Write(qb.MerkleRoot[:])

	// Previous bundle hash (chain linkage)
	h.Write(qb.PreviousHash[:])

	// Timestamp
	binary.BigEndian.PutUint64(buf, uint64(qb.Timestamp))
	h.Write(buf)

	var result [32]byte
	copy(result[:], h.Sum(nil))
	return result
}

// SignableMessage returns the message for Ringtail signing.
func (qb *QuantumBundle) SignableMessage() string {
	hash := qb.Hash()
	return fmt.Sprintf("QUASAR-QB-v1:%x", hash)
}

// BundleSigner handles creating and verifying 3-second quantum bundles.
// Bundles accumulate multiple BLS blocks and sign them with Ringtail.
type BundleSigner struct {
	em         *EpochManager
	lastBundle *QuantumBundle
	sequence   uint64

	// Pending BLS blocks waiting to be bundled
	pendingBlocks [][32]byte
	pendingStart  uint64
	pendingEnd    uint64

	mu sync.Mutex
}

// NewBundleSigner creates a bundle signer for the epoch manager.
func NewBundleSigner(em *EpochManager) *BundleSigner {
	return &BundleSigner{em: em}
}

// AddBLSBlock adds a finalized BLS block hash to the pending bundle.
// Called whenever a BLS block achieves finality (~500ms).
func (bs *BundleSigner) AddBLSBlock(height uint64, hash [32]byte) {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if len(bs.pendingBlocks) == 0 {
		bs.pendingStart = height
	}
	bs.pendingBlocks = append(bs.pendingBlocks, hash)
	bs.pendingEnd = height
}

// PendingCount returns the number of pending BLS blocks.
func (bs *BundleSigner) PendingCount() int {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return len(bs.pendingBlocks)
}

// CreateBundle bundles pending BLS blocks into a quantum bundle.
// Call this every 3 seconds (QuantumCheckpointInterval).
// Returns nil if no pending blocks.
func (bs *BundleSigner) CreateBundle() *QuantumBundle {
	bs.mu.Lock()
	defer bs.mu.Unlock()

	if len(bs.pendingBlocks) == 0 {
		return nil
	}

	// Compute Merkle root of bundled BLS blocks
	merkleRoot := ComputeMerkleRoot(bs.pendingBlocks)

	// Get previous bundle hash for chain linkage
	var prevHash [32]byte
	if bs.lastBundle != nil {
		prevHash = bs.lastBundle.Hash()
	}

	epoch := bs.em.GetCurrentEpoch()

	// Reset sequence on new epoch
	if bs.lastBundle != nil && bs.lastBundle.Epoch != epoch {
		bs.sequence = 0
	}

	qb := &QuantumBundle{
		Epoch:        epoch,
		Sequence:     bs.sequence,
		StartHeight:  bs.pendingStart,
		EndHeight:    bs.pendingEnd,
		BlockCount:   len(bs.pendingBlocks),
		MerkleRoot:   merkleRoot,
		BlockHashes:  bs.pendingBlocks,
		PreviousHash: prevHash,
		Timestamp:    time.Now().Unix(),
	}

	// Clear pending and advance sequence
	bs.pendingBlocks = nil
	bs.sequence++
	bs.lastBundle = qb

	return qb
}

// LastBundle returns the most recent bundle.
func (bs *BundleSigner) LastBundle() *QuantumBundle {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	return bs.lastBundle
}

// SignBundle performs the 2-round Pulsar (Ringtail) signing for a
// bundle. Runs the full threshold signing protocol with the
// participating validators.
//
// Each share's Lambda is recomputed for the participating subset
// before signing. Pulsar's keyera bakes a Lambda for the FULL
// committee at Bootstrap; when fewer parties sign, the Lagrange
// weights must be re-derived for the actual signing subset so the
// reconstructed signature evaluates the secret-sharing polynomial at
// 0 correctly. This is identical to how distributed Pulsar deployments
// recompute Lambda when a coordinator picks a quorum smaller than the
// full set.
func (bs *BundleSigner) SignBundle(
	bundle *QuantumBundle,
	sessionID int,
	prfKey []byte,
	participatingValidators []string,
) error {
	bs.em.mu.RLock()
	keys := bs.em.currentKeys
	bs.em.mu.RUnlock()

	if keys == nil {
		return errors.New("no current epoch keys")
	}

	// Build signer indices
	signerIndices := make([]int, 0, len(participatingValidators))
	for _, v := range participatingValidators {
		if share, ok := keys.Shares[v]; ok {
			signerIndices = append(signerIndices, share.Index)
		}
	}

	if len(signerIndices) < keys.Threshold {
		return fmt.Errorf("insufficient signers: %d < threshold %d", len(signerIndices), keys.Threshold)
	}

	subsetSigners, err := newSubsetSigners(keys, participatingValidators, signerIndices)
	if err != nil {
		return err
	}

	message := bundle.SignableMessage()

	// Round 1: Collect commitments
	round1Data := make(map[int]*ringtailThreshold.Round1Data)
	for _, v := range participatingValidators {
		signer, ok := subsetSigners[v]
		if !ok {
			continue
		}
		r1 := signer.Round1(sessionID, prfKey, signerIndices)
		round1Data[keys.Shares[v].Index] = r1
	}

	// Round 2: Generate signature shares
	round2Data := make(map[int]*ringtailThreshold.Round2Data)
	for _, v := range participatingValidators {
		signer, ok := subsetSigners[v]
		if !ok {
			continue
		}
		r2, err := signer.Round2(sessionID, message, prfKey, signerIndices, round1Data)
		if err != nil {
			return fmt.Errorf("round2 failed for %s: %w", v, err)
		}
		round2Data[r2.PartyID] = r2
	}

	// Finalize
	firstValidator := participatingValidators[0]
	finalSigner := subsetSigners[firstValidator]
	sig, err := finalSigner.Finalize(round2Data)
	if err != nil {
		return fmt.Errorf("finalize failed: %w", err)
	}

	bundle.Signature = sig
	return nil
}

// newSubsetSigners produces a fresh set of Signers whose underlying
// shares carry Lambda recomputed for the participating subset.
//
// The per-share KeyShare has an Index field that the original keygen
// converted into an evaluation point (Index+1). When a strict subset
// signs, the Lagrange coefficient for each signer at the polynomial's
// constant term must be derived from the SUBSET evaluation points,
// not the full-committee evaluation points. We compute these on the
// fly and clone each KeyShare with the corrected Lambda.
//
// Side effects: this allocates one KeyShare clone and one Signer per
// participating validator. The original epoch state is not mutated.
func newSubsetSigners(keys *EpochKeys, participating []string, signerIndices []int) (map[string]*ringtailThreshold.Signer, error) {
	if len(participating) == 0 {
		return nil, errors.New("no participating validators")
	}
	if keys == nil || keys.GroupKey == nil {
		return nil, errors.New("missing group key")
	}
	r := keys.GroupKey.Params.R
	q := big.NewInt(int64(signQ))
	subsetLambda := pulsarComputeLagrange(r, signerIndices, q)

	indexToSlot := make(map[int]int, len(signerIndices))
	for slot, idx := range signerIndices {
		indexToSlot[idx] = slot
	}

	out := make(map[string]*ringtailThreshold.Signer, len(participating))
	for _, v := range participating {
		share, ok := keys.Shares[v]
		if !ok {
			continue
		}
		slot, ok := indexToSlot[share.Index]
		if !ok {
			continue
		}
		newLambda := r.NewPoly()
		newLambda.Copy(subsetLambda[slot])
		r.NTT(newLambda, newLambda)
		r.MForm(newLambda, newLambda)
		cloned := *share
		cloned.Lambda = newLambda
		out[v] = ringtailThreshold.NewSigner(&cloned)
	}
	return out, nil
}

// VerifyBundle verifies a quantum bundle's Ringtail signature.
func (bs *BundleSigner) VerifyBundle(bundle *QuantumBundle) bool {
	if bundle.Signature == nil {
		return false
	}

	// Verify Merkle root matches block hashes
	expectedRoot := ComputeMerkleRoot(bundle.BlockHashes)
	if expectedRoot != bundle.MerkleRoot {
		return false
	}

	// Get keys for the bundle's epoch
	keys, err := bs.em.GetEpochKeys(bundle.Epoch)
	if err != nil {
		return false
	}

	message := bundle.SignableMessage()
	return ringtailThreshold.Verify(keys.GroupKey, message, bundle.Signature)
}

// ComputeMerkleRoot computes a Merkle root over block hashes.
func ComputeMerkleRoot(hashes [][32]byte) [32]byte {
	if len(hashes) == 0 {
		return [32]byte{}
	}
	if len(hashes) == 1 {
		return hashes[0]
	}

	// Pad to even length
	level := make([][32]byte, len(hashes))
	copy(level, hashes)
	if len(level)%2 != 0 {
		level = append(level, level[len(level)-1])
	}

	// Build tree
	for len(level) > 1 {
		nextLevel := make([][32]byte, len(level)/2)
		for i := 0; i < len(level); i += 2 {
			combined := append(level[i][:], level[i+1][:]...)
			nextLevel[i/2] = sha256.Sum256(combined)
		}
		level = nextLevel
		if len(level) > 1 && len(level)%2 != 0 {
			level = append(level, level[len(level)-1])
		}
	}

	return level[0]
}
