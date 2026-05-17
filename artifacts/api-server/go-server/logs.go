package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

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
	mu      sync.Mutex
	clients map[*client]struct{}
	emit_ch chan LogEntry
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

func (b *LogBroker) subscribe() *client {
	c := &client{ch: make(chan LogEntry, 64)}
	b.mu.Lock()
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	return c
}

func (b *LogBroker) unsubscribe(c *client) {
	b.mu.Lock()
	delete(b.clients, c)
	b.mu.Unlock()
}

func serveSSE(w http.ResponseWriter, r *http.Request, broker *LogBroker) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "SSE not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	c := broker.subscribe()
	defer broker.unsubscribe(c)

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
