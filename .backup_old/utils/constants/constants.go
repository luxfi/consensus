// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package constants

import (
	"github.com/luxfi/ids"
)

// Network IDs
const (
	MainnetID uint32 = 1
	TestnetID uint32 = 1919
	LocalID   uint32 = 12345
)

// NetworkIDToNetworkName maps network IDs to network names
var NetworkIDToNetworkName = map[uint32]string{
	MainnetID: "mainnet",
	TestnetID: "testnet",
	LocalID:   "local",
}

// Chain IDs
var (
	PlatformChainID = ids.Empty
	XChainID        = ids.Empty
	CChainID        = ids.Empty
)

// NetworkName returns the name of the network for the given ID
func NetworkName(networkID uint32) string {
	if name, ok := NetworkIDToNetworkName[networkID]; ok {
		return name
	}
	return "unknown"
}
