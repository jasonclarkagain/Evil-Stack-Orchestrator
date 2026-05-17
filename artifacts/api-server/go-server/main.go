package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

var startTime = time.Now()

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

	go taskStore.processLoop(logBroker)
	go logBroker.run()
	go seedInitialTasks(taskStore)

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
		var input TaskInput
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || strings.TrimSpace(input.Payload) == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid payload"})
			return
		}
		task := taskStore.create(input)
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
		serveSSE(w, r, logBroker)
	})

	handler := corsMiddleware(mux)

	log.Printf("Go engine starting on port %s", port)
	if err := http.ListenAndServe(":"+port, handler); err != nil {
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
