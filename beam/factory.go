// Copyright (C) 2025, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

// Factory returns new instances of Consensus
type Factory interface {
	New() Consensus
}
