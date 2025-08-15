// Copyright (C) 2024-2025, Lux Industries Inc. All rights reserved.
// See the file LICENSE for licensing terms.

package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "consensus",
	Short: "Lux consensus tools for parameter management, benchmarking, and testing",
	Long: `The consensus command provides various tools for working with Lux consensus parameters,
including parameter checking, simulation, benchmarking, and multi-host testing with ZeroMQ.

Key Features:
- Parameter validation and tuning
- Consensus simulation with various network conditions
- Distributed benchmarking across multiple machines
- Automatic core detection and multi-validator support
- Pure consensus performance testing without full blockchain`,
}

func main() {
	// Add subcommands
	rootCmd.AddCommand(
		checkCmd(),
		simCmd(),
		benchmarkCmd(),
		// benchCmd(),    // Disabled - needs ZMQ API update
		paramsCmd(),
		// zmqCmd(),      // Disabled - needs ZMQ API update
	)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

// Import functionality from existing commands
func checkCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check consensus parameters for safety and correctness",
		Long: `Analyze consensus parameters to ensure they meet safety requirements
and are properly configured for the intended network size.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Import logic from cmd/checker/main.go
			return runChecker(cmd, args)
		},
	}
}

func simCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sim",
		Short: "Simulate consensus with different parameters",
		Long: `Run consensus simulations to test different parameter configurations
and analyze their behavior under various network conditions.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Import logic from cmd/sim/main.go
			return runSimulator(cmd, args)
		},
	}
}

func benchmarkCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "benchmark",
		Short: "Benchmark consensus performance",
		Long: `Run performance benchmarks for consensus algorithms with different
parameters and network configurations.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			// Import logic from cmd/benchmark/main.go
			return runBenchmark(cmd, args)
		},
	}

	// Add flags for benchmark configuration
	cmd.Flags().Int("nodes", 100, "Number of nodes to simulate")
	cmd.Flags().Int("rounds", 1000, "Number of consensus rounds")
	cmd.Flags().String("transport", "local", "Transport type: local, zmq")
	cmd.Flags().String("zmq-bind", "tcp://*:5555", "ZeroMQ bind address for coordinator")
	cmd.Flags().Bool("parallel", false, "Run benchmarks in parallel")

	return cmd
}

func paramsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "params",
		Short: "Manage consensus parameters",
		Long: `Tools for managing consensus parameters including interactive configuration,
parameter tuning, and validation.`,
	}

	// Add subcommands for params
	cmd.AddCommand(
		&cobra.Command{
			Use:   "interactive",
			Short: "Interactive parameter configuration",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runInteractiveParams(cmd, args)
			},
		},
		&cobra.Command{
			Use:   "tune",
			Short: "Tune parameters for specific network conditions",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runParamsTune(cmd, args)
			},
		},
		&cobra.Command{
			Use:   "generate",
			Short: "Generate parameter configurations",
			RunE: func(cmd *cobra.Command, args []string) error {
				return runParamsGenerate(cmd, args)
			},
		},
	)

	return cmd
}

// Disabled until ZMQ API is updated
// func zmqCmd() *cobra.Command {
// 	cmd := &cobra.Command{
// 		Use:   "zmq",
// 		Short: "ZeroMQ-based multi-host consensus testing",
// 		Long: `Tools for running consensus tests across multiple hosts using ZeroMQ
// for high-performance message passing between nodes.`,
// 	}

// 	// Add subcommands for ZMQ testing
// 	cmd.AddCommand(
// 		&cobra.Command{
// 			Use:   "coordinator",
// 			Short: "Run ZMQ coordinator node",
// 			Long: `Start a coordinator node that manages consensus testing across
// multiple hosts connected via ZeroMQ.`,
// 			RunE: func(cmd *cobra.Command, args []string) error {
// 				return runZMQCoordinator(cmd, args)
// 			},
// 		},
// 		&cobra.Command{
// 			Use:   "worker",
// 			Short: "Run ZMQ worker node",
// 			Long: `Start a worker node that participates in consensus testing
// coordinated via ZeroMQ.`,
// 			RunE: func(cmd *cobra.Command, args []string) error {
// 				return runZMQWorker(cmd, args)
// 			},
// 		},
// 		&cobra.Command{
// 			Use:   "test",
// 			Short: "Run distributed consensus test",
// 			Long: `Execute a distributed consensus test across multiple hosts
// using ZeroMQ for communication.`,
// 			RunE: func(cmd *cobra.Command, args []string) error {
// 				return runZMQTest(cmd, args)
// 			},
// 		},
// 	)

// 	// Add common ZMQ flags
// 	for _, subCmd := range cmd.Commands() {
// 		subCmd.Flags().String("connect", "tcp://localhost:5555", "ZeroMQ coordinator address")
// 		subCmd.Flags().String("bind", "", "ZeroMQ bind address (coordinator only)")
// 		subCmd.Flags().Int("workers", 10, "Number of worker nodes")
// 		subCmd.Flags().Duration("timeout", 60, "Test timeout in seconds")
// 	}

// 	return cmd
// }
