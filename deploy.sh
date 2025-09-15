#!/bin/bash

echo "🚀 Deploying Lux Consensus Documentation to ui.lux.finance"

# Build the site
echo "📦 Building the site..."
cd docs-site
npm install
npm run build
cd ..

echo "✅ Build complete!"
echo ""
echo "📋 Next steps to deploy to ui.lux.finance:"
echo ""
echo "1. Using Vercel CLI:"
echo "   npm i -g vercel"
echo "   vercel --prod"
echo "   Then add custom domain: ui.lux.finance"
echo ""
echo "2. Using GitHub Pages:"
echo "   - Push to GitHub"
echo "   - Enable GitHub Pages in repository settings"
echo "   - Add CNAME record for ui.lux.finance"
echo ""
echo "3. Using Netlify:"
echo "   - Connect GitHub repo to Netlify"
echo "   - Set build command: cd docs-site && npm run build"
echo "   - Set publish directory: docs"
echo "   - Add custom domain: ui.lux.finance"
echo ""
echo "Built files are in: ./docs/"