import { useState } from "react"
import { Link, useLocation, useNavigate } from "@tanstack/react-router"
import { HugeiconsIcon } from "@hugeicons/react"
import {
  ApertureIcon,
  Delete02Icon,
  FolderOpenIcon,
  MessageSquareTextIcon,
  MoreHorizontalIcon,
  PencilEdit02Icon,
  PinIcon,
  PinOffIcon,
} from "@hugeicons/core-free-icons"

import { RenameChatDialog } from "@/components/rename-chat-dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
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
} from "@/components/ui/sidebar"
import { deleteChat, togglePinChat, useChats } from "@/lib/chat-store"
import type { Chat } from "@/types/chat"

export function NavRecents() {
  const { pathname } = useLocation()
  const navigate = useNavigate()
  const chats = useChats()
  const [renaming, setRenaming] = useState<Chat | null>(null)

  const handleDelete = (chatId: string) => {
    void deleteChat(chatId)
    if (pathname === `/chat/${chatId}` || pathname === `/crew/${chatId}`) {
      void navigate({ to: "/" })
    }
  }

  return (
    <SidebarGroup className="group-data-[collapsible=icon]:hidden">
      <SidebarGroupLabel>Recents</SidebarGroupLabel>
      <SidebarGroupAction aria-label="Filter recents" title="Filter">
        <HugeiconsIcon icon={MoreHorizontalIcon} strokeWidth={2} />
      </SidebarGroupAction>
      <SidebarMenu>
        {chats.length === 0 && (
          <SidebarMenuItem>
            <span className="block px-2 py-1.5 text-xs text-sidebar-foreground/50">
              No chats yet
            </span>
          </SidebarMenuItem>
        )}
        {chats.map((chat) => {
          // A crew run lives under its own address, so the list points at
          // where the conversation actually is.
          const crew = chat.mode === "crew"
          const href = crew ? `/crew/${chat.id}` : `/chat/${chat.id}`
          return (
            <SidebarMenuItem key={chat.id}>
              <SidebarMenuButton
                tooltip={chat.title}
                isActive={pathname === href}
                render={
                  crew ? (
                    <Link to="/crew/$chatId" params={{ chatId: chat.id }} />
                  ) : (
                    <Link to="/chat/$chatId" params={{ chatId: chat.id }} />
                  )
                }
              >
                <HugeiconsIcon
                  // A crew run wears the crew mark; an ordinary chat wears the
                  // message mark. A pinned chat wears the pin either way.
                  icon={
                    chat.pinned
                      ? PinIcon
                      : crew
                        ? ApertureIcon
                        : MessageSquareTextIcon
                  }
                  strokeWidth={2}
                  className="shrink-0"
                />
                <span className="truncate">{chat.title}</span>
              </SidebarMenuButton>
              <DropdownMenu>
                <DropdownMenuTrigger
                  render={
                    <SidebarMenuAction showOnHover>
                      <HugeiconsIcon
                        icon={MoreHorizontalIcon}
                        strokeWidth={2}
                        className="size-4"
                      />
                      <span className="sr-only">More</span>
                    </SidebarMenuAction>
                  }
                />
                <DropdownMenuContent align="end" sideOffset={4}>
                  <DropdownMenuItem onClick={() => setRenaming(chat)}>
                    <HugeiconsIcon icon={PencilEdit02Icon} strokeWidth={2} />
                    <span>Rename</span>
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onClick={() => void togglePinChat(chat.id, !chat.pinned)}
                  >
                    <HugeiconsIcon
                      icon={chat.pinned ? PinOffIcon : PinIcon}
                      strokeWidth={2}
                    />
                    <span>{chat.pinned ? "Unpin chat" : "Pin chat"}</span>
                  </DropdownMenuItem>
                  <DropdownMenuItem>
                    <HugeiconsIcon icon={FolderOpenIcon} strokeWidth={2} />
                    <span>Add to project</span>
                  </DropdownMenuItem>
                  <DropdownMenuSeparator />
                  <DropdownMenuItem
                    variant="destructive"
                    onClick={() => handleDelete(chat.id)}
                  >
                    <HugeiconsIcon icon={Delete02Icon} strokeWidth={2} />
                    <span>Delete chat</span>
                  </DropdownMenuItem>
                </DropdownMenuContent>
              </DropdownMenu>
            </SidebarMenuItem>
          )
        })}
      </SidebarMenu>

      {renaming ? (
        <RenameChatDialog
          key={renaming.id}
          chat={renaming}
          onOpenChange={(open) => {
            if (!open) setRenaming(null)
          }}
        />
      ) : null}
    </SidebarGroup>
  )
}
