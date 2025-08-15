// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package grpcutils

import (
	"context"
	"net"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ServerCloser is a wrapper for closing multiple gRPC servers
type ServerCloser struct {
	servers []*grpc.Server
}

// Add adds a server to be closed
func (s *ServerCloser) Add(server *grpc.Server) {
	s.servers = append(s.servers, server)
}

// Close stops all servers
func (s *ServerCloser) Close() {
	for _, srv := range s.servers {
		srv.GracefulStop()
	}
}

// NewListener creates a new TCP listener on a random port
func NewListener() (net.Listener, error) {
	return net.Listen("tcp", "127.0.0.1:0")
}

// NewServer creates a new gRPC server with default options
func NewServer() *grpc.Server {
	return grpc.NewServer()
}

// Serve starts serving on the given listener
func Serve(listener net.Listener, server *grpc.Server) error {
	return server.Serve(listener)
}

// Dial creates a new gRPC client connection
func Dial(addr string) (*grpc.ClientConn, error) {
	return grpc.Dial(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}

// DialContext creates a new gRPC client connection with context
func DialContext(ctx context.Context, addr string) (*grpc.ClientConn, error) {
	return grpc.DialContext(ctx, addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
}
