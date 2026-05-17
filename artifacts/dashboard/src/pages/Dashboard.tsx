import React, { useState, useEffect } from "react";
import { useGetTelemetry, getGetTelemetryQueryKey, useGetTaskSummary, getGetTaskSummaryQueryKey } from "@workspace/api-client-react";
import { LogStreamViewer } from "@/components/LogStreamViewer";
import { useLogStream } from "@/hooks/use-log-stream";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Activity, Cpu, Database, Network, PlayCircle, CheckCircle2, XCircle, ListTodo } from "lucide-react";
import { AreaChart, Area, ResponsiveContainer, YAxis } from "recharts";

export default function Dashboard() {
  const { data: telemetry } = useGetTelemetry({ query: { refetchInterval: 1000, queryKey: getGetTelemetryQueryKey() } });
  const { data: summary } = useGetTaskSummary({ query: { refetchInterval: 2000, queryKey: getGetTaskSummaryQueryKey() } });
  const { logs, isPaused, togglePause, clearLogs } = useLogStream(100);

  const [history, setHistory] = useState<{ time: number, cpu: number, mem: number }[]>([]);

  useEffect(() => {
    if (telemetry) {
      setHistory(prev => {
        const next = [...prev, { time: Date.now(), cpu: telemetry.cpu_percent, mem: telemetry.mem_used_mb }];
        if (next.length > 60) return next.slice(next.length - 60);
        return next;
      });
    }
  }, [telemetry]);

  const formatUptime = (seconds?: number) => {
    if (!seconds) return "0s";
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = seconds % 60;
    return `${h}h ${m}m ${s}s`;
  };

  return (
    <div className="space-y-6 flex flex-col h-full">
      <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-4 gap-4 flex-shrink-0">
        <Card className="bg-card/50 backdrop-blur border-border/50">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-mono tracking-tight text-muted-foreground">CPU_USAGE</CardTitle>
            <Cpu className="w-4 h-4 text-primary" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold font-mono text-foreground" data-testid="text-cpu-usage">
              {telemetry?.cpu_percent.toFixed(1) || "0.0"}%
            </div>
            <div className="h-[60px] mt-4 -mx-2">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={history}>
                  <Area type="monotone" dataKey="cpu" stroke="hsl(var(--primary))" fill="hsl(var(--primary)/0.2)" strokeWidth={2} isAnimationActive={false} />
                  <YAxis domain={[0, 100]} hide />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        <Card className="bg-card/50 backdrop-blur border-border/50">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-mono tracking-tight text-muted-foreground">MEM_USAGE</CardTitle>
            <Database className="w-4 h-4 text-accent" />
          </CardHeader>
          <CardContent>
            <div className="text-2xl font-bold font-mono text-foreground" data-testid="text-mem-usage">
              {telemetry?.mem_used_mb || 0} <span className="text-sm text-muted-foreground font-normal">/ {telemetry?.mem_total_mb || 0} MB</span>
            </div>
             <div className="h-[60px] mt-4 -mx-2">
              <ResponsiveContainer width="100%" height="100%">
                <AreaChart data={history}>
                  <Area type="monotone" dataKey="mem" stroke="hsl(var(--accent))" fill="hsl(var(--accent)/0.2)" strokeWidth={2} isAnimationActive={false} />
                  <YAxis domain={[0, telemetry?.mem_total_mb || 1000]} hide />
                </AreaChart>
              </ResponsiveContainer>
            </div>
          </CardContent>
        </Card>

        <Card className="bg-card/50 backdrop-blur border-border/50">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-mono tracking-tight text-muted-foreground">THROUGHPUT</CardTitle>
            <Network className="w-4 h-4 text-primary" />
          </CardHeader>
          <CardContent>
             <div className="text-2xl font-bold font-mono text-foreground" data-testid="text-throughput">
              {telemetry?.throughput_rps || 0} <span className="text-sm text-muted-foreground font-normal">RPS</span>
            </div>
            <div className="mt-4 flex flex-col gap-2">
               <div className="flex justify-between items-center text-xs font-mono">
                  <span className="text-muted-foreground">GOROUTINES</span>
                  <span className="text-foreground">{telemetry?.goroutines || 0}</span>
               </div>
               <div className="flex justify-between items-center text-xs font-mono">
                  <span className="text-muted-foreground">GC_CYCLES</span>
                  <span className="text-foreground">{telemetry?.gc_cycles || 0}</span>
               </div>
               <div className="flex justify-between items-center text-xs font-mono">
                  <span className="text-muted-foreground">UPTIME</span>
                  <span className="text-foreground">{formatUptime(telemetry?.uptime_seconds)}</span>
               </div>
            </div>
          </CardContent>
        </Card>

        <Card className="bg-card/50 backdrop-blur border-border/50">
          <CardHeader className="flex flex-row items-center justify-between pb-2">
            <CardTitle className="text-sm font-mono tracking-tight text-muted-foreground">TASK_QUEUE</CardTitle>
            <Activity className="w-4 h-4 text-accent" />
          </CardHeader>
          <CardContent>
             <div className="text-2xl font-bold font-mono text-foreground" data-testid="text-total-tasks">
              {summary?.total || 0} <span className="text-sm text-muted-foreground font-normal">TOTAL</span>
            </div>
             <div className="mt-4 flex flex-col gap-2">
               <div className="flex justify-between items-center text-xs font-mono">
                  <span className="text-muted-foreground flex items-center gap-1"><ListTodo className="w-3 h-3"/> PENDING</span>
                  <span className="text-foreground">{summary?.pending || 0}</span>
               </div>
               <div className="flex justify-between items-center text-xs font-mono">
                  <span className="text-primary flex items-center gap-1"><PlayCircle className="w-3 h-3"/> RUNNING</span>
                  <span className="text-primary">{summary?.running || 0}</span>
               </div>
               <div className="flex justify-between items-center text-xs font-mono">
                  <span className="text-accent flex items-center gap-1"><CheckCircle2 className="w-3 h-3"/> COMPLETED</span>
                  <span className="text-accent">{summary?.completed || 0}</span>
               </div>
                <div className="flex justify-between items-center text-xs font-mono">
                  <span className="text-destructive flex items-center gap-1"><XCircle className="w-3 h-3"/> FAILED</span>
                  <span className="text-destructive">{summary?.failed || 0}</span>
               </div>
            </div>
          </CardContent>
        </Card>
      </div>

      <div className="flex-1 min-h-[400px]">
        <LogStreamViewer logs={logs} isPaused={isPaused} togglePause={togglePause} clearLogs={clearLogs} height="100%" />
      </div>
    </div>
  );
}
