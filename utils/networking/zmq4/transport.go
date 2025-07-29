// Copyright (C) 2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package zmq4

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/luxfi/zmq4"
)

// Transport provides high-performance message passing for consensus testing
type Transport struct {
	nodeID  string
	ctx     context.Context
	cancel  context.CancelFunc
	pub     zmq4.Socket
	sub     zmq4.Socket
	router  zmq4.Socket
	dealer  map[string]zmq4.Socket
	basePort int
	
	mu       sync.RWMutex
	handlers map[string]MessageHandler
	peers    []string
	
	stopCh chan struct{}
	wg     sync.WaitGroup
}

// MessageHandler processes incoming messages
type MessageHandler func(msg *Message)

// Message represents a consensus message
type Message struct {
	Type      string          `json:"type"`
	From      string          `json:"from"`
	To        string          `json:"to,omitempty"`
	Height    uint64          `json:"height,omitempty"`
	Round     uint32          `json:"round,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Timestamp int64           `json:"timestamp"`
}

// NewTransport creates a new ZMQ4 transport
func NewTransport(ctx context.Context, nodeID string, basePort int) *Transport {
	tCtx, cancel := context.WithCancel(ctx)
	return &Transport{
		nodeID:   nodeID,
		ctx:      tCtx,
		cancel:   cancel,
		basePort: basePort,
		handlers: make(map[string]MessageHandler),
		dealer:   make(map[string]zmq4.Socket),
		stopCh:   make(chan struct{}),
	}
}

// Start initializes the transport
func (t *Transport) Start() error {
	// PUB socket for broadcasting
	t.pub = zmq4.NewPub(t.ctx)
	if err := t.pub.Listen(fmt.Sprintf("tcp://127.0.0.1:%d", t.basePort)); err != nil {
		return fmt.Errorf("failed to bind pub socket: %w", err)
	}
	
	// SUB socket for receiving broadcasts
	t.sub = zmq4.NewSub(t.ctx)
	t.sub.SetOption(zmq4.OptionSubscribe, "")
	
	// ROUTER socket for direct messages
	t.router = zmq4.NewRouter(t.ctx)
	if err := t.router.Listen(fmt.Sprintf("tcp://127.0.0.1:%d", t.basePort+1000)); err != nil {
		return fmt.Errorf("failed to bind router socket: %w", err)
	}
	
	t.wg.Add(2)
	go t.subLoop()
	go t.routerLoop()
	
	return nil
}

// Stop shuts down the transport
func (t *Transport) Stop() {
	close(t.stopCh)
	t.cancel()
	t.wg.Wait()
	
	if t.pub != nil {
		t.pub.Close()
	}
	if t.sub != nil {
		t.sub.Close()
	}
	if t.router != nil {
		t.router.Close()
	}
	
	t.mu.Lock()
	for _, dealer := range t.dealer {
		dealer.Close()
	}
	t.mu.Unlock()
}

// ConnectPeer adds a peer connection
func (t *Transport) ConnectPeer(peerID string, port int) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	// Subscribe to peer's broadcasts
	addr := fmt.Sprintf("tcp://127.0.0.1:%d", port)
	if err := t.sub.Dial(addr); err != nil {
		return fmt.Errorf("failed to connect sub to %s: %w", peerID, err)
	}
	
	// Create dealer for direct messages
	dealer := zmq4.NewDealer(t.ctx, zmq4.WithID(zmq4.SocketIdentity(t.nodeID)))
	
	routerAddr := fmt.Sprintf("tcp://127.0.0.1:%d", port+1000)
	if err := dealer.Dial(routerAddr); err != nil {
		return fmt.Errorf("failed to connect dealer to %s: %w", peerID, err)
	}
	
	t.dealer[peerID] = dealer
	t.peers = append(t.peers, peerID)
	
	return nil
}

// Broadcast sends a message to all connected peers
func (t *Transport) Broadcast(msg *Message) error {
	msg.From = t.nodeID
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	return t.pub.Send(zmq4.NewMsg(data))
}

// Send sends a direct message to a specific peer
func (t *Transport) Send(peerID string, msg *Message) error {
	t.mu.RLock()
	dealer, ok := t.dealer[peerID]
	t.mu.RUnlock()
	
	if !ok {
		return fmt.Errorf("no connection to peer %s", peerID)
	}
	
	msg.From = t.nodeID
	msg.To = peerID
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}
	
	return dealer.Send(zmq4.NewMsg(data))
}

// RegisterHandler registers a message handler for a specific type
func (t *Transport) RegisterHandler(msgType string, handler MessageHandler) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.handlers[msgType] = handler
}

// GetPeers returns the list of connected peers
func (t *Transport) GetPeers() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	peers := make([]string, len(t.peers))
	copy(peers, t.peers)
	return peers
}

func (t *Transport) subLoop() {
	defer t.wg.Done()
	
	for {
		select {
		case <-t.stopCh:
			return
		default:
			msg, err := t.sub.Recv()
			if err != nil {
				continue
			}
			
			var message Message
			if err := json.Unmarshal(msg.Bytes(), &message); err != nil {
				continue
			}
			
			// Skip our own broadcasts
			if message.From == t.nodeID {
				continue
			}
			
			t.handleMessage(&message)
		}
	}
}

func (t *Transport) routerLoop() {
	defer t.wg.Done()
	
	for {
		select {
		case <-t.stopCh:
			return
		default:
			msg, err := t.router.Recv()
			if err != nil {
				continue
			}
			
			// Router frames: [identity, empty, data]
			if len(msg.Frames) >= 3 {
				var message Message
				if err := json.Unmarshal(msg.Frames[2], &message); err != nil {
					continue
				}
				
				t.handleMessage(&message)
			}
		}
	}
}

func (t *Transport) handleMessage(msg *Message) {
	t.mu.RLock()
	handler, ok := t.handlers[msg.Type]
	t.mu.RUnlock()
	
	if ok {
		handler(msg)
	}
}