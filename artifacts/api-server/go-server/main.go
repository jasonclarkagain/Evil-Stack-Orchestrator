package main

import (
        "crypto/rand"
        "encoding/hex"
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

const sessionCookieName = "session"
const sessionTTL = 7 * 24 * time.Hour
const sessionMaxAge = int(sessionTTL / time.Second)

// sessionStore holds active opaque session tokens with server-side expiry.
// The cookie carries only the random token; the API_KEY never leaves the server.
type sessionStore struct {
        mu     sync.Mutex
        tokens map[string]time.Time // token → expiry
}

func newSessionStore() *sessionStore {
        s := &sessionStore{tokens: make(map[string]time.Time)}
        go s.pruneLoop()
        return s
}

func (s *sessionStore) issue() (string, error) {
        b := make([]byte, 32)
        if _, err := rand.Read(b); err != nil {
                return "", err
        }
        token := hex.EncodeToString(b)
        s.mu.Lock()
        s.tokens[token] = time.Now().Add(sessionTTL)
        s.mu.Unlock()
        return token, nil
}

func (s *sessionStore) valid(token string) bool {
        s.mu.Lock()
        expiry, ok := s.tokens[token]
        s.mu.Unlock()
        return ok && time.Now().Before(expiry)
}

func (s *sessionStore) revoke(token string) {
        s.mu.Lock()
        delete(s.tokens, token)
        s.mu.Unlock()
}

func (s *sessionStore) pruneLoop() {
        ticker := time.NewTicker(time.Hour)
        defer ticker.Stop()
        for range ticker.C {
                now := time.Now()
                s.mu.Lock()
                for token, expiry := range s.tokens {
                        if now.After(expiry) {
                                delete(s.tokens, token)
                        }
                }
                s.mu.Unlock()
        }
}

func requireAuth(sessions *sessionStore, next http.HandlerFunc) http.HandlerFunc {
        return func(w http.ResponseWriter, r *http.Request) {
                cookie, err := r.Cookie(sessionCookieName)
                if err != nil || !sessions.valid(cookie.Value) {
                        writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
                        return
                }
                next(w, r)
        }
}

var startTime = time.Now()

const (
        taskRateLimit  = 20
        taskRateWindow = time.Minute
        // Login rate limiting: 10 attempts per 15-minute window per IP.
        loginRateLimit  = 10
        loginRateWindow = 15 * time.Minute
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

// trustedOrigins returns the set of origins that are allowed to make
// credentialed cross-origin requests. Defaults to same-origin only (empty set)
// unless ALLOWED_ORIGINS is set to a comma-separated list.
func trustedOrigins() map[string]struct{} {
        set := make(map[string]struct{})
        raw := os.Getenv("ALLOWED_ORIGINS")
        if raw == "" {
                return set
        }
        for _, o := range strings.Split(raw, ",") {
                o = strings.TrimSpace(o)
                if o != "" {
                        set[o] = struct{}{}
                }
        }
        return set
}

func corsMiddleware(origins map[string]struct{}) func(http.Handler) http.Handler {
        return func(next http.Handler) http.Handler {
                return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
                        origin := r.Header.Get("Origin")
                        if origin != "" {
                                if _, ok := origins[origin]; ok {
                                        w.Header().Set("Access-Control-Allow-Origin", origin)
                                        w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
                                        w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Accept")
                                        w.Header().Set("Access-Control-Allow-Credentials", "true")
                                        w.Header().Set("Vary", "Origin")
                                }
                                // Origins not on the allowlist get no CORS headers — request
                                // will be blocked by the browser's same-origin policy.
                        }
                        if r.Method == http.MethodOptions {
                                w.WriteHeader(http.StatusNoContent)
                                return
                        }
                        next.ServeHTTP(w, r)
                })
        }
}

func writeJSON(w http.ResponseWriter, status int, v any) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(status)
        if err := json.NewEncoder(w).Encode(v); err != nil {
                log.Printf("writeJSON encode error: %v", err)
        }
}

