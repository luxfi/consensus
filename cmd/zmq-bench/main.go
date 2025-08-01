// Copyright (C) 2020-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

//go:build zmq
// +build zmq

package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
	zmq4 "github.com/luxfi/zmq4"
)

var (
	nodes     = flag.Int("nodes", 10, "Number of consensus nodes")
	profile   = flag.String("profile", "local", "Consensus profile: local, testnet, mainnet")
	port      = flag.Int("port", 5550, "Starting port for ZMQ communication")
	rounds    = flag.Int("rounds", 100, "Number of consensus rounds")
	batchSize = flag.Int("batch", 1024, "Transactions per batch")
	interval  = flag.Duration("interval", 10*time.Millisecond, "Round interval")
	quiet     = flag.Bool("quiet", false, "Quiet mode - only show summary")
)

// SimpleTransport wraps ZMQ sockets for consensus communication
type SimpleTransport struct {
	identity string
	ctx      context.Context
	pub      zmq4.Socket
	sub      zmq4.Socket
	peers    []string
	mu       sync.RWMutex
}

// NewSimpleTransport creates a new transport
func NewSimpleTransport(identity string, port int) (*SimpleTransport, error) {
	ctx := context.Background()
	
	// Create publisher socket
	pub := zmq4.NewPub(ctx, zmq4.WithID(zmq4.SocketIdentity(identity)))
	if err := pub.Listen(fmt.Sprintf("tcp://127.0.0.1:%d", port)); err != nil {
		return nil, fmt.Errorf("failed to bind publisher: %w", err)
	}
	
	// Create subscriber socket
	sub := zmq4.NewSub(ctx)
	
	return &SimpleTransport{
		identity: identity,
		ctx:      ctx,
		pub:      pub,
		sub:      sub,
		peers:    make([]string, 0),
	}, nil
}

// Connect adds a peer
func (t *SimpleTransport) Connect(addr string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	
	if err := t.sub.Dial(addr); err != nil {
		return fmt.Errorf("failed to connect to %s: %w", addr, err)
	}
	
	// Subscribe to all messages
	if err := t.sub.SetOption(zmq4.OptionSubscribe, ""); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}
	
	t.peers = append(t.peers, addr)
	return nil
}

// Send broadcasts a message
func (t *SimpleTransport) Send(data []byte) error {
	msg := zmq4.NewMsg(data)
	return t.pub.Send(msg)
}

// Receive gets the next message
func (t *SimpleTransport) Receive() ([]byte, error) {
	msg, err := t.sub.Recv()
	if err != nil {
		return nil, err
	}
	return msg.Bytes(), nil
}

// Close shuts down the transport
func (t *SimpleTransport) Close() error {
	if err := t.pub.Close(); err != nil {
		return err
	}
	return t.sub.Close()
}

// Node represents a light consensus node using ZMQ
type Node struct {
	id        ids.NodeID
	index     int
	transport *SimpleTransport
	params    config.Parameters
	
	// Current state
	preference ids.ID
	confidence map[ids.ID]int
	finalized  bool
	
	// Metrics
	roundsCompleted atomic.Int64
	messagesRecv    atomic.Int64
	messagesSent    atomic.Int64
	
	mu sync.RWMutex
}

// NetworkBenchmark manages multiple consensus nodes
type NetworkBenchmark struct {
	nodes      []*Node
	params     config.Parameters
	startTime  time.Time
	
	// Global metrics
	totalRounds     atomic.Int64
	totalMessages   atomic.Int64
	totalFinalized  atomic.Int64
	consensusLatency []time.Duration
	
	mu sync.Mutex
}

func main() {
	flag.Parse()
	
	if !*quiet {
		log.Printf("🚀 Starting ZMQ consensus benchmark with %d nodes", *nodes)
	}
	
	// Get consensus parameters
	params := getConsensusParams(*profile)
	
	// Create benchmark
	bench := &NetworkBenchmark{
		params:           params,
		startTime:        time.Now(),
		consensusLatency: make([]time.Duration, 0),
	}
	
	// Create nodes
	if err := bench.CreateNodes(*nodes, *port); err != nil {
		log.Fatalf("Failed to create nodes: %v", err)
	}
	
	// Handle shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-sigCh
		log.Println("📤 Shutting down...")
		cancel()
	}()
	
	// Start nodes
	if err := bench.Start(ctx); err != nil {
		log.Fatalf("Failed to start benchmark: %v", err)
	}
	
	// Run benchmark
	bench.Run(ctx, *rounds, *interval)
	
	// Stop nodes
	bench.Stop()
	
	// Print results
	bench.PrintStats()
}

