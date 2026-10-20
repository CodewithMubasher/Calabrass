import { HugeiconsIcon } from "@hugeicons/react"
import { ArrowDown01Icon, ArrowUp01Icon } from "@hugeicons/core-free-icons"
import { createRoute } from "@tanstack/react-router"
import { useLayoutEffect, useRef, useState } from "react"

import { cn } from "cn"
import { Button } from "@/components/ui/button"
import { Route as rootRoute } from "@/routes/__root"

export const Route = createRoute({
  getParentRoute: () => rootRoute,
  path: "/autonomous",
  component: AutonomousPage,
})

/**
 * The four corners of the page. Each one is a doorway into a kind of work the
 * app can take on by itself, and each carries the colour of its own mark: the
 * line that joins it to the mark of the app takes that colour while the card
 * is open.
 */
const CORNERS: {
  art: string
  title: string
  color: string
  side: "left" | "right"
  row: "top" | "bottom"
  tasks?: { label: string; done: number }[]
}[] = [
  {
    art: "/part1.svg",
    title: "Frontend Agent",
    color: "#4AD1E6",
    side: "left",
    row: "top",
    tasks: [
      { label: "Setup shadcn react vite", done: 100 },
      { label: "Build the dashboard shell", done: 80 },
      { label: "Wire the chat composer", done: 55 },
      { label: "Add the dark theme tokens", done: 30 },
      { label: "Ship the first screen", done: 10 },
    ],
  },
  {
    art: "/part2.svg",
    title: "Backend Agent",
    color: "#6CAEFF",
    side: "right",
    row: "top",
  },
  {
    art: "/part3.svg",
    title: "Architecture agent",
    color: "#9D79FF",
    side: "left",
    row: "bottom",
  },
  {
    art: "/part4.svg",
    title: "Testing agent",
    color: "#EB8ECC",
    side: "right",
    row: "bottom",
  },
]

type Connector = {
  key: string
  color: string
  x1: number
  y1: number
  x2: number
  y2: number
}

/** The Autonomous screen. Like every other page it sits on the shared wave. */
function AutonomousPage() {
  // One card at a time: opening one puts the others back to their resting size.
  const [open, setOpen] = useState<string | null>(null)

  const containerRef = useRef<HTMLDivElement>(null)
  const logoRef = useRef<HTMLImageElement>(null)
  const headers = useRef<Record<string, HTMLButtonElement | null>>({})
  const [connectors, setConnectors] = useState<Connector[]>([])

  // The lines are drawn from the middle of each card's inner edge to the
  // nearest corner of the mark. Both ends are measured from the laid-out page,
  // so the lines follow the mark whatever size the window is.
  useLayoutEffect(() => {
    const measure = () => {
      const container = containerRef.current
      const logo = logoRef.current
      if (!container || !logo) return

      const page = container.getBoundingClientRect()
      const mark = logo.getBoundingClientRect()
      const next: Connector[] = []

      for (const corner of CORNERS) {
        const header = headers.current[corner.title]
        if (!header) continue

        const head = header.getBoundingClientRect()
        const y1 = head.top + head.height / 2 - page.top
        const x1 =
          corner.side === "left" ? head.right - page.left : head.left - page.left
        const x2 =
          corner.side === "left" ? mark.left - page.left : mark.right - page.left
        const y2 =
          corner.row === "top" ? mark.top - page.top : mark.bottom - page.top

        next.push({ key: corner.title, color: corner.color, x1, y1, x2, y2 })
      }

      setConnectors(next)
    }

    measure()
    const observer = new ResizeObserver(measure)
    if (containerRef.current) observer.observe(containerRef.current)
    window.addEventListener("resize", measure)
    return () => {
      observer.disconnect()
      window.removeEventListener("resize", measure)
    }
  }, [])

  const toggle = (title: string) =>
    setOpen((current) => (current === title ? null : title))

  return (
    <div
      ref={containerRef}
      className="relative flex h-full min-h-full items-stretch p-6 max-md:p-4"
    >
      {/* The left column holds the two left cards, the right column the two
          right ones. The mark of the app sits between them. */}
      <div className="flex flex-1 flex-col justify-between">
        {CORNERS.filter((corner) => corner.side === "left").map((corner) => (
          <div
            key={corner.title}
            className={cn(
              "flex",
              corner.side === "left" ? "justify-start" : "justify-end",
              corner.row === "top" ? "items-start" : "items-end",
            )}
          >
            <CornerCard
              {...corner}
              open={open === corner.title}
              onToggle={() => toggle(corner.title)}
              registerHeader={(node) => {
                headers.current[corner.title] = node
              }}
            />
          </div>
        ))}
      </div>

      {/* The mark of the app, alone in the middle of the page. */}
      <img
        ref={logoRef}
        src="/vite.svg"
        alt="Calabrass"
        className="relative z-10 mx-6 size-20 shrink-0 self-center opacity-90 max-md:mx-3"
      />

      <div className="flex flex-1 flex-col justify-between">
        {CORNERS.filter((corner) => corner.side === "right").map((corner) => (
          <div
            key={corner.title}
            className={cn(
              "flex",
              corner.side === "left" ? "justify-start" : "justify-end",
              corner.row === "top" ? "items-start" : "items-end",
            )}
          >
            <CornerCard
              {...corner}
              open={open === corner.title}
              onToggle={() => toggle(corner.title)}
              registerHeader={(node) => {
                headers.current[corner.title] = node
              }}
            />
          </div>
        ))}
      </div>

      {/* The lines that join each card to the mark. They are painted under the
          cards and the mark, and they hold their place when a card opens. */}
      <svg className="pointer-events-none absolute inset-0 h-full w-full overflow-visible">
        {connectors.map((connector) => (
          <line
            key={connector.key}
            x1={connector.x1}
            y1={connector.y1}
            x2={connector.x2}
            y2={connector.y2}
            stroke={open === connector.key ? connector.color : "rgba(255,255,255,0.15)"}
            strokeWidth={1}
            strokeLinecap="round"
            className={open === connector.key ? "autonomous-flow" : undefined}
            style={{ transition: "stroke 300ms ease" }}
          />
        ))}
      </svg>
    </div>
  )
}

