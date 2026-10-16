import { useState } from "react"
import { HugeiconsIcon } from "@hugeicons/react"
import { Folder01Icon } from "@hugeicons/core-free-icons"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { createWorkspace } from "@/lib/workspace-store"
import type { WorkspaceRow } from "@/types/chat"

// The dialogs sit on the same dark panel as the reference project's, with our
// own popover token standing in for its neutral-900.
const SURFACE = "sm:max-w-md bg-popover border border-white/10"

function Field({
  label,
  children,
}: {
  label: string
  children: React.ReactNode
}) {
  return (
    <label className="block">
      <span className="mb-2 block text-sm text-white/60">{label}</span>
      {children}
    </label>
  )
}

function message(err: unknown, fallback: string) {
  return err instanceof Error ? err.message : fallback
}

function formatDate(at: number): string {
  if (!at) return ""
  return new Date(at).toLocaleDateString("en-US", {
    month: "short",
    day: "numeric",
    year: "numeric",
  })
}

/**
 * Adds a workspace. The server makes the folder as it stores the row, so the
 * workspace can be worked in as soon as it comes back.
 *
 * Mounted only while it is open, so the form starts blank without an effect.
 */
export function NewWorkspaceDialog({
  onOpenChange,
}: {
  onOpenChange: (open: boolean) => void
}) {
  const [name, setName] = useState("")
  const [description, setDescription] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (saving || !name.trim()) return

    setSaving(true)
    setError(null)
    try {
      await createWorkspace({
        name: name.trim(),
        description: description.trim(),
      })
      onOpenChange(false)
    } catch (err) {
      setError(message(err, "Could not create the workspace."))
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className={SURFACE}>
        <form onSubmit={submit}>
          <DialogHeader>
            <DialogTitle className="text-white/90">
              Create a workspace
            </DialogTitle>
          </DialogHeader>

          <div className="py-2">
            <div className="mb-5">
              <Field label="What are you working on?">
                <Input
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Workspace name"
                  autoFocus
                  className="h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30"
                />
              </Field>
            </div>
            <div>
              <Field label="What are you trying to achieve?">
                <textarea
                  value={description}
                  onChange={(event) => setDescription(event.target.value)}
                  placeholder="Describe your goal..."
                  rows={3}
                  className="w-full resize-none rounded-2xl border border-white/10 bg-white/5 px-3 py-2 text-sm text-white/80 transition-colors outline-none placeholder:text-white/30 focus:border-white/20"
                />
              </Field>
            </div>
            {error ? <p className="mt-4 text-sm text-red-400">{error}</p> : null}
          </div>

          <DialogFooter className="gap-2">
            <Button
              type="button"
              variant="ghost"
              onClick={() => onOpenChange(false)}
            >
              Cancel
            </Button>
            <Button type="submit" disabled={saving}>
              {saving ? "Creating…" : "Create workspace"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}

/**
 * Shows where one workspace lives and what it holds. Removing it is the card
 * menu's job; this is the read-only half of the workspace detail screen the
 * reference project opens from a card.
 */
export function WorkspaceInfoDialog({
  workspace,
  onOpenChange,
}: {
  workspace: WorkspaceRow
  onOpenChange: (open: boolean) => void
}) {
  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className={SURFACE}>
        <DialogHeader>
          <DialogTitle className="flex items-center gap-3 text-white/90">
            <span className="flex size-9 shrink-0 items-center justify-center rounded-xl bg-white/5">
              <HugeiconsIcon
                icon={Folder01Icon}
                strokeWidth={1.8}
                className="size-4 text-white/50"
              />
            </span>
            {workspace.name}
          </DialogTitle>
        </DialogHeader>

        <div className="py-2">
          <div className="mb-5">
            <span className="mb-2 block text-sm text-white/60">Folder</span>
            <code
              title={workspace.path}
              className="block truncate rounded-xl border border-white/10 bg-white/5 px-3 py-2 font-mono text-xs text-white/80"
            >
              {workspace.path || `workspace/${workspace.dir}`}
            </code>
            <p className="mt-2 text-xs text-white/40">
              The agent works in this folder, and can never reach outside it.
            </p>
          </div>

          {workspace.description ? (
            <div className="mb-5">
              <span className="mb-2 block text-sm text-white/60">
                Description
              </span>
              <p className="text-sm text-white/80">{workspace.description}</p>
            </div>
          ) : null}

          <div>
            <span className="mb-2 block text-sm text-white/60">Created</span>
            <p className="text-sm text-white/80">
              {formatDate(workspace.createdAt)}
            </p>
          </div>
        </div>

        <DialogFooter className="gap-2">
          <Button onClick={() => onOpenChange(false)}>Close</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
