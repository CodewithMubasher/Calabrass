import type { ReactNode } from "react"
import { HugeiconsIcon, type IconSvgElement } from "@hugeicons/react"
import { Cancel01Icon, Search01Icon } from "@hugeicons/core-free-icons"

import { Alert, AlertAction } from "@/components/ui/alert"

/**
 * The page frame every top-level screen shares: a title block with its actions
 * on the right, and a scrolling body under it. The measurements mirror the
 * reference project — pt-5 px-14 on the page, py-5 on the header row, pb-6 on
 * the body — so every page lines up with every other one.
 */
export function PageShell({
  title,
  subtitle,
  actions,
  error,
  onDismissError,
  children,
}: {
  title: string
  subtitle: string
  actions?: ReactNode
  error?: string | null
  onDismissError?: () => void
  children: ReactNode
}) {
  return (
    <div className="flex h-full flex-col px-14 pt-5">
      <div className="flex shrink-0 items-center justify-between py-5">
        <div>
          <h1 className="text-xl font-semibold text-white/90">{title}</h1>
          <p className="mt-0.5 text-sm text-white/50">{subtitle}</p>
        </div>
        {actions ? (
          <div className="flex items-center gap-3">{actions}</div>
        ) : null}
      </div>

      {error ? (
        <Alert variant="destructive" className="mb-4 shrink-0">
          {onDismissError ? (
            <AlertAction onClick={onDismissError} aria-label="Dismiss">
              <HugeiconsIcon icon={Cancel01Icon} strokeWidth={2} />
            </AlertAction>
          ) : null}
          {error}
        </Alert>
      ) : null}

      {/* min-h-0 lets the body shrink to the frame so it scrolls instead of
          growing past it, where the shell would clip it. */}
      <div className="min-h-0 flex-1 overflow-auto pb-6">{children}</div>
    </div>
  )
}

/** The round search box the page headers use. */
export function SearchInput({
  value,
  onChange,
  placeholder,
}: {
  value: string
  onChange: (value: string) => void
  placeholder: string
}) {
  return (
    <div className="relative">
      <HugeiconsIcon
        icon={Search01Icon}
        strokeWidth={2}
        className="absolute top-1/2 left-3 size-4 -translate-y-1/2 text-white/40"
      />
      <input
        type="text"
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
        aria-label={placeholder}
        className="w-56 rounded-full border border-white/10 bg-white/5 py-1.5 pr-3 pl-9 text-sm text-white/80 transition-colors outline-none placeholder:text-white/30 focus:border-white/20"
      />
    </div>
  )
}

/** A page that is still fetching: a spinner centred in the body. */
export function PageLoading() {
  return (
    <div className="flex h-full items-center justify-center">
      <div className="size-8 animate-spin rounded-full border-2 border-primary border-t-transparent" />
    </div>
  )
}

/** A page with nothing on it yet: a quiet icon over two lines of copy. */
export function PageEmpty({
  icon,
  title,
  hint,
}: {
  icon: IconSvgElement
  title: string
  hint?: string
}) {
  return (
    <div className="flex h-full items-center justify-center">
      <div className="text-center">
        <div className="mx-auto mb-3 flex size-12 items-center justify-center rounded-xl bg-white/5">
          <HugeiconsIcon
            icon={icon}
            strokeWidth={1.8}
            className="size-5 text-white/30"
          />
        </div>
        <p className="text-sm text-white/40">{title}</p>
        {hint ? <p className="mt-1 text-xs text-white/25">{hint}</p> : null}
      </div>
    </div>
  )
}
