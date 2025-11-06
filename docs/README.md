# Lux Consensus Documentation

Beautiful documentation site built with [Fumadocs](https://fumadocs.vercel.app/) and [@hanzo/ui](https://github.com/hanzoai/ui).

## Development

```bash
cd docs
pnpm install
pnpm dev
```

Visit http://localhost:3001

## Build

```bash
pnpm build
pnpm start
```

## Features

- 🌑 **Dark Mode First**: Beautiful dark theme by default
- ⚡ **Fast**: Built on Next.js 16 with RSC
- 🎨 **@hanzo/ui**: Uses Hanzo AI design system
- 📚 **Fumadocs**: Powerful MDX documentation framework
- 🔍 **Search**: Built-in search functionality
- 📱 **Responsive**: Mobile-friendly design
- 🎯 **Type-safe**: Full TypeScript support

## Structure

```
docs/
├── app/                    # Next.js app directory
│   ├── docs/              # Documentation pages
│   ├── layout.tsx         # Root layout
│   └── global.css         # Global styles
├── content/               # MDX documentation
│   └── docs/
│       ├── index.mdx      # Homepage
│       ├── sdk/           # SDK documentation
│       │   ├── go.mdx
│       │   ├── c.mdx
│       │   ├── rust.mdx
│       │   ├── python.mdx
│       │   └── cpp.mdx
│       └── benchmarks.mdx # Performance benchmarks
├── components/            # React components
├── source.config.ts       # Fumadocs configuration
├── tailwind.config.ts     # Tailwind CSS config
└── package.json           # Dependencies
```

## Adding Documentation

1. Create a new `.mdx` file in `content/docs/`
2. Add frontmatter:

```mdx
---
title: Your Page Title
description: Brief description
---

# Content goes here...
```

3. The page will automatically appear in the sidebar

## SDK Documentation

Each language SDK has its own documentation page:

- **Go**: Complete API reference with examples
- **C**: Native C API for embedded systems
- **Rust**: Safe Rust bindings
- **Python**: Pythonic API for research
- **C++**: GPU-accelerated with MLX

## Benchmarks

Real benchmark results from:
- Apple M1 Max
- Go 1.24.5
- Latest C, Rust, Python implementations

Updated automatically from `../benchmarks/results/`.

## Deployment

Deploy to Vercel, Netlify, or any static hosting:

```bash
pnpm build
# Output in .next/
```

## License

Copyright (C) 2025, Lux Industries Inc. All rights reserved.
