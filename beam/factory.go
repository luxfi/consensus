// Copyright (C) 2019-2024, Lux Indutries, Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package beam

// Factory returns new instances of Consensus
type Factory interface {
	New() Consensus
}
