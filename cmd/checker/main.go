package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/luxfi/consensus/config"
	"github.com/luxfi/consensus/core/wave"
	"github.com/luxfi/consensus/photon"
	"github.com/luxfi/consensus/protocol/quasar"
)

func main() {
	var (
		component = flag.String("component", "all", "Component to check (all, wave, photon, quasar, config)")
		verbose   = flag.Bool("v", false, "Verbose output")
	)
	flag.Parse()

	fmt.Println("Lux Consensus Health Checker")
	fmt.Println("============================")

	success := true

	switch *component {
	case "all":
		success = checkAll(*verbose)
	case "wave":
		success = checkWave(*verbose)
	case "photon":
		success = checkPhoton(*verbose)
	case "quasar":
		success = checkQuasar(*verbose)
	case "config":
		success = checkConfig(*verbose)
	default:
		fmt.Fprintf(os.Stderr, "Unknown component: %s\n", *component)
		os.Exit(1)
	}

	if success {
		fmt.Println("\n✅ All checks passed")
	} else {
		fmt.Println("\n❌ Some checks failed")
		os.Exit(1)
	}
}

func checkAll(verbose bool) bool {
	return checkConfig(verbose) &&
		checkWave(verbose) &&
		checkPhoton(verbose) &&
		checkQuasar(verbose)
}

func checkConfig(verbose bool) bool {
	fmt.Print("Checking config... ")
	
	// Verify all network configs are valid
	configs := map[string]config.Params{
		"mainnet": config.MainnetParams(),
		"testnet": config.TestnetParams(),
		"local":   config.LocalParams(),
		"xchain":  config.XChainParams(),
	}

	for name, cfg := range configs {
		if err := cfg.Verify(); err != nil {
			fmt.Printf("❌ %s config invalid: %v\n", name, err)
			return false
		}
		if verbose {
			fmt.Printf("\n  %s: K=%d, α=%.2f, β=%d", name, cfg.K, cfg.Alpha, cfg.Beta)
		}
	}

	fmt.Println("✅")
	return true
}

func checkWave(verbose bool) bool {
	fmt.Print("Checking Wave consensus... ")
	
	// Create and verify Wave engine
	cfg := config.DefaultParams()
	engine := wave.New[string](cfg)
	
	if engine == nil {
		fmt.Println("❌ Failed to create Wave engine")
		return false
	}

	// Test basic voting
	engine.Initialize("item1", 0)
	engine.RecordPoll("item1", 10)
	
	if verbose {
		fmt.Printf("\n  Engine created, test vote recorded")
	}

	fmt.Println("✅")
	return true
}

func checkPhoton(verbose bool) bool {
	fmt.Print("Checking Photon emitter... ")
	
	// Create and verify Photon emitter
	nodes := []string{"node1", "node2", "node3", "node4", "node5"}
	emitter := photon.NewUniformEmitter(nodes)
	
	if emitter == nil {
		fmt.Println("❌ Failed to create Photon emitter")
		return false
	}

	// Test emission
	selected, err := emitter.Emit(nil, 3, 12345)
	if err != nil {
		fmt.Printf("❌ Emission failed: %v\n", err)
		return false
	}

	if len(selected) != 3 {
		fmt.Printf("❌ Expected 3 nodes, got %d\n", len(selected))
		return false
	}

	if verbose {
		fmt.Printf("\n  Emitted %d nodes from %d", len(selected), len(nodes))
	}

	fmt.Println("✅")
	return true
}

func checkQuasar(verbose bool) bool {
	fmt.Print("Checking Quasar protocol... ")
	
	// Create and verify Quasar instance
	cfg := config.DefaultParams()
	q := quasar.New(cfg)
	
	if q == nil {
		fmt.Println("❌ Failed to create Quasar instance")
		return false
	}

	// Initialize
	err := q.Initialize(nil, []byte("bls-key"), []byte("pq-key"))
	if err != nil {
		fmt.Printf("❌ Initialization failed: %v\n", err)
		return false
	}

	if verbose {
		fmt.Printf("\n  Quasar initialized with K=%d, α=%.2f", cfg.K, cfg.Alpha)
	}

	fmt.Println("✅")
	return true
}