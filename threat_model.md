# Threat Model

## Project Overview

This project is an operational dashboard for an autonomous task engine. The production stack is a Vite/React dashboard under `artifacts/dashboard` and a Go HTTP API under `artifacts/api-server/go-server`, deployed on Replit autoscale. The dashboard polls telemetry, lists task state, opens a live log stream, and allows tasks to be created and deleted. The TypeScript Express server under `artifacts/api-server/src` is not the production runtime for deployed `/api` traffic; production is defined by `artifacts/api-server/.replit-artifact/artifact.toml` and builds/runs the Go server.

Assumptions for future scans:
- Only production-reachable paths matter for findings.
- `artifacts/mockup-sandbox` is development-only unless separate production routing is added.
- Replit provides TLS for deployed traffic.
- `NODE_ENV=production` in production.

## Assets

- **Task payloads and results** — task bodies may contain prompts, commands, dataset identifiers, or other operational data. Unauthorized reads expose business context; unauthorized writes or deletes let attackers manipulate engine behavior.
- **Operational logs and telemetry** — live log streams, throughput, goroutine counts, memory, and status data reveal internal behavior and may expose sensitive task content.
- **Service availability** — the API backs both the dashboard and the task engine simulation. Unbounded or attacker-triggered work can degrade or stop service for all users.
- **Deployment and database secrets** — the repo includes a DB package and environment-based configuration. Secrets must remain server-side and never appear in logs or client bundles.

## Trust Boundaries

- **Browser to Go API** — all dashboard interactions cross from an untrusted client to the production API at `/api/*`. Every sensitive operation must be authenticated, authorized, and validated server-side.
- **Cross-origin web pages to Go API** — any third-party website can cause a browser to send requests to the API. CORS policy and credential behavior define whether those responses are readable cross-origin.
- **Go API to in-memory task/log stores** — public HTTP requests directly mutate the in-memory queue and log distribution system. Missing backpressure or rate controls here can become production DoS issues.
- **Build-time source vs deployed runtime** — the repo contains a non-production Express implementation and a production Go service. Scans should prioritize the Go server for deployed API behavior.

## Scan Anchors

- Production API entry point: `artifacts/api-server/go-server/main.go`
- High-risk production code: `artifacts/api-server/go-server/{main.go,tasks.go,logs.go,telemetry.go}`
- Production frontend consuming those endpoints: `artifacts/dashboard/src/pages/*`, `artifacts/dashboard/src/hooks/use-log-stream.ts`
- Public surfaces currently appear to be `/api/healthz`, `/api/telemetry`, `/api/tasks`, `/api/tasks/summary`, `/api/tasks/{id}`, and `/api/logs/stream`
- Usually ignore as dev-only: `artifacts/mockup-sandbox/**`, `artifacts/api-server/src/**` unless deployment config changes

## Threat Categories

### Spoofing

The production API currently exposes operational endpoints without any authentication layer. The system must require a valid authenticated identity before allowing task creation, deletion, log streaming, or access to operational telemetry intended for trusted users.

### Tampering

Task creation and deletion directly alter shared engine state. The system must enforce server-side authorization on every mutating route, constrain who may choose agent identifiers or payload content, and reject attacker-driven state changes from unauthenticated or cross-origin contexts.

### Information Disclosure

The dashboard exposes live logs, task payloads, and engine telemetry. The system must ensure those responses are only available to authorized users and that CORS does not let arbitrary third-party origins read operational data from a victim’s browser.

### Denial of Service

Public endpoints can enqueue work, keep streaming connections open, and drive repeated polling. The system must bound queue growth, connection count, and request rate so one actor cannot exhaust goroutines, memory, or request processing capacity.

### Elevation of Privilege

If all operational routes are public, any internet user effectively gains operator-level control over the dashboard backend. The system must separate public health checks from privileged control-plane actions and enforce authorization server-side on every privileged route.