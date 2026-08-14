# OpsNexus Backend (`opsnexus-backend`)

[![Release](https://img.shields.io/badge/release-v0.5.0-blue.svg)](https://github.com/OpsNexusHQ/opsnexus-backend/releases/tag/v0.5.0)
[![Go Version](https://img.shields.io/badge/go-1.25+-00ADD8.svg)](https://golang.org)
[![License](https://img.shields.io/badge/license-MIT-green.svg)](LICENSE)

The core control plane and HTTP API server for **OpsNexus**, a cloud-native infrastructure monitoring and observability platform.

---

## 🏛️ Architecture Overview

The backend ingests periodic system metrics from Linux agents, persists telemetry in PostgreSQL (JSONB format), evaluates alert conditions in real time, manages incident workflows (Firing → Acknowledged → Resolved), dispatches notifications (Webhooks & Slack), and streams real-time updates via Server-Sent Events (SSE).

```text
┌────────────────┐        HTTP Telemetry (10s)        ┌────────────────┐
│  Linux Agent   ├───────────────────────────────────►│   Go Backend   │
│(opsnexus-agent)│                                    └───────┬────────┘
└────────────────┘                                            │
                                         ┌────────────────────┼────────────────────┐
                                         ▼                    ▼                    ▼
                                    PostgreSQL           Alert Engine           SSE Hub
                              (agents, telemetry,       (sustained rules,          │
                               alerts, comments,         ack, comments)            │
                               channels, tokens,              │                    │
                               telemetry_hourly)              ▼                    │
                                         │            Notification Queue           │
                                         │            & Worker Pool                │
                                         │                    │                    │
                                         │            ┌───────┴───────┐            │
                                         │            ▼               ▼            │
                                         │         Webhook          Slack          │
                                         ▼                                         ▼
                                  Retention Worker                          Real-Time Dashboard
                                  (30d purge / rollup)                      (React + TypeScript)
```

---

## ✨ Implemented Capabilities (v0.5.0)

- **Agent Registration & Heartbeats**: Tracks agent status (`healthy`, `stale`, `offline`).
- **Telemetry Ingestion**: Stores Linux metrics (CPU, Memory, Disk, Network, Uptime, Processes) as JSONB.
- **Server-Sent Events (SSE)**: Real-time broadcast channel at `GET /api/v1/events` for instant UI updates.
- **Alert Engine & Incident Workflow**: Evaluates sustained metric thresholds (`for_duration`), supports alert acknowledgement and incident comment threads.
- **Notification Queue & Webhooks**: Worker pool with exponential backoffs, HMAC-SHA256 signature verification (`X-OpsNexus-Signature`), and Slack integration.
- **Telemetry Retention & Archival**: Automatic purge worker enforcing `OPSNEXUS_TELEMETRY_RETENTION_DAYS` (default 30d) and hourly rollup summaries (`telemetry_hourly`).
- **Security & RBAC**: Optional API Token authentication (`OPSNEXUS_API_AUTH_ENABLED`) with `viewer`, `operator`, and `admin` roles.
- **Observability APIs**: Fleet overview, health classification, latest metrics, and historical time-series analytics.

---

## ⚙️ Configuration & Environment Variables

| Variable | Default | Description |
|---|---|---|
| `OPSNEXUS_HOST` | `0.0.0.0` | Server bind IP address |
| `OPSNEXUS_PORT` | `8080` | Server HTTP port |
| `OPSNEXUS_DATABASE_URL` | — | PostgreSQL connection string |
| `OPSNEXUS_HEALTHY_THRESHOLD` | `30s` | Duration threshold for healthy status |
| `OPSNEXUS_STALE_THRESHOLD` | `2m` | Duration threshold for stale status |
| `OPSNEXUS_CORS_ORIGINS` | `*` | Allowed CORS origins |
| `OPSNEXUS_RATE_LIMIT` | `120` | Requests per minute rate limit |
| `OPSNEXUS_TELEMETRY_RETENTION` | `30` | Telemetry retention in days |
| `OPSNEXUS_API_AUTH_ENABLED` | `false` | Enable Bearer token authentication |

---

## 🚀 Quickstart & Setup

### Requirements
- Go 1.25+
- PostgreSQL 14+

### Local Run
```bash
# 1. Clone & Navigate
git clone https://github.com/OpsNexusHQ/opsnexus-backend.git
cd opsnexus-backend

# 2. Set Database Connection
export OPSNEXUS_DATABASE_URL="postgres://opsnexus_user:opsnexus_password@localhost:5432/opsnexus?sslmode=disable"

# 3. Run Backend Server
go run ./cmd/server
```

### Run Tests
```bash
go test ./...
```

---

## 📡 Key API Endpoints

```text
GET  /health                                 # Health check
GET  /api/v1/events                          # SSE Real-time Event Stream
POST /api/v1/agents/register                 # Register Agent
POST /api/v1/agents/{id}/telemetry           # Ingest Agent Telemetry
GET  /api/v1/overview                        # Fleet Health Summary
GET  /api/v1/agents/{id}/analytics           # Historical Time-Series Analytics
GET  /api/v1/alerts                          # List Firing/Acked Alerts
POST /api/v1/alerts/{id}/acknowledge         # Acknowledge Incident
GET  /api/v1/notification-channels           # Manage Notification Webhooks
```

---

## 🗺️ Roadmap (Future Scope)

- [ ] **OpenTelemetry (OTLP) Ingest**: Native OTLP gRPC endpoint for APM traces.
- [ ] **eBPF Kernel Integration**: Zero-overhead network latency & dependency mapping.
- [ ] **Kubernetes Operator**: Native K8s agent daemonset & controller.
- [ ] **AI Incident RCA**: LLM-driven root cause analysis on firing alerts.
- [ ] **Multi-Tenancy**: Organization & tenant isolation.

---

## 📄 License

Part of the [OpsNexus](https://github.com/OpsNexusHQ) ecosystem. Licensed under the MIT License.
