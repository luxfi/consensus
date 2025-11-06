import { docs } from "@/.source"
import { DocsLayout } from "fumadocs-ui/layouts/docs"
import type { ReactNode } from "react"
import { BookOpen, Code, Cpu, Zap } from "lucide-react"

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <DocsLayout
      tree={docs.pageTree}
      nav={{
        title: (
          <div className="flex items-center gap-2">
            <Zap className="size-6 text-lux-400" />
            <span className="font-bold">Lux Consensus</span>
          </div>
        ),
        transparentMode: "top",
      }}
      sidebar={{
        defaultOpenLevel: 0,
        banner: (
          <div className="rounded-lg bg-gradient-to-br from-lux-500 to-lux-700 p-4 text-white">
            <h3 className="text-sm font-semibold">v1.21.0 Released! 🎉</h3>
            <p className="mt-1 text-xs opacity-90">
              Multi-language SDK with quantum integration
            </p>
          </div>
        ),
        footer: (
          <div className="flex flex-col gap-2 p-4 text-xs text-muted-foreground">
            <a
              href="https://github.com/luxfi/consensus"
              className="hover:text-foreground"
            >
              GitHub
            </a>
            <a href="https://lux.fi" className="hover:text-foreground">
              Lux Network
            </a>
          </div>
        ),
      }}
      links={[
        {
          text: "Documentation",
          url: "/docs",
          icon: <BookOpen className="size-4" />,
        },
        {
          text: "SDK",
          url: "/docs/sdk",
          icon: <Code className="size-4" />,
        },
        {
          text: "Benchmarks",
          url: "/docs/benchmarks",
          icon: <Cpu className="size-4" />,
        },
      ]}
    >
      {children}
    </DocsLayout>
  )
}
