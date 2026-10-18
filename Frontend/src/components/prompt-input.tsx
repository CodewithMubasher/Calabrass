import {
  useState,
  useRef,
  type ChangeEvent,
  type DragEvent,
  type KeyboardEvent,
  type PointerEvent,
} from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { cn } from "cn"
import "./prompt-input.css"
import {
  ArrowUp01Icon,
  BotIcon,
  ChevronDownIcon,
  PlusSignIcon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

import { Attachment } from "@/components/attachment"
import { ARTIFACT_DRAG_TYPE, decodeArtifactDrag, isArtifactDrag } from "@/lib/artifact-drag"
import {
  attachmentFromArtifact,
  attachmentFromFile,
  type AttachmentFile,
} from "@/lib/attachments"
import { selectModel, useModelState } from "@/lib/model-store"
import { useAgentsState } from "@/lib/agent-store"
import { useIsMobile } from "@/hooks/use-mobile"

/**
 * How much the assistant is allowed to do on a run. Build is the one you are in
 * unless you say otherwise: it can write, run and change things. Plan answers
 * the question and touches nothing but the tools it can read with.
 *
 * Each carries the brand colour it fills its pill with — Build the sky blue,
 * Plan the pink — split into the fill the thumb wears, the colour the label
 * reads in, and the colour the composer's own left edge takes. Written out as
 * whole class names because that is the only form the Tailwind scanner can see.
 */
type Mode = "build" | "plan"

const MODES: {
  value: Mode
  label: string
  hint: string
  fill: string
  text: string
  edge: string
}[] = [
  {
    value: "build",
    label: "Build",
    hint: "Build — every tool, files and commands",
    fill: "border-[#5B7FFF]/40 bg-[#5B7FFF]/15",
    text: "text-[#5B7FFF]",
    edge: "border-l-[#5B7FFF]",
  },
  {
    value: "plan",
    label: "Plan",
    hint: "Plan — read-only tools and the shell",
    fill: "border-[#FF8FB8]/40 bg-[#FF8FB8]/15",
    text: "text-[#FF8FB8]",
    edge: "border-l-[#FF8FB8]",
  },
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
  const [mode, setMode] = useState<Mode>("build")
  const [modelOpen, setModelOpen] = useState(false)
  const [attachments, setAttachments] = useState<AttachmentFile[]>([])
  /** True while a file from the artifacts rail is over the composer. */
  const [dropping, setDropping] = useState(false)
  const modelRef = useRef<HTMLDivElement>(null)
  const textareaRef = useRef<HTMLTextAreaElement>(null)
  const fileInputRef = useRef<HTMLInputElement>(null)
  const modeTrackRef = useRef<HTMLDivElement>(null)
  const modeThumbRef = useRef<HTMLDivElement>(null)
  /**
   * Where the mode thumb is while it is being held, in pixels from the start of
   * the track's content, or null when it is resting — the mode decides where it
   * sits then, and CSS animates it there.
   */
  const [thumbX, setThumbX] = useState<number | null>(null)
  // Where the pointer went down, and which end the thumb was at.
  const modeDrag = useRef({ pointer: 0, from: 0 })
  // Only ever used to tell one card from another.
  const attachmentId = useRef(0)
  // How deep into the composer the drag in flight is.
  const dragDepth = useRef(0)

  const { providers, loading: loadingModels, error: modelError, selected: model } =
    useModelState()
  const { agents } = useAgentsState()

  // A phone's composer is a phone's width. The long placeholder — the one with
  // the slashes in it — is cut off mid-word there, so a phone is asked the one
  // question it can read. The thread's composer keeps the long one: it is a
  // different screen, and this pass is the home one.
  const isMobile = useIsMobile()
  const placeholder =
    isMobile && !isInChat
      ? "Describe what you want to do"
      : "Describe what you want to build, / Commands @ files or sessions"

  // The one the thumb is wearing: its fill, and the colour its label reads in.
  const activeMode = MODES.find((item) => item.value === mode) ?? MODES[0]

  const selectedLabel = (() => {
    if (!model) return "Select model"
    // Check if it's an agent selection
    if (model.agentId) {
      const agent = agents.find((a) => a.id === model.agentId)
      if (agent) return `${agent.name} (Agent)`
    }
    for (const provider of providers) {
      const match = provider.models.find(
        (m) => m.id === model.model && provider.id === model.provider
      )
      if (match) return match.label
    }
    return model.model
  })()

  const canSend = value.trim() !== "" && !disabled

  const addAttachments = (added: AttachmentFile[]) =>
    setAttachments((current) => [...current, ...added])

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
    addAttachments(described)
  }

  /**
   * A file dragged out of the artifacts rail. The composer lights up while it is
   * overhead, and the drop describes the file the way the rail already does.
   */
  const handleDragEnter = (event: DragEvent<HTMLDivElement>) => {
    if (disabled || !isArtifactDrag(event.dataTransfer.types)) return
    // Enter fires again for each nested element the drag crosses, and leave is
    // not guaranteed to pair up, so leaving the composer is what a count of zero
    // means.
    dragDepth.current += 1
    setDropping(true)
  }

  const handleDragOver = (event: DragEvent<HTMLDivElement>) => {
    if (disabled || !isArtifactDrag(event.dataTransfer.types)) return
    // A drop is only allowed on something that says so.
    event.preventDefault()
    event.dataTransfer.dropEffect = "copy"
  }

  const handleDragLeave = () => {
    dragDepth.current = Math.max(0, dragDepth.current - 1)
    if (dragDepth.current === 0) setDropping(false)
  }

  const handleDrop = (event: DragEvent<HTMLDivElement>) => {
    if (!isArtifactDrag(event.dataTransfer.types)) return
    event.preventDefault()
    dragDepth.current = 0
    setDropping(false)
    if (disabled) return

    const raw = event.dataTransfer.getData(ARTIFACT_DRAG_TYPE)
    const file = decodeArtifactDrag(raw)
    if (file) {
      const id = String(attachmentId.current++)
      addAttachments([attachmentFromArtifact(file, id)])
    }
  }

  const handleKeyDown = (e: KeyboardEvent<HTMLTextAreaElement>) => {
    if (e.key === "Enter" && !e.shiftKey) {
      e.preventDefault()
      handleSend()
    }
  }

  /**
   * The mode switch is an iOS-style thumb: it slides when the mode changes, and
   * it can be taken hold of and moved. Holding is a pointer on the track, which
   * keeps the thumb and the track in one coordinate space — the pointer is
   * captured, so the move survives the cursor leaving either button.
   */
  const handleModeGrab = (event: PointerEvent<HTMLDivElement>) => {
    const width = modeThumbRef.current?.offsetWidth ?? 0
    const from = mode === "plan" ? width : 0
    modeDrag.current = { pointer: event.clientX, from }
    setThumbX(from)
    event.currentTarget.setPointerCapture(event.pointerId)
  }

  const handleModeMove = (event: PointerEvent<HTMLDivElement>) => {
    if (thumbX === null) return
    const width = modeThumbRef.current?.offsetWidth ?? 0
    const moved = modeDrag.current.from + (event.clientX - modeDrag.current.pointer)
    // Clamped to the track: the thumb can be held and moved, not thrown off it.
    setThumbX(Math.min(width, Math.max(0, moved)))
  }

  /** Let go, and wherever the pointer ended up is the mode. */
  const handleModeRelease = (event: PointerEvent<HTMLDivElement>) => {
    if (thumbX === null) return
    const track = modeTrackRef.current
    if (track) {
      const bounds = track.getBoundingClientRect()
      setMode(event.clientX - bounds.left < bounds.width / 2 ? "build" : "plan")
    }
    setThumbX(null)
  }

  const renderActions = () => (
    <div className="mt-3 flex items-center justify-between max-md:gap-2">
      <div className="flex items-center gap-2.5">
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
          className="flex size-7 shrink-0 cursor-pointer items-center justify-center rounded-full bg-white/5 text-white transition-colors hover:bg-white/10 disabled:cursor-not-allowed disabled:opacity-50"
        >
          <HugeiconsIcon icon={PlusSignIcon} strokeWidth={2} className="size-4" />
        </button>
        {/* A track with a thumb in it: the pill slides to the mode you are in,
            and can be taken hold of and dragged there instead. */}
        <div
          ref={modeTrackRef}
          role="group"
          aria-label="Mode"
          onPointerDown={handleModeGrab}
          onPointerMove={handleModeMove}
          onPointerUp={handleModeRelease}
          onPointerCancel={handleModeRelease}
          className="relative flex h-7 shrink-0 touch-none items-center rounded-full bg-black/40 text-xs select-none"
        >
          {/* The thumb fills its half of the track exactly — full height, full
              width, no gutter. Any inset at all reads as the track peeking out
              from behind the pill: on the sides when it sits left or right, top
              and bottom when it sits still. Flush, its own rounding lands on the
              track's caps and the two are concentric. Its size comes from the
              track rather than height and position classes, so they cannot
              drift apart. */}
          <div
            ref={modeThumbRef}
            aria-hidden="true"
            style={
              thumbX === null
                ? undefined
                : // `translate`, not `transform`: Tailwind v4's `translate-x-*`
                  // sets this property too, and the two would add up rather than
                  // replace each other — a held thumb sat one segment too far.
                  { translate: `${thumbX}px`, transition: "none" }
            }
            className={cn(
              "absolute top-0 bottom-0 left-0 w-14 rounded-full border transition-transform duration-200 ease-out",
              activeMode.fill,
              mode === "plan" ? "translate-x-full" : "translate-x-0"
            )}
          />
          {MODES.map((item) => {
            const active = item.value === mode
            return (
              <button
                key={item.value}
                type="button"
                title={item.hint}
                aria-pressed={active}
                onClick={() => setMode(item.value)}
                className={cn(
                  "relative z-10 flex h-6 w-14 cursor-pointer items-center justify-center rounded-full transition-colors",
                  active ? item.text : "text-white/40 hover:text-white/70"
                )}
              >
                {item.label}
              </button>
            )
          })}
        </div>
      </div>
      <div className="flex items-center gap-2">
        <div ref={modelRef} className="relative">
          <button
            onClick={() => setModelOpen(!modelOpen)}
            className="flex h-7 max-w-52 cursor-pointer items-center gap-1.5 rounded-full bg-white/0 px-2.5 text-xs text-white/70 hover:bg-white/10 max-md:max-w-28"
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
              {!loadingModels && !modelError && providers.length === 0 && agents.length === 0 && (
                <div className="px-3 py-2 text-xs text-white/40">
                  No models available — check backend API keys.
                </div>
              )}
              {agents.length > 0 && (
                <div>
                  <div className="px-3 pt-2 pb-1 text-[10px] font-medium tracking-wide text-white/40 uppercase">
                    Agents
                  </div>
                  {agents.map((agent) => {
                    const isActive = model?.agentId === agent.id
                    return (
                      <button
                        key={`agent:${agent.id}`}
                        onClick={() => {
                          selectModel({ provider: "", model: "", agentId: agent.id })
                          setModelOpen(false)
                        }}
                        className={cn(
                          "flex w-full cursor-pointer items-center justify-between gap-2 px-3 py-1.5 text-left text-xs text-white hover:bg-white/10",
                          isActive && "bg-white/5"
                        )}
                      >
                        <span className="truncate flex items-center gap-2">
                          <HugeiconsIcon icon={BotIcon} strokeWidth={2} className="size-3.5 text-white/50" />
                          {agent.name}
                        </span>
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
              )}
              {providers.map((provider) => (
                <div key={provider.id}>
                  <div className="px-3 pt-2 pb-1 text-[10px] font-medium tracking-wide text-white/40 uppercase">
                    {provider.name}
                  </div>
                  {provider.models.map((item) => {
                    const isActive =
                      model?.provider === provider.id && model?.model === item.id && !model?.agentId
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
          className="flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-full bg-primary text-white transition-opacity disabled:cursor-not-allowed disabled:opacity-50"
        >
          <HugeiconsIcon icon={ArrowUp01Icon} strokeWidth={2} className="size-5" />
        </button>
      </div>
    </div>
  )

  const renderComposer = () => (
    // The wrapper exists for the drop ring, which needs a borderless box of
    // exactly the composer's size to sit on.
    <div className={cn("w-full rounded-2xl", dropping && "drop-ring")}>
      <div
        onDragEnter={handleDragEnter}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        className={cn(
          "w-full rounded-2xl border border-white/10 border-l-2 px-4 pt-4 pb-2 shadow-sm transition-colors",
          // A phone's composer is docked at the foot of the screen, over the
          // wave band, so it wears the thread composer's darker glass: white
          // text on a mint glow does not read.
          "max-md:bg-black/40 max-md:backdrop-blur-xl",
          activeMode.edge,
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
          placeholder={placeholder}
          rows={2}
          className="w-full resize-none bg-transparent px-1 text-sm text-white placeholder:text-white/50 outline-none disabled:opacity-60"
        />
        {renderActions()}
      </div>
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
    //
    // A phone is the other way round — the heading takes the middle of the
    // screen and the composer sits under it, at the foot of the screen, which is
    // where a thumb already is. The heading row is what grows to push it there.
    <div className="flex h-full flex-col items-center px-4 pt-[20vh] pb-4 max-md:pt-0">
      <div className="flex w-full max-w-[780px] flex-col items-center gap-5 max-md:flex-1">
        <div className="flex items-center gap-2.5 max-md:flex-1">
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
