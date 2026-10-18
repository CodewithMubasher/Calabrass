import * as React from "react"
import { Link } from "@tanstack/react-router"
import {
  AddCircleHalfDotIcon,
  AiSpeechIcon,
  BotIcon,
  ChartRelationshipIcon,
  Folder01Icon,
  Settings02Icon,
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
    },
    {
      title: "Workspaces",
      to: "/workspaces",
      icon: Folder01Icon,
    },
    {
      title: "Agents",
      to: "/agents",
      icon: BotIcon,
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
      title: "Settings",
      to: "/settings",
      icon: Settings02Icon,
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
