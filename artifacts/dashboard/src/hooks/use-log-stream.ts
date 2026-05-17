import { useState, useEffect, useCallback } from "react";

export type LogLevel = "debug" | "info" | "success" | "error" | "system";

export interface LogMessage {
  level: LogLevel;
  agent: string;
  message: string;
  timestamp: string;
}

export function useLogStream(maxLogs = 1000) {
  const [logs, setLogs] = useState<LogMessage[]>([]);
  const [isPaused, setIsPaused] = useState(false);

  useEffect(() => {
    if (isPaused) return;

    const eventSource = new EventSource("/api/logs/stream");

    eventSource.onmessage = (event) => {
      try {
        const data = JSON.parse(event.data) as LogMessage;
        setLogs((prevLogs) => {
          const newLogs = [...prevLogs, data];
          if (newLogs.length > maxLogs) {
            return newLogs.slice(newLogs.length - maxLogs);
          }
          return newLogs;
        });
      } catch (err) {
        console.error("Failed to parse log message", err);
      }
    };

    eventSource.onerror = (error) => {
      console.error("EventSource error", error);
      eventSource.close();
    };

    return () => {
      eventSource.close();
    };
  }, [isPaused, maxLogs]);

  const clearLogs = useCallback(() => {
    setLogs([]);
  }, []);

  const togglePause = useCallback(() => {
    setIsPaused((prev) => !prev);
  }, []);

  return { logs, isPaused, togglePause, clearLogs };
}
