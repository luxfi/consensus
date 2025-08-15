// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package photon defines the atomic unit of consensus
package photon

import (
	"github.com/luxfi/consensus/types"
)

type SafetyTag struct {
	Epoch uint64
	Round uint64
	Conf  uint32 // local confidence snapshot
}

type Photon[ID comparable] struct {
	Item    ID
	Prefer  bool             // binary preference for FPC; or use enum
	Tag     SafetyTag
	Author  types.NodeID
	MsgRoot types.Digest     // commitment of payload
	BlsAgg  []byte           // optional single-signer or partial agg
	PQSig   []byte           // ringtail / pq signature
}