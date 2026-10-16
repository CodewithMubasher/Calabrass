import { useState } from "react"

import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { createConnector } from "@/lib/connector-store"

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

/**
 * Registers an MCP server.
 *
 * Mounted only while it is open, so the form starts blank without an effect.
 */
export function ConnectorDialog({
  onOpenChange,
}: {
  onOpenChange: (open: boolean) => void
}) {
  const [name, setName] = useState("")
  const [command, setCommand] = useState("")
  const [error, setError] = useState<string | null>(null)
  const [saving, setSaving] = useState(false)

  const ready = name.trim() !== "" && command.trim() !== ""

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (saving || !ready) return

    setSaving(true)
    setError(null)
    try {
      await createConnector({ name: name.trim(), command: command.trim() })
      onOpenChange(false)
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Could not add the connector."
      )
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
              Add MCP Connector
            </DialogTitle>
          </DialogHeader>

          <div className="py-2">
            <div className="mb-4">
              <Field label="Name">
                <Input
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  placeholder="Win Control"
                  autoFocus
                  className="h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30"
                />
              </Field>
            </div>
            <div>
              <Field label="Command">
                <Input
                  value={command}
                  onChange={(event) => setCommand(event.target.value)}
                  placeholder={'python "C:\\path\\to\\server.py"'}
                  className="h-9 border-white/10 bg-white/5 px-3 font-mono text-white/80 placeholder:text-white/30"
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
            <Button type="submit" disabled={saving || !ready}>
              {saving ? "Adding…" : "Add connector"}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  )
}
