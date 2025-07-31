// Copyright (C) 2020-2025, Lux Industries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package p2p

// EngineType represents the type of consensus engine
type EngineType int32

const (
	EngineType_ENGINE_TYPE_UNSPECIFIED EngineType = 0
	EngineType_ENGINE_TYPE_BEAM        EngineType = 1
	EngineType_ENGINE_TYPE_SNOWMAN     EngineType = 2
)

// String returns the string representation of the engine type
func (e EngineType) String() string {
	switch e {
	case EngineType_ENGINE_TYPE_BEAM:
		return "beam"
	case EngineType_ENGINE_TYPE_SNOWMAN:
		return "nova"
	default:
		return "unspecified"
	}
}