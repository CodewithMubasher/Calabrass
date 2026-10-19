import { useState, type ReactNode } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Delete02Icon, ViewIcon, ViewOffIcon } from "@hugeicons/core-free-icons"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import {
  createProvider,
  deleteProvider,
  saveProvider,
} from "@/lib/provider-store"
import type { ProviderRow } from "@/types/chat"

// The dialogs sit on the same dark panel as the reference project's, with our
// own popover token standing in for its neutral-900.
const SURFACE = "sm:max-w-md bg-popover border border-white/10"

function Field({
  label,
  hint,
  children,
}: {
  label: string
  hint?: string
  children: ReactNode
}) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm text-white/60">{label}</span>
      {children}
      {hint ? (
        <span className="mt-1.5 block text-xs text-white/40">{hint}</span>
      ) : null}
    </label>
  )
}

/** The API key field, with a reveal toggle. */
function KeyField({
  value,
  onChange,
  revealed,
  onReveal,
}: {
  value: string
  onChange: (value: string) => void
  revealed: boolean
  onReveal: () => void
}) {
  return (
    <Field label="API key" hint="Leave empty for a local server that takes no key.">
      <div className="relative">
        <Input
          value={value}
          onChange={(event) => onChange(event.target.value)}
          type={revealed ? "text" : "password"}
          placeholder="sk-…"
          autoComplete="off"
          spellCheck={false}
          className="h-9 border-white/10 bg-white/5 pr-8 font-mono text-xs text-white/80 placeholder:text-white/30"
        />
        <button
          type="button"
          onClick={onReveal}
          aria-label={revealed ? "Hide the API key" : "Show the API key"}
          className="absolute top-1/2 right-1.5 -translate-y-1/2 rounded-lg p-1 text-white/40 transition-colors hover:text-white/80"
        >
          <HugeiconsIcon
            icon={revealed ? ViewOffIcon : ViewIcon}
            strokeWidth={1.8}
            className="size-4"
          />
        </button>
      </div>
    </Field>
  )
}

function message(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback
}

/**
 * Adds a custom, OpenAI-compatible provider.
 *
 * Mounted only while it is open, so the form starts blank without an effect.
 */
export function AddProviderDialog({
  onOpenChange,
}: {
  onOpenChange: (open: boolean) => void
}) {
  const [name, setName] = useState("")
  const [baseUrl, setBaseUrl] = useState("")
  const [apiKey, setApiKey] = useState("")
  const [revealed, setRevealed] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const ready = name.trim() !== "" && baseUrl.trim() !== ""

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (saving || !ready) return

    setSaving(true)
    setError(null)
    try {
      await createProvider({
        name: name.trim(),
        baseUrl: baseUrl.trim(),
        apiKey: apiKey.trim(),
      })
      onOpenChange(false)
    } catch (err) {
      setError(message(err, "Could not add the provider."))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className={SURFACE}>
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle className="text-white">New provider</DialogTitle>
            <DialogDescription className="text-white/50">
              Any OpenAI-compatible endpoint: OpenRouter, Together, Ollama,
              LiteLLM…
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-3">
            <Field label="Name">
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                placeholder="OpenRouter"
                autoFocus
                className="h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30"
              />
            </Field>
            <Field
              label="Base URL"
              hint="Include the version, e.g. https://api.example.com/v1"
            >
              <Input
                value={baseUrl}
                onChange={(event) => setBaseUrl(event.target.value)}
                placeholder="https://api.example.com/v1"
                autoComplete="off"
                spellCheck={false}
                className="h-9 border-white/10 bg-white/5 px-3 font-mono text-xs text-white/80 placeholder:text-white/30"
              />
            </Field>
            <KeyField
              value={apiKey}
              onChange={setApiKey}
              revealed={revealed}
              onReveal={() => setRevealed((shown) => !shown)}
            />
          </div>

          {error ? <p className="text-sm text-red-400">{error}</p> : null}

          <DialogFooter>
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={saving || !ready}>
              {saving ? "Saving…" : "Save"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/**
 * Shows one provider's settings and edits them.
 *
 * Mounted per provider, keyed by id, so switching cards refills the form
 * without an effect.
 */
export function ProviderDetailDialog({
  provider,
  onOpenChange,
}: {
  provider: ProviderRow
  onOpenChange: (open: boolean) => void
}) {
  const [name, setName] = useState(provider.name)
  const [baseUrl, setBaseUrl] = useState(provider.baseUrl)
  const [apiKey, setApiKey] = useState(provider.apiKey)
  const [revealed, setRevealed] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const ready = name.trim() !== "" && baseUrl.trim() !== ""

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (saving || !ready) return

    setSaving(true)
    setError(null)
    try {
      await saveProvider(provider.id, {
        apiKey: apiKey.trim(),
        name: name.trim(),
        baseUrl: baseUrl.trim(),
      })
      onOpenChange(false)
    } catch (err) {
      setError(message(err, "Could not save the provider."))
    } finally {
      setSaving(false)
    }
  }

  async function remove() {
    if (saving) return

    setSaving(true)
    setError(null)
    try {
      await deleteProvider(provider.id)
      onOpenChange(false)
    } catch (err) {
      setError(message(err, "Could not remove the provider."))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className={SURFACE}>
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle className="text-white">{provider.name}</DialogTitle>
            <DialogDescription className="text-white/50">
              A provider you added. Its models are listed from its endpoint.
            </DialogDescription>
          </DialogHeader>

          <div className="grid gap-3">
            <Field label="Name">
              <Input
                value={name}
                onChange={(event) => setName(event.target.value)}
                className="h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30"
              />
            </Field>
            <Field label="Base URL">
              <Input
                value={baseUrl}
                onChange={(event) => setBaseUrl(event.target.value)}
                autoComplete="off"
                spellCheck={false}
                className="h-9 border-white/10 bg-white/5 px-3 font-mono text-xs text-white/80 placeholder:text-white/30"
              />
            </Field>

            <KeyField
              value={apiKey}
              onChange={setApiKey}
              revealed={revealed}
              onReveal={() => setRevealed((shown) => !shown)}
            />
          </div>

          {error ? <p className="text-sm text-red-400">{error}</p> : null}

          <DialogFooter className="sm:justify-between">
            <Button
              type="button"
              variant="destructive"
              onClick={() => void remove()}
              disabled={saving}
            >
              <HugeiconsIcon icon={Delete02Icon} strokeWidth={1.8} />
              Delete
            </Button>
            <div className="flex flex-col-reverse gap-2 sm:flex-row">
              <Button
                type="button"
                variant="ghost"
                onClick={() => onOpenChange(false)}
              >
                Cancel
              </Button>
              <Button type="submit" disabled={saving || !ready}>
                {saving ? "Saving…" : "Save"}
              </Button>
            </div>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
