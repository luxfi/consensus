#!/bin/bash

# Run consensus benchmarks with different node counts

echo "Lux Consensus Benchmarks"
echo "========================"
echo ""

# Test with different node counts
for nodes in 5 10 15 20; do
    echo "Testing with $nodes nodes:"
    echo "-------------------------"
    
    # Run simulation with local parameters
    echo "Simulation test:"
    ./bin/sim -network local -nodes $nodes -rounds 100 -sims 10
    
    echo ""
    echo "Parameter analysis for $nodes nodes:"
    ./bin/params -nodes $nodes -summary
    
    echo ""
    echo "Safety check for $nodes nodes:"
    ./bin/checker -network local -nodes $nodes -analyze
    
    echo ""
    echo "=================================="
    echo ""
done

# Test high TPS configuration
echo "High TPS Configuration Test:"
echo "---------------------------"
./bin/params -preset hightps -check
echo ""
./bin/sim -network hightps -nodes 10 -rounds 100 -sims 10

echo ""
echo "Benchmark complete!"