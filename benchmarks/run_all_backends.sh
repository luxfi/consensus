#!/bin/bash
# Comprehensive multi-backend benchmark suite
# Runs benchmarks on all available backends and generates comparison report

set -e

RESULTS_DIR="./results"
mkdir -p "$RESULTS_DIR"

echo "═══════════════════════════════════════════════════"
echo " Lux Consensus - Multi-Backend Benchmark Suite"
echo "═══════════════════════════════════════════════════"
echo ""

# Detect platform
PLATFORM=$(uname -s)
ARCH=$(uname -m)
echo "Platform: $PLATFORM $ARCH"
echo ""

# 1. Go Benchmarks
echo "━━━ Running Pure Go Benchmarks ━━━"
cd ../ai
go test -bench=. -benchmem -benchtime=3s -run=^$ > "../benchmarks/$RESULTS_DIR/go_benchmark.txt" 2>&1
echo "✓ Go benchmarks complete"
echo ""

# 2. Check for CGO/C backend
if [ -d "../pkg/c" ]; then
    echo "━━━ Running C Backend Benchmarks ━━━"
    cd ../pkg/c
    make bench > "../../benchmarks/$RESULTS_DIR/c_benchmark.txt" 2>&1 || echo "⚠ C benchmarks skipped (not available)"
    echo "✓ C benchmarks complete"
    echo ""
fi

# 3. Check for C++ backend
if [ -d "../pkg/cpp" ]; then
    echo "━━━ Running C++ Backend Benchmarks ━━━"
    cd ../pkg/cpp
    make bench > "../../benchmarks/$RESULTS_DIR/cpp_benchmark.txt" 2>&1 || echo "⚠ C++ benchmarks skipped (not available)"
    echo "✓ C++ benchmarks complete"
    echo ""
fi

# 4. Check for MLX backend (Apple Silicon only)
if [ "$ARCH" = "arm64" ] && [ "$PLATFORM" = "Darwin" ]; then
    if [ -d "../pkg/mlx" ]; then
        echo "━━━ Running MLX Backend Benchmarks (Apple Silicon) ━━━"
        cd ../pkg/mlx
        make bench > "../../benchmarks/$RESULTS_DIR/mlx_benchmark.txt" 2>&1 || echo "⚠ MLX benchmarks skipped (not available)"
        echo "✓ MLX benchmarks complete"
        echo ""
    fi
else
    echo "⚠ MLX benchmarks skipped (requires Apple Silicon)"
    echo ""
fi

# 5. Check for Rust backend
if [ -d "../pkg/rust" ]; then
    echo "━━━ Running Rust Backend Benchmarks ━━━"
    cd ../pkg/rust
    cargo bench > "../../benchmarks/$RESULTS_DIR/rust_benchmark.txt" 2>&1 || echo "⚠ Rust benchmarks skipped (not available)"
    echo "✓ Rust benchmarks complete"
    echo ""
fi

# Generate comparison report
echo "━━━ Generating Comparison Report ━━━"
cd ../../benchmarks
python3 generate_comparison.py || echo "⚠ Comparison generation skipped (Python not available)"

echo ""
echo "═══════════════════════════════════════════════════"
echo " Benchmark Suite Complete!"
echo "═══════════════════════════════════════════════════"
echo ""
echo "Results saved to:"
ls -lh "$RESULTS_DIR"/*.txt 2>/dev/null || echo "  (no results found)"
echo ""
echo "View comparison report:"
echo "  cat $RESULTS_DIR/comparison.md"
echo ""
