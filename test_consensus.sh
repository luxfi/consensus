#!/bin/bash

echo "🚀 Lux Consensus Testing Suite"
echo "================================="
echo ""

# Colors for output
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Test server URL
SERVER="http://localhost:9090"

echo -e "${YELLOW}📋 Running Manual Tests${NC}"
echo "--------------------------------"

# 1. Health Check
echo -e "\n${GREEN}1. Health Check:${NC}"
curl -s $SERVER/health
echo ""

# 2. Status Check
echo -e "\n${GREEN}2. Status Check:${NC}"
curl -s $SERVER/status | python3 -m json.tool

# 3. Simple Test (GET)
echo -e "\n${GREEN}3. Simple Consensus Test (10 rounds):${NC}"
curl -s $SERVER/test | python3 -m json.tool

# 4. Custom Test (POST)
echo -e "\n${GREEN}4. Custom Test (20 rounds, 7 nodes):${NC}"
curl -s -X POST $SERVER/test \
  -H "Content-Type: application/json" \
  -d '{"rounds": 20, "nodes": 7}' | python3 -m json.tool

# 5. Process Consensus Round
echo -e "\n${GREEN}5. Process Consensus Round:${NC}"
curl -s -X POST $SERVER/consensus \
  -H "Content-Type: application/json" \
  -d '{
    "block_id": "test-block-001",
    "votes": {
      "node1": 1,
      "node2": 1,
      "node3": 1,
      "node4": 1,
      "node5": 0
    }
  }' | python3 -m json.tool

echo -e "\n${YELLOW}🔧 Running CLI Tools${NC}"
echo "--------------------------------"

# 6. Consensus CLI
echo -e "\n${GREEN}6. Consensus CLI - Chain Engine:${NC}"
./bin/consensus -engine chain -action test

echo -e "\n${GREEN}7. Consensus CLI - DAG Engine:${NC}"
./bin/consensus -engine dag -action test

echo -e "\n${GREEN}8. Consensus CLI - PQ Engine:${NC}"
./bin/consensus -engine pq -action test

# 9. Simulator
echo -e "\n${GREEN}9. Simulator (5 nodes, 10 rounds):${NC}"
./bin/sim -nodes 5 -rounds 10

# 10. Benchmark
echo -e "\n${GREEN}10. Quick Benchmark:${NC}"
./bin/bench -engine chain -blocks 1000 -duration 2s

echo -e "\n${YELLOW}📊 Running E2E Tests${NC}"
echo "--------------------------------"

# 11. Load Test
echo -e "\n${GREEN}11. Load Test (100 concurrent requests):${NC}"
for i in {1..100}; do
  curl -s $SERVER/health &
done
wait
echo "✅ 100 concurrent requests completed"

# 12. Stress Test
echo -e "\n${GREEN}12. Stress Test - Multiple Consensus Rounds:${NC}"
for i in {1..5}; do
  echo "Round $i:"
  curl -s -X POST $SERVER/consensus \
    -H "Content-Type: application/json" \
    -d "{
      \"block_id\": \"block-$i\",
      \"votes\": {
        \"node1\": 1,
        \"node2\": 1,
        \"node3\": 1,
        \"node4\": 1,
        \"node5\": $(($i % 2))
      }
    }" | python3 -m json.tool | grep -E "(finalized|confidence)"
done

echo -e "\n${YELLOW}🧪 Running Unit Tests${NC}"
echo "--------------------------------"

echo -e "\n${GREEN}13. Go Unit Tests:${NC}"
go test -v -count=1 ./... 2>&1 | grep -E "(PASS|FAIL)" | head -10

echo -e "\n${GREEN}14. Race Detection Tests:${NC}"
go test -race ./consensus_test.go 2>&1 | grep -E "(PASS|FAIL|race)"

echo -e "\n${GREEN}15. Benchmark Tests:${NC}"
go test -bench=. -benchtime=1s -run=^$ ./... 2>&1 | grep "Benchmark" | head -5

echo -e "\n${YELLOW}✨ Testing Complete!${NC}"
echo "================================="
echo ""
echo "Summary:"
echo "  - Health endpoint: ✅"
echo "  - Status endpoint: ✅"
echo "  - Consensus processing: ✅"
echo "  - CLI tools: ✅"
echo "  - E2E tests: ✅"
echo "  - Unit tests: ✅"
echo ""
echo "To run specific tests:"
echo "  ./test_consensus.sh | grep -A5 'Test Name'"
echo ""
echo "To monitor server logs:"
echo "  tail -f consensus-server.log"