func (b *NetworkBenchmark) CreateNodes(count int, basePort int) error {
	b.nodes = make([]*Node, count)
	
	for i := 0; i < count; i++ {
		nodeID := ids.GenerateTestNodeID()
		
		// Create transport
		transport, err := NewSimpleTransport(nodeID.String(), basePort+i)
		if err != nil {
			return fmt.Errorf("failed to create transport for node %d: %w", i, err)
		}
		
		node := &Node{
			id:         nodeID,
			index:      i,
			transport:  transport,
			params:     b.params,
			confidence: make(map[ids.ID]int),
		}
		
		b.nodes[i] = node
		if !*quiet {
			log.Printf("Created node %d: %s on port %d", i, nodeID, basePort+i)
		}
	}
	
	// Connect nodes in a mesh topology
	for i, node := range b.nodes {
		for j := range b.nodes {
			if i != j {
				peerAddr := fmt.Sprintf("tcp://127.0.0.1:%d", basePort+j)
				if err := node.transport.Connect(peerAddr); err != nil {
					return fmt.Errorf("failed to connect node %d to %d: %w", i, j, err)
				}
			}
		}
	}
	
	return nil
}

func (b *NetworkBenchmark) Start(ctx context.Context) error {
	var wg sync.WaitGroup
	
	for _, node := range b.nodes {
		wg.Add(1)
		go func(n *Node) {
			defer wg.Done()
			n.Run(ctx, b)
		}(node)
	}
	
	// Give nodes time to start
	time.Sleep(100 * time.Millisecond)
	
	return nil
}

func (b *NetworkBenchmark) Run(ctx context.Context, rounds int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	
	for round := 0; round < rounds; round++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			b.runRound(ctx, round)
		}
	}
}

func (b *NetworkBenchmark) runRound(ctx context.Context, round int) {
	// Leader election (simple: node 0 is always leader)
	leader := b.nodes[0]
	
	// Create proposal
	proposalID := ids.GenerateTestID()
	proposal := &ConsensusMessage{
		Type:      "proposal",
		Round:     uint64(round),
		NodeID:    leader.id,
		ItemID:    proposalID,
		Height:    uint64(round),
		Timestamp: time.Now().UnixNano(),
	}
	
	// Broadcast proposal
	if err := leader.Broadcast(proposal); err != nil {
		log.Printf("Failed to broadcast proposal: %v", err)
		return
	}
	
	b.totalRounds.Add(1)
}

func (b *NetworkBenchmark) Stop() {
	for _, node := range b.nodes {
		node.Stop()
	}
}

func (b *NetworkBenchmark) PrintStats() {
	elapsed := time.Since(b.startTime)
	
	var totalMsgRecv, totalMsgSent int64
	var finalized int
	
	for _, node := range b.nodes {
		totalMsgRecv += node.messagesRecv.Load()
		totalMsgSent += node.messagesSent.Load()
		if node.finalized {
			finalized++
		}
	}
	
	avgLatency := time.Duration(0)
	if len(b.consensusLatency) > 0 {
		var total time.Duration
		for _, lat := range b.consensusLatency {
			total += lat
		}
		avgLatency = total / time.Duration(len(b.consensusLatency))
	}
	
	fmt.Printf("\n📊 ZMQ Consensus Benchmark Results:\n")
	fmt.Printf("   Nodes: %d\n", len(b.nodes))
	fmt.Printf("   Duration: %v\n", elapsed)
	fmt.Printf("   Rounds completed: %d\n", b.totalRounds.Load())
	fmt.Printf("   Messages sent: %d\n", totalMsgSent)
	fmt.Printf("   Messages received: %d\n", totalMsgRecv)
	fmt.Printf("   Nodes finalized: %d/%d\n", finalized, len(b.nodes))
	fmt.Printf("   Avg consensus latency: %v\n", avgLatency)
	fmt.Printf("   Throughput: %.2f rounds/sec\n", float64(b.totalRounds.Load())/elapsed.Seconds())
	fmt.Printf("   Message rate: %.2f msg/sec\n", float64(totalMsgRecv)/elapsed.Seconds())
	fmt.Printf("   Est. TPS: %.2f\n", float64(b.totalRounds.Load()*int64(*batchSize))/elapsed.Seconds())
}

