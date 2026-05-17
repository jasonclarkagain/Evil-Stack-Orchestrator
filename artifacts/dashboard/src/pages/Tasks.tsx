import React, { useState } from "react";
import { useListTasks, getListTasksQueryKey, useCreateTask, useDeleteTask } from "@workspace/api-client-react";
import { useQueryClient } from "@tanstack/react-query";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table";
import { Trash2, Plus, Terminal } from "lucide-react";
import { useToast } from "@/hooks/use-toast";

export default function Tasks() {
  const queryClient = useQueryClient();
  const { toast } = useToast();
  const { data: tasks, isLoading } = useListTasks({ query: { refetchInterval: 2000, queryKey: getListTasksQueryKey() } });
  const createTask = useCreateTask();
  const deleteTask = useDeleteTask();

  const [payload, setPayload] = useState("");
  const [agent, setAgent] = useState("");

  const handleCreate = (e: React.FormEvent) => {
    e.preventDefault();
    if (!payload.trim()) return;
    createTask.mutate({ data: { payload, agent: agent || undefined } }, {
      onSuccess: () => {
        setPayload("");
        setAgent("");
        queryClient.invalidateQueries({ queryKey: getListTasksQueryKey() });
        toast({ title: "Task dispatched", description: "The task has been added to the queue." });
      },
      onError: (err: any) => {
        toast({ title: "Dispatch failed", description: err.message, variant: "destructive" });
      }
    });
  };

  const handleDelete = (id: string) => {
    deleteTask.mutate({ id }, {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: getListTasksQueryKey() });
        toast({ title: "Task deleted" });
      }
    });
  };

  return (
    <div className="space-y-6 max-w-6xl mx-auto">
      <Card className="bg-card/50 backdrop-blur border-border/50">
        <CardHeader>
          <CardTitle className="text-lg font-mono">DISPATCH_TASK</CardTitle>
          <CardDescription>Enqueue a new task for the autonomous agents</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={handleCreate} className="flex gap-4 items-end">
            <div className="flex-1 space-y-2">
               <label className="text-xs font-mono text-muted-foreground">PAYLOAD</label>
               <Input 
                 placeholder="Execute command or JSON payload" 
                 value={payload} 
                 onChange={e => setPayload(e.target.value)} 
                 className="font-mono text-sm bg-background"
                 data-testid="input-task-payload"
               />
            </div>
             <div className="w-64 space-y-2">
               <label className="text-xs font-mono text-muted-foreground">AGENT (OPTIONAL)</label>
               <Input 
                 placeholder="e.g. build-agent" 
                 value={agent} 
                 onChange={e => setAgent(e.target.value)} 
                 className="font-mono text-sm bg-background"
                 data-testid="input-task-agent"
               />
            </div>
            <Button type="submit" disabled={createTask.isPending || !payload.trim()} data-testid="button-submit-task">
              {createTask.isPending ? <Terminal className="w-4 h-4 mr-2 animate-pulse" /> : <Plus className="w-4 h-4 mr-2" />}
              DISPATCH
            </Button>
          </form>
        </CardContent>
      </Card>

      <Card className="bg-card/50 backdrop-blur border-border/50">
         <CardHeader>
          <CardTitle className="text-lg font-mono">TASK_QUEUE</CardTitle>
        </CardHeader>
        <CardContent>
          {isLoading ? (
            <div className="py-8 text-center text-muted-foreground font-mono">Loading queue...</div>
          ) : (
            <Table>
              <TableHeader>
                <TableRow className="hover:bg-transparent border-border/50">
                  <TableHead className="w-32 font-mono">ID</TableHead>
                  <TableHead className="font-mono">STATUS</TableHead>
                  <TableHead className="font-mono">AGENT</TableHead>
                  <TableHead className="font-mono w-1/3">PAYLOAD</TableHead>
                  <TableHead className="font-mono">CREATED</TableHead>
                  <TableHead className="text-right"></TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {tasks?.length === 0 ? (
                  <TableRow className="hover:bg-transparent border-border/50">
                    <TableCell colSpan={6} className="text-center py-8 text-muted-foreground">
                      No tasks in queue.
                    </TableCell>
                  </TableRow>
                ) : (
                  tasks?.map((task) => (
                    <TableRow key={task.id} className="border-border/50 group">
                      <TableCell className="font-mono text-xs text-muted-foreground">{task.id.slice(0, 8)}</TableCell>
                      <TableCell>
                        <Badge 
                           variant="outline" 
                           className={`font-mono text-xs uppercase ${
                              task.status === 'running' ? 'border-primary text-primary animate-pulse shadow-[0_0_8px_var(--color-primary)]' :
                              task.status === 'completed' ? 'border-accent text-accent' :
                              task.status === 'failed' ? 'border-destructive text-destructive' :
                              'border-muted-foreground text-muted-foreground'
                           }`}
                           data-testid={`status-badge-${task.id}`}
                        >
                          {task.status}
                        </Badge>
                      </TableCell>
                      <TableCell className="font-mono text-xs">{task.agent || "auto"}</TableCell>
                      <TableCell className="font-mono text-xs truncate max-w-[200px]" title={task.payload}>{task.payload}</TableCell>
                      <TableCell className="font-mono text-xs text-muted-foreground">
                        {new Date(task.created_at).toLocaleString()}
                      </TableCell>
                      <TableCell className="text-right">
                        <Button 
                          variant="ghost" 
                          size="icon" 
                          className="h-8 w-8 text-muted-foreground opacity-0 group-hover:opacity-100 transition-opacity hover:text-destructive"
                          onClick={() => handleDelete(task.id)}
                          data-testid={`button-delete-task-${task.id}`}
                        >
                          <Trash2 className="h-4 w-4" />
                        </Button>
                      </TableCell>
                    </TableRow>
                  ))
                )}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
