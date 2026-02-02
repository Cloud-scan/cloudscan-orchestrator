# cloudscan-orchestrator

> Core orchestration service for CloudScan platform - manages scan lifecycle, dispatches Kubernetes jobs, and coordinates all scanning activities

---

## 🎯 Overview

The **cloudscan-orchestrator** is the heart of the CloudScan platform. It:
 
- 📋 Manages scan lifecycle (create, queue, execute, complete)
- 🚀 Dispatches Kubernetes Jobs for scanner runners
- 🗄️ Persists scan metadata and findings in PostgreSQL
- 🔄 Runs background workers (sweeper, cleaner, notifier)
- 🔐 Handles multi-tenant data isolation
- 📡 Exposes gRPC and HTTP APIs
- 📊 Collects and exposes Prometheus metrics

---

## 🏗️ Architecture

### Service Interactions

```
┌─────────────────┐
│   API Gateway   │
│   (REST/gRPC)   │
└────────┬────────┘
         │ CreateScan(artifact_id)
         │ GetScan(), ListScans()
         ▼
┌──────────────────────────────────────────────────┐
│          Orchestrator Service (This)             │
│                                                  │
│  ┌────────────┐  ┌──────────┐  ┌─────────────┐ │
│  │ gRPC Server│  │ Sweeper  │  │  Cleaner    │ │
│  │ (Port 9999)│  │ (Worker) │  │  (Worker)   │ │
│  └─────┬──────┘  └────┬─────┘  └──────┬──────┘ │
│        │              │                │        │
│        ▼              ▼                ▼        │
│  ┌──────────────────────────────────────────┐  │
│  │      Job Dispatcher (K8s client-go)      │  │
│  └──────────────┬───────────────────────────┘  │
└─────────────────┼──────────────────────────────┘
                  │
         ┌────────┼────────┐
         │        │        │
         │        ▼        │
         │  ┌──────────────────┐
         │  │ Storage Service  │
         │  │ (gRPC)           │
         │  │ GetArtifact()    │
         │  └──────────────────┘
         │        │
         │        │ presigned URL
         │        ▼
         │  ┌──────────────────┐
         │  │  S3/MinIO/GCS    │
         │  │  Object Storage  │
         │  └──────────────────┘
         │
         ▼
┌─────────────────────┐
│ Kubernetes Cluster  │
│                     │
│  ┌───────────────┐  │
│  │ Runner Job    │  │
│  │ (Pod)         │  │
│  │               │  │
│  │ - Downloads   │──┼──→ S3 (presigned URL)
│  │   source      │  │
│  │ - Runs        │  │
│  │   scanners    │  │
│  │ - Reports     │  │
│  │   findings    │──┼──→ Orchestrator.CreateFindings()
│  └───────────────┘  │      Orchestrator.UpdateScan()
└─────────────────────┘
         │
         ▼
┌─────────────────────┐
│   PostgreSQL        │
│   - Scans           │
│   - Findings        │
│   - Projects        │
└─────────────────────┘
```

### Code Structure

```
cloudscan-orchestrator
├── cmd/
│   └── main.go                    # Application entrypoint
├── pkg/
│   ├── controller/                # Main controller & component manager
│   │   ├── controller.go
│   │   ├── grpc.go               # gRPC server component
│   │   └── http.go               # HTTP server component
│   ├── dispatcher/                # Kubernetes job dispatcher
│   │   ├── dispatcher.go         # Job creation & dispatch logic
│   │   └── job_spec.go           # K8s Job spec builder
│   ├── handlers/
│   │   ├── grpc/                 # gRPC service implementations
│   │   │   ├── scans.go         # Scan management
│   │   │   ├── jobs.go          # Job operations
│   │   │   └── health.go        # Health checks
│   │   └── http/                 # HTTP handlers (optional REST API)
│   │       ├── scans.go
│   │       └── middleware.go
│   ├── persistence/               # Database layer
│   │   ├── scans.go              # Scan CRUD operations
│   │   ├── findings.go           # Findings storage
│   │   ├── projects.go           # Project management
│   │   └── users.go              # User management
│   ├── sweeper/                   # Background worker: job status monitor
│   │   └── sweeper.go
│   ├── cleaner/                   # Background worker: retention cleanup
│   │   └── cleaner.go
│   ├── authentication/            # Auth providers (JWT, mTLS)
│   │   ├── jwt.go
│   │   └── mtls.go
│   ├── metrics/                   # Prometheus metrics
│   │   └── metrics.go
│   └── config/                    # Configuration management
│       └── config.go
├── proto/                         # Protocol buffers definitions
│   └── scans.proto               # Scan service gRPC API
├── migrations/                    # Database migrations (SQL)
│   ├── 001_initial_schema.up.sql
│   └── 001_initial_schema.down.sql
├── Dockerfile
├── go.mod
├── go.sum
└── README.md
```

