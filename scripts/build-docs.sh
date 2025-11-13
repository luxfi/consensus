#!/bin/bash
set -e

echo "🏗️  Building Lux Consensus Documentation..."

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Navigate to docs directory
cd "$(dirname "$0")/../docs"

echo -e "${BLUE}📦 Installing dependencies...${NC}"
if command -v pnpm &> /dev/null; then
    pnpm install
else
    npm install
fi

echo -e "${BLUE}🔨 Building site...${NC}"
if command -v pnpm &> /dev/null; then
    pnpm build
else
    npm run build
fi

echo -e "${BLUE}📤 Exporting static site...${NC}"
if command -v pnpm &> /dev/null; then
    pnpm export
else
    npm run export
fi

echo -e "${GREEN}✅ Documentation built successfully!${NC}"
echo -e "${GREEN}📁 Output directory: docs/out${NC}"

# Optional: Start preview server
read -p "Start preview server? (y/n) " -n 1 -r
echo
if [[ $REPLY =~ ^[Yy]$ ]]; then
    echo -e "${BLUE}🌐 Starting preview server on http://localhost:3001${NC}"
    npx serve out -p 3001
fi