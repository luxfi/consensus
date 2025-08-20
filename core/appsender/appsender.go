package appsender

import (
    "context"
    "github.com/luxfi/ids"
    "github.com/luxfi/consensus/utils/set"
)

// AppSender sends application-level messages
type AppSender interface {
    // SendAppRequest sends an application-level request to the given nodes
    SendAppRequest(ctx context.Context, nodeIDs set.Set[ids.NodeID], requestID uint32, request []byte) error
    
    // SendAppResponse sends an application-level response to the given node
    SendAppResponse(ctx context.Context, nodeID ids.NodeID, requestID uint32, response []byte) error
    
    // SendAppGossip sends an application-level gossip message
    SendAppGossip(ctx context.Context, nodeIDs set.Set[ids.NodeID], message []byte) error
    
    // SendCrossChainAppRequest sends a cross-chain app request
    SendCrossChainAppRequest(ctx context.Context, chainID ids.ID, requestID uint32, request []byte) error
    
    // SendCrossChainAppResponse sends a cross-chain app response  
    SendCrossChainAppResponse(ctx context.Context, chainID ids.ID, requestID uint32, response []byte) error
    
    // SendCrossChainAppError sends a cross-chain app error
    SendCrossChainAppError(ctx context.Context, chainID ids.ID, requestID uint32, errorCode int32, errorMessage string) error
}
