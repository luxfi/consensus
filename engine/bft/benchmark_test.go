package bft

import (
	"crypto/rand"
	"errors"
	"testing"
	"time"

	"github.com/luxfi/ids"
	"github.com/luxfi/node/utils/crypto/bls"
	"github.com/luxfi/node/utils/crypto/bls/signer/localsigner"
	"github.com/luxfi/node/utils/set"
)

// Benchmark constants
const (
	defaultValidatorCount = 100
	defaultMessageSize    = 1024
	defaultBlockSize      = 4096
)

// BenchmarkVoteAggregation benchmarks the aggregation of votes into a quorum certificate
func BenchmarkVoteAggregation(b *testing.B) {
	benchmarks := []struct {
		name       string
		validators int
	}{
		{"10_validators", 10},
		{"50_validators", 50},
		{"100_validators", 100},
		{"200_validators", 200},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			// Setup validators and keys
			validators, signer, verifier := setupValidators(b, bench.validators)
			message := generateRandomMessage(defaultMessageSize)

			// Generate individual signatures
			signatures := make([]*bls.Signature, len(validators))
			for i, nodeID := range validators {
				sig, err := signer[nodeID].Sign(message)
				if err != nil {
					b.Fatalf("failed to sign: %v", err)
				}
				blsSig, err := bls.SignatureFromBytes(sig)
				if err != nil {
					b.Fatalf("failed to parse signature: %v", err)
				}
				signatures[i] = blsSig
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Aggregate signatures
				aggregator := &SignatureAggregator{
					verifier: verifier,
					signers:  set.NewSet[ids.NodeID](len(validators)),
					pks:      make([]*bls.PublicKey, 0, len(validators)),
				}

				for _, nodeID := range validators {
					aggregator.signers.Add(nodeID)
					if pk, exists := verifier.nodeID2PK[nodeID]; exists {
						aggregator.pks = append(aggregator.pks, pk)
					}
				}

				// Create aggregated signature
				aggSig, err := bls.AggregateSignatures(signatures)
				if err != nil {
					b.Fatalf("aggregation failed: %v", err)
				}
				_ = aggSig
			}
		})
	}
}

// BenchmarkSignatureVerification benchmarks BLS signature verification
func BenchmarkSignatureVerification(b *testing.B) {
	benchmarks := []struct {
		name        string
		messageSize int
	}{
		{"256_bytes", 256},
		{"1KB", 1024},
		{"4KB", 4096},
		{"16KB", 16384},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			// Setup single validator
			validators, signer, verifier := setupValidators(b, 1)
			nodeID := validators[0]
			message := generateRandomMessage(bench.messageSize)

			// Sign message
			sig, err := signer[nodeID].Sign(message)
			if err != nil {
				b.Fatalf("failed to sign: %v", err)
			}

			blsSig, err := bls.SignatureFromBytes(sig)
			if err != nil {
				b.Fatalf("failed to parse signature: %v", err)
			}

			pk := verifier.nodeID2PK[nodeID]

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Verify signature
				valid := bls.Verify(pk, blsSig, message)
				if !valid {
					b.Fatal("signature verification failed")
				}
			}
		})
	}
}

// BenchmarkByzantineFaultDetection benchmarks detection of Byzantine behavior
func BenchmarkByzantineFaultDetection(b *testing.B) {
	benchmarks := []struct {
		name             string
		totalValidators  int
		byzantinePercent float64
	}{
		{"100_validators_10pct_byzantine", 100, 0.10},
		{"100_validators_20pct_byzantine", 100, 0.20},
		{"100_validators_30pct_byzantine", 100, 0.30},
		{"200_validators_20pct_byzantine", 200, 0.20},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			validators, signer, verifier := setupValidators(b, bench.totalValidators)
			byzantineCount := int(float64(bench.totalValidators) * bench.byzantinePercent)

			// Create valid and invalid messages
			validMessage := generateRandomMessage(defaultMessageSize)
			invalidMessage := generateRandomMessage(defaultMessageSize)

			// Generate mixed signatures (some valid, some Byzantine)
			signatures := make(map[ids.NodeID][]byte, bench.totalValidators)
			for i, nodeID := range validators {
				var msg []byte
				if i < byzantineCount {
					// Byzantine node signs wrong message
					msg = invalidMessage
				} else {
					// Honest node signs correct message
					msg = validMessage
				}

				sig, err := signer[nodeID].Sign(msg)
				if err != nil {
					b.Fatalf("failed to sign: %v", err)
				}
				signatures[nodeID] = sig
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Detect Byzantine nodes
				byzantineNodes := detectByzantineNodes(verifier, signatures, validMessage)
				if len(byzantineNodes) != byzantineCount {
					b.Fatalf("expected %d byzantine nodes, got %d", byzantineCount, len(byzantineNodes))
				}
			}
		})
	}
}

