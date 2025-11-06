#!/usr/bin/env bash
set -euo pipefail

echo "🔄 Consolidating top-level packages..."
echo "======================================"

MOD="github.com/luxfi/consensus"

# 1. Move test-related packages to test/
echo "📦 Moving e2e/ → test/e2e/"
if [ -d "e2e" ] && [ ! -d "test/e2e" ]; then
    git mv e2e test/
fi

echo "📦 Moving consensustest/ → test/helpers/"
if [ -d "consensustest" ]; then
    mkdir -p test/helpers
    git mv consensustest/* test/helpers/ 2>/dev/null || cp -r consensustest/* test/helpers/
    rm -rf consensustest
fi

# 2. Move core primitives to core/
echo "📦 Moving block/ → core/block/"
if [ -d "block" ] && [ ! -d "core/block" ]; then
    git mv block core/
fi

echo "📦 Moving choices/ → core/choices/"
if [ -d "choices" ] && [ ! -d "core/choices" ]; then
    git mv choices core/
fi

echo "📦 Moving dag/ → core/dag/"
if [ -d "dag" ] && [ ! -d "core/dag" ]; then
    git mv dag core/
fi

echo "📦 Moving verify/ → core/verify/"
if [ -d "verify" ] && [ ! -d "core/verify" ]; then
    git mv verify core/
fi

echo "📦 Moving router/ → core/router/"
if [ -d "router" ] && [ ! -d "core/router" ]; then
    git mv router core/
fi

# 3. Handle types/ (merge with existing core/types.go)
echo "📦 Consolidating types/ into core/types/"
if [ -d "types" ]; then
    # Move types directory contents to core/types/
    if [ ! -d "core/types" ]; then
        git mv types core/
    else
        # Merge if core/types already exists
        for f in types/*; do
            filename=$(basename "$f")
            if [ -f "core/types/$filename" ]; then
                echo "⚠️  $filename already exists in core/types, skipping"
            else
                git mv "$f" core/types/
            fi
        done
        rmdir types 2>/dev/null || rm -rf types
    fi
fi

# 4. Move utilities to utils/
echo "📦 Moving codec/ → utils/codec/"
if [ -d "codec" ] && [ ! -d "utils/codec" ]; then
    git mv codec utils/
fi

echo "📦 Moving witness/ → utils/witness/"
if [ -d "witness" ] && [ ! -d "utils/witness" ]; then
    git mv witness utils/
fi

# 5. Move validator-related to validator/
echo "📦 Moving uptime/ → validator/uptime/"
if [ -d "uptime" ] && [ ! -d "validator/uptime" ]; then
    git mv uptime validator/
fi

# 6. Update import paths
echo "📝 Updating import paths..."

find . -name "*.go" -type f -not -path "./.git/*" -not -path "./node/*" -exec sed -i '' \
  -e "s|\"$MOD/e2e|\"$MOD/test/e2e|g" \
  -e "s|\"$MOD/consensustest|\"$MOD/test/helpers|g" \
  -e "s|\"$MOD/block\"|\"$MOD/core/block\"|g" \
  -e "s|\"$MOD/choices\"|\"$MOD/core/choices\"|g" \
  -e "s|\"$MOD/dag\"|\"$MOD/core/dag\"|g" \
  -e "s|\"$MOD/verify\"|\"$MOD/core/verify\"|g" \
  -e "s|\"$MOD/router\"|\"$MOD/core/router\"|g" \
  -e "s|\"$MOD/types\"|\"$MOD/core/types\"|g" \
  -e "s|\"$MOD/codec\"|\"$MOD/utils/codec\"|g" \
  -e "s|\"$MOD/witness\"|\"$MOD/utils/witness\"|g" \
  -e "s|\"$MOD/uptime|\"$MOD/validator/uptime|g" \
  {} \;

# 7. Tidy and format
echo "🧹 Tidying and formatting..."
go mod tidy
gofmt -w .

# 8. Run tests
echo "🧪 Running tests..."
if go test ./... -count=1 -short; then
  echo "✅ All tests passing"
else
  echo "⚠️  Some tests failed - review needed"
  exit 1
fi

echo ""
echo "======================================"
echo "✅ Package consolidation complete!"
echo ""
echo "Test packages → test/:"
echo "  - e2e/ → test/e2e/"
echo "  - consensustest/ → test/helpers/"
echo ""
echo "Core primitives → core/:"
echo "  - block/ → core/block/"
echo "  - choices/ → core/choices/"
echo "  - dag/ → core/dag/"
echo "  - verify/ → core/verify/"
echo "  - router/ → core/router/"
echo "  - types/ → core/types/"
echo ""
echo "Utilities → utils/:"
echo "  - codec/ → utils/codec/"
echo "  - witness/ → utils/witness/"
echo ""
echo "Validator-related → validator/:"
echo "  - uptime/ → validator/uptime/"
echo ""
