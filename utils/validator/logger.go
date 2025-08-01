// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package validator

import (
	"encoding/hex"

	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
	log "github.com/luxfi/log"
	"github.com/luxfi/consensus/set"
)

var _ SetCallbackListener = (*logger)(nil)

type logger struct {
	log      log.Logger
	subnetID ids.ID
	nodeIDs  set.Set[ids.NodeID]
}

// NewLogger returns a callback listener that will log validator set changes for
// the specified validators
func NewLogger(
	log log.Logger,
	subnetID ids.ID,
	nodeIDs ...ids.NodeID,
) SetCallbackListener {
	nodeIDSet := set.Of(nodeIDs...)
	return &logger{
		log:      log,
		subnetID: subnetID,
		nodeIDs:  nodeIDSet,
	}
}

func (l *logger) OnValidatorAdded(
	nodeID ids.NodeID,
	pk *bls.PublicKey,
	txID ids.ID,
	weight uint64,
) {
	if l.nodeIDs.Contains(nodeID) {
		var pkBytes []byte
		if pk != nil {
			pkBytes = bls.PublicKeyToCompressedBytes(pk)
		}
		l.log.Info("node added to validator set",
			"subnetID", l.subnetID,
			"nodeID", nodeID,
			"publicKey", hex.EncodeToString(pkBytes),
			"txID", txID,
			"weight", weight,
		)
	}
}

func (l *logger) OnValidatorRemoved(
	nodeID ids.NodeID,
	weight uint64,
) {
	if l.nodeIDs.Contains(nodeID) {
		l.log.Info("node removed from validator set",
			"subnetID", l.subnetID,
			"nodeID", nodeID,
			"weight", weight,
		)
	}
}

func (l *logger) OnValidatorWeightChanged(
	nodeID ids.NodeID,
	oldWeight uint64,
	newWeight uint64,
) {
	if l.nodeIDs.Contains(nodeID) {
		l.log.Info("validator weight changed",
			"subnetID", l.subnetID,
			"nodeID", nodeID,
			"previousWeight", oldWeight,
			"newWeight", newWeight,
		)
	}
}
