import React, { ReactNode } from "react";
import { Link, useLocation } from "wouter";
import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
  SidebarInset
} from "@/components/ui/sidebar";
import { LayoutDashboard, ListTodo, TerminalSquare, ShieldAlert } from "lucide-react";
import { useHealthCheck } from "@workspace/api-client-react";

export function AppShell({ children }: { children: ReactNode }) {
  const [location] = useLocation();
  const { data: health } = useHealthCheck({ query: { refetchInterval: 10000 } });

  const navigation = [
    { name: "Dashboard", href: "/", icon: LayoutDashboard },
    { name: "Tasks", href: "/tasks", icon: ListTodo },
    { name: "Logs", href: "/logs", icon: TerminalSquare },
  ];

  return (
    <SidebarProvider>
      <Sidebar variant="sidebar" collapsible="icon" className="border-r border-border/50">
        <SidebarHeader className="border-b border-border/50 py-4 px-4 flex items-center justify-center">
          <div className="flex items-center gap-2 font-mono font-bold tracking-tight text-primary">
            <TerminalSquare className="w-5 h-5" />
            <span className="group-data-[collapsible=icon]:hidden">ORCHESTRATOR</span>
          </div>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupLabel>Navigation</SidebarGroupLabel>
            <SidebarGroupContent>
              <SidebarMenu>
                {navigation.map((item) => (
                  <SidebarMenuItem key={item.name}>
                    <SidebarMenuButton
                      asChild
                      isActive={location === item.href}
                      tooltip={item.name}
                    >
                      <Link href={item.href} className="flex items-center gap-2">
                        <item.icon className="w-4 h-4" />
                        <span>{item.name}</span>
                      </Link>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <div className="mt-auto p-4 border-t border-border/50 text-xs font-mono">
          <div className="flex items-center gap-2 text-muted-foreground group-data-[collapsible=icon]:justify-center">
            <ShieldAlert className="w-4 h-4" />
            <span className="group-data-[collapsible=icon]:hidden">System Status</span>
          </div>
          <div className="mt-2 flex items-center gap-2 group-data-[collapsible=icon]:hidden">
             <div className={`w-2 h-2 rounded-full ${health?.status === 'ok' ? 'bg-accent shadow-[0_0_8px_var(--color-accent)]' : 'bg-destructive shadow-[0_0_8px_var(--color-destructive)] animate-pulse'}`} />
             <span className="capitalize">{health?.status || 'connecting...'}</span>
          </div>
        </div>
      </Sidebar>
      
      <SidebarInset className="bg-background relative min-h-screen flex flex-col">
        <header className="h-14 border-b border-border/50 flex items-center px-4 sticky top-0 bg-background/80 backdrop-blur z-10">
           <SidebarTrigger className="text-muted-foreground hover:text-foreground" />
        </header>
        <main className="flex-1 p-6 overflow-x-hidden">
          {children}
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}
