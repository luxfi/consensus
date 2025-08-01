#!/bin/bash
# Multi-node benchmark setup script

set -e

# Configuration
NODES=${1:-10}
BASE_PORT=${2:-30000}
BATCH_SIZE=${3:-4096}
INTERVAL=${4:-5ms}
ROUNDS=${5:-100}

echo "🚀 Starting $NODES-node consensus benchmark network"
echo "   Base port: $BASE_PORT"
echo "   Batch size: $BATCH_SIZE"
echo "   Interval: $INTERVAL"
echo "   Rounds: $ROUNDS"
echo ""

# Build the benchmark tool if needed
if [ ! -f bin/zmq-bench ]; then
    echo "Building zmq-bench..."
    make zmq-bench
fi

# Kill any existing zmq-bench processes
echo "Cleaning up any existing processes..."
pkill -f zmq-bench || true
sleep 1

# Start the benchmark
echo "Starting benchmark with $NODES nodes..."
./bin/zmq-bench \
    -nodes $NODES \
    -port $BASE_PORT \
    -batch $BATCH_SIZE \
    -interval $INTERVAL \
    -rounds $ROUNDS

echo ""
echo "Benchmark complete!"