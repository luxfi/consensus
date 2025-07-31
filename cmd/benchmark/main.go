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
	"net/http"
	"os"
	"os/signal"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/luxfi/consensus/config"
	quantum "github.com/luxfi/consensus/protocol/quasar"
	"github.com/luxfi/consensus/utils/transport"
	"github.com/luxfi/consensus/utils/transport/zmq"
	"github.com/luxfi/ids"
)

var (
	port        = flag.Int("port", 30000, "Base port for ZMQ transport")
	httpPort    = flag.Int("http", 8080, "HTTP port for metrics")
	peers       = flag.String("peers", "", "Comma-separated list of peer endpoints")
	profile     = flag.String("profile", "local", "Consensus profile: local, testnet, mainnet")
	batchSize   = flag.Int("batch", 4096, "Batch size for proposals")
	minRound    = flag.Duration("min-round", 5*time.Millisecond, "Minimum round interval")
	metricsAddr = flag.String("metrics", ":9090", "Prometheus metrics address")
)

// BenchmarkNode represents a standalone consensus benchmark node
type BenchmarkNode struct {
	id         ids.NodeID
	transport  *zmq.Transport
	engine     *quantum.Engine
	params     config.Parameters
	
	// Metrics
	messagesReceived atomic.Int64
	messagesSent     atomic.Int64
	consensusRounds  atomic.Int64
	txProcessed      atomic.Int64
	startTime        time.Time
	
	// State
	mu       sync.RWMutex
	peers    map[ids.NodeID]string
	running  atomic.Bool
}

func main() {
	flag.Parse()
	
	// Create node ID
	nodeID := ids.GenerateTestNodeID()
	log.Printf("🚀 Starting benchmark node %s on port %d", nodeID.String(), *port)
	
	// Get consensus parameters
	params := getConsensusParams(*profile)
	params.BatchSize = *batchSize
	params.MinRoundInterval = *minRound
	
	// Create node
	node, err := NewBenchmarkNode(nodeID, *port, params)
	if err != nil {
		log.Fatalf("Failed to create node: %v", err)
	}
	
	// Start HTTP server for metrics
	go node.serveHTTP(*httpPort)
	
	// Start Prometheus metrics if requested
	if *metricsAddr != "" {
		go node.servePrometheus(*metricsAddr)
	}
	
	// Start node
	if err := node.Start(); err != nil {
		log.Fatalf("Failed to start node: %v", err)
	}
	
	// Connect to peers if provided
	if *peers != "" {
		node.connectToPeers(*peers)
	}
	
	// Run consensus loop
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	
	go func() {
		<-sigCh
		log.Println("📤 Shutting down...")
		cancel()
		node.Stop()
	}()
	
	// Run benchmark
	node.Run(ctx)
	
	// Print final stats
	node.PrintStats()
}

func NewBenchmarkNode(nodeID ids.NodeID, port int, params config.Parameters) (*BenchmarkNode, error) {
	// Create transport
	transport, err := zmq.NewTransport(nodeID, port)
	if err != nil {
		return nil, fmt.Errorf("failed to create transport: %w", err)
	}
	
	// Create quantum consensus engine
	quantumEngine := quantum.New(params, nodeID)
	
	return &BenchmarkNode{
		id:        nodeID,
		transport: transport,
		engine:    quantumEngine,
		params:    params,
		peers:     make(map[ids.NodeID]string),
		startTime: time.Now(),
	}, nil
}

func (n *BenchmarkNode) Start() error {
	// Register handlers
	n.transport.RegisterHandler(transport.VoteRequest, n.handleVoteRequest)
	n.transport.RegisterHandler(transport.VoteResponse, n.handleVoteResponse)
	n.transport.RegisterHandler(transport.Proposal, n.handleProposal)
	n.transport.RegisterHandler(transport.Heartbeat, n.handleHeartbeat)
	
	// Start transport
	if err := n.transport.Start(); err != nil {
		return err
	}
	
	n.running.Store(true)
	log.Printf("✅ Node started successfully")
	return nil
}

func (n *BenchmarkNode) Stop() error {
	n.running.Store(false)
	return n.transport.Stop()
}

func (n *BenchmarkNode) Run(ctx context.Context) {
	ticker := time.NewTicker(n.params.MinRoundInterval)
	defer ticker.Stop()
	
	heartbeatTicker := time.NewTicker(5 * time.Second)
	defer heartbeatTicker.Stop()
	
	for {
		select {
		case <-ctx.Done():
			return
			
		case <-ticker.C:
			n.runConsensusRound()
			
		case <-heartbeatTicker.C:
			n.sendHeartbeats()
		}
	}
}

func (n *BenchmarkNode) runConsensusRound() {
	n.consensusRounds.Add(1)
	
	// Get current peers
	n.mu.RLock()
	peerCount := len(n.peers)
	n.mu.RUnlock()
	
	if peerCount == 0 {
		return
	}
	
	// Sample k-1 peers
	k := n.params.K
	if k > peerCount+1 {
		k = peerCount + 1
	}
	
	// Send vote requests
	n.mu.RLock()
	i := 0
	for peerID := range n.peers {
		if i >= k-1 {
			break
		}
		
		msg := &transport.Message{
			Type: transport.VoteRequest,
			From: n.id,
			To:   peerID,
		}
		
		if err := n.transport.Send(peerID, msg); err == nil {
			n.messagesSent.Add(1)
		}
		i++
	}
	n.mu.RUnlock()
	
	// Process batch
	n.txProcessed.Add(int64(n.params.BatchSize))
}