/** One card in a corner: the same surface the other pages draw their cards on. */
function CornerCard({
  art,
  title,
  color,
  row,
  tasks,
  open,
  onToggle,
  registerHeader,
}: {
  art: string
  title: string
  color: string
  row: "top" | "bottom"
  tasks?: { label: string; done: number }[]
  open: boolean
  onToggle: () => void
  registerHeader: (node: HTMLButtonElement | null) => void
}) {
  const body =
    tasks && tasks.length > 0 ? (
      <ul className="flex flex-1 flex-col gap-1.5 overflow-hidden px-4 pt-3 pb-2">
        {tasks.map((task) => (
          <TaskRow key={task.label} task={task} color={color} />
        ))}
      </ul>
    ) : (
      <div className="flex flex-1 items-center justify-center">
        <p className="text-sm text-white/25">No task yet</p>
      </div>
    )

  // The button sits at the foot of the panel: under the work on the top row,
  // over it on the bottom row, where the panel opens upwards.
  const stop = (
    <div className="px-4 pt-1 pb-3">
      <Button variant="destructive" size="sm" className="w-full rounded-xl">
        Stop
      </Button>
    </div>
  )

  return (
    <div
      className={cn(
        "relative flex w-60 shrink-0 flex-col overflow-hidden rounded-2xl border border-white/10 bg-white/5 transition-all duration-300 ease-out hover:border-white/20",
        // The card on the top row opens downwards and the one on the bottom row
        // opens upwards, so the name of the bottom card stays at the foot of
        // the page with the panel opening above it.
        row === "bottom" && "flex-col-reverse",
        open ? "h-96" : "h-12",
        open && "border-white/20",
      )}
    >
      <button
        ref={registerHeader}
        type="button"
        onClick={onToggle}
        aria-expanded={open}
        className="flex h-12 shrink-0 items-center gap-2.5 px-4 text-left"
      >
        <img src={art} alt="" className="size-5 shrink-0" />
        <h3 className="truncate text-sm font-medium text-white/90">{title}</h3>
        <HugeiconsIcon
          icon={open ? ArrowUp01Icon : ArrowDown01Icon}
          strokeWidth={1.8}
          className="ml-auto size-4 shrink-0 text-white/30"
        />
      </button>

      {open ? (
        row === "top" ? (
          <>
            {body}
            {stop}
          </>
        ) : (
          <>
            {stop}
            {body}
          </>
        )
      ) : null}
    </div>
  )
}

/** One task on the list: the little ring gauge of the usage page, then the name. */
function TaskRow({
  task,
  color,
}: {
  task: { label: string; done: number }
  color: string
}) {
  return (
    <li className="flex items-center gap-2.5">
      <span className="shrink-0" style={{ color }}>
        <TaskGauge percentage={task.done} />
      </span>
      <span className="min-w-0 truncate text-xs text-white/70">{task.label}</span>
    </li>
  )
}

/**
 * A ring that fills clockwise to the given percentage — the same mark the
 * usage page puts beside each provider row.
 */
function TaskGauge({ percentage }: { percentage: number }) {
  const normalized = Math.min(Math.max(percentage, 0), 100)
  const circumference = 2 * Math.PI * 42.5
  const strokePercent = (normalized / 100) * circumference

  return (
    <svg
      aria-hidden="true"
      fill="none"
      height="16"
      width="16"
      strokeWidth="2"
      viewBox="0 0 100 100"
      className="size-4 -rotate-90"
    >
      <circle
        cx="50"
        cy="50"
        r="42.5"
        strokeWidth="12"
        strokeLinecap="round"
        strokeLinejoin="round"
        className="opacity-20"
        stroke="currentColor"
        style={{ strokeDasharray: `${circumference} ${circumference}` }}
      />
      <circle
        cx="50"
        cy="50"
        r="42.5"
        strokeWidth="12"
        strokeLinecap="round"
        strokeLinejoin="round"
        stroke="currentColor"
        className="transition-all duration-300"
        style={{ strokeDasharray: `${strokePercent} ${circumference}` }}
      />
    </svg>
  )
}
