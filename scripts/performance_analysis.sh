#!/bin/bash

echo "=== Lux Consensus Performance Analysis ==="
echo ""
echo "This analysis shows theoretical consensus performance based on"
echo "network parameters and consensus configuration."
echo ""

# Function to calculate throughput
calculate_throughput() {
    local nodes=$1
    local k=$2
    local beta=$3
    local min_round_ms=$4
    local batch_size=$5
    
    # Calculate finality time in ms
    local finality_ms=$((beta * min_round_ms))
    
    # Calculate rounds per second
    local rounds_per_sec=$((1000 / min_round_ms))
    
    # Calculate theoretical TPS
    local tps=$((batch_size * rounds_per_sec))
    
    echo "Configuration:"
    echo "  Nodes: $nodes"
    echo "  Sample size (K): $k"
    echo "  Beta rounds: $beta"
    echo "  Min round interval: ${min_round_ms}ms"
    echo "  Batch size: $batch_size txns"
    echo ""
    echo "Performance:"
    echo "  Expected finality: ${finality_ms}ms"
    echo "  Rounds per second: $rounds_per_sec"
    echo "  Theoretical TPS: $tps"
    echo "  Messages per round: $k"
    echo "  Network msgs/sec: $((k * rounds_per_sec))"
}

echo "1. Local Network (5 nodes, 10 Gbps LAN)"
echo "----------------------------------------"
calculate_throughput 5 5 3 10 1024
echo ""

echo "2. Local Network - High TPS Mode"
echo "--------------------------------"
calculate_throughput 5 5 4 5 4096
echo ""

echo "3. Testnet (11 nodes)"
echo "---------------------"
calculate_throughput 11 11 6 25 2048
echo ""

echo "4. Mainnet (21 nodes)"
echo "---------------------"
calculate_throughput 21 21 8 50 4096
echo ""

echo "5. Theoretical Max - 10 nodes on 10 Gbps"
echo "----------------------------------------"
calculate_throughput 10 10 4 5 8192
echo ""

echo "=== Consensus Efficiency Analysis ==="
echo ""
echo "Network Requirements per Node:"
echo "  5 nodes:  ~50 KB/s (local)"
echo "  10 nodes: ~200 KB/s (high-speed)"
echo "  21 nodes: ~840 KB/s (mainnet)"
echo ""
echo "Note: Actual performance depends on:"
echo "- Network latency and bandwidth"
echo "- Transaction verification time"
echo "- State management overhead"
echo "- Hardware specifications"