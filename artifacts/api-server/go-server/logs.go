package main

import (
        "encoding/json"
        "errors"
        "fmt"
        "log"
        "net/http"
        "sync"
        "sync/atomic"
        "time"
)

const maxSSEClients = 100
const maxSSEPerToken = 3
const maxSSEDuration = 2 * time.Hour
const sseWriteTimeout = 10 * time.Second

type LogEntry struct {
        Level     string `json:"level"`
        Agent     string `json:"agent"`
        Message   string `json:"message"`
        Timestamp string `json:"timestamp"`
}

type client struct {
        ch    chan LogEntry
        token string
}

type LogBroker struct {
        mu           sync.Mutex
        clients      map[*client]struct{}
        emit_ch      chan LogEntry
        clientCount  int32
        tokenStreams map[string]int
}

func newLogBroker() *LogBroker {
        return &LogBroker{
                clients:     make(map[*client]struct{}),
                emit_ch:     make(chan LogEntry, 256),
                tokenStreams: make(map[string]int),
        }
}

func (b *LogBroker) run() {
        for entry := range b.emit_ch {
                b.mu.Lock()
                for c := range b.clients {
                        select {
                        case c.ch <- entry:
                        default:
                        }
                }
                b.mu.Unlock()
        }
}

func (b *LogBroker) emit(entry LogEntry) {
        select {
        case b.emit_ch <- entry:
        default:
        }
}

// Sentinel errors for subscribe rejections so callers can return distinct HTTP
// status codes: 429 for per-session cap and 503 for global capacity.
var errGlobalCapacity = errors.New("global SSE capacity exceeded")
var errTokenCapacity = errors.New("per-session SSE stream limit reached")

func (b *LogBroker) subscribe(token string) (*client, error) {
        if atomic.AddInt32(&b.clientCount, 1) > maxSSEClients {
                atomic.AddInt32(&b.clientCount, -1)
                return nil, errGlobalCapacity
        }
        b.mu.Lock()
        if b.tokenStreams[token] >= maxSSEPerToken {
                b.mu.Unlock()
                atomic.AddInt32(&b.clientCount, -1)
                return nil, errTokenCapacity
        }
        c := &client{ch: make(chan LogEntry, 64), token: token}
        b.clients[c] = struct{}{}
        b.tokenStreams[token]++
        b.mu.Unlock()
        return c, nil
}

func (b *LogBroker) unsubscribe(c *client) {
        b.mu.Lock()
        delete(b.clients, c)
        b.tokenStreams[c.token]--
        if b.tokenStreams[c.token] <= 0 {
                delete(b.tokenStreams, c.token)
        }
        b.mu.Unlock()
        atomic.AddInt32(&b.clientCount, -1)
}

// sseWrite sets a bounded per-write deadline, writes the formatted SSE frame,
// and flushes. It returns an error if the deadline cannot be set or the write
// fails (including a timeout from a stalled client). Using a per-write deadline
// instead of clearing the server WriteTimeout ensures that a slow or stalled
// client cannot hold a goroutine and a broker slot indefinitely.
func sseWrite(rc *http.ResponseController, w http.ResponseWriter, flusher http.Flusher, format string, args ...any) error {
        if err := rc.SetWriteDeadline(time.Now().Add(sseWriteTimeout)); err != nil {
                return err
        }
        if _, err := fmt.Fprintf(w, format, args...); err != nil {
                return err
        }
        flusher.Flush()
        return nil
}

func serveSSE(w http.ResponseWriter, r *http.Request, broker *LogBroker, sessions *sessionStore, token string) {
        flusher, ok := w.(http.Flusher)
        if !ok {
                http.Error(w, "SSE not supported", http.StatusInternalServerError)
                return
        }

        rc := http.NewResponseController(w)

        c, err := broker.subscribe(token)
        if err != nil {
                if errors.Is(err, errTokenCapacity) {
                        http.Error(w, "Stream limit reached for this session", http.StatusTooManyRequests)
                } else {
                        http.Error(w, "Too many active streams", http.StatusServiceUnavailable)
                }
                return
        }
        defer broker.unsubscribe(c)

        w.Header().Set("Content-Type", "text/event-stream")
        w.Header().Set("Cache-Control", "no-cache")
        w.Header().Set("Connection", "keep-alive")
        w.Header().Set("X-Accel-Buffering", "no")

        connectMsg := LogEntry{
                Level:     "system",
                Agent:     "Engine",
                Message:   "SSE stream connected — live log feed active",
                Timestamp: time.Now().Format(time.RFC3339),
        }
        if data, err := json.Marshal(connectMsg); err == nil {
                if err := sseWrite(rc, w, flusher, "data: %s\n\n", string(data)); err != nil {
                        return
                }
        }

        ticker := time.NewTicker(15 * time.Second)
        defer ticker.Stop()

        deadline := time.NewTimer(maxSSEDuration)
        defer deadline.Stop()

        ctx := r.Context()
        for {
                select {
                case <-ctx.Done():
                        return
                case <-deadline.C:
                        return
                case entry := <-c.ch:
                        if !sessions.valid(token) {
                                return
                        }
                        data, err := json.Marshal(entry)
                        if err != nil {
                                log.Printf("SSE marshal error: %v", err)
                                continue
                        }
                        if err := sseWrite(rc, w, flusher, "data: %s\n\n", string(data)); err != nil {
                                return
                        }
                case <-ticker.C:
                        if !sessions.valid(token) {
                                return
                        }
                        if err := sseWrite(rc, w, flusher, ": heartbeat\n\n"); err != nil {
                                return
                        }
                }
        }
}
