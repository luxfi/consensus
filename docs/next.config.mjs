import { createMDX } from "fumadocs-mdx/next"

// GitHub Pages deployment uses /<repo-name> as base path
const isGithubActions = process.env.GITHUB_ACTIONS === 'true'
const basePath = isGithubActions ? '/consensus' : ''

/** @type {import('next').NextConfig} */
const config = {
  output: 'export',
  basePath,
  assetPrefix: basePath,
  reactStrictMode: true,
  typescript: {
    ignoreBuildErrors: true,
  },
  experimental: {
    webpackBuildWorker: true,
  },
  images: {
    unoptimized: true,
  },
}

const withMDX = createMDX()

export default withMDX(config)