// BenchmarkConsensusRounds benchmarks full consensus rounds
func BenchmarkConsensusRounds(b *testing.B) {
	benchmarks := []struct {
		name       string
		validators int
		blockSize  int
	}{
		{"10_validators_1KB", 10, 1024},
		{"50_validators_4KB", 50, 4096},
		{"100_validators_4KB", 100, 4096},
		{"100_validators_16KB", 100, 16384},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			validators, signer, verifier := setupValidators(b, bench.validators)
			blockData := generateRandomMessage(bench.blockSize)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Simulate consensus round
				round := simulateConsensusRound(validators, signer, verifier, blockData)
				if !round.success {
					b.Fatal("consensus round failed")
				}
			}
		})
	}
}

// BenchmarkQuorumCertificateCreation benchmarks QC creation
func BenchmarkQuorumCertificateCreation(b *testing.B) {
	benchmarks := []struct {
		name       string
		validators int
	}{
		{"21_validators", 21},
		{"51_validators", 51},
		{"101_validators", 101},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			validators, signer, verifier := setupValidators(b, bench.validators)
			message := generateRandomMessage(defaultMessageSize)
			quorum := (bench.validators * 2 / 3) + 1 // Byzantine fault tolerance threshold

			// Collect signatures for quorum
			signatures := make([]*bls.Signature, 0, quorum)
			signers := make([]ids.NodeID, 0, quorum)

			for i := 0; i < quorum; i++ {
				nodeID := validators[i]
				sig, err := signer[nodeID].Sign(message)
				if err != nil {
					b.Fatalf("failed to sign: %v", err)
				}
				blsSig, err := bls.SignatureFromBytes(sig)
				if err != nil {
					b.Fatalf("failed to parse signature: %v", err)
				}
				signatures = append(signatures, blsSig)
				signers = append(signers, nodeID)
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Create quorum certificate
				aggSig, err := bls.AggregateSignatures(signatures)
				if err != nil {
					b.Fatalf("failed to aggregate signatures: %v", err)
				}

				qc := &QC{
					verifier: verifier,
					sig:      aggSig,
					signers:  signers,
				}

				// Verify QC
				err = qc.Verify(message)
				if err != nil {
					b.Fatalf("QC verification failed: %v", err)
				}
			}
		})
	}
}

// BenchmarkBlockValidation benchmarks block validation performance
func BenchmarkBlockValidation(b *testing.B) {
	benchmarks := []struct {
		name      string
		blockSize int
		txCount   int
	}{
		{"small_block_10tx", 1024, 10},
		{"medium_block_100tx", 4096, 100},
		{"large_block_500tx", 16384, 500},
		{"huge_block_1000tx", 32768, 1000},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			// Create mock block data
			blockData := generateMockBlock(bench.blockSize, bench.txCount)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				// Validate block structure
				valid := validateBlockStructure(blockData)
				if !valid {
					b.Fatal("block validation failed")
				}
			}
		})
	}
}

// Helper functions

