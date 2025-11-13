#!/bin/bash
set -e

echo "🚀 Deploying Lux Consensus Documentation to GitHub Pages..."

# Colors for output
GREEN='\033[0;32m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Get repository information
REPO_URL=$(git remote get-url origin)
REPO_NAME=$(basename -s .git "$REPO_URL")
REPO_OWNER=$(echo "$REPO_URL" | sed -n 's/.*github.com[:/]\([^/]*\).*/\1/p')

echo -e "${BLUE}📦 Repository: ${REPO_OWNER}/${REPO_NAME}${NC}"

# Navigate to project root
cd "$(dirname "$0")/.."

# Check if docs/out exists
if [ ! -d "docs/out" ]; then
    echo -e "${RED}❌ Error: docs/out directory not found${NC}"
    echo "Please run 'scripts/build-docs.sh' first"
    exit 1
fi

# Create temporary directory for deployment
TEMP_DIR=$(mktemp -d)
echo -e "${BLUE}📁 Using temp directory: ${TEMP_DIR}${NC}"

# Copy built site to temp directory
cp -r docs/out/* "$TEMP_DIR/"

# Initialize git in temp directory
cd "$TEMP_DIR"
git init
git config user.name "GitHub Actions"
git config user.email "actions@github.com"

# Create CNAME file for custom domain (if needed)
echo "consensus.lux.network" > CNAME

# Add .nojekyll to prevent Jekyll processing
touch .nojekyll

# Commit all files
git add -A
git commit -m "Deploy documentation $(date '+%Y-%m-%d %H:%M:%S')"

# Force push to gh-pages branch
echo -e "${BLUE}📤 Pushing to gh-pages branch...${NC}"
git push -f "$REPO_URL" HEAD:gh-pages

# Clean up
cd - > /dev/null
rm -rf "$TEMP_DIR"

echo -e "${GREEN}✅ Documentation deployed successfully!${NC}"
echo -e "${GREEN}🌐 View at: https://${REPO_OWNER}.github.io/${REPO_NAME}${NC}"
echo -e "${GREEN}🌐 Or: https://consensus.lux.network${NC}"

# Optional: Open in browser
if command -v open &> /dev/null; then
    read -p "Open in browser? (y/n) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        open "https://${REPO_OWNER}.github.io/${REPO_NAME}"
    fi
fi