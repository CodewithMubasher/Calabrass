import { Link, useLocation, useNavigate } from "@tanstack/react-router"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  Chat01Icon,
  EllipsisVerticalIcon,
  FilterHorizontalIcon,
} from "@hugeicons/core-free-icons"

import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupLabel,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuButton,
  SidebarMenuItem,
  useSidebar,
} from "@/components/ui/sidebar"
import { deleteChat, useChats } from "@/lib/chat-store"

export function NavRecents() {
  const { isMobile } = useSidebar()
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const chats = useChats()

  const handleDelete = (chatId: string) => {
    void deleteChat(chatId)
    if (pathname === `/chat/${chatId}`) {
      void navigate({ to: "/" })
    }
  }

  return (
    <SidebarGroup className="group-data-[collapsible=icon]:hidden">
      <SidebarGroupLabel>Recents</SidebarGroupLabel>
      <SidebarGroupAction aria-label="Filter recents" title="Filter">
        <HugeiconsIcon icon={FilterHorizontalIcon} strokeWidth={2} />
      </SidebarGroupAction>
      <SidebarMenu>
        {chats.length === 0 && (
          <SidebarMenuItem>
            <span className="block px-2 py-1.5 text-xs text-sidebar-foreground/50">
              No chats yet
            </span>
          </SidebarMenuItem>
        )}
        {chats.map((chat) => (
          <SidebarMenuItem key={chat.id}>
            <SidebarMenuButton
              tooltip={chat.title}
              isActive={pathname === `/chat/${chat.id}`}
              render={<Link to="/chat/$chatId" params={{ chatId: chat.id }} />}
            >
              <HugeiconsIcon icon={Chat01Icon} strokeWidth={2} />
              <span>{chat.title}</span>
            </SidebarMenuButton>
            <DropdownMenu>
              <DropdownMenuTrigger
                render={
                  <SidebarMenuAction showOnHover>
                    <HugeiconsIcon icon={EllipsisVerticalIcon} strokeWidth={2} />
                    <span className="sr-only">More</span>
                  </SidebarMenuAction>
                }
              />
              <DropdownMenuContent
                className="w-48 rounded-lg"
                side={isMobile ? "bottom" : "right"}
                align={isMobile ? "end" : "start"}
              >
                <DropdownMenuItem>
                  <span>Rename</span>
                </DropdownMenuItem>
                <DropdownMenuItem>
                  <span>Star</span>
                </DropdownMenuItem>
                <DropdownMenuItem
                  variant="destructive"
                  onClick={() => handleDelete(chat.id)}
                >
                  <span>Delete</span>
                </DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu>
          </SidebarMenuItem>
        ))}
      </SidebarMenu>
    </SidebarGroup>
  )
}
