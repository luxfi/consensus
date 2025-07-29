// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"sync"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/require"

	"github.com/luxfi/consensus"
	"github.com/luxfi/crypto/bls/signer/localsigner"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/api/metrics"
	"github.com/luxfi/consensus/chains/atomic"
	"github.com/luxfi/consensus/validators"
	"github.com/luxfi/consensus/validators/validatorstest"
	"github.com/luxfi/log"
	"github.com/luxfi/consensus/vms/platformvm/warp/warptest"
)

// beamNoOpAcceptor implements consensus.Acceptor for tests
type beamNoOpAcceptor struct{}

func (beamNoOpAcceptor) Accept(*consensus.Context, ids.ID, []byte) error {
	return nil
}

var _ consensus.Acceptor = beamNoOpAcceptor{}

// BeamContext returns a consensus.Context for beam tests
func BeamContext(tb testing.TB, chainID ids.ID) *consensus.Context {
	require := require.New(tb)

	secretKey, err := localsigner.New()
	require.NoError(err)
	publicKey := secretKey.PublicKey()

	sharedMemory := atomic.NewMemory()

	aliaser := ids.NewAliaser()
	require.NoError(aliaser.Alias(PChainID, "P"))
	require.NoError(aliaser.Alias(PChainID, PChainID.String()))
	require.NoError(aliaser.Alias(XChainID, "X"))
	require.NoError(aliaser.Alias(XChainID, XChainID.String()))
	require.NoError(aliaser.Alias(CChainID, "C"))
	require.NoError(aliaser.Alias(CChainID, CChainID.String()))

	validatorState := &validatorstest.State{
		GetMinimumHeightF: func(context.Context) (uint64, error) {
			return 0, nil
		},
		GetCurrentHeightF: func(context.Context) (uint64, error) {
			return 0, nil
		},
		GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
			return nil, nil
		},
		GetCurrentValidatorSetF: func(context.Context, ids.ID) (map[ids.ID]*validators.GetCurrentValidatorOutput, uint64, error) {
			return nil, 0, nil
		},
		GetSubnetIDF: func(_ context.Context, chainID ids.ID) (ids.ID, error) {
			switch chainID {
			case PChainID, XChainID, CChainID:
				return ids.Empty, nil
			default:
				return chainID, nil
			}
		},
	}

	warpSigner := warptest.NewSigner(secretKey, chainID)

	return &consensus.Context{
		NetworkID:       10, // UnitTestID
		SubnetID:        chainID,
		ChainID:         chainID,
		NodeID:          ids.GenerateTestNodeID(),
		PublicKey:       publicKey,
		XChainID:        XChainID,
		CChainID:        CChainID,
		LUXAssetID:      LUXAssetID,
		Log:             log.NewNoOpLogger(),
		Lock:            sync.RWMutex{},
		SharedMemory:    sharedMemory,
		BCLookup:        aliaser,
		Metrics:         metrics.NewMultiGatherer(),
		WarpSigner:      warpSigner,
		ValidatorState:  validatorState,
		ChainDataDir:    "",
		Registerer:      prometheus.NewRegistry(),
		BlockAcceptor:   beamNoOpAcceptor{},
	}
}

// BeamConsensusContext returns a consensus.Context configured for consensus tests
func BeamConsensusContext(ctx *consensus.Context) *consensus.ConsensusContext {
	return &consensus.ConsensusContext{
		Context:         ctx,
		PrimaryAlias:    ctx.ChainID.String(),
		Registerer:      prometheus.NewRegistry(),
		BlockAcceptor:   beamNoOpAcceptor{},
		TxAcceptor:      beamNoOpAcceptor{},
		VertexAcceptor:  beamNoOpAcceptor{},
	}
}