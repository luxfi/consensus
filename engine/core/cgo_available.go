// Copyright (C) 2019-2024, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo
// +build cgo

package core

// cgoAvailable returns true when CGO is enabled
func cgoAvailable() bool {
	return true
}