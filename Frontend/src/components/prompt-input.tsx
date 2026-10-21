import {
  useState,
  useRef,
  useEffect,
  useImperativeHandle,
  forwardRef,
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
  BorderAll02Icon,
  BotIcon,
  ChevronDownIcon,
  PlusSignIcon,
  Tick02Icon,
} from "@hugeicons/core-free-icons"

import { Attachment } from "@/components/attachment"
import {
  ARTIFACT_DRAG_TYPE,
  decodeArtifactDrag,
  isArtifactDrag,
} from "@/lib/artifact-drag"
import {
  attachmentFromArtifact,
  attachmentFromFile,
  type AttachmentFile,
} from "@/lib/attachments"
import { selectModel, useModelState } from "@/lib/model-store"
import { useAgentsState } from "@/lib/agent-store"
import { useIsMobile } from "@/hooks/use-mobile"

/**
 * Who does the work on a run. Chat is the one you are in unless you say
 * otherwise: the assistant writes, runs and changes things itself. Crew hands
 * the task to the agent team, and the run it starts lives under its own
 * address.
 *
 * Each carries the brand colour it fills its pill with — Chat the sky blue,
 * Crew the purple — split into the fill the thumb wears, the colour the label
 * reads in, and the colour the composer's own left edge takes. Written out as
 * whole class names because that is the only form the Tailwind scanner can see.
 */
export type Mode = "chat" | "crew"

const MODES: {
  value: Mode
  label: string
  hint: string
  fill: string
  text: string
  solid: string
  edge: string
}[] = [
  {
    value: "chat",
    label: "Chat",
    hint: "Chat — every tool, files and commands",
    fill: "border-[#5B7FFF]/40 bg-[#5B7FFF]/15",
    text: "text-[#5B7FFF]",
    solid: "bg-[#5B7FFF]",
    edge: "border-l-[#5B7FFF]",
  },
  {
    value: "crew",
    label: "Crew",
    hint: "Crew — long and complex work, handed to the agent team",
    fill: "border-[#9D79FF]/40 bg-[#9D79FF]/15",
    text: "text-[#9D79FF]",
    solid: "bg-[#9D79FF]",
    edge: "border-l-[#9D79FF]",
  },
]

export interface PromptInputRef {
  getMode: () => Mode
}

interface PromptInputProps {
  onSend?: (message: string) => void
  onStop?: () => void
  heading?: string
  compact?: boolean
  /** Bottom-docked composer used inside a chat thread. */
  isInChat?: boolean
  /** Blocks input while a reply is still streaming or an action is pending. */
  disabled?: boolean
  /** Whether a reply is currently streaming (shows stop button). */
  isStreaming?: boolean
  /** Current chat mode. */
  mode?: Mode
  /** Called when mode changes. */
  onModeChange?: (mode: Mode) => void
  /**
   * Another surface sits directly on top of the composer, so the gap that is
   * usually above it is not wanted.
   */
  attached?: boolean
  /**
   * The Autonomous page's composer: the page takes the task and runs with it,
   * so there is no mode to pick and no model to pick. A single label says so.
   */
  autonomous?: boolean
  /** The hint the empty composer shows, when a page has its own question. */
  placeholder?: string
  /**
   * Where the group sits when it is not in a chat thread. "home" centres it in
   * the page, under the heading, the way the first screen does. "bottom" puts
   * it at the foot of the page, for a page that has work of its own above it.
   */
  placement?: "home" | "bottom"
}

