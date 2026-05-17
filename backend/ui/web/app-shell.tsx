"use client";

import { type CSSProperties, useEffect, useState } from "react";
import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  MessageSquare, Bot, Radio, Zap, Brain, BarChart3, Settings,
  LayoutDashboard, SearchIcon, BellIcon, Target, ActivityIcon, BugIcon, GithubIcon,
} from "lucide-react";
import { useAgent } from "@/lib/agent-context";
import { Skeleton } from "@/components/ui/skeleton";
import { Button } from "@/components/ui/button";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import {
  Sidebar, SidebarContent, SidebarGroup, SidebarGroupContent,
  SidebarHeader, SidebarMenu, SidebarMenuButton, SidebarMenuItem,
  SidebarProvider, SidebarTrigger,
} from "@/components/ui/sidebar";

const NAV = [
  { label: "Dashboard", href: "/dashboard", icon: LayoutDashboard },
  { label: "Chat", href: "/", icon: MessageSquare },
  { label: "Agents", href: "/agents", icon: Bot },
  { label: "Channels", href: "/channels", icon: Radio },
  { label: "Skills", href: "/skills", icon: Zap },
  { label: "Knowledge", href: "/knowledge", icon: Brain },
  { label: "Analytics", href: "/analytics", icon: BarChart3 },
  { label: "Mission Control", href: "/mission-control", icon: Target },
  { label: "Settings", href: "/settings", icon: Settings },
];

function ChatList() {
  const [sessions, setSessions] = useState<any[]>([]);
  const { agents, activeAgent, setActiveAgent } = useAgent();

  useEffect(() => {
    const load = () => fetch("/api/sessions").then(r => r.json()).then(d => setSessions((d.sessions || []).slice(0, 20))).catch(() => {});
    load();
    const interval = setInterval(load, 5000);
    return () => clearInterval(interval);
  }, []);

  const deleteSession = async (id: string, e: React.MouseEvent) => {
    e.preventDefault(); e.stopPropagation();
    await fetch("/api/sessions", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ action: "delete", id }) });
    setSessions(prev => prev.filter(s => s.id !== id));
  };

  return (
    <div className="flex flex-col gap-1">
      {/* Agents */}
      {agents.map(a => (
        <button key={a.id} onClick={() => setActiveAgent(a)}
          className={"flex items-center gap-3 py-2.5 px-2 rounded-md transition-colors w-full text-left " + (activeAgent?.id === a.id ? "bg-accent" : "hover:bg-accent/50")}>
          <div className="size-9 rounded-full bg-primary/10 flex items-center justify-center shrink-0">
            <Bot className="size-4 text-primary" />
          </div>
          <div className="flex-1 min-w-0">
            <p className="text-xs font-medium truncate">{a.display_name || a.agent_key}</p>
            <p className="text-[10px] text-muted-foreground truncate">{a.model}</p>
          </div>
        </button>
      ))}

      {agents.length === 0 && (
        <div className="text-center py-6">
          <p className="text-xs text-muted-foreground">No agents yet</p>
          <Link href="/agents" className="text-xs text-primary">Create an agent</Link>
        </div>
      )}
    </div>
  );
}


function SidebarStats() {
  const [stats, setStats] = useState({ agents: 0, sessions: 0 });
  useEffect(() => {
    Promise.all([
      fetch("/v1/agents", { headers: { Authorization: "Bearer test123" } }).then(r => r.json()),
      fetch("/v1/sessions", { headers: { Authorization: "Bearer test123" } }).then(r => r.json()),
    ]).then(([a, s]) => setStats({ agents: a.agents?.length || 0, sessions: s.sessions?.length || 0 })).catch(() => {});
  }, []);
  return (
    <div className="mt-2 flex flex-col px-4">
      <div className="mb-4 grid grid-cols-2 gap-4">
        <div className="flex flex-col items-start gap-2 rounded-md border border-dashed p-2">
          <p className="text-xs text-muted-foreground">Agents</p>
          <p className="text-sm font-semibold">{stats.agents}</p>
        </div>
        <div className="flex flex-col items-start gap-2 rounded-md border border-dashed p-2">
          <p className="text-xs text-muted-foreground">Sessions</p>
          <p className="text-sm font-semibold">{stats.sessions}</p>
        </div>
      </div>
    </div>
  );
}

function PageTitle() {
  const pathname = usePathname();
  const { activeAgent } = useAgent();

  const titles: Record<string, string> = {
    "/dashboard": "Dashboard", "/agents": "Agents", "/channels": "Channels",
    "/skills": "Skills", "/knowledge": "Knowledge", "/analytics": "Analytics",
    "/mission-control": "Mission Control", "/settings": "Settings",
  };

  if (pathname === "/" && activeAgent) {
    return (
      <div className="flex items-center gap-2.5">
        <div className="size-7 rounded-full bg-primary/10 flex items-center justify-center">
          <Bot className="size-3.5 text-primary" />
        </div>
        <div>
          <p className="text-sm font-semibold leading-tight">{activeAgent.display_name || activeAgent.agent_key}</p>
          <p className="text-[10px] text-muted-foreground leading-tight">{activeAgent.model}</p>
        </div>
      </div>
    );
  }

  return <h1 className="text-sm font-semibold">{titles[pathname] || "Qorven"}</h1>;
}

function GatewayStatus() {
  return (
    <div className="flex items-center gap-2 text-xs text-muted-foreground">
      <span className="relative flex size-2">
        <span className="animate-ping absolute inline-flex h-full w-full rounded-full bg-green-400 opacity-75" />
        <span className="relative inline-flex size-2 rounded-full bg-green-500" />
      </span>
      <span className="hidden sm:inline">Connected</span>
    </div>
  );
}

