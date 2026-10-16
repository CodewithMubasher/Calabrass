import { useState } from "react"

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
import { renameChat } from "@/lib/chat-store"
import type { Chat } from "@/types/chat"

/** Retitles a chat. Mounted only while open, so the field starts filled. */
export function RenameChatDialog({
  chat,
  onOpenChange,
}: {
  chat: Chat
  onOpenChange: (open: boolean) => void
}) {
  const [title, setTitle] = useState(chat.title)
  const [saving, setSaving] = useState(false)

  const ready = title.trim() !== "" && title.trim() !== chat.title

  async function submit(event: React.FormEvent) {
    event.preventDefault()
    if (saving || !ready) return

    setSaving(true)
    try {
      await renameChat(chat.id, title)
      onOpenChange(false)
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-md border border-white/10 bg-popover">
        <form onSubmit={submit} className="grid gap-5">
          <DialogHeader>
            <DialogTitle className="text-white/90">Rename chat</DialogTitle>
            <DialogDescription className="text-white/50">
              The name shown in your recents.
            </DialogDescription>
          </DialogHeader>

          <Input
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            aria-label="Chat name"
            maxLength={120}
            autoFocus
            className="h-9 border-white/10 bg-white/5 px-3 text-white/80 placeholder:text-white/30"
          />

          <DialogFooter className="gap-2">
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
