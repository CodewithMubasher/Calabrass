import { useState, useRef, type ChangeEvent, type KeyboardEvent } from "react"
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
  Tick02Icon,
} from "@hugeicons/core-free-icons"

import { Attachment } from "@/components/attachment"
import { attachmentFromFile, type AttachmentFile } from "@/lib/attachments"
import { selectModel, useModelState } from "@/lib/model-store"

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
  /** Blocks input while a reply is still streaming. */
  disabled?: boolean
}

export function PromptInput({
  onSend,
  heading,
  compact = false,
  isInChat = false,
  disabled = false,
}: PromptInputProps) {
  const [value, setValue] = useState("")
  const [selected, setSelected] = useState("readonly")
  const [open, setOpen] = useState(false)
  const [modelOpen, setModelOpen] = useState(false)
  const [attachments, setAttachments] = useState<AttachmentFile[]>([])
  const ref = useRef<HTMLDivElement>(null)
  const modelRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  // Only ever used to tell one card from another.
  const attachmentId = useRef(0)

  const { providers, loading: loadingModels, error: modelError, selected: model } =
    useModelState()

  const current = options.find((o) => o.value === selected)

  const selectedLabel = (() => {
    if (!model) return "Select model"
    for (const provider of providers) {
      const match = provider.models.find(
        (m) => m.id === model.model && provider.id === model.provider
      )
      if (match) return match.label
    }
    return model.model
  })()

  const canSend = value.trim() !== "" && !disabled

  const handleSend = () => {
    if (!canSend) return
    onSend?.(value.trim())
    setValue("")
    // The attachments went with the message. They are only described here for
    // now — nothing reads them on the way out — so the composer starts clean.
    setAttachments([])
    textareaRef.current?.focus()
  }

  /**
   * Picked files become cards. Each one is read in the browser for its length
   * and nothing else: no upload, no bytes kept.
   */
  const handlePickedFiles = async (event: ChangeEvent<HTMLInputElement>) => {
    const picked = Array.from(event.target.files ?? [])
    // Cleared so picking the same file twice in a row still counts as a pick.
    event.target.value = ""
    if (picked.length === 0) return

    const described = await Promise.all(
      picked.map((file) =>
        attachmentFromFile(file, String(attachmentId.current++))
      )
    )
    setAttachments((current) => [...current, ...described])
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
        <input
          ref={fileInputRef}
          type="file"
          multiple
          className="hidden"
          onChange={handlePickedFiles}
        />
        <button
          type="button"
          aria-label="Attach files"
          disabled={disabled}
          onClick={() => fileInputRef.current?.click()}
          className="flex size-7 cursor-pointer items-center justify-center rounded-full bg-white/5 text-white transition-colors hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-50"
        >
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
            className="flex h-7 max-w-52 cursor-pointer items-center gap-1.5 rounded-full bg-white/0 px-2.5 text-xs text-white/70 hover:bg-white/10"
          >
            <span className="truncate">{selectedLabel}</span>
            <HugeiconsIcon
              icon={ChevronDownIcon}
              strokeWidth={2}
              className={`size-3.5 shrink-0 transition-transform duration-200 ${modelOpen ? "rotate-180" : ""}`}
            />
          </button>
          {modelOpen && (
            <div className="scrollbar-thin absolute right-0 bottom-full z-50 mb-1 max-h-65 w-56 overflow-x-hidden overflow-y-auto rounded-xl border border-white/10 bg-neutral-900 shadow-lg">
              {loadingModels && (
                <div className="px-3 py-2 text-xs text-white/40">Loading models…</div>
              )}
              {!loadingModels && modelError && (
                <div className="px-3 py-2 text-xs text-red-400">{modelError}</div>
              )}
              {!loadingModels && !modelError && providers.length === 0 && (
                <div className="px-3 py-2 text-xs text-white/40">
                  No models available — check backend API keys.
                </div>
              )}
              {providers.map((provider) => (
                <div key={provider.id}>
                  <div className="px-3 pt-2 pb-1 text-[10px] font-medium tracking-wide text-white/40 uppercase">
                    {provider.name}
                  </div>
                  {provider.models.map((item) => {
                    const isActive =
                      model?.provider === provider.id && model?.model === item.id
                    return (
                      <button
                        key={`${provider.id}:${item.id}`}
                        onClick={() => {
                          selectModel({ provider: provider.id, model: item.id })
                          setModelOpen(false)
                        }}
                        className={cn(
                          "flex w-full cursor-pointer items-center justify-between gap-2 px-3 py-1.5 text-left text-xs text-white hover:bg-white/10",
                          isActive && "bg-white/5"
                        )}
                      >
                        <span className="truncate">{item.label}</span>
                        {isActive && (
                          <HugeiconsIcon
                            icon={Tick02Icon}
                            strokeWidth={2}
                            className="size-3.5 shrink-0 text-primary"
                          />
                        )}
                      </button>
                    )
                  })}
                </div>
              ))}
            </div>
          )}
        </div>
        <button
          onClick={handleSend}
          disabled={!canSend}
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
          ? "bg-black/40 backdrop-blur-xl"
          : "bg-white/5 backdrop-blur-[2px]"
      )}
    >
      {attachments.length > 0 ? (
        <div className="mb-3 flex flex-wrap items-start gap-3 px-1">
          {attachments.map((file) => (
            <Attachment
              key={file.id}
              file={file}
              onRemove={() =>
                setAttachments((current) =>
                  current.filter((item) => item.id !== file.id)
                )
              }
            />
          ))}
        </div>
      ) : null}
      <textarea
        ref={textareaRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={handleKeyDown}
        disabled={disabled}
        placeholder="Describe what you want to build, / Commands @ files or sessions"
        rows={2}
        className="w-full resize-none bg-transparent px-1 text-sm text-white placeholder:text-white/50 outline-none disabled:opacity-60"
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
    // Anchored from the top rather than centred: attaching a file grows the
    // composer downward, and the heading above it stays where it is.
    <div className="flex h-full flex-col items-center px-4 pt-[20vh] pb-4">
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
