import { createMDX } from "fumadocs-mdx/next"

/** @type {import('next').NextConfig} */
const config = {
  // Static export for GitHub Pages / S3 / Cloudflare
  output: "export",

  // Use trailing slashes for static hosts
  trailingSlash: true,

  // Required for static export
  images: {
    unoptimized: true,
  },

  basePath: process.env.NEXT_PUBLIC_BASE_PATH || "",
  reactStrictMode: true,
  swcMinify: true,

  typescript: {
    ignoreBuildErrors: true,
  },
}

const withMDX = createMDX()

export default withMDX(config)
