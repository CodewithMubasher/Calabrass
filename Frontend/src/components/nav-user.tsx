import { HugeiconsIcon } from "@hugeicons/react"
import {
  LogOutIcon,
  More01Icon,
  SparklesIcon,
  UserCircleIcon,
} from "@hugeicons/core-free-icons"
import { useNavigate } from "@tanstack/react-router"

import { Avatar, AvatarFallback } from "@/components/ui/avatar"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar"
import { signOut, useSession } from "@/lib/auth"

/**
 * The account at the foot of the sidebar: who is signed in, and the way out.
 *
 * The name and the email are the session's, not a placeholder — this is the one
 * place in the app that says who it thinks you are. Signing out drops the
 * session and hands the router the login page, which the gate in the root route
 * then keeps shut behind you.
 *
 * Renders nothing without a session, which only happens on the way to the login
 * page: there is nobody to show.
 */
export function NavUser() {
  const user = useSession()
  const { isMobile } = useSidebar()
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
        <DropdownMenu>
          <DropdownMenuTrigger
            render={
              <SidebarMenuButton
                size="lg"
                className="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground"
              >
                <Avatar className="size-8 rounded-lg">
                  <AvatarFallback className="rounded-lg">
                    {initials}
                  </AvatarFallback>
                </Avatar>
                <div className="grid flex-1 text-left leading-tight">
                  <span className="truncate font-semibold">{user.name}</span>
                  <span className="truncate text-xs">{user.email}</span>
                </div>
                <HugeiconsIcon
                  icon={More01Icon}
                  strokeWidth={2}
                  className="ml-auto size-4"
                />
              </SidebarMenuButton>
            }
          />
          <DropdownMenuContent
            className="w-(--dropdown-menu-trigger-width) min-w-56 rounded-lg"
            side={isMobile ? "bottom" : "right"}
            align="end"
            sideOffset={4}
          >
            <DropdownMenuGroup>
              <DropdownMenuLabel className="p-0 font-normal">
                <div className="flex items-center gap-2 px-1 py-1.5 text-left text-sm">
                  <Avatar className="size-8 rounded-lg">
                    <AvatarFallback className="rounded-lg">
                      {initials}
                    </AvatarFallback>
                  </Avatar>
                  <div className="grid flex-1 text-left leading-tight">
                    <span className="truncate font-semibold">{user.name}</span>
                    <span className="truncate text-xs">{user.email}</span>
                  </div>
                </div>
              </DropdownMenuLabel>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem>
                <HugeiconsIcon icon={SparklesIcon} strokeWidth={2} />
                Upgrade to Pro
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuGroup>
              <DropdownMenuItem>
                <HugeiconsIcon icon={UserCircleIcon} strokeWidth={2} />
                Profile
              </DropdownMenuItem>
            </DropdownMenuGroup>
            <DropdownMenuSeparator />
            <DropdownMenuItem onClick={handleSignOut}>
              <HugeiconsIcon icon={LogOutIcon} strokeWidth={2} />
              Log out
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </SidebarMenuItem>
    </SidebarMenu>
  )
}
