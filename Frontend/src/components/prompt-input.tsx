import * as React from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { SentIcon } from "@hugeicons/core-free-icons"
import { cn } from "cn"

interface PromptInputProps {
  /** Placeholder text, shown at reduced opacity */
  placeholder?: string
  /** Called with the trimmed text on submit (send button or Enter) */
  onSubmit?: (value: string) => void
  className?: string
}

/**
 * Horizontal prompt card with a 2px primary left border, slight mica
 * transparency (so the wave background shows through), an auto-growing
 * textarea and a send button that is disabled until there is text.
 *
 * Enter submits, Shift+Enter inserts a newline.
 */
export function PromptInput({
  placeholder = "Describe want you want to do, / commands @ files or session",
  onSubmit,
  className,
}: PromptInputProps) {
  const [value, setValue] = React.useState("")
  const textareaRef = React.useRef<HTMLTextAreaElement>(null)

  const canSend = value.trim().length > 0

  // Auto-grow: reset to auto height, then grow up to the cap (~4 rows).
  React.useEffect(() => {
    const el = textareaRef.current
    if (!el) return
    el.style.height = "auto"
    el.style.height = `${Math.min(el.scrollHeight, 160)}px`
  }, [value])

  const submit = () => {
    if (!canSend) return
    onSubmit?.(value.trim())
    setValue("")
  }

  const handleKeyDown = (event: React.KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key === "Enter" && !event.shiftKey) {
      event.preventDefault()
      submit()
    }
  }

  return (
    <div
      className={cn(
        "relative flex w-full max-w-2xl items-end gap-2 rounded-2xl border border-border/60 border-l-2 border-l-primary bg-background/90 p-3 shadow-md backdrop-blur-sm",
        className
      )}
      onClick={() => textareaRef.current?.focus()}
    >
      <textarea
        ref={textareaRef}
        rows={1}
        value={value}
        onChange={(event) => setValue(event.target.value)}
        onKeyDown={handleKeyDown}
        placeholder={placeholder}
        aria-label="Prompt"
        className="max-h-40 min-h-8 flex-1 resize-none overflow-hidden bg-transparent text-sm leading-6 text-foreground outline-none placeholder:text-muted-foreground/60"
      />
      <button
        type="button"
        onClick={submit}
        disabled={!canSend}
        aria-label="Send prompt"
        className={cn(
          "flex size-8 shrink-0 items-center justify-center rounded-full transition-colors",
          canSend
            ? "bg-primary text-primary-foreground hover:bg-primary/90"
            : "cursor-not-allowed bg-muted text-muted-foreground/60"
        )}
      >
        <HugeiconsIcon icon={SentIcon} strokeWidth={2} />
      </button>
    </div>
  )
}

export default PromptInput
