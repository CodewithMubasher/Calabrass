import { HugeiconsIcon } from "@hugeicons/react"
import { LogOutIcon } from "@hugeicons/core-free-icons"
import { useNavigate } from "@tanstack/react-router"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"
import { signOut, useSession } from "@/lib/auth"

/**
 * The account at the foot of the sidebar: who is signed in, and the way out.
 *
 * The name and the email are the session's, not a placeholder — this is the one
 * place in the app that says who it thinks you are. The icon on the right is
 * the way out: it drops the session and hands the router the login page, which
 * the gate in the root route then keeps shut behind you.
 *
 * Renders nothing without a session, which only happens on the way to the login
 * page: there is nobody to show.
 */
export function NavUser() {
  const user = useSession()
  const navigate = useNavigate()

  if (!user) return null

  const initials = user.name.trim().slice(0, 2).toUpperCase() || "?"

  const handleSignOut = () => {
    signOut()
    void navigate({ to: "/login" })
  }

  return (
    <SidebarMenu>
      <SidebarMenuItem>
        {/* The account is a row, not a menu: the way out is the icon on the
            right of it, so there is nothing to open. */}
        <SidebarMenuButton
          size="lg"
          render={<div />}
          className="hover:bg-transparent"
        >
          <Avatar className="size-8 rounded-lg">
            <AvatarFallback className="rounded-lg">{initials}</AvatarFallback>
          </Avatar>
          <div className="grid flex-1 text-left leading-tight">
            <span className="truncate font-semibold">{user.name}</span>
            <span className="truncate text-xs">{user.email}</span>
          </div>
          <SidebarMenuButton
            size="sm"
            tooltip="Log out"
            onClick={handleSignOut}
            aria-label="Log out"
            className="size-7 w-auto shrink-0"
          >
            <HugeiconsIcon icon={LogOutIcon} strokeWidth={2} className="size-4" />
          </SidebarMenuButton>
        </SidebarMenuButton>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
