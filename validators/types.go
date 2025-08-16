package validators

import (
	"github.com/luxfi/crypto/bls"
	"github.com/luxfi/ids"
)

// GetValidatorOutput contains validator information
type GetValidatorOutput struct {
	NodeID    ids.NodeID
	PublicKey *bls.PublicKey
	Weight    uint64
}
