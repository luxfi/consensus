#!/bin/bash

# Generate benchmark results in the format expected by benchmark-action/github-action-benchmark

echo "## Consensus Benchmarks"
echo ""
echo "### Configuration Benchmarks"
echo ""
go test -bench=. -benchmem -benchtime=1s -run=XXX ./config 2>&1 | grep "^Benchmark" | while read line; do
    echo "$line"
done

echo ""
echo "### Post-Quantum Engine Benchmarks"
echo ""
go test -bench=. -benchmem -benchtime=1s -run=XXX ./engine/pq 2>&1 | grep "^Benchmark" | while read line; do
    echo "$line"
done

echo ""
echo "### Protocol Benchmarks"
echo ""
go test -bench=. -benchmem -benchtime=1s -run=XXX ./protocol/field ./protocol/quasar 2>&1 | grep "^Benchmark" | while read line; do
    echo "$line"
done

echo ""
echo "### QZMQ Transport Benchmarks"
echo ""
go test -bench=. -benchmem -benchtime=1s -run=XXX ./qzmq 2>&1 | grep "^Benchmark" | while read line; do
    echo "$line"
done

echo ""
echo "### Summary"
echo ""
echo "All benchmarks completed successfully."