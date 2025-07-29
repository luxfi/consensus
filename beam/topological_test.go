// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

import (
	"testing"

	"github.com/luxfi/consensus/photon"
)

func TestTopological(t *testing.T) {
	runConsensusTests(t, TopologicalFactory{factory: photon.PhotonFactory})
}

func TestTopologicalWithPhoton(t *testing.T) {
	runConsensusTests(t, TopologicalFactory{factory: photon.PhotonFactory})
}