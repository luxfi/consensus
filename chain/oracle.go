package chain

import (
	"context"
	"errors"
)

// ErrNotOracle is returned when the block is not an oracle block
var ErrNotOracle = errors.New("block is not an oracle")

// OracleBlock provides oracle functionality for blocks
type OracleBlock interface {
	Block
	Options(context.Context) ([2]Block, error)
}
