//go:build ignore

// Copyright (C) 2019-2025, Ava Labs, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package bft

import (
	"context"
	"errors"
	"testing"

	simplex "github.com/luxfi/bft"
	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus/engine/chain/block"
	"github.com/luxfi/consensus/engine/chain/chaintest"
	"github.com/luxfi/consensus/validator"
	"github.com/luxfi/database"
	"github.com/luxfi/database/memdb"
	"github.com/luxfi/ids"
	"github.com/luxfi/log"
	"github.com/luxfi/node/utils/constants"
	"github.com/luxfi/node/utils/crypto/bls"
	"github.com/luxfi/node/utils/crypto/bls/signer/localsigner"
)

type newBlockConfig struct {
	// If prev is nil, newBlock will create the genesis block
	prev *Block
	// If round is 0, it will be set to one higher than the prev's round
	round uint64
}

func newTestBlock(t *testing.T, config newBlockConfig) *Block {
	if config.prev == nil {
		block := &Block{
			vmBlock: &wrappedBlock{
				TestBlock: chaintest.Genesis,
				vm:        newTestVM(),
			},
			metadata: genesisMetadata,
		}
		bytes, err := block.Bytes()
		require.NoError(t, err)

		digest := computeDigest(bytes)
		block.digest = digest

		block.blockTracker = newBlockTracker(block)
		return block
	}
	if config.round == 0 {
		config.round = config.prev.metadata.Round + 1
	}

	vmBlock := chaintest.BuildChild(config.prev.vmBlock.(*wrappedBlock).TestBlock)
	block := &Block{
		vmBlock: &wrappedBlock{
			TestBlock: vmBlock,
			vm:        config.prev.vmBlock.(*wrappedBlock).vm,
		},
		blockTracker: config.prev.blockTracker,
		metadata: simplex.ProtocolMetadata{
			Version: 1,
			Epoch:   1,
			Round:   config.round,
			Seq:     vmBlock.Height(),
			Prev:    config.prev.digest,
		},
	}

	bytes, err := block.Bytes()
	require.NoError(t, err)

	digest := computeDigest(bytes)
	block.digest = digest
	return block
}

func newTestValidatorInfo(allNodes []*testNode) map[ids.NodeID]*validators.GetValidatorOutput {
	vds := make(map[ids.NodeID]*validators.GetValidatorOutput, len(allNodes))
	for _, node := range allNodes {
		vds[node.validator.NodeID] = &node.validator
	}

	return vds
}

func newEngineConfig(t *testing.T, numNodes uint64) *Config {
	return newNetworkConfigs(t, numNodes)[0]
}

type testNode struct {
	validator validators.GetValidatorOutput
	signFunc  SignFunc
}

// newNetworkConfigs creates a slice of Configs for testing purposes.
// they are initialized with a common chainID and a set of validators.
func newNetworkConfigs(t *testing.T, numNodes uint64) []*Config {
	require.Positive(t, numNodes)

	chainID := ids.GenerateTestID()

	testNodes := generateTestNodes(t, numNodes)

	configs := make([]*Config, 0, numNodes)
	for _, node := range testNodes {
		config := &Config{
			Ctx: SimplexChainContext{
				NodeID:    node.validator.NodeID,
				ChainID:   chainID,
				NetworkID: constants.UnitTestID,
			},
			Log:        log.NoLog{},
			Validators: newTestValidatorInfo(testNodes),
			SignBLS:    node.signFunc,
			DB:         memdb.New(),
		}
		configs = append(configs, config)
	}

	return configs
}

func generateTestNodes(t *testing.T, num uint64) []*testNode {
	nodes := make([]*testNode, num)
	for i := uint64(0); i < num; i++ {
		ls, err := localsigner.New()
		require.NoError(t, err)

		nodeID := ids.GenerateTestNodeID()
		pk := ls.PublicKey()
		pkBytes := bls.PublicKeyToCompressedBytes(pk)
		nodes[i] = &testNode{
			validator: validators.GetValidatorOutput{
				NodeID:    nodeID,
				PublicKey: pkBytes,
			},
			signFunc: ls.Sign,
		}
	}
	return nodes
}

