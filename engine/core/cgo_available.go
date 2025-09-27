// Copyright (C) 2019-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build cgo && !zmq
// +build cgo,!zmq

package core

// cgoAvailable returns true when CGO is enabled (but ZMQ is not required)
func cgoAvailable() bool {
	return true
}
