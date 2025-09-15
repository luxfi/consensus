package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"time"

	"github.com/luxfi/consensus"
	"github.com/luxfi/consensus/config"
	"github.com/luxfi/ids"
)

type ConsensusServer struct {
	engine consensus.Engine
	config config.Parameters
}

type StatusResponse struct {
	Engine     string            `json:"engine"`
	Network    string            `json:"network"`
	Healthy    bool              `json:"healthy"`
	Parameters config.Parameters `json:"parameters"`
	Health     interface{}       `json:"health,omitempty"`
}

type TestRequest struct {
	Rounds int `json:"rounds"`
	Nodes  int `json:"nodes"`
}

type TestResponse struct {
	Success    bool    `json:"success"`
	Rounds     int     `json:"rounds"`
	Accepts    int     `json:"accepts"`
	Rejects    int     `json:"rejects"`
	AvgTime    string  `json:"avg_time"`
	Confidence float64 `json:"confidence"`
}

func (s *ConsensusServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	ctx := context.Background()

	health := make(map[string]interface{})
	health["healthy"] = true

	if adapter, ok := s.engine.(interface {
		HealthCheck(context.Context) (interface{}, error)
	}); ok {
		if h, err := adapter.HealthCheck(ctx); err == nil {
			if healthMap, ok := h.(map[string]interface{}); ok {
				health = healthMap
			} else if healthBoolMap, ok := h.(map[string]bool); ok {
				for k, v := range healthBoolMap {
					health[k] = v
				}
			}
		}
	}

	healthy := true
	if val, ok := health["healthy"].(bool); ok {
		healthy = val
	}

	resp := StatusResponse{
		Engine:     "chain",
		Network:    "mainnet",
		Healthy:    healthy,
		Parameters: s.config,
		Health:     health,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (s *ConsensusServer) handleTest(w http.ResponseWriter, r *http.Request) {
	var req TestRequest
	if r.Method == http.MethodPost {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
	} else {
		req.Rounds = 10
		req.Nodes = 5
	}

	// Run simple consensus test
	accepts := 0
	rejects := 0
	totalTime := time.Duration(0)

	for i := 0; i < req.Rounds; i++ {
		start := time.Now()

		// Simulate consensus round
		blockID := ids.GenerateTestID()

		// Simulate votes from nodes
		acceptVotes := 0
		for j := 0; j < req.Nodes; j++ {
			if j < (req.Nodes * 8 / 10) { // 80% vote to accept
				acceptVotes++
			}
		}

		if float64(acceptVotes)/float64(req.Nodes) >= s.config.Alpha {
			accepts++
		} else {
			rejects++
		}

		totalTime += time.Since(start)

		// Log the vote
		log.Printf("Round %d: BlockID=%s, Accept=%d/%d\n", i+1, blockID, acceptVotes, req.Nodes)
	}

	avgTime := totalTime / time.Duration(req.Rounds)
	confidence := float64(accepts) / float64(req.Rounds)

	resp := TestResponse{
		Success:    true,
		Rounds:     req.Rounds,
		Accepts:    accepts,
		Rejects:    rejects,
		AvgTime:    avgTime.String(),
		Confidence: confidence * 100,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func (s *ConsensusServer) handleConsensus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		BlockID string         `json:"block_id"`
		Votes   map[string]int `json:"votes"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Process consensus
	blockID, err := ids.FromString(req.BlockID)
	if err != nil {
		blockID = ids.GenerateTestID()
	}

	// Calculate if consensus reached
	totalVotes := 0
	acceptVotes := 0
	for _, count := range req.Votes {
		totalVotes += count
		if count > 0 {
			acceptVotes += count
		}
	}

	finalized := false
	if totalVotes > 0 {
		confidence := float64(acceptVotes) / float64(totalVotes)
		finalized = confidence >= s.config.Alpha
	}

	resp := map[string]interface{}{
		"block_id":   blockID.String(),
		"finalized":  finalized,
		"votes":      req.Votes,
		"confidence": float64(acceptVotes) / float64(totalVotes) * 100,
		"alpha":      s.config.Alpha * 100,
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

func main() {
	var (
		port    = flag.String("port", "8080", "Server port")
		network = flag.String("network", "mainnet", "Network configuration")
	)
	flag.Parse()

	// Initialize consensus engine
	engine := consensus.NewChainEngine()
	var params config.Parameters

	switch *network {
	case "testnet":
		params = config.TestnetParams()
	case "local":
		params = config.LocalParams()
	default:
		params = config.MainnetParams()
	}

	server := &ConsensusServer{
		engine: engine,
		config: params,
	}

	// Set up routes
	mux := http.NewServeMux()
	mux.HandleFunc("/status", server.handleStatus)
	mux.HandleFunc("/test", server.handleTest)
	mux.HandleFunc("/consensus", server.handleConsensus)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write([]byte("OK")); err != nil {
			log.Printf("Error writing response: %v", err)
		}
	})

	log.Printf("Starting consensus server on port %s with %s config", *port, *network)
	log.Printf("Endpoints:")
	log.Printf("  GET  /status    - Get engine status")
	log.Printf("  GET  /health    - Health check")
	log.Printf("  GET  /test      - Run consensus test")
	log.Printf("  POST /test      - Run consensus test with custom params")
	log.Printf("  POST /consensus - Process consensus round")

	// Create server with timeouts to avoid G114 warning
	srv := &http.Server{
		Addr:         ":" + *port,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}
