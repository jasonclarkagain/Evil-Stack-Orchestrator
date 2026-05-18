package main

import (
        "fmt"
        "math/rand"
        "sort"
        "sync"
        "time"
)

type TaskStatus string

const (
        StatusPending   TaskStatus = "pending"
        StatusRunning   TaskStatus = "running"
        StatusCompleted TaskStatus = "completed"
        StatusFailed    TaskStatus = "failed"
)

const (
        maxTaskStoreSize     = 500
        maxConcurrentWorkers = 50
        maxTaskPayloadBytes  = 32 * 1024
)

var agents = []string{"Claude", "DeepSeek", "Hermes"}

type Task struct {
        ID          string     `json:"id"`
        Payload     string     `json:"payload"`
        Agent       string     `json:"agent"`
        Status      TaskStatus `json:"status"`
        CreatedAt   string     `json:"created_at"`
        CompletedAt *string    `json:"completed_at"`
        ErrorMsg    *string    `json:"error_msg"`
        Result      *string    `json:"result"`
}

type TaskInput struct {
        Payload string `json:"payload"`
        Agent   string `json:"agent,omitempty"`
}

type TaskSummary struct {
        Total     int `json:"total"`
        Pending   int `json:"pending"`
        Running   int `json:"running"`
        Completed int `json:"completed"`
        Failed    int `json:"failed"`
}

type TaskStore struct {
        mu        sync.RWMutex
        tasks     map[string]*Task
        queue     chan string
        idCtr     int
        workerSem chan struct{}
}

func newTaskStore() *TaskStore {
        return &TaskStore{
                tasks:     make(map[string]*Task),
                queue:     make(chan string, 128),
                workerSem: make(chan struct{}, maxConcurrentWorkers),
        }
}

func (s *TaskStore) nextID() string {
        s.idCtr++
        return fmt.Sprintf("task-%06d-%04x", s.idCtr, rand.Intn(0xffff))
}

// evictCompleted removes completed and failed tasks to free capacity.
// Must be called with s.mu held for writing.
// It evicts the oldest finished tasks up to the number needed to bring the
// store below maxTaskStoreSize.
func (s *TaskStore) evictCompleted(need int) {
        type entry struct {
                id          string
                completedAt string
        }
        var finished []entry
        for id, t := range s.tasks {
                if t.Status == StatusCompleted || t.Status == StatusFailed {
                        ca := ""
                        if t.CompletedAt != nil {
                                ca = *t.CompletedAt
                        }
                        finished = append(finished, entry{id, ca})
                }
        }
        // Sort oldest-first so we evict the least-recently-completed tasks.
        sort.Slice(finished, func(i, j int) bool {
                return finished[i].completedAt < finished[j].completedAt
        })
        for i := 0; i < need && i < len(finished); i++ {
                delete(s.tasks, finished[i].id)
        }
}

func (s *TaskStore) create(input TaskInput) (Task, error) {
        if len(input.Payload) > maxTaskPayloadBytes {
                return Task{}, fmt.Errorf("payload too large: maximum %d bytes allowed", maxTaskPayloadBytes)
        }

        s.mu.Lock()

        if len(s.tasks) >= maxTaskStoreSize {
                // Attempt to reclaim capacity by evicting finished tasks before
                // giving up.  This prevents completed work from permanently
                // exhausting the global task store.
                need := len(s.tasks) - maxTaskStoreSize + 1
                s.evictCompleted(need)
                if len(s.tasks) >= maxTaskStoreSize {
                        s.mu.Unlock()
                        return Task{}, fmt.Errorf("task store full: maximum %d tasks reached", maxTaskStoreSize)
                }
        }

        agent := input.Agent
        if agent == "" {
                agent = agents[rand.Intn(len(agents))]
        }

        task := &Task{
                ID:        s.nextID(),
                Payload:   input.Payload,
                Agent:     agent,
                Status:    StatusPending,
                CreatedAt: time.Now().Format(time.RFC3339),
        }
        s.tasks[task.ID] = task
        s.mu.Unlock()

        select {
        case s.queue <- task.ID:
                return *task, nil
        default:
                s.mu.Lock()
                delete(s.tasks, task.ID)
                s.mu.Unlock()
                return Task{}, fmt.Errorf("server busy: task queue full")
        }
}

func (s *TaskStore) list() []Task {
        s.mu.RLock()
        defer s.mu.RUnlock()
        out := make([]Task, 0, len(s.tasks))
        for _, t := range s.tasks {
                out = append(out, *t)
        }
        sort.Slice(out, func(i, j int) bool {
                return out[i].CreatedAt > out[j].CreatedAt
        })
        return out
}

