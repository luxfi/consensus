// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build zmq
// +build zmq

package zmq

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/go-zeromq/zmq4"
	"github.com/luxfi/consensus/utils/transport"
	"github.com/luxfi/ids"
)

// Transport implements high-performance ZeroMQ transport for consensus
type Transport struct {
	nodeID    ids.NodeID
	endpoint  string
	pub       zmq.Socket
	sub       zmq.Socket
	router    zmq.Socket
	dealers   map[ids.NodeID]zmq.Socket
	handlers  map[transport.MessageType]transport.Handler
	mu        sync.RWMutex
	ctx       context.Context
	cancel    context.CancelFunc
	wg        sync.WaitGroup
}

// NewTransport creates a new ZeroMQ transport
func NewTransport(nodeID ids.NodeID, port int) (*Transport, error) {
	ctx, cancel := context.WithCancel(context.Background())
	endpoint := fmt.Sprintf("tcp://127.0.0.1:%d", port)
	
	// Create PUB socket for broadcasting
	pub := zmq.NewPub(ctx)
	if err := pub.Listen(endpoint); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to bind pub socket: %w", err)
	}
	
	// Create SUB socket for receiving broadcasts
	sub := zmq.NewSub(ctx)
	
	// Create ROUTER socket for direct messaging
	routerEndpoint := fmt.Sprintf("tcp://127.0.0.1:%d", port+1000)
	router := zmq.NewRouter(ctx)
	if err := router.Listen(routerEndpoint); err != nil {
		pub.Close()
		cancel()
		return nil, fmt.Errorf("failed to bind router socket: %w", err)
	}
	
	t := &Transport{
		nodeID:   nodeID,
		endpoint: endpoint,
		pub:      pub,
		sub:      sub,
		router:   router,
		dealers:  make(map[ids.NodeID]zmq.Socket),
		handlers: make(map[transport.MessageType]transport.Handler),
		ctx:      ctx,
		cancel:   cancel,
	}
	
	// Subscribe to all messages
	if err := sub.SetOption(zmq4.OptionSubscribe, ""); err != nil {
		pub.Close()
		sub.Close()
		router.Close()
		cancel()
		return nil, fmt.Errorf("failed to set subscribe option: %w", err)
	}
	
	return t, nil
}

// NodeID returns the node ID
func (t *Transport) NodeID() ids.NodeID {
	return t.nodeID
}

// Connect connects to a peer
func (t *Transport) Connect(peerID ids.NodeID, endpoint string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	// Connect SUB socket to peer's PUB
	if err := t.sub.Dial(endpoint); err != nil {
		return fmt.Errorf("failed to connect sub socket: %w", err)
	}
	
	// Create DEALER socket for direct messages
	dealer := zmq.NewDealer(t.ctx)
	dealerEndpoint := fmt.Sprintf("tcp://%s:%d", getHost(endpoint), getPort(endpoint)+1000)
	if err := dealer.Dial(dealerEndpoint); err != nil {
		return fmt.Errorf("failed to connect dealer socket: %w", err)
	}
	
	t.dealers[peerID] = dealer
	return nil
}

// Broadcast sends a message to all connected peers
func (t *Transport) Broadcast(msg *transport.Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	
	zmqMsg := zmq.NewMsgFrom(data)
	return t.pub.Send(zmqMsg)
}

// Send sends a message to a specific peer
func (t *Transport) Send(peerID ids.NodeID, msg *transport.Message) error {
	t.mu.RLock()
	dealer, ok := t.dealers[peerID]
	t.mu.RUnlock()
	
	if !ok {
		return fmt.Errorf("no connection to peer %s", peerID)
	}
	
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}
	
	zmqMsg := zmq.NewMsgFrom(data)
	return dealer.Send(zmqMsg)
}

// RegisterHandler registers a message handler
func (t *Transport) RegisterHandler(msgType transport.MessageType, handler transport.Handler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handlers[msgType] = handler
}

// Start starts the transport
func (t *Transport) Start() error {
	// Start broadcast receiver
	t.wg.Add(1)
	go t.receiveBroadcasts()
	
	// Start direct message receiver
	t.wg.Add(1)
	go t.receiveDirectMessages()
	
	return nil
}

// Stop stops the transport
func (t *Transport) Stop() error {
	t.cancel()
	t.wg.Wait()
	
	// Close all sockets
	t.pub.Close()
	t.sub.Close()
	t.router.Close()
	
	t.mu.Lock()
	for _, dealer := range t.dealers {
		dealer.Close()
	}
	t.mu.Unlock()
	
	return nil
}

// receiveBroadcasts handles incoming broadcast messages
func (t *Transport) receiveBroadcasts() {
	defer t.wg.Done()
	
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			msg, err := t.sub.Recv()
			if err != nil {
				if t.ctx.Err() != nil {
					return
				}
				// Small sleep to avoid busy loop
				time.Sleep(10 * time.Millisecond)
				continue
			}
			
			if len(msg.Frames) > 0 {
				t.handleMessage(msg.Frames[0])
			}
		}
	}
}

// receiveDirectMessages handles incoming direct messages
func (t *Transport) receiveDirectMessages() {
	defer t.wg.Done()
	
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
			msg, err := t.router.Recv()
			if err != nil {
				if t.ctx.Err() != nil {
					return
				}
				// Small sleep to avoid busy loop
				time.Sleep(10 * time.Millisecond)
				continue
			}
			
			// Router receives identity frame first, then message
			if len(msg.Frames) >= 2 {
				t.handleMessage(msg.Frames[1])
			}
		}
	}
}

// handleMessage processes an incoming message
func (t *Transport) handleMessage(data []byte) {
	var msg transport.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return
	}
	
	// Skip messages from self
	if msg.From == t.nodeID {
		return
	}
	
	t.mu.RLock()
	handler, ok := t.handlers[msg.Type]
	t.mu.RUnlock()
	
	if ok {
		handler(msg.From, &msg)
	}
}

// Helper functions
func getHost(endpoint string) string {
	// Extract host from tcp://host:port
	return "127.0.0.1" // Simplified for local testing
}

func getPort(endpoint string) int {
	// Extract port from tcp://host:port
	var port int
	fmt.Sscanf(endpoint, "tcp://127.0.0.1:%d", &port)
	return port
}