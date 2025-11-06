import {
  defineConfig,
  defineDocs,
  frontmatterSchema,
} from "fumadocs-mdx/config"
import rehypePrettyCode from "rehype-pretty-code"
import { z } from "zod"

export default defineConfig({
  mdxOptions: {
    rehypePlugins: [
      [
        rehypePrettyCode,
        {
          theme: {
            dark: "github-dark-dimmed",
            light: "github-light",
          },
          keepBackground: false,
          defaultLang: "go",
        },
      ],
    ],
  },
})

export const docs = defineDocs({
  dir: "content/docs",
  docs: {
    schema: frontmatterSchema.extend({
      language: z.enum(["go", "c", "rust", "python", "cpp"]).optional(),
      sdk: z.boolean().optional(),
      benchmark: z.boolean().optional(),
    }),
  },
})
