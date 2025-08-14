// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

// Package quasar provides finality service API
package quasar

type FinalityBundle struct {
	Epoch   uint64
	Root    []byte
	BLSAgg  []byte
	PQBatch []byte
	Binding []byte
}

type Client interface {
	SubmitCheckpoint(epoch uint64, root []byte, attest []byte) error
	FetchBundle(epoch uint64) (FinalityBundle, error)
	Verify(FinalityBundle) bool
}