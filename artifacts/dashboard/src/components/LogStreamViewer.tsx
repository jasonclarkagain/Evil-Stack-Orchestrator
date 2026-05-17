import React, { useEffect, useRef } from "react";
import { LogMessage } from "@/hooks/use-log-stream";
import { Pause, Play, Trash2 } from "lucide-react";
import { Button } from "@/components/ui/button";

interface LogStreamViewerProps {
  logs: LogMessage[];
  isPaused: boolean;
  togglePause: () => void;
  clearLogs: () => void;
  height?: string;
  autoScroll?: boolean;
}

const levelColors: Record<string, string> = {
  debug: "text-muted-foreground",
  info: "text-primary",
  success: "text-accent",
  error: "text-destructive",
  system: "text-purple-400",
};

export function LogStreamViewer({ logs, isPaused, togglePause, clearLogs, height = "400px", autoScroll = true }: LogStreamViewerProps) {
  const endRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (autoScroll && !isPaused) {
      endRef.current?.scrollIntoView({ behavior: "smooth" });
    }
  }, [logs, autoScroll, isPaused]);

  return (
    <div className="flex flex-col border border-border/50 bg-card rounded-md overflow-hidden relative h-full">
      <div className="flex items-center justify-between p-2 border-b border-border/50 bg-background/50 backdrop-blur z-10 sticky top-0">
        <div className="text-xs font-mono font-bold tracking-wider text-muted-foreground">LIVE_LOGS</div>
        <div className="flex items-center gap-2">
          <Button variant="ghost" size="icon" className="h-6 w-6 text-muted-foreground hover:text-foreground" onClick={togglePause} data-testid="button-toggle-pause">
            {isPaused ? <Play className="h-3 w-3" /> : <Pause className="h-3 w-3" />}
          </Button>
          <Button variant="ghost" size="icon" className="h-6 w-6 text-muted-foreground hover:text-foreground" onClick={clearLogs} data-testid="button-clear-logs">
            <Trash2 className="h-3 w-3" />
          </Button>
        </div>
      </div>
      <div className="p-4 overflow-y-auto font-mono text-xs flex-1 scroll-smooth" style={{ height }}>
        {logs.length === 0 ? (
          <div className="text-muted-foreground italic opacity-50">Waiting for logs...</div>
        ) : (
          logs.map((log, i) => (
            <div key={i} className="mb-1 flex gap-3 animate-in slide-in-from-left-2 opacity-0 fade-in duration-300 fill-mode-forwards" style={{ animationDelay: `${Math.min(i * 10, 100)}ms` }}>
              <span className="text-muted-foreground opacity-50 shrink-0">{new Date(log.timestamp).toLocaleTimeString(undefined, { hour12: false })}</span>
              <span className={`shrink-0 w-16 uppercase font-bold ${levelColors[log.level] || "text-foreground"}`}>[{log.level}]</span>
              {log.agent && <span className="text-muted-foreground/70 shrink-0">[{log.agent}]</span>}
              <span className="text-foreground break-all">{log.message}</span>
            </div>
          ))
        )}
        <div ref={endRef} />
      </div>
    </div>
  );
}
