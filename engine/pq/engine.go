package pq

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/crypto/mldsa"
	"github.com/luxfi/ids"
	coronaThreshold "github.com/luxfi/threshold/protocols/corona"
)

// Engine defines the post-quantum consensus engine
type Engine interface {
	// Start starts the engine
	Start(context.Context, uint32) error

	// Stop stops the engine
	Stop(context.Context) error

	// HealthCheck performs a health check
	HealthCheck(context.Context) (interface{}, error)

	// IsBootstrapped returns whether the engine is bootstrapped
	IsBootstrapped() bool

	// VerifyQuantumSignature verifies a post-quantum signature
	VerifyQuantumSignature([]byte, []byte, []byte) error

	// GenerateQuantumProof generates a quantum-resistant proof
	GenerateQuantumProof(context.Context, ids.ID) ([]byte, error)
}

// PostQuantum implements post-quantum consensus engine
type PostQuantum struct {
	bootstrapped bool
	algorithm    string // ML-DSA, ML-KEM, etc.

	mu sync.RWMutex
	// Real signing engine, optional. When set, GenerateQuantumProof
	// produces a serialized QuasarCert via TripleSignRound1.
	signer *quasar.Signer
	// Verification keys, optional. When set, VerifyQuantumSignature
	// performs real BLS+Corona+ML-DSA verification on a serialized
	// QuasarCert.
	blsAggKey   *bls.PublicKey
	rtGroupKey  *coronaThreshold.GroupKey
	mldsaPubKey *mldsa.PublicKey

	// certPolicy, when set, makes cert verification policy-driven: the
	// chain's config.CertPolicy decides which legs are mandatory
	// (RequiredLegs), independent of which legs the cert bytes carry.
	// Default (CertModeOff, Hybrid) verifies the supplied-key legs via
	// the implied-policy path; SetCertPolicy upgrades to strict
	// policy-enforced verification (e.g. strict-PQ chains).
	certPolicy    config.CertPolicy
	certPolicySet bool
}

// New creates a new post-quantum consensus engine
func New() *PostQuantum {
	return &PostQuantum{
		bootstrapped: false,
		algorithm:    "ML-DSA-65", // Default to ML-DSA-65
	}
}

// AttachSigner wires a real PQ signer for proof generation.
func (pq *PostQuantum) AttachSigner(s *quasar.Signer) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.signer = s
}

// AttachVerifyKeys wires real PQ verification keys. blsAggKey is required;
// rtGroupKey and mldsaPubKey are optional (skipped if nil).
func (pq *PostQuantum) AttachVerifyKeys(blsAggKey *bls.PublicKey, rtGroupKey *coronaThreshold.GroupKey, mldsaPubKey *mldsa.PublicKey) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.blsAggKey = blsAggKey
	pq.rtGroupKey = rtGroupKey
	pq.mldsaPubKey = mldsaPubKey
}

// SetCertPolicy binds the chain's config.CertPolicy so cert verification
// is policy-driven: VerifyQuantumSignature then rejects any cert missing
// a RequiredLegs() leg (or its key), regardless of the leg bytes the cert
// carries. This is how a strict-PQ chain forces the PQ legs to be present.
func (pq *PostQuantum) SetCertPolicy(cp config.CertPolicy) {
	pq.mu.Lock()
	defer pq.mu.Unlock()
	pq.certPolicy = cp
	pq.certPolicySet = true
}

// Start starts the engine
func (pq *PostQuantum) Start(ctx context.Context, requestID uint32) error {
	pq.bootstrapped = true
	return nil
}

// Stop stops the engine
func (pq *PostQuantum) Stop(ctx context.Context) error {
	return nil
}

// HealthCheck performs a health check
func (pq *PostQuantum) HealthCheck(ctx context.Context) (interface{}, error) {
	return map[string]interface{}{
		"healthy":   true,
		"algorithm": pq.algorithm,
	}, nil
}

// IsBootstrapped returns whether the engine is bootstrapped
func (pq *PostQuantum) IsBootstrapped() bool {
	return pq.bootstrapped
}

