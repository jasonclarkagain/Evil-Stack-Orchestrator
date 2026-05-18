package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

var startTime = time.Now()

const (
	taskRateLimit        = 20
	taskRateWindow       = time.Minute
	sseConnectRateLimit  = 10
	sseConnectRateWindow = time.Minute
)

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string][]time.Time
	rate    int
	window  time.Duration
}

func newRateLimiter(rate int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		clients: make(map[string][]time.Time),
		rate:    rate,
		window:  window,
	}
}

func (rl *rateLimiter) allow(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	now := time.Now()
	cutoff := now.Add(-rl.window)
	prev := rl.clients[ip]
	filtered := prev[:0]
	for _, t := range prev {
		if t.After(cutoff) {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) >= rl.rate {
		rl.clients[ip] = filtered
		return false
	}
	rl.clients[ip] = append(filtered, now)
	return true
}

func (rl *rateLimiter) pruneLoop() {
	ticker := time.NewTicker(5 * rl.window)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-rl.window)
		rl.mu.Lock()
		for ip, times := range rl.clients {
			active := times[:0]
			for _, t := range times {
				if t.After(cutoff) {
					active = append(active, t)
				}
			}
			if len(active) == 0 {
				delete(rl.clients, ip)
			} else {
				rl.clients[ip] = active
			}
		}
		rl.mu.Unlock()
	}
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "" {
			origin = "*"
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("writeJSON encode error: %v", err)
	}
}

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	taskStore := newTaskStore()
	logBroker := newLogBroker()
	taskLimiter := newRateLimiter(taskRateLimit, taskRateWindow)
	sseConnectLimiter := newRateLimiter(sseConnectRateLimit, sseConnectRateWindow)

	go taskStore.processLoop(logBroker)
	go logBroker.run()
	go seedInitialTasks(taskStore)
	go taskLimiter.pruneLoop()
	go sseConnectLimiter.pruneLoop()

	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	mux.HandleFunc("GET /api/telemetry", func(w http.ResponseWriter, r *http.Request) {
		snap := getTelemetrySnapshot()
		writeJSON(w, http.StatusOK, snap)
	})

	mux.HandleFunc("GET /api/tasks/summary", func(w http.ResponseWriter, r *http.Request) {
		summary := taskStore.summary()
		writeJSON(w, http.StatusOK, summary)
	})

	mux.HandleFunc("GET /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		tasks := taskStore.list()
		writeJSON(w, http.StatusOK, tasks)
	})

	mux.HandleFunc("POST /api/tasks", func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !taskLimiter.allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "rate limit exceeded"})
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxTaskPayloadBytes)
		var input TaskInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			var mbe *http.MaxBytesError
			if errors.As(err, &mbe) {
				writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
			} else {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			}
			return
		}
		if strings.TrimSpace(input.Payload) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		task, err := taskStore.create(input)
		if err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": err.Error()})
			return
		}
		logBroker.emit(LogEntry{
			Level:     "info",
			Agent:     task.Agent,
			Message:   fmt.Sprintf("Task %s dispatched: %s", shortID(task.ID), truncate(task.Payload, 60)),
			Timestamp: time.Now().Format(time.RFC3339),
		})
		writeJSON(w, http.StatusCreated, task)
	})

	mux.HandleFunc("DELETE /api/tasks/{id}", func(w http.ResponseWriter, r *http.Request) {
		id := r.PathValue("id")
		if !taskStore.delete(id) {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("GET /api/logs/stream", func(w http.ResponseWriter, r *http.Request) {
		ip := clientIP(r)
		if !sseConnectLimiter.allow(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many stream connections"})
			return
		}
		serveSSE(w, r, logBroker)
	})

	handler := corsMiddleware(mux)

	srv := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    64 * 1024,
	}

	log.Printf("Go engine starting on port %s", port)
	if err := srv.ListenAndServe(); err != nil {
		log.Fatalf("Server error: %v", err)
	}
}

func shortID(id string) string {
	if len(id) >= 8 {
		return id[:8]
	}
	return id
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
