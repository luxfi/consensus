// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build !cgo
// +build !cgo

package core

// cgoAvailable returns false when CGO is disabled
func cgoAvailable() bool {
	return false
}