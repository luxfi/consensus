// Copyright (C) 2024-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/spf13/cobra"
	zmq "github.com/pebbe/zmq4"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/factories"
	"github.com/luxfi/consensus/poll"
	"github.com/luxfi/ids"
)

type ZMQMessage struct {
	Type      string          `json:"type"`
	NodeID    int             `json:"node_id"`
	Round     int             `json:"round"`
	Votes     []string        `json:"votes,omitempty"`
	Choice    string          `json:"choice,omitempty"`
	Finalized bool            `json:"finalized,omitempty"`
	Params    *config.Parameters `json:"params,omitempty"`
}

type ZMQCoordinator struct {
	socket     *zmq.Socket
	nodes      map[int]*NodeState
	params     config.Parameters
	round      int
	mu         sync.Mutex
}

type NodeState struct {
	id        int
	choice    ids.ID
	finalized bool
	lastSeen  time.Time
}

func runZMQCoordinator(cmd *cobra.Command, args []string) error {
	bind, _ := cmd.Flags().GetString("bind")
	workers, _ := cmd.Flags().GetInt("workers")
	rounds, _ := cmd.Flags().GetInt("rounds")
	
	fmt.Printf("=== ZMQ Consensus Coordinator ===\n")
	fmt.Printf("Binding to: %s\n", bind)
	fmt.Printf("Expected workers: %d\n", workers)
	fmt.Printf("Max rounds: %d\n", rounds)
	
	// Create ZMQ socket
	socket, err := zmq.NewSocket(zmq.ROUTER)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	defer socket.Close()
	
	if err := socket.Bind(bind); err != nil {
		return fmt.Errorf("failed to bind: %w", err)
	}
	
	coordinator := &ZMQCoordinator{
		socket: socket,
		nodes:  make(map[int]*NodeState),
		params: config.DefaultParameters,
	}
	
	// Wait for workers to connect
	fmt.Println("\nWaiting for workers to connect...")
	connected := 0
	timeout := time.After(30 * time.Second)
	
	for connected < workers {
		select {
		case <-timeout:
			fmt.Printf("Timeout: only %d/%d workers connected\n", connected, workers)
			if connected == 0 {
				return fmt.Errorf("no workers connected")
			}
			workers = connected
			break
			
		default:
			// Set receive timeout
			socket.SetRcvtimeo(1 * time.Second)
			
			msg, err := socket.RecvMessage(0)
			if err != nil {
				continue
			}
			
			if len(msg) < 2 {
				continue
			}
			
			// Parse message
			identity := msg[0]
			data := msg[1]
			
			var zmqMsg ZMQMessage
			if err := json.Unmarshal([]byte(data), &zmqMsg); err != nil {
				continue
			}
			
			if zmqMsg.Type == "connect" {
				connected++
				coordinator.nodes[zmqMsg.NodeID] = &NodeState{
					id:       zmqMsg.NodeID,
					lastSeen: time.Now(),
				}
				
				// Send parameters
				response := ZMQMessage{
					Type:   "params",
					Params: &coordinator.params,
				}
				
				respData, _ := json.Marshal(response)
				socket.SendMessage(identity, respData)
				
				fmt.Printf("Worker %d connected (%d/%d)\n", zmqMsg.NodeID, connected, workers)
			}
		}
	}
	
	fmt.Printf("\nAll workers connected. Starting consensus...\n")
	
	// Run consensus rounds
	startTime := time.Now()
	finalizedCount := 0
	
	for round := 0; round < rounds && finalizedCount < workers; round++ {
		coordinator.round = round
		
		// Broadcast round start
		broadcast := ZMQMessage{
			Type:  "round_start",
			Round: round,
		}
		broadcastData, _ := json.Marshal(broadcast)
		
		// Send to all workers
		for nodeID := range coordinator.nodes {
			socket.SendMessage(fmt.Sprintf("node-%d", nodeID), broadcastData)
		}
		
		// Collect votes
		votes := make(map[int][]ids.ID)
		received := 0
		
		for received < workers-finalizedCount {
			msg, err := socket.RecvMessage(0)
			if err != nil {
				continue
			}
			
			if len(msg) < 2 {
				continue
			}
			
			identity := msg[0]
			data := msg[1]
			
			var zmqMsg ZMQMessage
			if err := json.Unmarshal([]byte(data), &zmqMsg); err != nil {
				continue
			}
			
			if zmqMsg.Type == "vote_request" {
				// Node is requesting votes from peers
				nodeVotes := coordinator.gatherVotes(zmqMsg.NodeID)
				
				response := ZMQMessage{
					Type:  "votes",
					Votes: nodeVotes,
				}
				
				respData, _ := json.Marshal(response)
				socket.SendMessage(identity, respData)
				received++
				
			} else if zmqMsg.Type == "status_update" {
				// Node is reporting its status
				node := coordinator.nodes[zmqMsg.NodeID]
				if zmqMsg.Choice != "" {
					node.choice, _ = ids.FromString(zmqMsg.Choice)
				}
				if zmqMsg.Finalized && !node.finalized {
					node.finalized = true
					finalizedCount++
					fmt.Printf("Node %d finalized (total: %d/%d)\n", 
						zmqMsg.NodeID, finalizedCount, workers)
				}
			}
		}
		
		// Progress update
		if round%10 == 0 {
			fmt.Printf("Round %d: %d/%d nodes finalized\n", 
				round, finalizedCount, workers)
		}
	}
	
	elapsed := time.Since(startTime)
	
	// Results
	fmt.Printf("\n=== Consensus Results ===\n")
	fmt.Printf("Total rounds: %d\n", coordinator.round)
	fmt.Printf("Total time: %v\n", elapsed)
	fmt.Printf("Finalized nodes: %d/%d\n", finalizedCount, workers)
	
	// Count choices
	choiceCounts := make(map[ids.ID]int)
	for _, node := range coordinator.nodes {
		if node.finalized {
			choiceCounts[node.choice]++
		}
	}
	
	fmt.Println("\nFinal choices:")
	for choice, count := range choiceCounts {
		fmt.Printf("  %s: %d nodes\n", choice, count)
	}
	
	// Send shutdown signal
	shutdown := ZMQMessage{Type: "shutdown"}
	shutdownData, _ := json.Marshal(shutdown)
	for nodeID := range coordinator.nodes {
		socket.SendMessage(fmt.Sprintf("node-%d", nodeID), shutdownData)
	}
	
	return nil
}

