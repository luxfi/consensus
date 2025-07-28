// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package grpcutils

import (
	"net"
)

// ServerCloser represents a server that can be closed
type ServerCloser interface {
	Stop()
	GracefulStop()
}

// Dial creates a client connection to the given target
func Dial(target string) (net.Conn, error) {
	return net.Dial("tcp", target)
}

// NewListener creates a new network listener
func NewListener() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}