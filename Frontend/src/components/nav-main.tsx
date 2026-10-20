import { Link, useLocation } from "@tanstack/react-router"
import { HugeiconsIcon } from "@hugeicons/react"
import type { IconSvgElement } from "@hugeicons/react"

import { Badge } from "@/components/ui/badge"
import {
  SidebarGroup,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
} from "@/components/ui/sidebar"

export type NavTo =
  | "/"
  | "/cal-live"
  | "/workspaces"
  | "/connectors"
  | "/agents"
  | "/schedules"
  | "/providers"
  | "/usage"
  | "/autonomous"

export interface NavItem {
  title: string
  to: NavTo
  icon: IconSvgElement
  /** A short marker shown beside the title, such as "Beta". */
  badge?: string
}

export function NavMain({ items }: { items: NavItem[] }) {
  const { pathname } = useLocation()

  return (
    <SidebarGroup>
      <SidebarMenu>
        {items.map((item) => (
          <SidebarMenuItem key={item.title}>
            <SidebarMenuButton
              tooltip={item.title}
              isActive={pathname === item.to}
              render={<Link to={item.to} />}
            >
              <HugeiconsIcon icon={item.icon} strokeWidth={2} />
              <span>{item.title}</span>
              {item.badge ? (
                <Badge className="ml-auto h-4 border-transparent bg-sky-50 px-1.5 text-[10px] text-sky-700 dark:bg-sky-950 dark:text-sky-300">
                  {item.badge}
                </Badge>
              ) : null}
            </SidebarMenuButton>
          </SidebarMenuItem>
        ))}
      </SidebarMenu>
    </SidebarGroup>
  )
}