func setupValidators(b *testing.B, count int) ([]ids.NodeID, map[ids.NodeID]*BLSSigner, *BLSVerifier) {
	validators := make([]ids.NodeID, count)
	signers := make(map[ids.NodeID]*BLSSigner, count)
	nodeID2PK := make(map[ids.NodeID]*bls.PublicKey, count)

	chainID := ids.GenerateTestID()
	networkID := uint32(1)

	for i := 0; i < count; i++ {
		// Generate BLS key pair using localsigner
		signer, err := localsigner.New()
		if err != nil {
			b.Fatalf("failed to generate signer: %v", err)
		}
		pk := signer.PublicKey()

		// Generate node ID
		nodeID := ids.GenerateTestNodeID()
		validators[i] = nodeID

		// Create signer wrapper
		signers[nodeID] = &BLSSigner{
			chainID:   chainID,
			networkID: networkID,
			signBLS:   signer.Sign,
		}

		nodeID2PK[nodeID] = pk
	}

	verifier := &BLSVerifier{
		nodeID2PK:              nodeID2PK,
		networkID:              networkID,
		chainID:                chainID,
		canonicalNodeIDs:       validators,
		canonicalNodeIDIndices: make(map[ids.NodeID]int, count),
	}

	for i, nodeID := range validators {
		verifier.canonicalNodeIDIndices[nodeID] = i
	}

	return validators, signers, verifier
}

func generateRandomMessage(size int) []byte {
	msg := make([]byte, size)
	_, _ = rand.Read(msg)
	return msg
}

func detectByzantineNodes(verifier *BLSVerifier, signatures map[ids.NodeID][]byte, correctMessage []byte) []ids.NodeID {
	byzantineNodes := make([]ids.NodeID, 0)

	for nodeID, sigBytes := range signatures {
		sig, err := bls.SignatureFromBytes(sigBytes)
		if err != nil {
			byzantineNodes = append(byzantineNodes, nodeID)
			continue
		}

		pk, exists := verifier.nodeID2PK[nodeID]
		if !exists {
			byzantineNodes = append(byzantineNodes, nodeID)
			continue
		}

		// Verify against correct message
		if !bls.Verify(pk, sig, correctMessage) {
			byzantineNodes = append(byzantineNodes, nodeID)
		}
	}

	return byzantineNodes
}

type consensusRound struct {
	success    bool
	duration   time.Duration
	signatures int
}

func simulateConsensusRound(validators []ids.NodeID, signers map[ids.NodeID]*BLSSigner, verifier *BLSVerifier, blockData []byte) consensusRound {
	start := time.Now()
	quorum := (len(validators) * 2 / 3) + 1

	// Collect votes
	votes := 0
	for i := 0; i < quorum && i < len(validators); i++ {
		nodeID := validators[i]
		sig, err := signers[nodeID].Sign(blockData)
		if err != nil {
			continue
		}

		// Verify vote
		blsSig, err := bls.SignatureFromBytes(sig)
		if err != nil {
			continue
		}

		pk := verifier.nodeID2PK[nodeID]
		if bls.Verify(pk, blsSig, blockData) {
			votes++
		}
	}

	return consensusRound{
		success:    votes >= quorum,
		duration:   time.Since(start),
		signatures: votes,
	}
}

func generateMockBlock(size int, txCount int) []byte {
	block := make([]byte, size)
	_, _ = rand.Read(block)

	// Add mock transaction count header
	if size > 4 {
		block[0] = byte(txCount >> 24)
		block[1] = byte(txCount >> 16)
		block[2] = byte(txCount >> 8)
		block[3] = byte(txCount)
	}

	return block
}

func validateBlockStructure(blockData []byte) bool {
	if len(blockData) < 4 {
		return false
	}

	// Extract transaction count
	txCount := int(blockData[0])<<24 | int(blockData[1])<<16 | int(blockData[2])<<8 | int(blockData[3])

	// Basic validation
	if txCount < 0 || txCount > 10000 {
		return false
	}

	// Validate block size is reasonable for tx count
	minSize := 4 + (txCount * 32) // Minimum 32 bytes per tx
	if len(blockData) < minSize {
		return false
	}

	return true
}

// BenchmarkSignatureAggregator specifically tests SignatureAggregator performance
func BenchmarkSignatureAggregator(b *testing.B) {
	benchmarks := []struct {
		name       string
		validators int
	}{
		{"small_10", 10},
		{"medium_50", 50},
		{"large_100", 100},
		{"xlarge_200", 200},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			validators, signer, verifier := setupValidators(b, bench.validators)
			message := generateRandomMessage(defaultMessageSize)

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				aggregator := &SignatureAggregator{
					verifier: verifier,
					signers:  set.NewSet[ids.NodeID](bench.validators),
					pks:      make([]*bls.PublicKey, 0, bench.validators),
				}

				// Add signatures
				for _, nodeID := range validators {
					sig, _ := signer[nodeID].Sign(message)
					_ = aggregator.Add(nodeID, sig)
				}

				// Finalize aggregation
				_, _ = aggregator.QuorumCertificate()
			}
		})
	}
}

