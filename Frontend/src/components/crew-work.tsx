import { HugeiconsIcon } from "@hugeicons/react"
import { File02Icon } from "@hugeicons/core-free-icons"

import { cn } from "cn"
import { CodeBlock } from "@/components/code-block"

/**
 * The two artifacts a delegation produces, rendered the same way wherever they
 * are read: in the timeline node that records the hand-off, and in the crew
 * rail that keeps the files. A todo is a task and its context; a report is a
 * status, a summary, the files it touched, its checks and its error.
 *
 * Keeping one view for both means the panel cannot drift into showing raw JSON
 * while the timeline shows prose.
 */

/** A parsed JSON object, or undefined when the text is not one. */
export function parsedObject(
  raw?: string
): Record<string, unknown> | undefined {
  if (!raw || raw.trim() === "") return undefined
  try {
    const value: unknown = JSON.parse(raw)
    if (value !== null && typeof value === "object" && !Array.isArray(value)) {
      return value as Record<string, unknown>
    }
  } catch {
    return undefined
  }
  return undefined
}

export function text(value: unknown): string {
  return typeof value === "string" ? value.trim() : ""
}

export function textList(value: unknown): string[] {
  if (!Array.isArray(value)) return []
  return value.filter(
    (item): item is string => typeof item === "string" && item.trim() !== ""
  )
}

/** How a report's status reads: done, blocked, failed, or something else. */
export function statusTone(status: string): string {
  switch (status.toLowerCase()) {
    case "completed":
      return "border-emerald-500/20 bg-emerald-500/10 text-emerald-300"
    case "blocked":
      return "border-amber-500/20 bg-amber-500/10 text-amber-300"
    case "failed":
      return "border-red-500/20 bg-red-500/10 text-red-300"
    default:
      return "border-white/10 bg-white/5 text-white/50"
  }
}

/**
 * The todo the Orchestrator handed out. A stored todo carries the worker's name
 * and the paths it was saved at; the timeline's copy is only the tool input, so
 * every field is optional.
 */
export function TodoView({ todo }: { todo?: Record<string, unknown> }) {
  const task = text(todo?.task)
  const context = text(todo?.context)
  const worker = text(todo?.workerName)

  return (
    <div className="min-w-0 rounded-lg border border-white/10 bg-white/5 p-3">
      <div className="mb-1.5 flex flex-wrap items-center gap-2">
        <p className="font-mono text-[10px] text-white/40">Todo</p>
        {worker !== "" ? (
          <span className="text-[10px] text-white/35">{worker}</span>
        ) : null}
      </div>
      <p className="text-[12.5px] leading-relaxed whitespace-pre-wrap text-white/70">
        {task !== "" ? task : "(the worker was given no task text)"}
      </p>
      {context !== "" ? (
        <p className="mt-2 border-t border-white/10 pt-2 text-[11.5px] leading-relaxed whitespace-pre-wrap text-white/45">
          <span className="text-white/35">Context: </span>
          {context}
        </p>
      ) : null}
    </div>
  )
}

/**
 * The report a worker sent back. `failed` is the tool call's own failure — a
 * delegation that never reached a report — and is what turns the empty state
 * from "still waiting" into the reason it stopped.
 */
export function ReportView({
  report,
  failed = false,
  failure = "",
}: {
  report?: Record<string, unknown>
  failed?: boolean
  failure?: string
}) {
  const status = text(report?.status)
  const summary = text(report?.summary)
  const worker = text(report?.workerName)
  const files = textList(report?.filesChanged)
  const checks = textList(report?.checks)
  const error = text(report?.error) || (failed ? failure.trim() : "")
  const broke = failed || status.toLowerCase() === "failed"

  return (
    <div
      className={cn(
        "min-w-0 rounded-lg border p-3",
        broke ? "border-red-500/15 bg-red-500/5" : "border-white/10 bg-white/5"
      )}
    >
      <p className="mb-2 font-mono text-[10px] text-white/40">Report</p>

      {report ? (
        <div className="flex flex-col gap-2">
          <div className="flex flex-wrap items-center gap-2">
            <span
              className={cn(
                "rounded-full border px-2 py-0.5 text-[10px] font-medium",
                statusTone(status)
              )}
            >
              {status !== "" ? status : "unknown"}
            </span>
            {worker !== "" ? (
              <span className="text-[10px] text-white/40">{worker}</span>
            ) : null}
          </div>

          <p className="text-[12.5px] leading-relaxed whitespace-pre-wrap text-white/70">
            {summary !== "" ? summary : "(no summary)"}
          </p>

          {files.length > 0 ? (
            <div>
              <p className="mb-1 text-[10px] text-white/40">Files changed</p>
              <ul className="flex flex-col gap-0.5">
                {files.map((file) => (
                  <li
                    key={file}
                    className="flex items-start gap-1.5 font-mono text-[11px] text-white/60"
                  >
                    <HugeiconsIcon
                      icon={File02Icon}
                      strokeWidth={2}
                      className="mt-0.5 size-3 shrink-0 text-white/35"
                    />
                    <span className="break-all">{file}</span>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}

          {checks.length > 0 ? (
            <div>
              <p className="mb-1 text-[10px] text-white/40">Checks</p>
              <div className="flex flex-wrap gap-1">
                {checks.map((check) => (
                  <span
                    key={check}
                    className="rounded-full border border-white/10 bg-white/5 px-2 py-0.5 text-[10px] text-white/55"
                  >
                    {check}
                  </span>
                ))}
              </div>
            </div>
          ) : null}

          {error !== "" ? (
            <p className="rounded-lg border border-red-500/15 bg-red-500/5 p-2 text-[11.5px] leading-relaxed whitespace-pre-wrap text-red-300/80">
              {error}
            </p>
          ) : null}
        </div>
      ) : (
        <p className="text-[11.5px] leading-relaxed text-white/40">
          {failed
            ? failure || "The delegation failed."
            : "Waiting for the worker's report…"}
        </p>
      )}
    </div>
  )
}

/**
 * A crew file's text as the form it describes. It is a todo or a report when
 * the file name says so and the text parses; anything else stays readable as
 * JSON rather than disappearing.
 */
export function CrewWorkView({
  kind,
  content,
}: {
  kind: "todo" | "report"
  content: string
}) {
  const parsed = parsedObject(content)
  if (!parsed) {
    return <CodeBlock language="json">{content}</CodeBlock>
  }
  return kind === "todo" ? (
    <TodoView todo={parsed} />
  ) : (
    <ReportView report={parsed} />
  )
}