func (c *ZMQCoordinator) gatherVotes(nodeID int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	
	// Simple K-random sampling
	votes := make([]string, 0, c.params.K)
	
	i := 0
	for _, node := range c.nodes {
		if i >= c.params.K {
			break
		}
		votes = append(votes, node.choice.String())
		i++
	}
	
	return votes
}

func runZMQWorker(cmd *cobra.Command, args []string) error {
	connect, _ := cmd.Flags().GetString("connect")
	nodeID, _ := cmd.Flags().GetInt("node-id")
	
	fmt.Printf("=== ZMQ Consensus Worker %d ===\n", nodeID)
	fmt.Printf("Connecting to: %s\n", connect)
	
	// Create ZMQ socket
	socket, err := zmq.NewSocket(zmq.DEALER)
	if err != nil {
		return fmt.Errorf("failed to create socket: %w", err)
	}
	defer socket.Close()
	
	// Set identity
	socket.SetIdentity(fmt.Sprintf("node-%d", nodeID))
	
	if err := socket.Connect(connect); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	
	// Send connect message
	connectMsg := ZMQMessage{
		Type:   "connect",
		NodeID: nodeID,
	}
	connectData, _ := json.Marshal(connectMsg)
	socket.SendMessage(connectData)
	
	// Wait for parameters
	msg, err := socket.RecvMessage(0)
	if err != nil {
		return fmt.Errorf("failed to receive parameters: %w", err)
	}
	
	var paramsMsg ZMQMessage
	if err := json.Unmarshal([]byte(msg[0]), &paramsMsg); err != nil {
		return fmt.Errorf("failed to parse parameters: %w", err)
	}
	
	if paramsMsg.Type != "params" || paramsMsg.Params == nil {
		return fmt.Errorf("invalid parameters message")
	}
	
	params := poll.ConvertConfigParams(*paramsMsg.Params)
	
	// Create consensus instance
	factory := factories.ConfidenceFactory
	consensus := factory.NewUnary(params)
	
	// Initial choice
	var choice ids.ID
	if nodeID%2 == 0 {
		choice = ids.GenerateTestID()
	} else {
		choice = ids.GenerateTestID()
	}
	
	fmt.Printf("Starting consensus with initial choice: %s\n", choice)
	
	// Main loop
	for {
		msg, err := socket.RecvMessage(0)
		if err != nil {
			continue
		}
		
		var zmqMsg ZMQMessage
		if err := json.Unmarshal([]byte(msg[0]), &zmqMsg); err != nil {
			continue
		}
		
		switch zmqMsg.Type {
		case "round_start":
			if consensus.Finalized() {
				continue
			}
			
			// Request votes
			voteReq := ZMQMessage{
				Type:   "vote_request",
				NodeID: nodeID,
				Round:  zmqMsg.Round,
			}
			voteReqData, _ := json.Marshal(voteReq)
			socket.SendMessage(voteReqData)
			
		case "votes":
			// Process votes
			votes := make([]ids.ID, len(zmqMsg.Votes))
			for i, v := range zmqMsg.Votes {
				votes[i], _ = ids.FromString(v)
			}
			
			consensus.RecordPoll(votes)
			choice = consensus.Preference()
			
			// Send status update
			status := ZMQMessage{
				Type:      "status_update",
				NodeID:    nodeID,
				Choice:    choice.String(),
				Finalized: consensus.Finalized(),
			}
			statusData, _ := json.Marshal(status)
			socket.SendMessage(statusData)
			
		case "shutdown":
			fmt.Println("Received shutdown signal")
			return nil
		}
	}
}

func runZMQTest(cmd *cobra.Command, args []string) error {
	// This would coordinate a full test across multiple machines
	fmt.Println("=== ZMQ Distributed Consensus Test ===")
	fmt.Println("This command would:")
	fmt.Println("1. Deploy worker binaries to remote hosts")
	fmt.Println("2. Start coordinator on local machine")
	fmt.Println("3. Start workers on remote hosts")
	fmt.Println("4. Run consensus test")
	fmt.Println("5. Collect and analyze results")
	fmt.Println("\nNot yet implemented - use coordinator and worker commands manually")
	
	return nil
}

func init() {
	// Add node-id flag to worker command
	workerCmd := &cobra.Command{
		Use:   "worker",
		Short: "Run ZMQ worker node",
		RunE:  runZMQWorker,
	}
	workerCmd.Flags().Int("node-id", 0, "Unique node identifier")
	workerCmd.MarkFlagRequired("node-id")
	
	// Add rounds flag to coordinator
	coordCmd := &cobra.Command{
		Use:   "coordinator",
		Short: "Run ZMQ coordinator node",
		RunE:  runZMQCoordinator,
	}
	coordCmd.Flags().Int("rounds", 100, "Maximum consensus rounds")
}