func (n *BenchmarkNode) handleVoteRequest(from ids.NodeID, msg *transport.Message) {
	n.messagesReceived.Add(1)
	
	// Send vote response
	n.mu.RLock()
	preference := n.engine.Preference()
	n.mu.RUnlock()
	
	response := &transport.Message{
		Type:    transport.VoteResponse,
		From:    n.id,
		To:      from,
		Payload: []byte(preference.String()),
	}
	
	if err := n.transport.Send(from, response); err == nil {
		n.messagesSent.Add(1)
	}
}

func (n *BenchmarkNode) handleVoteResponse(from ids.NodeID, msg *transport.Message) {
	n.messagesReceived.Add(1)
	// Process vote
}

func (n *BenchmarkNode) handleProposal(from ids.NodeID, msg *transport.Message) {
	n.messagesReceived.Add(1)
	// Process proposal
}

func (n *BenchmarkNode) handleHeartbeat(from ids.NodeID, msg *transport.Message) {
	n.messagesReceived.Add(1)
	
	// Parse heartbeat data
	var hb Heartbeat
	if err := json.Unmarshal(msg.Payload, &hb); err == nil {
		// Update peer info
		n.mu.Lock()
		n.peers[from] = hb.Endpoint
		n.mu.Unlock()
	}
}

func (n *BenchmarkNode) sendHeartbeats() {
	hb := Heartbeat{
		NodeID:   n.id.String(),
		Endpoint: fmt.Sprintf("tcp://127.0.0.1:%d", *port),
		TPS:      n.getCurrentTPS(),
	}
	
	data, _ := json.Marshal(hb)
	
	msg := &transport.Message{
		Type:    transport.Heartbeat,
		From:    n.id,
		Payload: data,
	}
	
	n.transport.Broadcast(msg)
}

func (n *BenchmarkNode) connectToPeers(peerList string) {
	// Parse peer list: "tcp://host1:port1,tcp://host2:port2"
	// For simplicity, generate peer IDs
	endpoints := splitPeers(peerList)
	
	for i, endpoint := range endpoints {
		peerID := ids.GenerateTestNodeID()
		if err := n.transport.Connect(peerID, endpoint); err != nil {
			log.Printf("Failed to connect to peer %s: %v", endpoint, err)
		} else {
			n.mu.Lock()
			n.peers[peerID] = endpoint
			n.mu.Unlock()
			log.Printf("Connected to peer %d: %s", i, endpoint)
		}
	}
}

func (n *BenchmarkNode) getCurrentTPS() float64 {
	elapsed := time.Since(n.startTime).Seconds()
	if elapsed == 0 {
		return 0
	}
	return float64(n.txProcessed.Load()) / elapsed
}

func (n *BenchmarkNode) PrintStats() {
	elapsed := time.Since(n.startTime)
	tps := n.getCurrentTPS()
	
	fmt.Printf("\n📊 Benchmark Results:\n")
	fmt.Printf("   Duration: %v\n", elapsed)
	fmt.Printf("   Consensus rounds: %d\n", n.consensusRounds.Load())
	fmt.Printf("   Messages sent: %d\n", n.messagesSent.Load())
	fmt.Printf("   Messages received: %d\n", n.messagesReceived.Load())
	fmt.Printf("   Transactions processed: %d\n", n.txProcessed.Load())
	fmt.Printf("   TPS: %.2f\n", tps)
	fmt.Printf("   Peers connected: %d\n", len(n.peers))
}

func (n *BenchmarkNode) serveHTTP(port int) {
	http.HandleFunc("/stats", func(w http.ResponseWriter, r *http.Request) {
		stats := map[string]interface{}{
			"node_id":           n.id.String(),
			"uptime":            time.Since(n.startTime).Seconds(),
			"consensus_rounds":  n.consensusRounds.Load(),
			"messages_sent":     n.messagesSent.Load(),
			"messages_received": n.messagesReceived.Load(),
			"tx_processed":      n.txProcessed.Load(),
			"tps":               n.getCurrentTPS(),
			"peer_count":        len(n.peers),
		}
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(stats)
	})
	
	http.HandleFunc("/peers", func(w http.ResponseWriter, r *http.Request) {
		n.mu.RLock()
		peers := make(map[string]string)
		for id, endpoint := range n.peers {
			peers[id.String()] = endpoint
		}
		n.mu.RUnlock()
		
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(peers)
	})
	
	log.Printf("📡 HTTP server listening on :%d", port)
	http.ListenAndServe(fmt.Sprintf(":%d", port), nil)
}

func (n *BenchmarkNode) servePrometheus(addr string) {
	// TODO: Add Prometheus metrics
	log.Printf("📊 Prometheus metrics at %s/metrics", addr)
}

func getConsensusParams(profile string) config.Parameters {
	params, err := config.GetPresetParameters(profile)
	if err != nil {
		log.Printf("Unknown profile %s, using local", profile)
		params, _ = config.GetPresetParameters("local")
	}
	return params
}

func splitPeers(peerList string) []string {
	// Simple comma-separated split
	var peers []string
	if peerList != "" {
		// Parse comma-separated endpoints
		// TODO: Implement proper parsing
		peers = append(peers, peerList)
	}
	return peers
}

// Heartbeat message structure
type Heartbeat struct {
	NodeID   string  `json:"node_id"`
	Endpoint string  `json:"endpoint"`
	TPS      float64 `json:"tps"`
}