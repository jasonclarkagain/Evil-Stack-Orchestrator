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

type LogEntry struct {
	Level     string `json:"level"`
	Agent     string `json:"agent"`
	Message   string `json:"message"`
	Timestamp string `json:"timestamp"`
}

type client struct {
	ch chan LogEntry
}

type LogBroker struct {
	mu          sync.Mutex
	clients     map[*client]struct{}
	emit_ch     chan LogEntry
	clientCount int32
}

func newLogBroker() *LogBroker {
	return &LogBroker{
		clients: make(map[*client]struct{}),
		emit_ch: make(chan LogEntry, 256),
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

func (b *LogBroker) subscribe() (*client, error) {
	if atomic.AddInt32(&b.clientCount, 1) > maxSSEClients {
		atomic.AddInt32(&b.clientCount, -1)
		return nil, errors.New("too many SSE clients")
	}
	c := &client{ch: make(chan LogEntry, 64)}
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	return c, nil
}

func (b *LogBroker) unsubscribe(c *client) {
	b.mu.Lock()
	delete(b.clients, c)
	b.mu.Unlock()
	atomic.AddInt32(&b.clientCount, -1)
}

func serveSSE(w http.ResponseWriter, r *http.Request, broker *LogBroker) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	c, err := broker.subscribe()
	if err != nil {
		http.Error(w, "Too many active streams", http.StatusServiceUnavailable)
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
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case entry := <-c.ch:
			data, err := json.Marshal(entry)
			if err != nil {
				log.Printf("SSE marshal error: %v", err)
				continue
			}
			_, err = fmt.Fprintf(w, "data: %s\n\n", data)
			if err != nil {
				return
			}
			flusher.Flush()
		case <-ticker.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		}
	}
}
