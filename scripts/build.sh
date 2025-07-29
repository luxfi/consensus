#!/bin/bash
# Temporary build script to work around node package issues

echo "Building consensus tools..."

# Build params (should work)
echo "Building params..."
go build -o bin/params ./cmd/params || echo "❌ Failed to build params"

# Build checker (should work)
echo "Building checker..."
go build -o bin/checker ./cmd/checker || echo "❌ Failed to build checker"

# Build sim (should work)  
echo "Building simulator..."
go build -o bin/sim ./cmd/sim || echo "❌ Failed to build sim"

# Try to build zmq-bench standalone
echo "Building zmq-bench..."
cd cmd/zmq-bench
go build -tags zmq -o ../../bin/zmq-bench . || echo "⚠️  Failed to build zmq-bench"
cd ../..

# Build consensus CLI
echo "Building consensus CLI..."
cd cmd/consensus
go mod tidy
go build -o ../../bin/consensus . || echo "⚠️  Failed to build consensus CLI"
cd ../..

echo "✅ Build complete! Successfully built tools are in bin/"
ls -la bin/ 2>/dev/null || echo "No binaries built successfully"