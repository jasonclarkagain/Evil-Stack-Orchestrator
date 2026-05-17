import React from "react";
import { LogStreamViewer } from "@/components/LogStreamViewer";
import { useLogStream } from "@/hooks/use-log-stream";

export default function Logs() {
  const { logs, isPaused, togglePause, clearLogs } = useLogStream(5000);

  return (
    <div className="h-[calc(100vh-6rem)] w-full">
      <LogStreamViewer 
        logs={logs} 
        isPaused={isPaused} 
        togglePause={togglePause} 
        clearLogs={clearLogs} 
        height="100%" 
      />
    </div>
  );
}
