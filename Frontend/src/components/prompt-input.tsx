import { useState, useRef, type KeyboardEvent } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { cn } from "cn"
import "./prompt-input.css"
import {
  ArrowUp01Icon,
  ChevronDownIcon,
  FlashIcon,
  PaperclipIcon,
  PlusSignIcon,
  Shield01Icon,
  ShieldAlertIcon,
} from "@hugeicons/core-free-icons"

const options = [
  { value: "readonly", label: "Read Only", icon: Shield01Icon },
  { value: "auto", label: "Auto", icon: FlashIcon },
  { value: "full", label: "Full Access", icon: ShieldAlertIcon },
]

interface PromptInputProps {
  onSend?: (message: string) => void
  heading?: string
  compact?: boolean
  /** Bottom-docked composer used inside a chat thread. */
  isInChat?: boolean
}

export function PromptInput({
  onSend,
  heading,
  compact = false,
  isInChat = false,
}: PromptInputProps) {
  const [value, setValue] = useState("")
  const [selected, setSelected] = useState("readonly")
  const [open, setOpen] = useState(false)
  const [modelOpen, setModelOpen] = useState(false)
  const ref = useRef<HTMLDivElement>(null)
  const modelRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)

  const current = options.find((o) => o.value === selected)

  const handleSend = () => {
    if (!value.trim()) return
    onSend?.(value.trim())
    setValue("")
    textareaRef.current?.focus()
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  const renderActions = () => (
    <div className="mt-3 flex items-center justify-between">
      <div className="flex items-center gap-2.5">
        <button className="flex size-7 cursor-pointer items-center justify-center rounded-full bg-white/5 text-white">
          <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} className="size-4" />
        </button>
        <button className="flex size-7 cursor-pointer items-center justify-center rounded-full bg-white/5 text-white transition-colors hover:bg-white/10">
          <HugeiconsIcon icon={PaperclipIcon} strokeWidth={2} className="size-4" />
        </button>
        <div ref={ref} className="relative">
          <button
            onClick={() => setOpen(!open)}
            className="flex h-7 cursor-pointer items-center gap-1.5 rounded-full border-transparent bg-white/0 px-2.5 text-xs text-white/70 hover:bg-white/10"
          >
            {current && (
              <HugeiconsIcon
                icon={current.icon}
                strokeWidth={2}
                className="size-3.5"
              />
            )}
            <span>{current?.label}</span>
            <HugeiconsIcon
              icon={ChevronDownIcon}
              strokeWidth={2}
              className={`size-3.5 transition-transform duration-200 ${open ? "rotate-180" : ""}`}
            />
          </button>
          {open && (
            <div className="absolute bottom-full left-0 z-50 mb-1 w-36 overflow-hidden rounded-xl border border-white/10 bg-neutral-900 shadow-lg">
              {options.map((opt) => (
                <button
                  key={opt.value}
                  onClick={() => {
                    setSelected(opt.value)
                    setOpen(false)
                  }}
                  className={`flex w-full cursor-pointer items-center gap-2 px-3 py-1.5 text-xs text-white hover:bg-white/10 ${selected === opt.value ? "bg-white/5" : ""}`}
                >
                  <HugeiconsIcon icon={opt.icon} strokeWidth={2} className="size-3.5" />
                  <span>{opt.label}</span>
                </button>
              ))}
            </div>
          )}
        </div>
      </div>
      <div className="flex items-center gap-2">
        <div ref={modelRef} className="relative">
          <button
            onClick={() => setModelOpen(!modelOpen)}
            className="flex h-7 cursor-pointer items-center gap-1.5 rounded-full bg-white/0 px-2.5 text-xs text-white/70 hover:bg-white/10"
          >
            <span>Select model</span>
            <HugeiconsIcon
              icon={ChevronDownIcon}
              strokeWidth={2}
              className={`size-3.5 transition-transform duration-200 ${modelOpen ? "rotate-180" : ""}`}
            />
          </button>
          {modelOpen && (
            <div className="absolute right-0 bottom-full z-50 mb-1 w-50 max-h-65 overflow-y-auto overflow-x-hidden rounded-xl border border-white/10 bg-neutral-900 shadow-lg">
              <div className="px-3 py-2 text-xs text-white/40">
                No models available — check backend API keys.
              </div>
            </div>
          )}
        </div>
        <button
          onClick={handleSend}
          disabled={!value.trim()}
          className="flex size-8 cursor-pointer items-center justify-center rounded-full bg-primary text-white transition-opacity disabled:cursor-not-allowed disabled:opacity-50"
        >
          <HugeiconsIcon icon={ArrowUp01Icon} strokeWidth={2} className="size-5" />
        </button>
      </div>
    </div>
  )

  const renderComposer = () => (
    <div
      className={cn(
        "w-full rounded-2xl border border-white/10 border-l-2 border-l-primary px-4 pt-4 pb-2 shadow-sm",
        isInChat
          ? "bg-neutral-900/80 backdrop-blur-xl"
          : "bg-white/5 backdrop-blur-[2px]"
      )}
    >
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        placeholder="Describe what you want to build, / Commands @ files or sessions"
        rows={2}
        className="w-full resize-none bg-transparent px-1 text-sm text-white placeholder:text-white/50 outline-none"
      />
      {renderActions()}
    </div>
  )

  if (isInChat) {
    return (
      <div className="w-full p-4">
        <div className="mx-auto w-full max-w-[780px]">{renderComposer()}</div>
      </div>
    )
  }

  return (
    <div className="flex h-full -mt-20 items-center justify-center p-4">
      <div className="flex w-full max-w-[780px] flex-col items-center gap-5">
        <div className="flex items-center gap-2.5">
          <img
            src="/vite.svg"
            alt="Calabrass"
            className={`prompt-logo ${compact ? "size-5" : "size-7"}`}
          />
          <h1
            className={`prompt-heading ${compact ? "text-lg font-medium" : "text-2xl font-medium"}`}
          >
            {heading || "What can I do for you?"}
          </h1>
        </div>
        {renderComposer()}
      </div>
    </div>
  )
}

export default PromptInput
