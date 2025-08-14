// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package binding implements cryptographic binding of multiple signatures
package binding

import "crypto/sha256"

func Merkle3(msgRoot, blsAgg, pqBatch []byte) []byte {
	h := sha256.New()
	h.Write([]byte{0})
	h.Write(msgRoot)
	l0 := h.Sum(nil)

	h.Reset()
	h.Write([]byte{1})
	h.Write(blsAgg)
	l1 := h.Sum(nil)
	
	h.Reset()
	h.Write([]byte{2})
	h.Write(pqBatch)
	l2 := h.Sum(nil)

	h.Reset()
	h.Write(l0)
	h.Write(l1)
	h.Write(l2)
	return h.Sum(nil)
}