#!/bin/bash
# Distributed multi-machine benchmark setup

set -e

# Configuration
MODE=${1:-coordinator}  # coordinator or worker
COORDINATOR_IP=${2:-localhost}
COORDINATOR_PORT=${3:-5555}
NODES=${4:-100}
ROUNDS=${5:-1000}

if [ "$MODE" = "coordinator" ]; then
    echo "🎯 Starting as COORDINATOR"
    echo "   Bind address: tcp://*:$COORDINATOR_PORT"
    echo "   Total nodes: $NODES"
    echo "   Rounds: $ROUNDS"
    echo ""
    echo "Workers should connect to: tcp://$(hostname -I | awk '{print $1}'):$COORDINATOR_PORT"
    echo ""
    
    ./bin/consensus benchmark \
        --transport zmq \
        --zmq-bind "tcp://*:$COORDINATOR_PORT" \
        --nodes $NODES \
        --rounds $ROUNDS
else
    echo "🔗 Starting as WORKER"
    echo "   Connecting to: tcp://$COORDINATOR_IP:$COORDINATOR_PORT"
    echo ""
    
    ./bin/consensus benchmark \
        --transport zmq \
        --zmq-bind "tcp://$COORDINATOR_IP:$COORDINATOR_PORT"
fi