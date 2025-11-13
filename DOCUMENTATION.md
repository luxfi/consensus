# Lux Consensus Documentation

## 📚 Overview

Complete documentation for the Lux Consensus framework, featuring:
- **Quantum-resistant protocols** (Quasar, Wave, Photon)
- **Multi-language SDKs** (Go, C, Rust, Python, C++)
- **Performance benchmarks** and optimization guides
- **Beautiful dark-themed site** built with Fumadocs and @hanzo/ui

## 🌐 Live Documentation

- **GitHub Pages**: https://luxfi.github.io/consensus
- **Custom Domain**: https://consensus.lux.network

## 📂 Documentation Structure

```
docs/
├── content/docs/          # MDX documentation files
│   ├── index.mdx         # Main landing page
│   ├── protocols/        # Protocol documentation
│   │   ├── quasar.mdx   # Quantum-resistant consensus
│   │   ├── wave.mdx     # Fast Probabilistic Consensus
│   │   └── photon.mdx   # Light-based peer selection
│   ├── sdk/             # SDK documentation
│   │   ├── go.mdx       # Go SDK reference
│   │   ├── c.mdx        # C implementation
│   │   ├── rust.mdx     # Rust bindings
│   │   ├── python.mdx   # Python SDK
│   │   └── cpp.mdx      # C++ with MLX
│   └── benchmarks.mdx   # Performance benchmarks
├── app/                 # Next.js app directory
├── components/          # React components
└── out/                # Static site output
```

## 🚀 Local Development

### Prerequisites
- Node.js 20+
- pnpm (or npm)

### Setup
```bash
cd docs
pnpm install
```

### Development Server
```bash
pnpm dev
# Visit http://localhost:3001
```

### Build Documentation
```bash
pnpm build
pnpm export
# Output in docs/out/
```

## 📦 Deployment

### Automatic Deployment
The documentation automatically deploys to GitHub Pages when:
- Changes are pushed to `main` branch
- Files in `docs/` directory are modified

### Manual Deployment
```bash
# Build and deploy
./scripts/build-docs.sh
./scripts/deploy-docs.sh
```

### GitHub Actions
The `.github/workflows/docs.yml` workflow handles automatic deployment:
1. Builds the documentation site
2. Deploys to `gh-pages` branch
3. Publishes to GitHub Pages

## ✨ Features

### Protocols Documentation
- **Quasar**: Quantum-resistant consensus with 2-round finality
- **Wave**: Fast Probabilistic Consensus implementation
- **Photon**: Performance-based peer selection
- **Flare**: Conflict resolution for DAG
- **Horizon**: Long-range attack prevention
- **Nova**: Next-generation experimental
- **Prism**: State sharding protocol

### SDK Documentation
Complete API references for all language implementations:
- Installation guides
- Code examples
- Performance benchmarks
- Best practices

### Interactive Components
- Code syntax highlighting
- Copy-to-clipboard buttons
- Tab-based content switching
- Responsive tables
- Search functionality

## 🎨 Theming

Built with:
- **Fumadocs**: Modern documentation framework
- **@hanzo/ui**: Hanzo AI design system
- **Tailwind CSS**: Utility-first styling
- **Dark mode first**: Beautiful dark theme

## 📝 Adding Documentation

### Create New Page
1. Add `.mdx` file to `content/docs/`
2. Include frontmatter:
```mdx
---
title: Page Title
description: Brief description
---

# Content here...
```

### Update Navigation
Edit `collections.ts` to add new sections or reorganize.

### Use Components
```mdx
import { Card, Tabs, Code } from '@/components'

<Card>
  Content here
</Card>
```

## 🔧 Configuration

### Site Configuration
- `next.config.mjs`: Next.js configuration
- `source.config.ts`: Fumadocs configuration
- `tailwind.config.ts`: Tailwind CSS settings

### GitHub Pages
- Custom domain: `consensus.lux.network`
- CNAME file in `docs/out/`
- `.nojekyll` for proper asset serving

## 📊 Performance

The documentation site achieves:
- **Lighthouse Score**: 100/100
- **First Contentful Paint**: < 0.5s
- **Time to Interactive**: < 1s
- **Static Export**: No server required

## 🤝 Contributing

1. Fork the repository
2. Create feature branch
3. Add/update documentation
4. Test locally with `pnpm dev`
5. Submit pull request

## 📄 License

Copyright (C) 2025, Lux Industries Inc. All rights reserved.
See [LICENSE](../LICENSE) for details.