func (s *TaskStore) delete(id string) bool {
        s.mu.Lock()
        defer s.mu.Unlock()
        if _, ok := s.tasks[id]; !ok {
                return false
        }
        delete(s.tasks, id)
        return true
}

func (s *TaskStore) summary() TaskSummary {
        s.mu.RLock()
        defer s.mu.RUnlock()
        var sum TaskSummary
        sum.Total = len(s.tasks)
        for _, t := range s.tasks {
                switch t.Status {
                case StatusPending:
                        sum.Pending++
                case StatusRunning:
                        sum.Running++
                case StatusCompleted:
                        sum.Completed++
                case StatusFailed:
                        sum.Failed++
                }
        }
        return sum
}

func (s *TaskStore) setRunning(id string) {
        s.mu.Lock()
        defer s.mu.Unlock()
        if t, ok := s.tasks[id]; ok {
                t.Status = StatusRunning
        }
}

func (s *TaskStore) setDone(id string, success bool, result string) {
        s.mu.Lock()
        defer s.mu.Unlock()
        t, ok := s.tasks[id]
        if !ok {
                return
        }
        now := time.Now().Format(time.RFC3339)
        t.CompletedAt = &now
        if success {
                t.Status = StatusCompleted
                t.Result = &result
        } else {
                t.Status = StatusFailed
                t.ErrorMsg = &result
        }
}

var runningMsgs = []string{
        "Parsing payload tokens...",
        "Loading model context window...",
        "Running inference pass 1/3...",
        "Running inference pass 2/3...",
        "Running inference pass 3/3...",
        "Post-processing output...",
        "Validating result schema...",
}

var completedResults = []string{
        "Extraction complete. 97 tokens processed.",
        "Analysis done. Confidence: 0.94.",
        "Synthesis complete. Output ready.",
        "Task resolved. No anomalies detected.",
        "Execution successful. Pipeline flushed.",
}

var failedReasons = []string{
        "Context window overflow.",
        "Model temperature divergence.",
        "Upstream timeout: 30s exceeded.",
}

func (s *TaskStore) processLoop(broker *LogBroker) {
        for id := range s.queue {
                s.workerSem <- struct{}{}
                go func(taskID string) {
                        defer func() { <-s.workerSem }()

                        s.mu.RLock()
                        t, ok := s.tasks[taskID]
                        if !ok {
                                s.mu.RUnlock()
                                return
                        }
                        agent := t.Agent
                        payload := t.Payload
                        s.mu.RUnlock()

                        s.setRunning(taskID)
                        broker.emit(LogEntry{
                                Level:     "info",
                                Agent:     agent,
                                Message:   fmt.Sprintf("[%s] Starting task %s — %s", agent, shortID(taskID), truncate(payload, 50)),
                                Timestamp: time.Now().Format(time.RFC3339),
                        })

                        for _, msg := range runningMsgs {
                                delay := time.Duration(300+rand.Intn(500)) * time.Millisecond
                                time.Sleep(delay)
                                broker.emit(LogEntry{
                                        Level:     "debug",
                                        Agent:     agent,
                                        Message:   fmt.Sprintf("[%s] %s", agent, msg),
                                        Timestamp: time.Now().Format(time.RFC3339),
                                })
                        }

                        success := rand.Float64() > 0.15
                        var outcome string
                        var level string
                        if success {
                                outcome = completedResults[rand.Intn(len(completedResults))]
                                level = "success"
                        } else {
                                outcome = failedReasons[rand.Intn(len(failedReasons))]
                                level = "error"
                        }

                        s.setDone(taskID, success, outcome)
                        broker.emit(LogEntry{
                                Level:     level,
                                Agent:     agent,
                                Message:   fmt.Sprintf("[%s] Task %s %s — %s", agent, shortID(taskID), map[bool]string{true: "completed", false: "FAILED"}[success], outcome),
                                Timestamp: time.Now().Format(time.RFC3339),
                        })
                }(id)
        }
}

var seedPayloads = []string{
        "Analyze dataset batch #417 for anomalous token sequences",
        "Summarize RAG corpus: financial_reports_q4.jsonl",
        "Run multi-step chain on customer_intents_2025.csv",
        "Validate embedding drift for model checkpoint v2.1",
        "Extract entities from doc_batch_092 and cross-reference knowledge graph",
        "Classify sentiment across 2,300 inbound messages",
}

func seedInitialTasks(store *TaskStore) {
        time.Sleep(500 * time.Millisecond)
        for _, p := range seedPayloads {
                store.create(TaskInput{Payload: p})
                time.Sleep(time.Duration(200+rand.Intn(300)) * time.Millisecond)
        }
}
