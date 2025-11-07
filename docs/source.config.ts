import {
  defineConfig,
  defineDocs,
} from "fumadocs-mdx/config"
import rehypePrettyCode from "rehype-pretty-code"

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
  docs: { async: true },  // Enable async mode to avoid bundling MDX at build
})
