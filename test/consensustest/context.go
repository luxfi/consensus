// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensustest

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/luxfi/crypto/bls/signer/localsigner"
	"github.com/luxfi/ids"
	"github.com/luxfi/consensus/chains/atomic"
	"github.com/luxfi/consensus/core"
	"github.com/luxfi/consensus/api/metrics"
	"github.com/luxfi/consensus/utils/validators"
	"github.com/luxfi/consensus/utils/validators/validatorstest"
	log "github.com/luxfi/log"
)

var (
	PChainID   = ids.GenerateTestID()
	XChainID   = ids.GenerateTestID()
	CChainID   = ids.GenerateTestID()
	LUXAssetID = ids.GenerateTestID()

	errMissing = errors.New("missing")

	_ core.Acceptor = noOpAcceptor{}
)

type noOpAcceptor struct{}

func (noOpAcceptor) Accept(*core.Context, ids.ID, []byte) error {
	return nil
}

func ConsensusContext(ctx *core.Context) *core.Context {
	ctx.PrimaryAlias = ctx.ChainID.String()
	ctx.Metrics = metrics.NewMultiGatherer()
	ctx.BlockAcceptor = noOpAcceptor{}
	ctx.TxAcceptor = noOpAcceptor{}
	ctx.VertexAcceptor = noOpAcceptor{}
	return ctx
}

func Context(tb testing.TB, chainID ids.ID) *core.Context {
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
		GetSubnetIDF: func(_ context.Context, chainID ids.ID) (ids.ID, error) {
			switch chainID {
			case PChainID, XChainID, CChainID:
				return ids.Empty, nil
			default:
				return ids.Empty, errMissing
			}
		},
		GetValidatorSetF: func(context.Context, uint64, ids.ID) (map[ids.NodeID]*validators.GetValidatorOutput, error) {
			return map[ids.NodeID]*validators.GetValidatorOutput{}, nil
		},
	}

	return &core.Context{
		NetworkID:       10, // UnitTestID
		SubnetID:        ids.Empty,
		ChainID:         chainID,
		NodeID:          ids.GenerateTestNodeID(),
		PublicKey:       publicKey,

		XChainID:   XChainID,
		CChainID:   CChainID,
		LUXAssetID: LUXAssetID,

		Log:          log.NewNoOpLogger(),
		SharedMemory: sharedMemory,
		BCLookup:     aliaser,
		Metrics:      metrics.NewMultiGatherer(),

		ValidatorState: validatorState,
		ChainDataDir:   tb.TempDir(),
	}
}
