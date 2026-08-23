import "./global.css"
import { RootProvider } from "@hanzo/docs-ui/provider/next"
import { ZenMono } from "@hanzo/font/mono"
import { ZenSans } from "@hanzo/font/sans"
import type { ReactNode } from "react"

export const metadata = {
  title: {
    default: "Lux Consensus Documentation",
    template: "%s | Lux Consensus",
  },
  description: "Quasar consensus engine with post-quantum finality for Lux blockchain",
}

export default function Layout({ children }: { children: ReactNode }) {
  return (
    <html
      lang="en"
      className={`${ZenSans.variable} ${ZenMono.variable}`}
      suppressHydrationWarning
    >
      <body className="min-h-svh bg-background font-sans antialiased">
        <RootProvider
          search={{
            enabled: true,
          }}
          theme={{
            enabled: true,
            defaultTheme: "dark",
          }}
        >
          <div className="relative flex min-h-svh flex-col bg-background">
            {children}
          </div>
        </RootProvider>
      </body>
    </html>
  )
}
