import type { MDXComponents } from "mdx/types"
import defaultMdxComponents from "@hanzo/ui/docs/mdx"

export function useMDXComponents(components: MDXComponents): MDXComponents {
  return {
    ...defaultMdxComponents,
    ...components,
  }
}
