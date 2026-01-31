# CloudScan Orchestrator - Architecture

## Overview

The **cloudscan-orchestrator** is the core service that manages scan lifecycles. It follows a microservices architecture pattern where storage operations are delegated to a separate storage service.

---

## System Architecture

```
┌──────────────────┐
│                  │
│   CloudScan UI   │
│  (Spring Boot)   │
│                  │
└────────┬─────────┘
         │ gRPC/HTTP
         │
         ▼
┌──────────────────────────────────────────────────────────┐
│                                                          │
│           CloudScan Orchestrator (this service)          │
│                                                          │
│  ┌──────────────┐    ┌──────────────┐   ┌────────────┐ │
│  │  gRPC Server │    │  HTTP Server │   │  Workers   │ │
│  │              │    │              │   │            │ │
│  │  - CreateScan│    │  - Health    │   │  - Sweeper │ │
│  │  - GetScan   │    │  - Metrics   │   │  - Cleaner │ │
│  │  - ListScans │    │  - Ready     │   │            │ │
│  │  - CancelScan│    │              │   │            │ │
│  └──────────────┘    └──────────────┘   └────────────┘ │
│                                                          │
│  ┌──────────────┐    ┌──────────────┐   ┌────────────┐ │
│  │  PostgreSQL  │    │  Kubernetes  │   │  Storage   │ │
│  │  Repository  │    │  Dispatcher  │   │  Client    │ │
│  │              │    │              │   │  (gRPC)    │ │
│  │  - Scans     │    │  - Create    │   │            │ │
│  │  - Findings  │    │    Jobs      │   │            │ │
│  │  - Projects  │    │  - Monitor   │   │            │ │
│  │  - Orgs      │    │    Status    │   │            │ │
│  └──────────────┘    └──────────────┘   └────────────┘ │
│                                                          │
└────────┬──────────────────────┬────────────────┬────────┘
         │                      │                │
         │                      │                │ gRPC
         ▼                      ▼                ▼
┌──────────────────┐  ┌──────────────────┐  ┌──────────────────┐
│                  │  │                  │  │                  │
│   PostgreSQL     │  │   Kubernetes     │  │  CloudScan       │
│   Database       │  │   Cluster        │  │  Storage Service │
│                  │  │                  │  │                  │
│  - scans         │  │  ┌────────────┐  │  │  ┌────────────┐  │
│  - findings      │  │  │   Runner   │  │  │  │  MinIO/S3  │  │
│  - projects      │  │  │    Job     │──┼──┼─▶│  GCS/Azure │  │
│  - organizations │  │  └────────────┘  │  │  └────────────┘  │
│                  │  │                  │  │                  │
└──────────────────┘  └──────────────────┘  └──────────────────┘
```

---

## Key Design Principles

### 1. **Separation of Concerns**

**Orchestrator** responsibilities:
- Scan lifecycle management (create, update, cancel, complete)
- Kubernetes job dispatching
- Scan metadata persistence
- Finding aggregation
- Multi-tenant data isolation

**Storage Service** responsibilities:
- Object storage abstraction (S3/MinIO/GCS/Azure)
- Presigned URL generation
- Artifact metadata tracking
- Upload/download operations

**Why separate?**
- Storage backends can be swapped without changing orchestrator
- Storage service can be scaled independently
- Clear API boundary between services

### 2. **Presigned URL Pattern**

Instead of proxying file uploads/downloads through the orchestrator:

1. **Client** calls `orchestrator.CreateScan()`
2. **Orchestrator** calls `storage.CreateArtifact()` → gets `artifact_id` + `presigned_url`
3. **Orchestrator** stores `artifact_id` in database
4. **Orchestrator** returns `presigned_url` to client
5. **Client** uploads directly to S3/MinIO using presigned URL
6. **Runner Job** does the same for downloading source/uploading results

**Benefits:**
- No file proxying (saves orchestrator resources)
- Direct S3/MinIO upload (faster, more reliable)
- Orchestrator only handles metadata

### 3. **Interface-Based Design**

All external dependencies are defined as interfaces:

```go
// Defined in internal/interfaces/

type ScanRepository interface { ... }        // PostgreSQL
type StorageClient interface { ... }         // Storage service gRPC client
type JobDispatcher interface { ... }         // Kubernetes API
type CacheService interface { ... }          // Redis
```

**Benefits:**
- Easy to mock for testing
- Can swap implementations (e.g., local storage → S3)
- Clear contracts between layers

---

## Data Flow

### Creating a Scan

```
1. UI → orchestrator.CreateScan(repo_url, branch, scan_types)

2. Orchestrator:
   - Generate scan ID
   - Call storage.CreateArtifact("source.zip") → get artifact_id_1 + upload_url
   - Store scan in PostgreSQL with artifact_id_1
   - Return scan_id + upload_url to UI

3. UI:
   - Download source code from Git
   - ZIP the source
   - PUT to upload_url (direct to MinIO/S3)

4. UI → orchestrator.StartScan(scan_id)

5. Orchestrator:
   - Create Kubernetes Job for runner
   - Pass artifact_id_1 (source) to job as env var
   - Update scan status to "queued"

6. Runner Job (in Kubernetes):
   - Call orchestrator.GetArtifact(artifact_id_1) → get download_url
   - Download source from download_url
   - Run scanners (Semgrep, Trivy, TruffleHog, ScanCode)
   - Generate SARIF results
   - Call storage.CreateArtifact("results.sarif") → get artifact_id_2 + upload_url
   - Upload results to upload_url
   - Call orchestrator.CompleteScan(scan_id, artifact_id_2, findings)

7. Orchestrator:
   - Store findings in PostgreSQL
   - Update scan status to "completed"
   - Update scan with results artifact_id_2
```

