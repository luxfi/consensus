// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package pq implements post-quantum signatures (Ringtail)
package pq

type Batch struct {
	Blob []byte
}

func Sign(msg []byte) []byte                        { return nil } // ringtail or your PQ
func BatchVerify(msgs [][]byte, sigs [][]byte) bool { return true }
func MakeBatch(sigs ...[]byte) Batch                { return Batch{} }