---

## 🚀 Quick Start

### Prerequisites

- Go 1.23+
- PostgreSQL 15+
- Kubernetes cluster (for job dispatching)
- kubectl configured

### Development Setup

```bash
# Clone the repository
cd cloudscan-orchestrator

# Install dependencies
go mod download

# Run PostgreSQL locally (via Docker)
docker run --name postgres \
  -e POSTGRES_PASSWORD=postgres \
  -e POSTGRES_DB=cloudscan \
  -p 5432:5432 \
  -d postgres:15

# Run database migrations
# TODO: Add migration tool (e.g., golang-migrate)

# Run the service
go run cmd/main.go \
  --db-host=localhost \
  --db-port=5432 \
  --db-name=cloudscan \
  --db-user=postgres \
  --db-password=postgres \
  --grpc-port=9999 \
  --http-port=8081
```

### Configuration

Configuration can be provided via:
1. Environment variables (recommended for production)
2. Config file (`config.yaml`)
3. Command-line flags

**Environment Variables:**
```bash
# Database
export DB_HOST=localhost
export DB_PORT=5432
export DB_NAME=cloudscan
export DB_USER=postgres
export DB_PASSWORD=postgres

# Kubernetes
export KUBE_NAMESPACE=cloudscan
export KUBE_IN_CLUSTER=false  # Set to true when running in K8s

# Ports
export GRPC_PORT=9999
export HTTP_PORT=8081

# Observability
export PROMETHEUS_PORT=9090
export JAEGER_URL=http://jaeger:14268/api/traces

# Storage Service
export STORAGE_SERVICE_URL=cloudscan-storage:8082
```

---

## 📡 API

### gRPC API

The orchestrator exposes gRPC services defined in `proto/scans.proto`:

**Key RPCs:**
- `CreateScan` - Start a new security scan
- `GetScan` - Retrieve scan status and results
- `ListScans` - List scans with filters
- `CancelScan` - Cancel a running scan
- `GetFindings` - Get security findings for a scan
- `UpdateScan` - Update scan metadata

**Example gRPC call:**
```bash
grpcurl -plaintext \
  -d '{"project_id": "proj-123", "scan_types": ["sast", "sca"]}' \
  localhost:9999 \
  cloudscan.ScanService.CreateScan
```

### HTTP API (Optional)

If enabled, provides RESTful endpoints:

```
POST   /api/v1/scans              # Create scan
GET    /api/v1/scans/:id          # Get scan details
GET    /api/v1/scans              # List scans
DELETE /api/v1/scans/:id          # Cancel scan
GET    /api/v1/scans/:id/findings # Get findings
```

---

## 🔄 Background Workers

### Sweeper

Monitors Kubernetes Jobs and updates scan status:

- Polls K8s Job status every 30 seconds
- Updates scan state: `queued` → `running` → `completed`/`failed`
- Handles job failures and retries
- Cleans up completed jobs after retention period

### Cleaner

Enforces data retention policies:

- Deletes scans older than configured retention (default: 90 days)
- Removes associated findings and artifacts
- Runs daily at midnight
- Configurable retention per organization

---

