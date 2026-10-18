import * as React from "react"
import { cn } from "cn"

function Textarea({ className, ...props }: React.ComponentProps<"textarea">) {
  return (
    <textarea
      data-slot="textarea"
      className={cn(
        "min-h-20 w-full resize-none rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm text-white/80 transition-colors outline-none placeholder:text-white/30 hover:border-white/15 focus:border-white/25 disabled:cursor-not-allowed disabled:opacity-50",
        className
      )}
      {...props}
    />
  )
}

export { Textarea }