// newTestFinalization creates a new finalization over the BlockHeader, by collecting a
// quorum of signatures from the provided configs.
func newTestFinalization(t *testing.T, configs []*Config, bh simplex.BlockHeader) simplex.Finalization {
	quorum := simplex.Quorum(len(configs))
	finalizedVotes := make([]*simplex.FinalizeVote, 0, quorum)

	for _, config := range configs[:quorum] {
		vote := simplex.ToBeSignedFinalization{
			BlockHeader: bh,
		}
		signer, _ := NewBLSAuth(config)
		sig, err := vote.Sign(&signer)
		require.NoError(t, err)
		finalizedVotes = append(finalizedVotes, &simplex.FinalizeVote{
			Finalization: vote,
			Signature: simplex.Signature{
				Signer: config.Ctx.NodeID[:],
				Value:  sig,
			},
		})
	}

	_, verifier := NewBLSAuth(configs[0])
	sigAgg := &SignatureAggregator{verifier: &verifier}

	finalization, err := simplex.NewFinalization(&loggerWrapper{configs[0].Log}, sigAgg, finalizedVotes)
	require.NoError(t, err)
	return finalization
}

func newTestVM() *wrappedVM {
	return &wrappedVM{
		VM: &chaintest.VM{},
		blocks: map[ids.ID]*chaintest.TestBlock{
			chaintest.Genesis.ID(): chaintest.Genesis,
		},
	}
}

// wrappedBlock wraps a test block in a VM so that on Accept, it is stored in the VM's block store.
type wrappedBlock struct {
	*chaintest.TestBlock
	vm      *wrappedVM
	VerifyV error // Mock error for Verify
}

type wrappedVM struct {
	*chaintest.VM
	blocks map[ids.ID]*chaintest.TestBlock

	// Mock functions for testing
	WaitForEventF func(context.Context) (interface{}, error)
	BuildBlockF   func(context.Context) (block.Block, error)
}

func (wb *wrappedBlock) Accept(ctx context.Context) error {
	if err := wb.TestBlock.Accept(ctx); err != nil {
		return err
	}

	wb.vm.blocks[wb.ID()] = wb.TestBlock
	return nil
}

func (wb *wrappedBlock) Verify(ctx context.Context) error {
	if wb.VerifyV != nil {
		return wb.VerifyV
	}
	return wb.TestBlock.Verify(ctx)
}

func (v *wrappedVM) GetBlockIDAtHeight(_ context.Context, height uint64) (ids.ID, error) {
	for _, block := range v.blocks {
		if block.Height() == height {
			return block.ID(), nil
		}
	}
	return ids.Empty, database.ErrNotFound
}

func (v *wrappedVM) GetBlock(_ context.Context, id ids.ID) (block.Block, error) {
	block, exists := v.blocks[id]
	if !exists {
		return nil, database.ErrNotFound
	}
	return block, nil
}

func (v *wrappedVM) LastAccepted(_ context.Context) (ids.ID, error) {
	// find the block with the highest height
	if len(v.blocks) == 0 {
		return ids.Empty, database.ErrNotFound
	}

	lastAccepted := chaintest.Genesis
	for _, block := range v.blocks {
		if block.Height() > lastAccepted.Height() {
			lastAccepted = block
		}
	}

	return lastAccepted.ID(), nil
}

func (v *wrappedVM) WaitForEvent(ctx context.Context) (interface{}, error) {
	if v.WaitForEventF != nil {
		return v.WaitForEventF(ctx)
	}
	return nil, errors.New("WaitForEvent not implemented")
}

func (v *wrappedVM) BuildBlock(ctx context.Context) (block.Block, error) {
	if v.BuildBlockF != nil {
		return v.BuildBlockF(ctx)
	}
	return nil, errors.New("BuildBlock not implemented")
}

func (v *wrappedVM) Initialize(
	ctx context.Context,
	chainCtx interface{},
	db interface{},
	genesisBytes []byte,
	upgradeBytes []byte,
	configBytes []byte,
	msgChan interface{},
	fxs []interface{},
	appSender interface{},
) error {
	return nil
}

func (v *wrappedVM) ParseBlock(ctx context.Context, bytes []byte) (block.Block, error) {
	if v.VM != nil && v.VM.ParseBlockF != nil {
		return v.VM.ParseBlockF(ctx, bytes)
	}
	return &wrappedBlock{
		TestBlock: &chaintest.TestBlock{BytesV: bytes},
		vm:        v,
	}, nil
}

func (v *wrappedVM) SetPreference(ctx context.Context, id ids.ID) error {
	return nil
}
