// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package horizon provides DAG ordering tools and geometry operations
package horizon

type BlockMeta struct {
	Author  string
	Round   uint64
	Parents [][]byte
}

type CertPattern struct { /* omitted */
}

func HasCertificatePattern(b BlockMeta, support map[uint64][]BlockMeta) bool { return false }
func HasSkipPattern(slot uint64, support map[uint64][]BlockMeta) bool        { return false }
