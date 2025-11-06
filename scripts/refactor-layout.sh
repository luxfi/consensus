#!/usr/bin/env bash
set -euo pipefail

echo "🔄 Starting Lux Consensus Go-Idiomatic Refactor..."
echo "=================================================="

# Safety checks
git rev-parse --is-inside-work-tree >/dev/null || { echo "❌ Not a git repo"; exit 1; }

# Create refactor branch
BRANCH="refactor/go-idiomatic-layout-$(date +%Y%m%d)"
git checkout -b "$BRANCH" || { echo "⚠️  Branch exists, using it"; git checkout "$BRANCH"; }

MOD="github.com/luxfi/consensus"

echo "✅ Created branch: $BRANCH"
echo ""

# 1) Protocol consolidation - move top-level protocol packages
echo "📦 Step 1: Moving protocol packages..."
mkdir -p protocol/prism protocol/photon protocol/wave/fpc

# Move prism
if [ -d "prism" ]; then
  echo "  Moving prism/ → protocol/prism/"
  git mv prism/* protocol/prism/ 2>/dev/null || cp -r prism/* protocol/prism/
  rmdir prism 2>/dev/null || rm -rf prism
fi

# Move photon
if [ -d "photon" ]; then
  echo "  Moving photon/ → protocol/photon/"
  git mv photon/* protocol/photon/ 2>/dev/null || cp -r photon/* protocol/photon/
  rmdir photon 2>/dev/null || rm -rf photon
fi

# Move wave (keeping fpc subdirectory)
if [ -d "wave" ] && [ ! -d "protocol/wave/wave.go" ]; then
  echo "  Moving wave/ → protocol/wave/"
  git mv wave/* protocol/wave/ 2>/dev/null || cp -r wave/* protocol/wave/
  rmdir wave 2>/dev/null || rm -rf wave
fi

echo "✅ Protocol packages consolidated"
echo ""

# 2) Singularize validators → validator
echo "📦 Step 2: Singularizing package names..."
if [ -d "validators" ]; then
  echo "  Renaming validators/ → validator/"
  git mv validators validator 2>/dev/null || mv validators validator
fi

echo "✅ Package names singularized"
echo ""

# 3) Update import paths
echo "📝 Step 3: Updating import paths..."

# Find and replace import paths in all Go files
find . -name "*.go" -type f -not -path "./.git/*" -not -path "./node/*" -not -path "./pkg/*" -exec sed -i '' \
  -e "s|$MOD/prism|$MOD/protocol/prism|g" \
  -e "s|$MOD/photon|$MOD/protocol/photon|g" \
  -e "s|$MOD/wave|$MOD/protocol/wave|g" \
  -e "s|$MOD/validators|$MOD/validator|g" \
  {} \;

echo "✅ Import paths updated"
echo ""

# 4) Tidy and format
echo "🧹 Step 4: Tidying and formatting..."
go mod tidy
gofmt -w .

echo "✅ Code formatted"
echo ""

# 5) Run tests
echo "🧪 Step 5: Running tests..."
if go test ./... -count=1; then
  echo "✅ All tests passing"
else
  echo "⚠️  Some tests failed - review needed"
  exit 1
fi

echo ""
echo "=================================================="
echo "✅ Refactor complete!"
echo ""
echo "Next steps:"
echo "  1. Review changes: git status"
echo "  2. Run tests: go test ./..."
echo "  3. Commit: git commit -am 'refactor: Go-idiomatic package layout'"
echo ""