// BenchmarkParallelSignatureVerification benchmarks parallel signature verification
func BenchmarkParallelSignatureVerification(b *testing.B) {
	benchmarks := []struct {
		name       string
		validators int
		parallel   int
	}{
		{"100_validators_1_thread", 100, 1},
		{"100_validators_4_threads", 100, 4},
		{"100_validators_8_threads", 100, 8},
		{"200_validators_8_threads", 200, 8},
	}

	for _, bench := range benchmarks {
		b.Run(bench.name, func(b *testing.B) {
			validators, signer, verifier := setupValidators(b, bench.validators)
			message := generateRandomMessage(defaultMessageSize)

			// Prepare signatures
			signatures := make(map[ids.NodeID][]byte, bench.validators)
			for _, nodeID := range validators {
				sig, _ := signer[nodeID].Sign(message)
				signatures[nodeID] = sig
			}

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				verifyParallel(verifier, signatures, message, bench.parallel)
			}
		})
	}
}

func verifyParallel(verifier *BLSVerifier, signatures map[ids.NodeID][]byte, message []byte, parallel int) int {
	type result struct {
		nodeID ids.NodeID
		valid  bool
	}

	ch := make(chan result, len(signatures))
	sem := make(chan struct{}, parallel)

	for nodeID, sigBytes := range signatures {
		sem <- struct{}{}
		go func(nID ids.NodeID, sb []byte) {
			defer func() { <-sem }()

			sig, err := bls.SignatureFromBytes(sb)
			if err != nil {
				ch <- result{nodeID: nID, valid: false}
				return
			}

			pk, exists := verifier.nodeID2PK[nID]
			if !exists {
				ch <- result{nodeID: nID, valid: false}
				return
			}

			valid := bls.Verify(pk, sig, message)
			ch <- result{nodeID: nID, valid: valid}
		}(nodeID, sigBytes)
	}

	validCount := 0
	for range signatures {
		r := <-ch
		if r.valid {
			validCount++
		}
	}

	return validCount
}

// Type definitions to satisfy compiler (these would normally come from the BFT package)

type SignatureAggregator struct {
	verifier *BLSVerifier
	signers  set.Set[ids.NodeID]
	pks      []*bls.PublicKey
}

func (sa *SignatureAggregator) Add(nodeID ids.NodeID, signature []byte) error {
	sa.signers.Add(nodeID)
	if pk, exists := sa.verifier.nodeID2PK[nodeID]; exists {
		sa.pks = append(sa.pks, pk)
	}
	return nil
}

func (sa *SignatureAggregator) QuorumCertificate() (*QC, error) {
	// Mock implementation
	return &QC{
		verifier: sa.verifier,
		signers:  sa.signers.List(),
	}, nil
}

type QC struct {
	verifier *BLSVerifier
	sig      *bls.Signature
	signers  []ids.NodeID
}

func (qc *QC) Verify(msg []byte) error {
	quorum := (len(qc.verifier.nodeID2PK) * 2 / 3) + 1
	if len(qc.signers) < quorum {
		return errUnexpectedSigners
	}
	return nil
}

type BLSSigner struct {
	chainID   ids.ID
	networkID uint32
	signBLS   func(msg []byte) (*bls.Signature, error)
}

func (s *BLSSigner) Sign(message []byte) ([]byte, error) {
	sig, err := s.signBLS(message)
	if err != nil {
		return nil, err
	}
	return bls.SignatureToBytes(sig), nil
}

type BLSVerifier struct {
	nodeID2PK              map[ids.NodeID]*bls.PublicKey
	networkID              uint32
	chainID                ids.ID
	canonicalNodeIDs       []ids.NodeID
	canonicalNodeIDIndices map[ids.NodeID]int
}

// Error definitions
var (
	errUnexpectedSigners = errors.New("unexpected number of signers")
)