func setSessionCookie(w http.ResponseWriter, value string, maxAge int) {
        http.SetCookie(w, &http.Cookie{
                Name:     sessionCookieName,
                Value:    value,
                Path:     "/",
                MaxAge:   maxAge,
                HttpOnly: true,
                Secure:   true,
                SameSite: http.SameSiteStrictMode,
        })
}

func main() {
        port := os.Getenv("PORT")
        if port == "" {
                port = "8080"
        }

        apiKey := os.Getenv("API_KEY")
        if apiKey == "" {
                log.Fatal("API_KEY environment variable must be set")
        }

        sessions := newSessionStore()
        taskStore := newTaskStore()
        logBroker := newLogBroker()
        taskLimiter := newRateLimiter(taskRateLimit, taskRateWindow)
        loginLimiter := newRateLimiter(loginRateLimit, loginRateWindow)

        go taskStore.processLoop(logBroker)
        go logBroker.run()
        go seedInitialTasks(taskStore)
        go taskLimiter.pruneLoop()
        go loginLimiter.pruneLoop()

        mux := http.NewServeMux()

        mux.HandleFunc("GET /api/healthz", func(w http.ResponseWriter, r *http.Request) {
                writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
        })

        mux.HandleFunc("POST /api/auth/login", func(w http.ResponseWriter, r *http.Request) {
                ip := clientIP(r)
                if !loginLimiter.allow(ip) {
                        writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many login attempts"})
                        return
                }
                r.Body = http.MaxBytesReader(w, r.Body, 4*1024)
                var body struct {
                        Password string `json:"password"`
                }
                if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
                        var mbe *http.MaxBytesError
                        if errors.As(err, &mbe) {
                                writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "request body too large"})
                        } else {
                                writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password required"})
                        }
                        return
                }
                if body.Password == "" {
                        writeJSON(w, http.StatusBadRequest, map[string]string{"error": "password required"})
                        return
                }
                if body.Password != apiKey {
                        writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
                        return
                }
                token, err := sessions.issue()
                if err != nil {
                        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "session error"})
                        return
                }
                setSessionCookie(w, token, sessionMaxAge)
                writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
        })

        mux.HandleFunc("POST /api/auth/logout", func(w http.ResponseWriter, r *http.Request) {
                if cookie, err := r.Cookie(sessionCookieName); err == nil {
                        sessions.revoke(cookie.Value)
                }
                setSessionCookie(w, "", -1)
                writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
        })

        mux.HandleFunc("GET /api/auth/me", requireAuth(sessions, func(w http.ResponseWriter, r *http.Request) {
                writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
        }))

        mux.HandleFunc("GET /api/telemetry", requireAuth(sessions, func(w http.ResponseWriter, r *http.Request) {
                snap := getTelemetrySnapshot()
                writeJSON(w, http.StatusOK, snap)
        }))

        mux.HandleFunc("GET /api/tasks/summary", requireAuth(sessions, func(w http.ResponseWriter, r *http.Request) {
                summary := taskStore.summary()
                writeJSON(w, http.StatusOK, summary)
        }))

        mux.HandleFunc("GET /api/tasks", requireAuth(sessions, func(w http.ResponseWriter, r *http.Request) {
                tasks := taskStore.list()
                writeJSON(w, http.StatusOK, tasks)
        }))

        mux.HandleFunc("POST /api/tasks", requireAuth(sessions, func(w http.ResponseWriter, r *http.Request) {
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
        }))

        mux.HandleFunc("DELETE /api/tasks/{id}", requireAuth(sessions, func(w http.ResponseWriter, r *http.Request) {
                id := r.PathValue("id")
                if !taskStore.delete(id) {
                        writeJSON(w, http.StatusNotFound, map[string]string{"error": "task not found"})
                        return
                }
                w.WriteHeader(http.StatusNoContent)
        }))

        mux.HandleFunc("GET /api/logs/stream", requireAuth(sessions, func(w http.ResponseWriter, r *http.Request) {
                serveSSE(w, r, logBroker)
        }))

        handler := corsMiddleware(trustedOrigins())(mux)

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
