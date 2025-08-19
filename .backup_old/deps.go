// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package consensus

import (
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// Heavy dependencies that should NOT be stored in context
// Use the luxfi/log and luxfi/metric packages directly instead

// GetValidatorOutput contains validator information
type GetValidatorOutput struct {
	NodeID    ids.NodeID
	PublicKey *bls.PublicKey
	Weight    uint64
}
