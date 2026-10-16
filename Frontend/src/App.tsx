import {
  SidebarInset,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar"
import { AppSidebar } from "@/components/app-sidebar"
import WaveBackground from "@/components/wave-background"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import { PromptInput } from "@/components/prompt-input"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  BubbleChatTemporaryIcon,
  Files01Icon,
  GiftIcon,
} from "@hugeicons/core-free-icons"

export function App() {
  return (
    <SidebarProvider>
      <AppSidebar />
      <SidebarInset className="relative overflow-hidden bg-background">
        <WaveBackground fixed />
        <header className="relative z-10 flex h-16 shrink-0 items-center gap-2 transition-[width,height] ease-linear">
          <div className="flex items-center gap-2 px-4">
            <SidebarTrigger className="-ml-1 size-[29px]" />
          </div>

          {/* Centered badge */}
          <div className="pointer-events-none absolute inset-x-0 flex justify-center">
            <Badge className="pointer-events-auto bg-sky-50 text-sky-700 dark:bg-sky-950 dark:text-sky-300 h-6 gap-1 px-2.5 text-xs font-medium">
              <HugeiconsIcon icon={GiftIcon} strokeWidth={2} />
              Upgrade for free
            </Badge>
          </div>

          {/* Right corner icons — same size as the sidebar trigger button */}
          <div className="ml-auto flex items-center gap-1 px-4">
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Files"
              className="size-[29px]"
            >
              <HugeiconsIcon icon={Files01Icon} strokeWidth={2} />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              aria-label="Messages"
              className="size-[29px]"
            >
              <HugeiconsIcon icon={BubbleChatTemporaryIcon} strokeWidth={2} />
            </Button>
          </div>
        </header>
        <main className="relative z-10 flex flex-1 flex-col items-center justify-center p-4">
          <PromptInput onSubmit={(value) => console.log("prompt:", value)} />
        </main>
      </SidebarInset>
    </SidebarProvider>
  )
}

export default App
