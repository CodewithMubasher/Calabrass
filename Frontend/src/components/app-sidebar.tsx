import * as React from "react"
import { Link } from "@tanstack/react-router"
import {
  AddCircleHalfDotIcon,
  AiSpeechIcon,
  ApertureIcon,
  ChartRelationshipIcon,
  File02Icon,
  Folder01Icon,
  PieChartIcon,
  ThreeDScaleIcon,
  TimeScheduleIcon,
} from "@hugeicons/core-free-icons"

import { NavMain, type NavItem } from "@/components/nav-main"
import { NavRecents } from "@/components/nav-recents"
import { NavUser } from "@/components/nav-user"
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarRail,
} from "@/components/ui/sidebar"

const data = {
  navMain: [
    {
      title: "New Chat",
      to: "/",
      icon: AddCircleHalfDotIcon,
    },
    {
      title: "Cal Live",
      to: "/cal-live",
      icon: AiSpeechIcon,
      badge: "Beta",
    },
    {
      title: "Workspaces",
      to: "/workspaces",
      icon: Folder01Icon,
    },
    {
      title: "Crew work",
      to: "/agents",
      icon: ApertureIcon,
    },
    {
      title: "Connectors",
      to: "/connectors",
      icon: ChartRelationshipIcon,
    },
    {
      title: "Schedules",
      to: "/schedules",
      icon: TimeScheduleIcon,
    },
    {
      title: "Providers",
      to: "/providers",
      icon: ThreeDScaleIcon,
    },
    {
      title: "Usage",
      to: "/usage",
      icon: PieChartIcon,
    },
    {
      title: "Ledger",
      to: "/ledger",
      icon: File02Icon,
    },
  ] as NavItem[],
}

export function AppSidebar({ ...props }: React.ComponentProps<typeof Sidebar>) {
  return (
    <Sidebar
      collapsible="icon"
      className="bg-sidebar/90 backdrop-blur-md"
      {...props}
    >
      <SidebarHeader>
        <SidebarMenu>
          <SidebarMenuItem>
            <SidebarMenuButton
              size="lg"
              render={
                <Link to="/">
                  <img src="/vite.svg" alt="Calabrass" className="size-6" />
                  <div className="grid flex-1 text-left leading-tight">
                    <span className="truncate font-semibold">Calabrass</span>
                  </div>
                </Link>
              }
            />
          </SidebarMenuItem>
        </SidebarMenu>
      </SidebarHeader>
      <SidebarContent>
        <NavMain items={data.navMain} />
        <NavRecents />
      </SidebarContent>
      <SidebarFooter>
        <NavUser />
      </SidebarFooter>
      <SidebarRail />
    </Sidebar>
  )
}