// VerifyQuantumSignature verifies a serialized QuasarCert.
//
// signature must be the bytes produced by QuasarCert.MarshalBinary.
// message is the digest the cert commits to.
// publicKey is reserved for future single-key paths; current implementation
// uses verification keys attached via AttachVerifyKeys.
func (pq *PostQuantum) VerifyQuantumSignature(message, signature, publicKey []byte) error {
	pq.mu.RLock()
	blsAggKey := pq.blsAggKey
	rtGroupKey := pq.rtGroupKey
	mldsaPubKey := pq.mldsaPubKey
	cp := pq.certPolicy
	cpSet := pq.certPolicySet
	pq.mu.RUnlock()

	cert := &quasar.QuasarCert{}
	if err := cert.UnmarshalBinary(signature); err != nil {
		return fmt.Errorf("pq: decode QuasarCert: %w", err)
	}

	var mldsaKeys []*mldsa.PublicKey
	if mldsaPubKey != nil {
		mldsaKeys = []*mldsa.PublicKey{mldsaPubKey}
	}

	// Policy-driven path: the chain's CertPolicy decides the mandatory
	// leg set; a strict-PQ policy makes BLS optional and the PQ legs
	// required (and is verified WITHOUT a BLS key for a pure-PQ cert).
	if cpSet {
		keys := quasar.CertKeys{
			BLS:    blsAggKey,
			Corona: rtGroupKey,
			MLDSA:  mldsaKeys,
		}
		if !cert.VerifyUnderPolicy(message, cp, keys) {
			return errors.New("pq: QuasarCert verification failed (policy)")
		}
		return nil
	}

	// Implied-policy path (no chain CertPolicy bound): the supplied key
	// set declares the required legs. BLS is required here.
	if blsAggKey == nil {
		return errors.New("pq: no BLS aggregate verify key attached")
	}
	if !cert.VerifyWithRealKeys(message, blsAggKey, rtGroupKey, mldsaKeys) {
		return errors.New("pq: QuasarCert verification failed")
	}
	return nil
}

// GenerateQuantumProof generates a serialized QuasarCert for the given block.
// Requires AttachSigner to have been called with a fully-configured signer.
func (pq *PostQuantum) GenerateQuantumProof(ctx context.Context, blockID ids.ID) ([]byte, error) {
	pq.mu.RLock()
	s := pq.signer
	pq.mu.RUnlock()
	if s == nil {
		return nil, errors.New("pq: no signer attached")
	}

	// Pick any configured validator.
	validatorID, err := pickValidator(s)
	if err != nil {
		return nil, err
	}

	// Build canonical message: SHA-256(blockID || "lux-pq-engine-v1").
	h := sha256.New()
	h.Write(blockID[:])
	h.Write([]byte("lux-pq-engine-v1"))
	msg := h.Sum(nil)

	prfKeyArr := sha256.Sum256(blockID[:])
	prfKey := prfKeyArr[:]
	sessionID := int(blockID[0])<<24 | int(blockID[1])<<16 | int(blockID[2])<<8 | int(blockID[3])

	sig, _, err := s.TripleSignRound1(ctx, validatorID, msg, sessionID, prfKey)
	if err != nil {
		// Fall back to legacy single-key path for signers without BLS
		// threshold configuration.
		legacy, lerr := s.SignMessageWithContext(ctx, validatorID, msg)
		if lerr != nil {
			return nil, fmt.Errorf("pq: sign block: %w", err)
		}
		sig = legacy
	}

	cert := &quasar.QuasarCert{
		BLS: append([]byte(nil), sig.BLS...),
	}
	if len(sig.MLDSA) > 0 {
		cert.MLDSARollup = quasar.EncodeMLDSASigs([][]byte{sig.MLDSA})
	}
	return cert.MarshalBinary()
}

// pickValidator returns any validator ID configured on the signer.
func pickValidator(s *quasar.Signer) (string, error) {
	// Use exported helpers: AddValidator stores into validators map but
	// there is no public iterator, so use any configured BLS key.
	// For test-driver paths the caller knows which validator they added;
	// we expose a helper here that callers can override by attaching
	// a signer that already has at least one validator.
	if s == nil {
		return "", errors.New("pq: nil signer")
	}
	id := s.AnyValidatorID()
	if id == "" {
		return "", errors.New("pq: signer has no configured validators")
	}
	return id, nil
}
