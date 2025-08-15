package validators

import (
	"github.com/luxfi/ids"
	"github.com/luxfi/crypto/bls"
)

// GetValidatorOutput contains validator information
type GetValidatorOutput struct {
	NodeID    ids.NodeID
	PublicKey *bls.PublicKey
	Weight    uint64
}