## 🗄️ Database Schema

Key tables:

**organizations** - Multi-tenant isolation
```sql
CREATE TABLE organizations (
    id UUID PRIMARY KEY,
    name TEXT NOT NULL,
    slug TEXT UNIQUE NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
);
```

**scans** - Scan metadata (partitioned by date)
```sql
CREATE TABLE scans (
    id UUID PRIMARY KEY,
    organization_id UUID NOT NULL,
    project_id UUID REFERENCES projects(id),
    status TEXT NOT NULL, -- queued, running, completed, failed
    scan_types TEXT[] NOT NULL,
    created_at TIMESTAMP DEFAULT NOW()
) PARTITION BY RANGE (created_at);
```

**findings** - Security vulnerabilities
```sql
CREATE TABLE findings (
    id UUID PRIMARY KEY,
    scan_id UUID REFERENCES scans(id),
    severity TEXT NOT NULL, -- critical, high, medium, low
    title TEXT NOT NULL,
    file_path TEXT,
    line_number INT
);
```

See `migrations/` for full schema.

---

## 🔐 Authentication

Supports multiple authentication methods:

1. **JWT** (recommended for UI/API access)
   - Tokens issued by API gateway
   - Validated on each request

2. **mTLS** (for service-to-service)
   - Client certificates validated
   - Used by scanner runners

3. **Disabled** (development only)
   - No authentication

---

## 📊 Metrics

Prometheus metrics exposed on `/metrics`:

**Key metrics:**
- `cloudscan_scans_total{status="completed|failed"}` - Total scans
- `cloudscan_scan_duration_seconds` - Scan duration histogram
- `cloudscan_queue_depth` - Number of queued scans
- `cloudscan_findings_total{severity="critical|high|medium|low"}` - Total findings
- `cloudscan_k8s_jobs_active` - Active Kubernetes jobs

---

## 🧪 Testing

```bash
# Run unit tests
go test ./pkg/...

# Run with coverage
go test -cover ./pkg/...

# Run integration tests (requires PostgreSQL)
go test -tags=integration ./pkg/...

# Run specific package tests
go test ./pkg/dispatcher/...
```

---

## 🐳 Docker

### Build

```bash
# Build for linux/amd64
docker build -t cloudscan/orchestrator:latest .

# Multi-platform build
docker buildx build \
  --platform linux/amd64,linux/arm64 \
  -t cloudscan/orchestrator:latest \
  --push .
```

### Run

```bash
docker run -p 9999:9999 -p 8081:8081 \
  -e DB_HOST=postgres \
  -e DB_PASSWORD=secret \
  cloudscan/orchestrator:latest
```

---

## 🚢 Deployment

### Kubernetes

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: cloudscan-orchestrator
spec:
  replicas: 3
  selector:
    matchLabels:
      app: cloudscan-orchestrator
  template:
    metadata:
      labels:
        app: cloudscan-orchestrator
    spec:
      serviceAccountName: cloudscan-orchestrator
      containers:
      - name: orchestrator
        image: cloudscan/orchestrator:latest
        ports:
        - containerPort: 9999  # gRPC
        - containerPort: 8081  # HTTP
        - containerPort: 9090  # Metrics
        env:
        - name: DB_HOST
          value: postgres
        - name: KUBE_IN_CLUSTER
          value: "true"
        resources:
          requests:
            cpu: 500m
            memory: 512Mi
          limits:
            cpu: 2000m
            memory: 2Gi
```

**Service Account:**
The orchestrator needs permissions to create/manage Kubernetes Jobs.

See [cloudscan-umbrella](https://github.com/cloudscan/cloudscan-umbrella) for complete Helm deployment.

---

## 🤝 Contributing

1. Fork the repository
2. Create a feature branch
3. Make changes with tests
4. Submit a pull request 

**Code style:**
- Follow Go conventions
- Run `gofmt` and `golint`
- Add unit tests for new features

---

## 📄 License 

Apache 2.0 - See [LICENSE](../LICENSE)
