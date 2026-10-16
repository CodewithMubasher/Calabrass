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

export function NavRecents({
  chats,
}: {
  chats: {
    name: string
    url: string
  }[]
}) {
  const { isMobile } = useSidebar()

  return (
    <SidebarGroup className="group-data-[collapsible=icon]:hidden">
      <SidebarGroupLabel>Recents</SidebarGroupLabel>
      <SidebarGroupAction aria-label="Filter recents" title="Filter">
        <HugeiconsIcon icon={FilterHorizontalIcon} strokeWidth={2} />
      </SidebarGroupAction>
      <SidebarMenu>
        {chats.map((chat) => (
          <SidebarMenuItem key={chat.name}>
            <SidebarMenuButton render={<a href={chat.url} />}>
              <HugeiconsIcon icon={Chat01Icon} strokeWidth={2} />
              <span>{chat.name}</span>
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
                <DropdownMenuItem variant="destructive">
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
