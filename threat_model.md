# Threat Model

## Project Overview

This project is an operational dashboard for an autonomous task engine. The production stack is a Vite/React dashboard under `artifacts/dashboard` and a Go HTTP API under `artifacts/api-server/go-server`, deployed on Replit autoscale with path-based routing so the dashboard is served at `/` and the API at `/api`. The dashboard polls telemetry, lists task state, opens a live log stream, and allows tasks to be created and deleted. The TypeScript Express server under `artifacts/api-server/src` is not the production runtime for deployed `/api` traffic; production is defined by `artifacts/api-server/.replit-artifact/artifact.toml` and builds/runs the Go server.

Assumptions for future scans:
- Only production-reachable paths matter for findings.
- `artifacts/mockup-sandbox` is development-only unless separate production routing is added.
- Replit provides TLS for deployed traffic.
- `NODE_ENV=production` in production.

## Assets

- **Task payloads and results** — task bodies may contain prompts, commands, dataset identifiers, or other operational data. Unauthorized reads expose business context; unauthorized writes or deletes let attackers manipulate engine behavior.
- **Operational logs and telemetry** — live log streams, throughput, goroutine counts, memory, and status data reveal internal behavior and may expose sensitive task content.
- **Operator session state** — the Go API issues opaque session cookies after password authentication. Cookie theft or server-side session bypass would expose the full control plane.
- **Service availability** — the API backs both the dashboard and the task engine simulation. Attacker-controlled connections, request bodies, queued tasks, or streams can degrade or stop service for all users.
- **Deployment and database secrets** — the repo includes a DB package and environment-based configuration. Secrets must remain server-side and never appear in logs or client bundles.

## Trust Boundaries

- **Browser to Go API** — all dashboard interactions cross from an untrusted client to the production API at `/api/*`. Sensitive operations must be authenticated, authorized, and validated server-side.
- **Cross-origin web pages to Go API** — any third-party website can trigger browser requests to the API. Cookie flags and CORS policy determine whether those requests carry ambient credentials or expose readable responses.
- **Go API to in-memory task/log stores** — public and authenticated HTTP requests directly mutate in-memory queue, task, session, and log-broker state. Missing byte limits, backpressure, or deadlines here can become production DoS issues.
- **Build-time source vs deployed runtime** — the repo contains a non-production Express implementation and a production Go service. Scans should prioritize the Go server for deployed API behavior.

## Scan Anchors

- Production API entry point: `artifacts/api-server/go-server/main.go`
- High-risk production code: `artifacts/api-server/go-server/{main.go,tasks.go,logs.go,telemetry.go}`
- Production frontend consuming those endpoints: `artifacts/dashboard/src/pages/*`, `artifacts/dashboard/src/context/auth.tsx`, `artifacts/dashboard/src/hooks/use-log-stream.ts`
- Public production surfaces: `/api/healthz`, `/api/auth/login`, `/api/auth/logout`
- Authenticated production surfaces: `/api/auth/me`, `/api/telemetry`, `/api/tasks`, `/api/tasks/summary`, `/api/tasks/{id}`, `/api/logs/stream`
- Usually ignore as dev-only: `artifacts/mockup-sandbox/**`, `artifacts/api-server/src/**` unless deployment config changes

## Threat Categories

### Spoofing

The production API now has a shared operator authentication layer: `POST /api/auth/login` exchanges the operator password for an opaque server-side session cookie, and operational endpoints validate that cookie. The system must keep that session boundary intact by issuing unpredictable tokens, expiring and revoking them server-side, and rejecting unauthenticated access to telemetry, task control, and log streaming.

### Tampering

Task creation and deletion directly alter shared engine state. The system must continue enforcing server-side authentication on every mutating route, validate request structure and acceptable field sizes, and avoid letting attackers inject unbounded or malformed task data that can corrupt operational behavior or overload shared state.

### Information Disclosure

The dashboard exposes live logs, task payloads, task results, and engine telemetry. Those responses must remain limited to authenticated sessions, and cross-origin requests must stay deny-by-default unless a deliberate allowlist is configured. Session cookies must remain `HttpOnly`, `Secure`, and resistant to ambient cross-site use.

### Denial of Service

Availability is the primary remaining risk area. Public and authenticated endpoints accept network connections, JSON request bodies, polling traffic, and streaming subscriptions. The production Go server must enforce connection/read/write/idle timeouts, bound headers and request bodies, limit queue growth and resident task bytes, and cap concurrent streaming clients so one actor cannot exhaust goroutines, memory, sockets, or response bandwidth.

### Elevation of Privilege

This service acts as a single shared-operator control plane, so the main privilege boundary is unauthenticated vs authenticated access rather than per-user RBAC. The system must preserve that boundary by ensuring only authenticated sessions can reach operator functions and by preventing any parsing, routing, or state-handling flaw from turning public endpoints into operator-level access.