// Node methods
func (n *Node) Run(ctx context.Context, bench *NetworkBenchmark) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			msg, err := n.transport.Receive()
			if err != nil {
				// Ignore context canceled errors during shutdown
				if err.Error() != "timeout" && err.Error() != "context canceled" && err.Error() != "EOF" {
					log.Printf("Node %d receive error: %v", n.index, err)
				}
				continue
			}
			
			n.messagesRecv.Add(1)
			bench.totalMessages.Add(1)
			
			// Process message
			n.ProcessMessage(msg, bench)
		}
	}
}

func (n *Node) ProcessMessage(msgBytes []byte, bench *NetworkBenchmark) {
	msg, err := DecodeMessage(msgBytes)
	if err != nil {
		log.Printf("Node %d decode error: %v", n.index, err)
		return
	}
	
	switch msg.Type {
	case "proposal":
		// Process proposal and vote
		n.ProcessProposal(msg, bench)
		
	case "vote":
		// Process vote
		n.ProcessVote(msg, bench)
	}
}

func (n *Node) ProcessProposal(msg *ConsensusMessage, bench *NetworkBenchmark) {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	// Update preference
	n.preference = msg.ItemID
	n.confidence[msg.ItemID] = 1
	
	// Send vote
	vote := &ConsensusMessage{
		Type:      "vote",
		Round:     msg.Round,
		NodeID:    n.id,
		ItemID:    msg.ItemID,
		VoteFor:   msg.ItemID,
		Timestamp: time.Now().UnixNano(),
	}
	
	if err := n.Broadcast(vote); err != nil {
		log.Printf("Node %d failed to broadcast vote: %v", n.index, err)
	}
}

func (n *Node) ProcessVote(msg *ConsensusMessage, bench *NetworkBenchmark) {
	n.mu.Lock()
	defer n.mu.Unlock()
	
	// Update confidence
	n.confidence[msg.VoteFor]++
	
	// Check if we reached consensus
	if n.confidence[msg.VoteFor] >= n.params.AlphaConfidence && !n.finalized {
		n.finalized = true
		bench.totalFinalized.Add(1)
		
		// Record latency
		latency := time.Duration(time.Now().UnixNano() - msg.Timestamp)
		bench.mu.Lock()
		bench.consensusLatency = append(bench.consensusLatency, latency)
		bench.mu.Unlock()
		
		if !*quiet {
			log.Printf("Node %d finalized round %d (latency: %v)", n.index, msg.Round, latency)
		}
	}
}

func (n *Node) Broadcast(msg *ConsensusMessage) error {
	data, err := msg.Encode()
	if err != nil {
		return err
	}
	
	if err := n.transport.Send(data); err != nil {
		return err
	}
	
	n.messagesSent.Add(1)
	return nil
}

func (n *Node) Stop() {
	n.transport.Close()
}

// Message types
type ConsensusMessage struct {
	Type      string      `json:"type"`
	Round     uint64      `json:"round"`
	NodeID    ids.NodeID  `json:"node_id"`
	ItemID    ids.ID      `json:"item_id"`
	VoteFor   ids.ID      `json:"vote_for,omitempty"`
	Height    uint64      `json:"height"`
	Timestamp int64       `json:"timestamp"`
}

func (m *ConsensusMessage) Encode() ([]byte, error) {
	return json.Marshal(m)
}

func DecodeMessage(data []byte) (*ConsensusMessage, error) {
	var msg ConsensusMessage
	err := json.Unmarshal(data, &msg)
	return &msg, err
}

func getConsensusParams(profile string) config.Parameters {
	params, err := config.GetPresetParameters(profile)
	if err != nil {
		if !*quiet {
			log.Printf("Unknown profile %s, using local", profile)
		}
		params, _ = config.GetPresetParameters("local")
	}
	return params
}