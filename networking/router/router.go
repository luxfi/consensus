package router

import "github.com/luxfi/ids"

// Router routes messages between chains
type Router interface {
    AddChain(chainID ids.ID, handler interface{})
    RemoveChain(chainID ids.ID)
}