const PromptInput = forwardRef<PromptInputRef, PromptInputProps>(
  (
    {
      onSend,
      onStop,
      heading,
      compact = false,
      isInChat = false,
      disabled = false,
      mode = "chat",
      onModeChange,
      isStreaming = false,
      attached = false,
      placement = "home",
      autonomous = false,
      placeholder: placeholderText,
    },
    ref
  ) => {
    const [value, setValue] = useState("")
    const [internalMode, setInternalMode] = useState<Mode>(mode)
    const [modelOpen, setModelOpen] = useState(false)
    const [attachments, setAttachments] = useState<AttachmentFile[]>([])
    /** True while a file from the artifacts rail is over the composer. */
    const [dropping, setDropping] = useState(false)
    const [thumbX, setThumbX] = useState<number | null>(null)
    // Where the pointer went down, and which end the thumb was at.
    // `x` is the latest thumb position kept in a ref so release never reads a
    // stale render's state; `moved` tells a click apart from a drag.
    const modeDrag = useRef({
      active: false,
      moved: false,
      pointer: 0,
      from: 0,
      x: 0,
      segment: 0,
    })
    // Only ever used to tell one card from another.
    const attachmentId = useRef(0)
    // How deep into the composer the drag in flight is.
    const dragDepth = useRef(0)

    const modelRef = useRef<HTMLDivElement>(null)
    const textareaRef = useRef<HTMLTextAreaElement>(null)
    const fileInputRef = useRef<HTMLInputElement>(null)
    const modeTrackRef = useRef<HTMLDivElement>(null)
    const modeThumbRef = useRef<HTMLDivElement>(null)

    // Sync internal mode with prop
    useEffect(() => {
      setInternalMode(mode)
    }, [mode])

    const setMode = (newMode: Mode) => {
      if (newMode === internalMode) return
      setInternalMode(newMode)
      setModelOpen(false)
      onModeChange?.(newMode)
    }

    // Expose getMode to parent
    useImperativeHandle(ref, () => ({
      getMode: () => internalMode,
    }))

    const {
      providers,
      loading: loadingModels,
      selected: model,
    } = useModelState()
    const { agents } = useAgentsState()

    // A phone's composer is a phone's width. The long placeholder — the one with
    // the slashes in it — is cut off mid-word there, so a phone is asked the one
    // question it can read. The thread's composer keeps the long one: it is a
    // different screen, and this pass is the home one. A page that has its own
    // one-line question hands it in instead.
    const isMobile = useIsMobile()
    const defaultPlaceholder =
      isMobile && !isInChat
        ? "Describe what you want to do, or /image a sunset"
        : "Describe what you want to build, / Commands @ files or sessions, /image a landscape"
    // Crew has a line of its own; Chat keeps the default. A page that hands in
    // its own question still wins over both.
    const placeholder =
      placeholderText ??
      (internalMode === "crew"
        ? "Hand off your task to the crew and sit back and relax"
        : defaultPlaceholder)

    // The one the thumb is wearing: its fill, and the colour its label reads in.
    const activeMode =
      MODES.find((item) => item.value === internalMode) ?? MODES[0]

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
     * The mode switch is a two-segment pill: Chat | Crew.
     * The thumb is exactly 1/3 of the track and slides to the active segment.
     * Click a label or drag the thumb — on release it snaps to the nearest half.
     */
    const modeIndex = MODES.findIndex((item) => item.value === internalMode)

    const handleModeGrab = (event: PointerEvent<HTMLDivElement>) => {
      if (event.button !== 0) return
      const track = modeTrackRef.current
      if (!track) return
      const segment = track.getBoundingClientRect().width / MODES.length
      const from = segment * modeIndex
      modeDrag.current = {
        active: true,
        moved: false,
        pointer: event.clientX,
        from,
        x: from,
        segment,
      }
      event.currentTarget.setPointerCapture(event.pointerId)
    }

    const handleModeMove = (event: PointerEvent<HTMLDivElement>) => {
      const drag = modeDrag.current
      if (!drag.active) return
      const delta = event.clientX - drag.pointer
      // A few pixels of wobble is still a click, not a drag.
      if (!drag.moved && Math.abs(delta) < 4) return
      drag.moved = true
      // The thumb's left edge can travel from the first segment to the last one
      // and no further — its right edge stays inside the track.
      drag.x = Math.min(drag.segment * (MODES.length - 1), Math.max(0, drag.from + delta))
      setThumbX(drag.x)
    }

    /** On release: a click picks the segment under the pointer, a drag snaps. */
    const handleModeRelease = (event: PointerEvent<HTMLDivElement>) => {
      const drag = modeDrag.current
      if (!drag.active) return
      drag.active = false
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId)
      }

      let index = modeIndex
      if (drag.moved) {
        index = Math.round(drag.x / drag.segment)
      } else if (modeTrackRef.current) {
        const rect = modeTrackRef.current.getBoundingClientRect()
        index = Math.floor((event.clientX - rect.left) / drag.segment)
      }
      index = Math.min(MODES.length - 1, Math.max(0, index))

      setThumbX(null)
      setMode(MODES[index].value)
    }

    const handleModeCancel = (event: PointerEvent<HTMLDivElement>) => {
      modeDrag.current.active = false
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId)
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
            <HugeiconsIcon
              icon={PlusSignIcon}
              strokeWidth={2}
              className="size-4"
            />
          </button>
          {/* A track with a thumb in it: the pill slides to the mode you are in,
              and can be taken hold of and dragged there instead. The Autonomous
              composer has no mode to be in, so it has no track. */}
          {!autonomous && (
            <div
              ref={modeTrackRef}
              role="group"
              aria-label="Mode"
              onPointerDown={handleModeGrab}
              onPointerMove={handleModeMove}
              onPointerUp={handleModeRelease}
              onPointerCancel={handleModeCancel}
              className="relative flex h-7 shrink-0 touch-none items-center rounded-full bg-black/40 text-xs select-none"
            >
              {/* The thumb fills one half of the track exactly — full height, full
                  width, no gutter. Two modes = two equal segments. */}
              <div
                ref={modeThumbRef}
                aria-hidden="true"
                // One mechanism for position: the `translate` property. Resting,
                // it is a whole number of the thumb's own widths (the thumb is a
                // half of the track, so 100% is exactly one segment). Dragging,
                // it is pixels with the transition off.
                style={
                  thumbX === null
                    ? { translate: `${modeIndex * 100}%` }
                    : { translate: `${thumbX}px`, transition: "none" }
                }
                className={cn(
                  "absolute top-0 bottom-0 left-0 w-1/2 rounded-full border transition-[translate] duration-200 ease-out",
                  activeMode.fill
                )}
              />
              {MODES.map((item) => {
                const active = item.value === internalMode
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
          )}
        </div>
        <div className="flex items-center gap-2">
          {autonomous ? (
            <span className="px-2.5 text-xs text-white/50">
              Autonomous mode
            </span>
          ) : internalMode === "crew" ? (
            // Crew hands the task to the agent team, so there is no model to pick:
            // a single fixed name stands in for the menu.
            <span className="px-2.5 text-xs text-white/70">Crew</span>
          ) : (
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
                <div className="absolute right-0 bottom-full z-50 mb-1 max-h-65 w-56 scrollbar-thin overflow-x-hidden overflow-y-auto rounded-xl border border-white/10 bg-neutral-900 shadow-lg">
                  {loadingModels && (
                    <div className="px-3 py-2 text-xs text-white/40">
                      Loading models…
                    </div>
                  )}
                  {!loadingModels &&
                    providers.length === 0 &&
                    agents.length === 0 && (
                      <div className="px-3 py-2 text-xs text-white/40">
                        No models available — add a provider on the Providers
                        page.
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
                              selectModel({
                                provider: "",
                                model: "",
                                agentId: agent.id,
                              })
                              setModelOpen(false)
                            }}
                            className={cn(
                              "flex w-full cursor-pointer items-center justify-between gap-2 px-3 py-1.5 text-left text-xs text-white hover:bg-white/10",
                              isActive && "bg-white/5"
                            )}
                          >
                            <span className="flex items-center gap-2 truncate">
                              <HugeiconsIcon
                                icon={BotIcon}
                                strokeWidth={2}
                                className="size-3.5 text-white/50"
                              />
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
                          model?.provider === provider.id &&
                          model?.model === item.id &&
                          !model?.agentId
                        return (
                          <button
                            key={`${provider.id}:${item.id}`}
                            onClick={() => {
                              selectModel({
                                provider: provider.id,
                                model: item.id,
                              })
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
          )}
          {isStreaming ? (
            <button
              onClick={onStop}
              disabled={!onStop}
              // Explicitly prevent parent disabled state from affecting this button
              className={cn(
                "flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-full transition-opacity disabled:cursor-not-allowed disabled:opacity-50",
                activeMode.text,
                // Force enable: never inherit disabled from parent
                "pointer-events-auto",
                "[&:disabled]:pointerEvents-auto",
                "[&:disabled]:opacity-100"
              )}
              style={{ pointerEvents: "auto", zIndex: 10 }}
            >
              <HugeiconsIcon
                icon={BorderAll02Icon}
                strokeWidth={2}
                className="size-5"
              />
            </button>
          ) : (
            <button
              onClick={handleSend}
              disabled={!canSend}
              className={cn(
                "flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-full text-white transition-opacity disabled:cursor-not-allowed disabled:opacity-50",
                activeMode.solid
              )}
            >
              <HugeiconsIcon
                icon={ArrowUp01Icon}
                strokeWidth={2}
                className="size-5"
              />
            </button>
          )}
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
            "w-full rounded-2xl border border-l-2 border-white/10 px-4 pt-4 pb-2 shadow-sm transition-colors",
            // A phone's composer is docked at the foot of the screen, over the
            // wave band, so it wears the thread composer's darker glass: white
            // text on a mint glow does not read.
            "max-md:bg-black/40 max-md:backdrop-blur-xl",
            activeMode.edge,
            isInChat
              ? "bg-black/40 backdrop-blur-xl"
              : autonomous
                ? // The Autonomous composer stands over the wave on its own, so
                  // it is a shade more solid than the home screen's glass and
                  // blurs a little more of what is behind it.
                  "bg-white/10 backdrop-blur-sm"
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
            disabled={isStreaming}
            placeholder={placeholder}
            rows={2}
            className="w-full resize-none bg-transparent px-1 text-sm text-white outline-none placeholder:text-white/50 disabled:opacity-60"
          />
          {renderActions()}
        </div>
      </div>
    )

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

    if (isInChat) {
      return (
        <div className={cn("w-full p-4", attached && "pt-0")}>
          <div className="mx-auto w-full max-w-[780px]">{renderComposer()}</div>
        </div>
      )
    }

    // "bottom" is the other placement: the same group, taken out of the top of
    // the page and put at its foot, for a page that has work of its own above
    // it. The gap above the heading belongs to the home screen, where the group
    // hangs from the top.
    const atBottom = placement === "bottom"

    return (
      // Anchored from the top rather than centred: attaching a file grows the
      // composer downward, and the heading above it stays where it is. The offset
      // is a fixed one rather than a share of the viewport height: `vh` measures
      // the web viewport, which is a different height in a browser (below its
      // toolbar) than in the desktop shell (below the OS title bar), so the
      // composer landed at a different spot in each.
      //
      // A phone is the other way round — the heading takes the middle of the
      // screen and the composer sits under it, at the foot of the screen, which
      // is where a thumb already is. The heading row is what grows to push it there.
      <div
        className={cn(
          "flex h-full flex-col px-4 pb-4",
          atBottom
            ? "items-center justify-end"
            : "items-center pt-40 max-md:pt-0"
        )}
      >
        <div
          className={cn(
            "flex w-full max-w-[780px] flex-col items-center gap-5",
            !atBottom && "max-md:flex-1"
          )}
        >
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
)

PromptInput.displayName = "PromptInput"

export { PromptInput }
