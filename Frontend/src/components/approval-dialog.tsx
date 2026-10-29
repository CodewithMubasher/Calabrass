import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { decide, useApprovals } from "@/lib/approval-store"
import type { Approval } from "@/types/chat"

const SURFACE = "sm:max-w-lg bg-popover border border-white/10"

/**
 * Asks the person before the agent does something that needs their say-so: run
 * a command, reach outside the project, or act on the outside world.
 *
 * It is modal on purpose and has no way to dismiss it without answering: the
 * agent is stopped, waiting, and an unanswered request counts as a refusal when
 * it times out. The oldest waiting request is shown first.
 */
export function ApprovalDialog() {
  const { approvals, answering } = useApprovals()
  const current = approvals[0]
  if (!current) return null

  const busy = answering.includes(current.id)
  const risky = current.tier >= 3

  return (
    <Dialog open onOpenChange={() => {}}>
      <DialogContent className={SURFACE}>
        <DialogHeader>
          <DialogTitle className="text-white/90">
            {risky ? "Allow this? It is hard to undo" : "Allow the agent to do this?"}
          </DialogTitle>
          <DialogDescription className="text-white/50">
            {describeTier(current)}
          </DialogDescription>
        </DialogHeader>

        <div className="grid gap-2">
          <p className="text-sm text-white/80">{current.summary}</p>
          {current.detail && current.detail !== current.summary ? (
            <pre className="max-h-48 overflow-auto rounded-xl border border-white/10 bg-black/30 p-3 font-mono text-xs break-words whitespace-pre-wrap text-white/70">
              {current.detail}
            </pre>
          ) : null}
          {approvals.length > 1 ? (
            <p className="text-xs text-white/35">
              {approvals.length - 1} more waiting after this one.
            </p>
          ) : null}
        </div>

        <DialogFooter className="gap-2 sm:flex-wrap">
          <Button
            type="button"
            variant="ghost"
            disabled={busy}
            onClick={() => void decide(current.id, "deny")}
          >
            Deny
          </Button>
          {current.canRemember && current.pattern ? (
            <Button
              type="button"
              variant="ghost"
              disabled={busy}
              onClick={() => void decide(current.id, "allow_pattern")}
              title="Allow this kind of command in this project from now on"
            >
              Always allow “{current.pattern}” here
            </Button>
          ) : null}
          {current.canRemember ? (
            <Button
              type="button"
              variant="ghost"
              disabled={busy}
              onClick={() => void decide(current.id, "allow_run")}
              title="Allow commands for the rest of this conversation. Anything riskier still asks."
            >
              Allow for this session
            </Button>
          ) : null}
          <Button
            type="button"
            disabled={busy}
            onClick={() => void decide(current.id, "allow_once")}
          >
            Allow once
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

function describeTier(a: Approval): string {
  switch (a.tier) {
    case 2:
      return "It wants to run something on your computer."
    case 3:
      return "It wants to reach outside the project, or do something that cannot easily be undone. This is asked every time."
    case 4:
      return "It wants to act on the outside world: send, publish, push or delete something. This is asked every time."
    default:
      return a.tierName
  }
}
