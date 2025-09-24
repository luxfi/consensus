#!/bin/bash

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

echo -e "${BLUE}🚀 Starting Lux Consensus Documentation Server${NC}"
echo ""

# Kill any existing server on port 8000
lsof -ti:8000 | xargs kill -9 2>/dev/null

# Change to docs directory
cd "$(dirname "$0")/docs"

# Start the server
echo -e "${GREEN}✅ Documentation server starting...${NC}"
echo ""
echo -e "${YELLOW}📚 Documentation available at:${NC}"
echo -e "${GREEN}   http://localhost:8000${NC}"
echo ""
echo -e "${YELLOW}📖 Quick Links:${NC}"
echo -e "   Main Page:        ${BLUE}http://localhost:8000/${NC}"
echo -e "   C Docs:          ${BLUE}http://localhost:8000/c/${NC}"
echo -e "   C++ Docs:        ${BLUE}http://localhost:8000/cpp/${NC}"
echo -e "   Go Docs:         ${BLUE}http://localhost:8000/go/${NC}"
echo -e "   Python Docs:     ${BLUE}http://localhost:8000/python/${NC}"
echo -e "   Rust Docs:       ${BLUE}http://localhost:8000/rust/${NC}"
echo ""
echo -e "${YELLOW}Press Ctrl+C to stop the server${NC}"
echo ""

# Start Python HTTP server
python3 -m http.server 8000