export function AppShell({ children }: { children: React.ReactNode }) {
  const pathname = usePathname();

  return (
    <div className="flex min-h-dvh w-full">
      <SidebarProvider defaultOpen={false} style={{ "--sidebar-width-icon": "3.5625rem" } as CSSProperties}>
        {/* Rail icon sidebar */}
        <Sidebar collapsible="icon" className="[&_[data-slot=sidebar-inner]]:bg-card">
          <SidebarHeader>
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton
                  size="lg"
                  className="gap-2.5 !bg-transparent group-data-[collapsible=icon]:!size-9 group-data-[collapsible=icon]:!p-1"
                  asChild
                >
                  <Link href="/">
                    <div className="flex size-6 items-center justify-center rounded-lg bg-primary">
                      <span className="text-sm font-bold text-primary-foreground">V</span>
                    </div>
                    <span className="text-sm font-semibold">Qorven</span>
                  </Link>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </SidebarHeader>
          <SidebarContent>
            <SidebarGroup>
              <SidebarGroupContent>
                <SidebarMenu>
                  {NAV.map((item) => (
                    <SidebarMenuItem key={item.href}>
                      <SidebarMenuButton
                        className="[&>svg]:text-primary group-data-[collapsible=icon]:!size-8 [&>svg]:size-[1.1rem]"
                        tooltip={item.label}
                        isActive={pathname === item.href}
                        asChild
                      >
                        <Link href={item.href}>
                          <item.icon />
                          <span>{item.label}</span>
                        </Link>
                      </SidebarMenuButton>
                    </SidebarMenuItem>
                  ))}
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </SidebarContent>
          <div className="mt-auto p-2 flex flex-col gap-1">
            <SidebarMenu>
              <SidebarMenuItem>
                <SidebarMenuButton tooltip="Report Bug" className="group-data-[collapsible=icon]:!size-8 [&>svg]:size-[1.1rem]" asChild>
                  <a href="https://github.com/Qorven/qorven/issues" target="_blank"><BugIcon /><span>Bug Report</span></a>
                </SidebarMenuButton>
              </SidebarMenuItem>
              <SidebarMenuItem>
                <SidebarMenuButton tooltip="GitHub" className="group-data-[collapsible=icon]:!size-8 [&>svg]:size-[1.1rem]" asChild>
                  <a href="https://github.com/Qorven/qorven" target="_blank"><GithubIcon /><span>GitHub</span></a>
                </SidebarMenuButton>
              </SidebarMenuItem>
            </SidebarMenu>
          </div>
        </Sidebar>

        {/* Secondary context sidebar — hidden on mobile */}
        <div className="bg-muted sticky top-0 flex h-dvh w-65 flex-col border-r max-lg:hidden">
          <div className="px-4 py-3.5 flex items-center justify-between">
            <span className="text-sm font-bold text-primary">Qorven</span>
            <Link href="/" className="text-muted-foreground hover:text-foreground"><MessageSquare className="size-4" /></Link>
          </div>
          <div className="px-4 pb-3">
            <div className="flex items-center gap-1.5 rounded-md border px-3 py-1.5 text-sm text-muted-foreground">
              <SearchIcon className="size-3.5" /><span>Search...</span>
            </div>
          </div>
          <SidebarStats />
          <div className="overflow-y-auto px-4 pb-3.5">
            <div className="mb-6 flex flex-col">
              <p className="text-foreground/70 mb-2 text-sm">Quick Access</p>
              <div className="grid grid-cols-2 gap-4">
                {[
                  { icon: MessageSquare, label: "New Chat", href: "/" },
                  { icon: Bot, label: "Agents", href: "/agents" },
                  { icon: Radio, label: "Channels", href: "/channels" },
                  { icon: Target, label: "Mission", href: "/mission-control" },
                ].map((item) => (
                  <Link key={item.href} href={item.href} className="hover:bg-primary/5 flex flex-col items-center gap-2 rounded-md border px-2 py-4">
                    <item.icon className="size-5" />
                    <p className="text-sm">{item.label}</p>
                  </Link>
                ))}
              </div>
            </div>
            <ChatList />
          </div>
        </div>

        {/* Main content */}
        <div className="flex flex-1 flex-col">
          <header className="bg-card sticky top-0 z-50 flex items-center justify-between gap-6 border-b px-4 py-2 sm:px-6">
            <div className="flex items-center gap-4">
              <SidebarTrigger className="md:hidden [&_svg]:!size-5" />
              <PageTitle />
            </div>
            <div className="flex items-center gap-3 sm:gap-6">
              <GatewayStatus />
              <div className="flex items-center gap-1.5">
                <Button variant="outline" size="icon"><ActivityIcon className="size-4" /></Button>
                <Button variant="outline" size="icon" className="relative">
                  <BellIcon className="size-4" />
                  <span className="bg-destructive absolute -top-0.5 -right-0.5 size-2 rounded-full" />
                </Button>
              </div>
              <Button variant="ghost" size="icon" className="size-9.5">
                <Avatar className="size-9.5 rounded-md">
                  <AvatarFallback className="rounded-md text-xs">VS</AvatarFallback>
                </Avatar>
              </Button>
            </div>
          </header>
          <main className="size-full flex-1">{children}</main>
        </div>
      </SidebarProvider>
    </div>
  );
}
