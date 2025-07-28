// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package chain

import (
	"testing"

	"github.com/luxfi/consensus/poll"
)

func TestTopological(t *testing.T) {
	runConsensusTests(t, TopologicalFactory{factory: poll.DefaultFactory})
}
