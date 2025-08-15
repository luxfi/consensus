package validators

import (
	"context"
	"github.com/luxfi/ids"
)

// ValidatorState provides validator state operations
type ValidatorState interface {
	GetSubnetID(ctx context.Context, chainID ids.ID) (ids.ID, error)
	GetValidatorSet(height uint64, subnetID ids.ID) (map[ids.NodeID]uint64, error)
}
