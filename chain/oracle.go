package chain

import (
    "context"
)

// OracleBlock provides oracle functionality for blocks
type OracleBlock interface {
    Block
    Options(context.Context) ([2]Block, error)
}