### Retrieving Scan Results

```
1. UI → orchestrator.GetScan(scan_id)

2. Orchestrator:
   - Query PostgreSQL for scan
   - Return scan metadata + artifact_ids

3. UI → orchestrator.GetArtifact(artifact_id_2)

4. Orchestrator:
   - Call storage.GetArtifact(artifact_id_2) → get presigned download_url
   - Return download_url to UI

5. UI:
   - Download SARIF file directly from MinIO/S3 using presigned URL
```

---

## Database Schema

**PostgreSQL 15+ with table partitioning**

```sql
-- Multi-tenant isolation
organizations (id, name, slug, created_at)

-- Code projects
projects (id, organization_id, name, repo_url, default_branch)

-- Scans (partitioned by created_at for performance)
scans (
    id, organization_id, project_id, user_id, status,
    scan_types[], repository_url, branch, commit_sha,
    source_archive_key,    -- artifact_id from storage service
    job_name, job_namespace,
    findings_count, critical_count, high_count, medium_count, low_count,
    started_at, completed_at, error_message,
    created_at, updated_at
)
PARTITION BY RANGE (created_at);

-- Security findings
findings (
    id, scan_id, scan_type, tool_name, severity,
    title, description, file_path, start_line, end_line,
    cve_id, cwe_id, cvss_score,
    package_name, package_version, fixed_version,
    remediation, references, raw_output, created_at
)
```

---

## APIs

### gRPC API (Port 9999)

```protobuf
service ScanService {
  // Scan management
  rpc CreateScan(CreateScanRequest) returns (CreateScanResponse);
  rpc GetScan(GetScanRequest) returns (Scan);
  rpc ListScans(ListScansRequest) returns (ListScansResponse);
  rpc CancelScan(CancelScanRequest) returns (Empty);
  rpc UpdateScan(UpdateScanRequest) returns (Empty);

  // Findings
  rpc GetFindings(GetFindingsRequest) returns (GetFindingsResponse);

  // Storage proxy (calls storage service)
  rpc GetUploadURL(GetUploadURLRequest) returns (GetUploadURLResponse);
  rpc GetDownloadURL(GetDownloadURLRequest) returns (GetDownloadURLResponse);
}
```

### HTTP API (Port 8081)

```
GET  /health             # Health check
GET  /ready              # Readiness check
GET  /metrics            # Prometheus metrics
```

---

## Background Workers

### Sweeper

**Purpose:** Monitor Kubernetes job status and update scans

```go
Every 30 seconds:
  1. Query PostgreSQL for scans with status = "queued" or "running"
  2. For each scan:
     - Get Kubernetes job status
     - If job succeeded → update scan to "completed"
     - If job failed → update scan to "failed" + error_message
     - If job running → keep as "running"
  3. Clean up completed jobs after retention period (ttlSecondsAfterFinished)
```

### Cleaner

**Purpose:** Enforce retention policies

```go
Every day at midnight:
  1. Query PostgreSQL for scans older than retention period (default: 90 days)
  2. For each old scan:
     - Get all artifact_ids
     - Call storage.DeleteArtifacts(artifact_ids)
     - Delete findings from PostgreSQL
     - Delete scan from PostgreSQL (or mark as deleted)
  3. Log cleanup statistics
```

---

## Configuration

**Environment Variables:**

```bash
# Server
GRPC_PORT=9999
HTTP_PORT=8081
METRICS_PORT=9090
LOG_LEVEL=info

# Database
DB_HOST=localhost
DB_PORT=5432
DB_USER=cloudscan
DB_PASSWORD=changeme
DB_NAME=orchestrator
DB_SSLMODE=prefer

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379
REDIS_PASSWORD=""

# Storage Service (gRPC client)
STORAGE_SERVICE_HOST=cloudscan-storage
STORAGE_SERVICE_PORT=8082

# Kubernetes
KUBE_NAMESPACE=cloudscan
KUBE_IN_CLUSTER=false
RUNNER_IMAGE=cloudscan/cloudscan-runner
RUNNER_VERSION=latest

# Observability
PROMETHEUS_ENABLED=true
JAEGER_URL=http://jaeger:14268/api/traces
```

---

## Development vs Production

### Local Development (KIND)

```yaml
onPrem:
  postgresql: true  # Deploy PostgreSQL in cluster
  redis: true       # Deploy Redis in cluster
  minio: true       # Deploy MinIO in cluster

KUBE_IN_CLUSTER: false
STORAGE_SERVICE_HOST: localhost:8082
```

### Production (EKS/GKE/AKS)

```yaml
onPrem:
  postgresql: false  # Use RDS/Cloud SQL
  redis: false       # Use ElastiCache/Memorystore
  minio: false       # Use S3/GCS directly

KUBE_IN_CLUSTER: true
STORAGE_SERVICE_HOST: cloudscan-storage.cloudscan.svc.cluster.local
```

---

## Security

1. **Multi-tenancy:** All queries filtered by `organization_id`
2. **mTLS:** gRPC communication between services (optional)
3. **JWT:** Authentication tokens from API gateway
4. **RBAC:** Kubernetes ServiceAccount with minimal permissions
5. **Presigned URLs:** Time-limited, scoped access to storage

---

## Next Steps

1. ✅ Domain models defined
2. ✅ Interfaces defined
3. ✅ PostgreSQL repository implemented
4. ✅ Database migrations created
5. ✅ Configuration management
6. ⏳ Create Kubernetes job dispatcher
7. ⏳ Implement gRPC service
8. ⏳ Implement background workers
9. ⏳ Build storage service (separate repo)
10. ⏳ Build runner (separate repo)