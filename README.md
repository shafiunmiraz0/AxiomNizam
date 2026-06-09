# AxiomNizam

<p align="center">
	<img src="frontend/templates/axiomnizam-logo-minimal.svg" alt="AxiomNizam logo" width="180" />
</p>

Comprehensive platform for API control-plane workflows, multi-database REST services, GraphQL, data platform orchestration (ETL/CDC), GIS and analytics dashboards, and NetIntel observability.

This README is intentionally detailed and aligned with the current code in this repository.

## Table of Contents

1. Overview
2. Architecture and Services
3. Project Size Snapshot
4. What We Did So Far
5. Quick Start
6. Authentication, Authorization, and Security
7. Feature Coverage
8. REST API Coverage
9. GraphQL Coverage
10. Dashboard and UI Coverage
11. GIS Coverage
12. Network Intelligence (NetIntel) Coverage
13. CLI Full Command Reference
14. Internal Module Coverage
15. Frontend Template Coverage
16. Configuration and Environment Variables
17. Project Structure
18. Appendix: Feature-to-File Evidence Matrix
19. Troubleshooting
20. License

## Overview

AxiomNizam contains:

- Backend API server in Go (Gin) on port 8000.
- Frontend dashboard server in Go (Gin + templates) on port 7000.
- Keycloak-based authentication and token validation.
- Fine-grained RBAC middleware for auth-only and admin/system-manager routes.
- Platform services backed by embedded Raft storage (default) or external etcd for multi-tenant operations.
- Query logging, API metrics, and rate limiting.
- Data tooling: API Builder, CSV upload, dashboard generation, GIS conversion, and file malware scanning.

## Architecture and Services

The current runtime architecture is layered:

- Presentation layer: frontend Gin server on port 7000 with role-based dashboard routes.
- API layer: backend Gin server on port 8000 with auth, data, control-plane, and extension APIs.
- Control-plane layer: resource APIs and reconcile runtime loop backed by embedded Raft (default) or external etcd, on a dedicated runtime port (default 8001).
- Platform services layer: bulk/eventbus/export/webhook/stream/tenant/rbac/versioning/lineage/tracing managers, plus Conductor, IAM, and native object storage modules.

Storage backend options:

- `STORAGE_BACKEND=raft` — Embedded Raft + go-memdb + BoltDB. Single-binary deployment, no external state store needed. Recommended for self-hosted and development.
- `STORAGE_BACKEND=etcd` — External etcd cluster. Backward-compatible with existing deployments.

Default services in docker-compose:

- axiomnizam: backend API, http://localhost:8000
- axiomnizam-frontend: frontend UI, http://localhost:7000
- keycloak: identity provider, http://localhost:8080
- postgres: relational storage

Optional services:

- etcd: distributed state (only needed when STORAGE_BACKEND=etcd)
- openclaw-gateway: OpenClaw OpenAI-compatible gateway, http://localhost:18789
- ollama: local model runtime, http://localhost:11434
- ollama-init: one-shot TinyLlama bootstrap (model pull)
- valkey: cache/state support

Runtime notes:

- API server runs on configured API_HOST:API_PORT (default 0.0.0.0:8000).
- Internal runtime component starts on runtime port 8001 by default.
- Conductor routes are mounted at /api/v1/conductor and /ws/conductor when messaging backends initialize.
- IAM routes are mounted when IAM system initialization succeeds (including /iam/* and OIDC well-known endpoints).
- Storage routes are mounted under /api/v1/storage when object storage initialization succeeds.

## Project Size Snapshot

<!-- README_METRICS:START -->
Code inventory snapshot (workspace scan on 2026-06-01):

- Total code files (.go/.js/.ts/.tsx/.css/.html/.sql/.sh/.yaml/.yml): 1112
- Total code lines: 298454
- Go files (repository): 1041
- Go lines (repository): 244995
- Internal modules: 111
- Internal Go files: 987
- Internal Go lines: 230315

Counting method used:

- Excluded directories: .git, vendor, node_modules, dist, build.
- Counts include tests and generated source files committed in this repository.
- Line counts are physical lines across matching files.
<!-- README_METRICS:END -->

Regenerate this metrics block before release:

```bash
go run ./scripts/update_readme_metrics.go
```

## What We Did So Far

Recent updates completed in this repository:

- Added SQL Assistant panel integration path for API Builder backend and frontend.
- Added OpenClaw gateway integration for SQL assistant chat-completions.
- Added Ollama runtime and model bootstrap flow in docker-compose for TinyLlama.
- Added OpenClaw startup config seeding for Ollama provider and default model ref.
- Added OpenClaw model compatibility tuning for TinyLlama (context window metadata and tools compatibility).
- Improved SQL assistant fallback warnings to distinguish unreachable endpoint, provider credential errors, internal model errors, and timeout/cancel conditions.
- Increased SQL assistant timeout via environment variable to better support local model latency.

### Kubernetes-Style Reconcile Loop Migration (2026-04-27)

Migrated the entire platform from imperative CRUD to the AxiomNizam K8s-style
control-plane architecture. **All code phases complete.**

Key numbers:
- **33 reconciler controllers** running (29 GenericController + 3 runtime + 1 storage)
- **0 unwired reconcilers** — every reconciler is running
- **13 modules** with dual-write, **12** with authoritative path
- **30 etcd prefixes** monitored, **50+ new files**, **16+ modified**

Phases:
- **Phase 0 ✅** — Observability: metrics, `/health/reconcilers`, structured logging, etcd keyspace
- **Phase 1 ✅** — Shadow mode: 24 GenericControllers with work queues + panic recovery
- **Phase 2 ✅** — Dual-write: 13 handlers write to etcd alongside managers
- **Phase 3 ✅** — Authoritative: 12 handlers return 202 when `RECONCILER_AUTHORITATIVE_<MODULE>=true`
- **Phase 4 ⏳** — Operational: activate flags, 48h bake per module, then cleanup
- **Phase 5 ✅** — Wire remaining: jobs, etl, cdc, policies, datasource, iam/users, apiscanner
- **Phase 6 ⚠️** — GIS, analytics, transform, notification, netintel done. Only api_builder remains (dedicated sprint)

### Module Consistency: Handler Pattern Standardization (Phase 8)

**Phase 8 ✅** (completed 2026-05-25) — Standardize Handler Pattern: replace `gin.H` map literals with typed response structs.

Key results:
- **18 modules** processed: `cdc`, `encryption`, `jobs`, `netintel`, `security`, `schemaregistry`, `database`, `quality`, `datasource`, `federation`, `rbac`, `catalog`, `governance`, `featurestore`, `anonymization`, `streamanalytics`, `resources`, `slo`
- **175 `gin.H` occurrences** replaced with typed DTOs across all handler files
- **39/39 dto.go files** created or extended with response structs
- **All 18 modules** at 0 `gin.H` in handler files (1031→0 total across project)
- **Full build passes clean** — `go build ./...` succeeds

### Module Consistency: Models Pattern Standardization (Phase 9)

**Phase 9 ✅** (completed 2026-05-26) — Extract domain types into `models/` subdirectories.

Key results:
- **37 modules** now have `models/` directories (up from 6 pre-existing)
- **31 new `models/` directories** created with Resource types extracted
- **Type aliases** in parent packages ensure zero import breakage
- **All Resource types** include `GetObjectMeta`, `GetTypeMeta`, `GetStatus`, `SetStatus`, `DeepCopy`, `GetKey`, `GetGeneration`, `GetObservedGeneration` methods
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Repository Interfaces (Phase 10)

**Phase 10 ✅** (completed 2026-05-26) — Standardize repository interfaces separate from implementations.

Key results:
- **3 modules** with new `repositories/` packages: `storage`, `iam`, `jobs` (antivirus skipped — self-contained engine)
- **11 repository interfaces** created: `BucketRepository`, `RealmRepository`, `ClientRepository`, `UserRepository`, `RoleRepository`, `GroupRepository`, `ClientScopeRepository`, `IdentityProviderRepository`, `SessionRepository`, `EventRepository`, `JobRepository`
- **Compile-time checks** in each `check.go` verify concrete types satisfy interfaces
- **Alignment scores updated:** storage 7/8, iam 5/8, jobs 2/8
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Metrics Pattern (Phase 11)

**Phase 11 ✅** (completed 2026-05-26) — Standardize Prometheus metrics per module.

Key results:
- **4 modules** with new `metrics/` packages: `iam`, `jobs`, `antivirus`, `conductor`
- **IAM metrics:** 12 counters, 2 gauges, 3 histograms — auth attempts, tokens, permissions, sessions
- **Antivirus metrics:** 10 counters, 4 gauges, 2 histograms — scan counts, detection rates, engine timing
- **Conductor metrics:** 12 counters, 5 gauges, 3 histograms — messages, workflows, step durations
- **Jobs metrics:** labels.go + doc.go re-exports existing MetricsCollector
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Audit Pattern (Phase 12)

**Phase 12 ✅** (completed 2026-05-26) — Standardize audit logging with KV persistence across security-sensitive modules.

Key results:
- **4 modules** with new `audit/` packages: `iam`, `storage`, `antivirus`, `jobs`
- **Storage:** renamed `events/` → `audit/` (old events/ kept as re-export wrapper)
- **IAM audit:** 8 Log methods (Auth, TokenIssued, TokenRevoked, PermissionCheck, UserCreated, SessionCreated, SessionRevoked, RoleAssigned)
- **Antivirus audit:** 4 Log methods (ScanResult, ThreatDetected, EngineEvent, SignatureReload)
- **Jobs audit:** 7 Log methods (JobCreated, JobStarted, JobCompleted, JobFailed, JobCancelled, JobRetried, DLQEvent)
- **All loggers** have `ConfigureKVPersistence(kv)` following the gatekeeper pattern
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Eliminate Global Singletons (Phase 13)

**Phase 13 ✅** (completed 2026-05-26) — Eliminate global singletons, replace with constructor injection.

Key results:
- **9 of 19 singletons eliminated** (5 deleted, 4 converted to local instances)
- **Deleted:** GlobalWorkflowTriggerManager, GlobalDiffEngine, GlobalAuditLogger, GlobalDataPlatformIntegration, GlobalDataAccessControl, GlobalComplianceAuditor, GlobalCatalogIntegration, GlobalDataQualityMonitor, GlobalDataLineageAnalyzer
- **init() removed:** workflows/engine.go init() → RegisterBuiltinHandlers() method
- **Constructors refactored:** NewDataPlatformIntegration(), NewHealthMonitor(), NewPlatformMetricsCollector() take parameters
- **10 singletons deferred** — still have active consumers in main.go, integration/, cmd/
- **Full build passes clean** — `go build .` + `go build ./cmd/axiomnizamctl/` succeed

### Module Consistency: system.go Bootstrap (Phase 15)

**Phase 15 ✅** (completed 2026-05-26) — Core modules have `system.go` with standard bootstrap interface.

Key results:
- **7 modules** now have `system.go`: storage (pre-existing), iam (SetKVStore added), scanner, antivirus, jobs, conductor, cache
- **Standard interface:** `NewSystem()`, `Name()`, `Start()`, `Stop()`, `SetKVStore()`, `RegisterRoutes()`
- **Scanner system:** wraps Orchestrator + Metrics with KV persistence
- **Antivirus system:** wraps Engine with RegisterRoutes + Start/Stop lifecycle
- **Jobs system:** wraps JobManagerImpl + V1Handler with RegisterRoutes
- **Conductor system:** wraps Manager with RegisterRoutes
- **Cache system:** wraps Manager
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Central Type Package (Phase 16)

**Phase 16 ✅** (completed 2026-05-26) — Audit and document central type package status.

Key findings:
- **`resources/models/`** already serves as central type package (94 importers across all modules)
- **`contracts/module.go`** defines shared lifecycle interfaces (Module, KVStoreProvider, RoutesRegistrar, Startable)
- **`User` type** has 4 definitions across modules — only type meeting 3+ threshold (20+ modules import legacy `models.User`)
- **Role/Permission** duplicated but only cross 2 module boundaries each
- **Tenant/Job** have single definitions, no duplication
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Error Handling (Phase 17)

**Phase 17 ✅** (completed 2026-05-26) — Standardize typed error handling across all modules.

Key results:
- **`internal/errors/`** created with 12 sentinel errors + 5 typed error structs + HTTP mapper
- **4 modules** with `errors.go`: storage (6 sentinels), iam (10 sentinels), jobs (2 sentinels), antivirus (4 sentinels)
- **HTTP mapper:** `errors.HTTPStatusFromError()` maps typed errors to HTTP status codes
- **`ErrorResponse`** struct for standard JSON error responses
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Test Infrastructure (Phase 18)

**Phase 18 ✅** (completed 2026-05-26) — Shared test helpers and per-module test fixtures.

Key results:
- **`internal/testutil/`** created with helpers.go (Context, SkipIfShort) + mocks.go (MockKVStore)
- **4 modules** with `testutil/fixtures.go`: storage, iam, jobs, scanner
- **MockKVStore** — in-memory mock of platform/store.KVStore with Keys/Count/Clear
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Configurable Timeouts & URLs (Phase 19)

**Phase 19 ✅** (completed 2026-05-26) — No hardcoded values in production code.

Key results:
- **7 core modules** have `config/` with `LoadFromEnv()` + `DefaultConfig()` (prior session)
- **Grafana credentials** now env-configurable: `GRAFANA_ADMIN_USER`, `GRAFANA_ADMIN_PASSWORD`
- **5 observability endpoints** now env-configurable: `PROMETHEUS_URL`, `GRAFANA_URL`, `LOKI_URL`, `JAEGER_URL`
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Reconciler Standardization (Phase 20)

**Phase 20 ✅** (completed 2026-05-26) — All controllers follow K8s workqueue + rate-limiting pattern.

Key results:
- **30 controllers** use `GenericController[T]` in main.go
- **Rate limiter upgraded** to `DefaultControllerRateLimiter()` (per-item exponential + 10 QPS token bucket)
- **K8s-style health probes** — `internal/health/health.go` has `/healthz`, `/readyz`, `/livez` framework
- **ReconcilerMetrics** wired to all 30 controllers via Prometheus
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Event Bus Standardization (Phase 21)

**Phase 21 ✅** (completed 2026-05-26) — Audit of events/ vs eventbus/ — no merge needed.

Key findings:
- **`internal/events/`** — K8s-style EventRecorder for resource audit trails (RecordedEvent, InvolvedObject)
- **`internal/eventbus/`** — CloudEvents-style pub/sub for async messaging (EventBusEvent, topics, DLQ)
- **No overlap** — packages serve different purposes (audit vs messaging)
- **KV persistence** already in events/audit.go from Phase 12
- **Full build passes clean** — `go build .` succeeds

### Module Consistency: Storage Backend Abstraction (Phase 22)

**Phase 22 ✅** (completed 2026-05-26) — Clean backend interface for storage (MinIO pattern).

Key results:
- **`models.Backend`** interface in `storage/models/backend.go` — 80+ methods (buckets, objects, versioning, lifecycle, encryption, CORS, etc.)
- **`native.Backend`** — filesystem implementation
- **`s3client.Client`** — S3-compatible implementation
- **Dual-mode persistence** — `BucketStore` supports both etcd and Raft KV via `ConfigurePersistence()` / `ConfigureKVPersistence()`
- **Runtime selection** — `STORAGE_BACKEND=raft|etcd` env var
- **Full build passes clean** — `go build .` succeeds

### Operational Runbook

To activate the reconcile loop for a module in production:

```bash
# Step 1: Verify shadow mode is running (default)
curl http://localhost:8000/health/reconcilers
# Check: all modules show "initialized: true"

# Step 2: Enable dual-write for one module at a time
export DUAL_WRITE_VERSIONING=true
# Wait 48 hours, check /health/reconcilers for errors

# Step 3: Enable authoritative mode
export RECONCILER_AUTHORITATIVE_VERSIONING=true
# Wait 48 hours, check error rates

# Step 4: Repeat for next module
# Order: versioning → audit → tracing → encryption → rbac → webhooks → tenant → eventbus → export → bulk

# Instant rollback (any time):
export RECONCILER_AUTHORITATIVE_VERSIONING=false
```

Migration is complete — all code phases delivered.

New infrastructure:

- `internal/reconciler/instrumented.go` — structured logging wrapper
- `internal/metrics/reconciler_metrics.go` — per-module counters and health
- `internal/metrics/etcd_keyspace.go` — etcd prefix key-count monitoring
- `internal/platform/controller/generic_controller.go` — reusable watch+queue+worker controller
- `internal/platform/featureflags/flags.go` — per-module migration flags
- `internal/platform/dualwrite/dualwrite.go` — async best-effort etcd write helper
- 22 `resource.go` files — declarative resource types with TypeMeta/ObjectMeta/Spec/Status
- 22 `reconciler.go` files — reconcilers implementing `reconciler.Reconciler`
- 13 `dualwrite_handler.go` files — dual-write + authoritative path extensions

Runtime status notes:

- OpenClaw and Ollama services are configured under the compose profile openclaw.
- TinyLlama model ref is configured as ollama/tinyllama for SQL assistant calls.
- If the model response exceeds timeout, backend returns rule-based SQL suggestions with an explicit timeout warning.

### Platform Completion: Enterprise Data Platform Modules (2026-04-28)


Implemented the full enterprise data platform as 7 workstreams of declarative
resources with reconcilers, completing the AxiomNizam K8s/Nomad-style control plane.

**All 7 workstreams — 100% complete:**

- **WS-1: Data Catalog** (`internal/catalog/`) — Metadata registry, auto-discovery, PII detection, full-text search, collections
- **WS-2: Data Quality** (`internal/quality/rules/`, `internal/contracts/`) — 15 check types, data contracts, SLA tracking
- **WS-3: Schema Registry** (`internal/schemaregistry/`) — Versioned schemas, Protobuf/Avro/JSON support, evolution manager
- **WS-4: Observability** (`internal/alerting/`, `internal/slo/`, `internal/costing/`) — Alert rules, SLO tracking, cost attribution
- **WS-5: Federated Query** (`internal/federation/`) — Cross-source queries, optimizer, result cache, materialized views, profiler
- **WS-6: Governance** (`internal/governance/`) — Compliance policies (GDPR/HIPAA/SOC2/PCI-DSS), enforcer, auto-classification, erasure
- **WS-7: Analytics & ML** (`internal/featurestore/`, `internal/streamanalytics/`, `internal/anonymization/`, `internal/mlpipeline/`) — Feature store, streaming aggregation, anonymization, ML pipelines


### etcd Replacement: Nomad-Style Embedded Storage (2026-04-29) ✅

Completed the phased migration from external etcd to an embedded Raft + go-memdb + BoltDB
storage layer, inspired by HashiCorp Nomad's architecture.
See [docs/RAFT_STORAGE_GUIDE.md](docs/RAFT_STORAGE_GUIDE.md) for operational details.

**All 7 phases complete:**

- **Phase 1 ✅** — `MemDBStore[T]`: in-memory `ResourceStore[T]` implementation using `hashicorp/go-memdb` with primary key and namespace secondary indexes.
- **Phase 2 ✅** — Raft FSM: Finite State Machine with Apply/Snapshot/Restore, JSON-encoded command dispatch, and full state serialisation for Raft compaction.
- **Phase 3 ✅** — Raft Server: embedded `hashicorp/raft` node with BoltDB log+stable stores, TCP transport, file-based snapshots, single-node bootstrap.
- **Phase 4 ✅** — `RaftStore[T]`: unified `ResourceStore[T]` that reads from go-memdb and writes through Raft consensus.
- **Phase 5 ✅** — Feature-flagged integration: `BackendManager` wired into `main.go` with `STORAGE_BACKEND=raft`. All 30 reconciler stores use backend-agnostic `NewStore` factory.
- **Phase 6 ✅** — Direct etcd user migration: `KVStore` interface with `EtcdKVStore` and `MemDBKVStore` implementations for workflows, vectorplus, reviewflow, storage, and IAM modules.
- **Phase 7 ✅** — etcd made optional: etcd connection skipped when `STORAGE_BACKEND=raft`, docker-compose etcd service moved to profile, `depends_on` removed.

Key numbers:
- **11 new files** created across `internal/platform/store/`, `internal/platform/raft/`, and `internal/platform/featureflags/`
- **3 new dependencies**: `hashicorp/go-memdb`, `hashicorp/raft`, `hashicorp/raft-boltdb/v2`
- **4 store implementations**: `EtcdStore[T]`, `MemDBStore[T]`, `RaftStore[T]`, plus `KVStore` (etcd + memdb)
- **30 store creations** in main.go migrated to backend-agnostic `NewStore`
- **etcd is now optional** — not required when `STORAGE_BACKEND=raft`

New files:
```
internal/platform/store/memdb_schema.go   — go-memdb table/index schema definitions
internal/platform/store/memdb_store.go    — MemDBStore[T] implementation
internal/platform/store/raft_store.go     — RaftStore[T] implementation
internal/platform/store/backend.go        — BackendManager + NewStore factory
internal/platform/store/tables.go         — Central resource table registry
internal/platform/store/kvstore.go        — KVStore interface + EtcdKVStore + MemDBKVStore
internal/platform/raft/commands.go        — Raft log entry command types
internal/platform/raft/fsm.go            — Raft FSM (Apply/Snapshot/Restore)
internal/platform/raft/config.go         — Raft server configuration
internal/platform/raft/server.go         — Raft server lifecycle and peer management
```

To run with embedded Raft (no etcd needed):
```bash
export STORAGE_BACKEND=raft
docker compose up -d  # etcd will not start
```

To run with etcd (default, backward-compatible):
```bash
export STORAGE_BACKEND=etcd  # or leave unset
docker compose --profile etcd up -d
```

Post-implementation audit (2026-04-29): fixed 3 runtime bugs that would have prevented
`STORAGE_BACKEND=raft` from working correctly:
- JWT secret initialization timing: deferred to after BackendManager init in Raft mode
- FSM/KVStore type mismatch: aligned `kvFSMEntry` fields with `kvEntry` schema, added JSON-based fallback extraction
- Module persistence wiring: `ConfigureGlobalPersistence` calls skipped when etcd is nil (Raft mode), with deferred KV init after BackendManager
- Platform managers: removed hard etcd requirement — managers now work in-memory when etcd is nil (Raft mode)
- IAM system: migrated to use `KVStore` interface — IAM now works with both etcd and Raft backends. Added `iamBackend` interface, `kvStoreBackend` adapter, and `NewKV*Repository` constructors. IAM initialization deferred to after BackendManager in Raft mode. IAM routes registered after deferred init. Raft leader election wait added before KV writes.

### Platform Hardening: Coding Standards Enforcement (2026-05-03) ✅

Executed a systematic 10-section audit against `docs/CODING_PRACTICES.md` and remediated
all critical findings to harden the codebase for production readiness.

**Key results:**

- **Logging migration (Phase 3):** All 9 legacy handler files migrated from `log.Printf` to structured `logging.Z()` + zap — 57 log calls converted, 130+ structured log points across 38 files
- **Auth error hardening:** 5 IAM error responses genericized (no longer leak internal status codes, content types, or body summaries to clients)
- **Resilience:** All webhook and external HTTP operations wrapped with `resilience.DoVoid()` backoff/retry
- **Import standardization:** All handler and module files follow stdlib → internal → external import ordering
- **Naming collision fix:** Resolved `EvalConditionType` collision in `internal/alerting`
- **Quality rules alignment:** Fixed schema field mismatches in `internal/quality`
- **Build verification:** `go build ./...` and `go vet ./...` both pass clean (111 modules, 986 Go files)

See [docs/CODING_PRACTICES.md](docs/CODING_PRACTICES.md) for the full standards reference and [docs/PLATFORM_COMPLETION_PLAN.md](docs/PLATFORM_COMPLETION_PLAN.md) for the overall platform status.

## Quick Start

### Docker Compose (recommended)

```bash
docker compose up -d --build
```

Endpoints:

- Backend: http://localhost:8000
- Frontend: http://localhost:7000
- Keycloak: http://localhost:8080

Enable SQL assistant provider stack (OpenClaw + Ollama TinyLlama):

```bash
docker compose --profile openclaw up -d openclaw-gateway ollama ollama-init
```

Stop:

```bash
docker compose down
```

### Local Development

Backend:

```bash
go run main.go
```

Frontend:

```bash
cd frontend
go run main.go
```

## Authentication, Authorization, and Security

### Auth Endpoints

- POST /auth/login
- POST /auth/refresh
- POST /auth/logout
- GET /auth/validate
- GET /auth/token-status
- GET /auth/admin/tokens-status

### Public Health Endpoints

- GET /health
- GET /status
- GET /distributed

### Keycloak-Only Mode

Strict mode flag:

- KEYCLOAK_ONLY_AUTH=true

When enabled:

- Login uses Keycloak only.
- Local/demo fallback auth path is disabled.

### Roles and RBAC

Core privileged checks:

- admin middleware for strict admin-only routes.
- adminOrSys middleware for routes allowing admin and system-manager.

Accepted system-manager aliases in checks:

- system-manager
- sysadmin
- system_admin
- system-admin

### Rate Limiting

Token-bound rate limits:

- RATE_LIMIT_MAX_CALLS
- RATE_LIMIT_VALIDITY_MINUTES

Default in repo:

- 500 calls
- 10 minutes

### Platform User to Keycloak Sync

Endpoint:

- POST /api/v1/users

When KEYCLOAK_USER_SYNC_ENABLED=true, platform user creation includes:

1. Create user in Keycloak Admin API.
2. Ensure and assign realm role.
3. Store local platform user record.

Important sync env vars:

- KEYCLOAK_USER_SYNC_ENABLED
- KEYCLOAK_HOST
- KEYCLOAK_PORT
- KEYCLOAK_ADMIN_REALM
- KEYCLOAK_ADMIN_CLIENT_ID
- KEYCLOAK_ADMIN_USERNAME
- KEYCLOAK_ADMIN_PASSWORD

## Feature Coverage

Primary feature areas (validated from current route wiring and internal modules):

- Core API and auth: health/status/distributed, JWT/OAuth flows, token lifecycle, and token-bound rate limiting.
- Multi-database APIs: SQL CRUD and dynamic query endpoints for MySQL, MariaDB, PostgreSQL, Percona, Oracle, plus MongoDB/Firebase user handlers.
- GraphQL APIs with schema and playground.
- Control-plane APIs: namespaced resources, policy/workflow resources, workflow execution history, datasource registry, and job scheduling lifecycle.
- Platform service APIs: bulk operations, eventbus ack and DLQ replay, exports, webhooks, stream subscriptions, tenancy, RBAC access requests, versioning, lineage, and tracing.
- Conductor APIs: producer/consumer lifecycle, backend connection management, publish/stream, and DLQ replay.
- IAM APIs: OIDC metadata/JWKS, IAM auth, admin user/client/role operations, and IAM v2 realm/group/scope/session/event features.
- Native object storage APIs: bucket/object operations, presign/share, access keys, bucket policies, lifecycle, metrics, and governance controls.
- Data platform APIs: ETL and CDC pipelines, connectors/catalogs/observability, and platform overview.
- API Builder APIs: custom API CRUD/runtime invocation, CSV upload, dashboard/GIS generation, conversion workflows, file malware scan, API scan reports, and SQL assistant.
- Domain dashboards: admin/manager/system-manager, GIS, analytics, CDC/ETL, NetIntel, conductor, governance, operations-center, IAM admin, object storage, and lineage/version.
- Extension modules: kubeplus admission/scheduler/CRD, netintel mode detectors, vectorplus similarity search, and reviewflow scoring/quality pipeline.
- Autonomous orchestration internals: autopilot health/election decisions, planner plan-applier, binpack scheduler strategies, deployment rollout controller, node drainer, eval broker, heartbeat tracker, periodic dispatcher, service registry, and snapshot framing.
- Control-runtime style internals: apimachinery utility stack, informer cache fan-out, controller manager lifecycle, and workqueue/controller primitives.
- CLI operational tooling: discovery scans, wait checks, Trivy-based scan commands, and integration governance commands.

## Roadmap 1-5 Feature and Command References

### No. 1 Workflow execution lifecycle

- Backend APIs:
	- POST /api/v1/workflows/:name/run
	- GET /api/v1/workflows/:name/executions
	- GET /api/v1/workflows/executions/:id
- CLI commands:
	- workflow run [name]
	- workflow status [name|execution-id]

### No. 2 RBAC access request lifecycle

- Backend APIs:
	- POST /api/v1/rbac/access-requests
	- GET /api/v1/rbac/access-requests
	- POST /api/v1/rbac/access-requests/:id/approve
	- POST /api/v1/rbac/access-requests/:id/reject
- CLI commands:
	- rbacx access-request-list
	- rbacx access-request-create
	- rbacx access-request-approve [request-id]
	- rbacx access-request-reject [request-id]

### No. 3 Job scheduling APIs

- Backend APIs:
	- GET /api/v1/jobs/schedules
	- POST /api/v1/jobs/:id/schedule
	- DELETE /api/v1/jobs/:id/schedule
- CLI commands:
	- job schedule-list
	- job schedule-set [job-id]
	- job schedule-remove [job-id]

### No. 4 Tracing ingestion and ingestion-audit logging

- Backend APIs:
	- POST /api/v1/tracing/traces
	- POST /api/v1/tracing/spans
	- GET /api/v1/tracing/ingestion/audit
- CLI commands:
	- trace ingest
	- trace ingestion-audit-list

### No. 5 Event bus ack and DLQ replay

- Backend APIs:
	- POST /api/v1/eventbus/events/:id/ack
	- POST /api/v1/eventbus/dlq/:id/replay
- CLI commands:
	- eventbus ack [event-id]
	- eventbus dlq-replay [dlq-id]

## REST API Coverage

### Core Auth and Health

- /health
- /status
- /distributed
- /auth/*
- /auth/token-status

### GraphQL (REST-exposed route handlers)

- POST /api/graphql
- GET /api/graphql/schema
- GET /api/graphql/playground

### Multi-Database CRUD

Read and write APIs for:

- /api/mysql/users*
- /api/mariadb/users*
- /api/postgres/users*
- /api/percona/users*
- /api/mongodb/users*
- /api/firebase/users*
- /api/oracle/users*

### Dynamic Query APIs

Supported prefixes:

- /api/mysql/query
- /api/mariadb/query
- /api/postgres/query
- /api/percona/query
- /api/oracle/query

Also:

- /api/{db}/query/batch
- /api/{db}/schema
- /api/{db}/logs
- /api/{db}/stats

### Transformation APIs

- /api/transform/rules*
- /api/transform/apply
- /api/transform/batch
- /api/transform/preview
- /api/transform/test/*
- /api/transform/rules/export
- /api/transform/rules/import

### Admin and Platform User APIs

- /api/admin/database/*
- /api/admin/table/*
- /api/admin/metrics/*
- /api/v1/users*

### Notification APIs

- /api/notifications/send
- /api/notifications/health
- /api/notifications/status

### CLI Auth APIs

- /api/v1/auth/login
- /api/v1/auth/verify
- /api/v1/auth/whoami

### Kubernetes-style and Control-plane APIs

- /api/v1/namespaces/:namespace/:kind*
- /api/v1/apis
- /api/v1/policies
- /api/v1/workflows
- /api/v1/workflows/:name/run
- /api/v1/workflows/:name/executions
- /api/v1/workflows/executions/:id
- /api/v1/datasources*

### Extension APIs (new)

- /api/v1/kubeplus/admission/policies
- /api/v1/kubeplus/admission/evaluate
- /api/v1/kubeplus/scheduler/nodes*
- /api/v1/kubeplus/scheduler/score
- /api/v1/kubeplus/scheduler/pick
- /api/v1/kubeplus/crd/definitions*
- /api/v1/kubeplus/crd/validate
- /api/v1/vectorplus/records*
- /api/v1/vectorplus/search
- /api/v1/vectorplus/similarity
- /api/v1/reviewflow/items*
- /api/v1/reviewflow/score
- /api/v1/reviewflow/quality
- /api/v1/jobs*
- /api/v1/jobs/schedules
- /api/v1/jobs/:id/schedule

### Platform Service APIs

- /api/v1/bulk/operations*
- /api/v1/eventbus*
- /api/v1/eventbus/events/:id/ack
- /api/v1/eventbus/dlq/:id/replay
- /api/v1/exports*
- /api/v1/export-templates*
- /api/v1/webhooks*
- /api/v1/streams*
- /api/v1/streaming/subscriptions*
- /api/v1/tenants*
- /api/v1/rbac*
- /api/v1/rbac/access-requests*
- /api/v1/rbac/access-requests/:id/approve
- /api/v1/rbac/access-requests/:id/reject
- /api/v1/versioning*
- /api/v1/lineage*
- /api/v1/tracing*
- /api/v1/tracing/traces
- /api/v1/tracing/spans
- /api/v1/tracing/ingestion/audit

### Conductor Messaging APIs

- /api/v1/conductor/producers*
- /api/v1/conductor/consumers*
- /api/v1/conductor/publish
- /api/v1/conductor/messages
- /api/v1/conductor/dlq*
- /api/v1/conductor/connections*
- /api/v1/conductor/stats
- /api/v1/conductor/stream
- /ws/conductor

### IAM and OIDC APIs

- /.well-known/openid-configuration
- /.well-known/jwks.json
- /realms/:realm/.well-known/openid-configuration
- /realms/:realm/protocol/openid-connect/*
- /oauth/authorize
- /oauth/token
- /iam/auth/*
- /iam/admin/*
- /iam/v2/*

### Object Storage APIs

- /api/v1/storage/health
- /api/v1/storage/stats
- /api/v1/storage/events*
- /api/v1/storage/buckets*
- /api/v1/storage/buckets/:bucket/objects*
- /api/v1/storage/buckets/:bucket/presign
- /api/v1/storage/buckets/:bucket/shares*
- /api/v1/storage/policies*
- /api/v1/storage/access-keys*
- /api/v1/storage/system/metrics*

### Runtime Custom API Execution

- /api/custom
- /api/custom/*path

### Data Platform and Specialized APIs

- /api/v1/gis*
- /api/v1/gis/dashboards*
- /api/v1/analytics*
- /api/v1/etl*
- /api/v1/cdc*
- /api/v1/data-platform/overview
- /api/v1/builder*
- /api/v1/netintel*

API Builder details (implemented routes):

- /api/v1/builder/summary
- /api/v1/builder/apis*
- /api/v1/builder/csv/upload
- /api/v1/builder/csv/uploads*
- /api/v1/builder/csv/uploads/:id/generate-dashboard
- /api/v1/builder/csv/uploads/:id/generate-gis
- /api/v1/builder/convert/analyze
- /api/v1/builder/convert/dashboard-to-gis
- /api/v1/builder/convert/gis-to-dashboard
- /api/v1/builder/conversions
- /api/v1/builder/scanner/scan
- /api/v1/builder/scanner/scans
- /api/v1/builder/scanner/health
- /api/v1/builder/api-scanner/scan
- /api/v1/builder/api-scanner/reports
- /api/v1/builder/api-scanner/reports/:id
- /api/v1/builder/api-scanner/reports/bulk-delete
- /api/v1/builder/sql-assistant/chat
- /api/v1/builder/dashboards/:id

## GraphQL Coverage

Endpoints:

- POST /api/graphql
- GET /api/graphql/schema
- GET /api/graphql/playground

Implementation summary:

- GraphQL handler uses SQL backend preference order with PostgreSQL first fallback chain.
- Access is protected by auth middleware.

## Dashboard and UI Coverage

Frontend routes:

- /
- /signup
- /login
- /admin
- /system-manager
- /manager
- /gis
- /analytics
- /cdc-etl
- /netintel
- /conductor
- /governance
- /operations-center
- /lineage-version
- /iam-admin
- /object-storage

Frontend role normalization handles aliases such as sysadmin to system-manager.

## GIS Coverage

GIS API groups:

- /api/v1/gis/summary
- /api/v1/gis/layers*
- /api/v1/gis/regions*
- /api/v1/gis/markers*
- /api/v1/gis/datasets*

Specialized GIS dashboards:

- /api/v1/gis/dashboards
- /api/v1/gis/dashboards/:type
- /api/v1/gis/dashboards/:type/summary

## Network Intelligence (NetIntel) Coverage

NetIntel API group:

- /api/v1/netintel/summary
- /api/v1/netintel/observability
- /api/v1/netintel/log-types
- /api/v1/netintel/parsers*
- /api/v1/netintel/logs*
- /api/v1/netintel/topology*
- /api/v1/netintel/heatmap
- /api/v1/netintel/trends
- /api/v1/netintel/predictions
- /api/v1/netintel/tracks*
- /api/v1/netintel/anomalies*
- /api/v1/netintel/alerts*
- /api/v1/netintel/forecasts*
- /api/v1/netintel/modes
- /api/v1/netintel/modes/:name
- /api/v1/netintel/modes/events
- /api/v1/netintel/modes/:name/events
- /api/v1/netintel/modes/detect

## CLI Full Command Reference

Verified top-level commands:

- alerts
- api
- apibank
- bulk
- catalog
- cert
- completion
- compliance
- config
- current-user
- datasource
- diff
- discover
- eventbus
- events
- exportx
- health
- incidents
- job
- lineage
- lineagex
- login
- logout
- mesh
- metrics
- policy
- quality
- rbacx
- scan
- status
- stream
- tenant
- trace
- version
- versioning
- wait
- webhook
- workflow

### Complete Command Tree

```text
axiomnizamctl
├─ login [server-url]
├─ logout
├─ current-user
├─ api
│  ├─ create
│  ├─ list
│  ├─ get [name]
│  ├─ update [name]
│  ├─ delete [name]
│  ├─ apply -f [file]
│  ├─ describe [name]
│  └─ diff -f [file]
├─ apibank
│  ├─ create
│  ├─ list
│  ├─ get [bank-name]
│  ├─ add-api [bank-name]
│  └─ search
├─ policy
│  ├─ apply -f [file]
│  ├─ list
│  ├─ get [name]
│  ├─ delete [name]
│  ├─ describe [name]
│  └─ diff -f [file]
├─ workflow
│  ├─ apply -f [file]
│  ├─ list
│  ├─ run [name]
│  ├─ status [name|execution-id]
│  ├─ describe [name]
│  └─ diff -f [file]
├─ datasource
│  ├─ create
│  ├─ list
│  ├─ test [name]
│  ├─ apply -f [file]
│  ├─ delete [name]
│  ├─ describe [name]
│  ├─ diff -f [file]
│  ├─ get [name]
│  └─ update [name]
├─ job
│  ├─ create
│  ├─ list
│  ├─ get [job-id]
│  ├─ run [name]
│  ├─ delete [name]
│  ├─ logs [job-id]
│  ├─ cancel [job-id]
│  ├─ describe [job-id]
│  ├─ diff -f [file]
│  ├─ status [job-id]
│  ├─ schedule-list
│  ├─ schedule-set [job-id]
│  └─ schedule-remove [job-id]
├─ mesh
│  ├─ list
│  ├─ status
│  ├─ domain
│  │  ├─ create
│  │  ├─ list
│  │  └─ get [domain-name]
│  ├─ product
│  │  ├─ create
│  │  ├─ list
│  │  └─ get
│  ├─ subscribe
│  └─ lineage
├─ tenant
│  ├─ list
│  ├─ get [id]
│  └─ create
├─ rbacx
│  ├─ roles
│  ├─ create-role
│  ├─ check
│  ├─ access-request-list
│  ├─ access-request-create
│  ├─ access-request-approve [request-id]
│  └─ access-request-reject [request-id]
├─ eventbus
│  ├─ ack [event-id]
│  └─ dlq-replay [dlq-id]
├─ webhook
│  ├─ list
│  ├─ create
│  └─ test [id]
├─ stream
│  ├─ list
│  ├─ create
│  └─ cancel [id]
├─ exportx
│  ├─ list
│  ├─ create
│  └─ progress [id]
├─ bulk
│  ├─ list
│  ├─ submit
│  └─ progress [id]
├─ versioning
│  ├─ history [resource-type] [resource-id]
│  ├─ diff [resource-type] [resource-id]
│  └─ rollback [resource-type] [resource-id]
├─ trace
│  ├─ search
│  ├─ get [trace-id]
│  ├─ ingest
│  └─ ingestion-audit-list
├─ lineagex
│  ├─ graph [resource-type] [resource-id]
│  └─ impact [resource-type] [resource-id]
├─ incidents
│  └─ overview
├─ health
│  └─ check
├─ alerts
│  ├─ check
│  └─ list
├─ metrics
│  └─ collect
├─ catalog
│  ├─ search [query]
│  └─ list
├─ compliance
│  ├─ check
│  ├─ report
│  └─ audit
├─ quality
│  ├─ analyze
│  └─ check
├─ lineage
│  └─ trace [resource]
├─ config
│  ├─ view
│  ├─ current-context
│  ├─ use-context [context-name]
│  ├─ get-clusters
│  ├─ set-context [context-name] --cluster=[cluster] --user=[user]
│  ├─ set-cluster [cluster-name] --server=[server-url]
│  ├─ delete-context [context-name]
│  └─ rename-context [old-name] [new-name]
├─ status
├─ events
│  ├─ get [resource-kind] [resource-name]
│  └─ list
├─ diff
│  └─ diff -f resource.yaml
├─ cert
│  ├─ status
│  └─ renew
├─ discover
│  ├─ api URL
│  └─ domain DOMAIN
├─ scan
│  ├─ api URL
│  ├─ graphql URL
│  ├─ image IMAGE
│  ├─ fs PATH
│  ├─ k8s PATH
│  └─ repo PATH
├─ wait
│  ├─ tcp ADDRESS [ADDRESS...]
│  ├─ dns RECORD_TYPE ADDRESS
│  ├─ http URL
│  ├─ grpc-health TARGET
│  ├─ k8s-pod
│  ├─ mysql DSN
│  ├─ postgresql DSN
│  ├─ mongodb URI
│  ├─ redis ADDRESS
│  ├─ rabbitmq URL
│  ├─ kafka BROKER [BROKER...]
│  ├─ influxdb URL
│  ├─ temporal TARGET
│  ├─ custom COMMAND [ARGS...]
│  └─ external WAIT4X_ARGS...
├─ completion [bash|zsh|fish|powershell]
└─ version
```

### Global CLI Flags

- --kubeconfig
- --context
- --namespace
- --output
- --verbose
- --dry-run
- --help
- --version

### CLI Execution Modes

CLI commands in this repository are a mix of:

- API-backed commands: call backend REST endpoints (for example api, policy, workflow, datasource, job, tenant, rbacx, eventbus, webhook, stream, exportx, bulk, versioning, trace, lineagex).
- Local integration commands: run local integration analyzers without requiring backend routes for every operation (for example health, alerts, metrics, catalog, compliance, quality, lineage, and parts of mesh behavior).
- Utility commands: operational helpers and scanners (for example cert, discover, scan, wait, completion, version).

This split is intentional in the current codebase.

## Internal Module Coverage

Internal scan snapshot (2026-06-01):

- Module folders under internal/: 111
- Go files under internal/: 987
- Go lines under internal/: 230315

Largest modules by Go lines:

- utils (36 files, 13215 lines)
- kubeplus (6 files, 12624 lines)
- antivirus (40 files, 12049 lines)
- iam (42 files, 11996 lines)
- platform (40 files, 11437 lines)
- gatekeeper (98 files, 9358 lines)
- storage (29 files, 9143 lines)
- jobs (35 files, 8811 lines)
- netintel (16 files, 7524 lines)
- policies (16 files, 6222 lines)
- apibuilder (18 files, 5964 lines)
- scanner (21 files, 5456 lines)
- vectorplus (2 files, 5100 lines)
- reviewflow (2 files, 4870 lines)
- apimachinery (29 files, 4577 lines)
- integration (14 files, 3825 lines)

Full internal module inventory (alphabetical):

| Module | Go Files | Go Lines |
|---|---:|---:|
| admission | 3 | 451 |
| alerting | 7 | 2113 |
| analytics | 1 | 161 |
| anonymization | 7 | 859 |
| antivirus | 40 | 12049 |
| apibanks | 10 | 1167 |
| apibuilder | 18 | 5964 |
| apimachinery | 29 | 4577 |
| apiscanner | 10 | 2571 |
| apiserver | 9 | 1598 |
| audit | 11 | 1161 |
| auth | 4 | 1108 |
| autopilot | 1 | 164 |
| blocking | 1 | 136 |
| bootstrapsecrets | 1 | 102 |
| bulk | 9 | 913 |
| cache | 9 | 1969 |
| catalog | 8 | 1843 |
| cdc | 12 | 2595 |
| client | 8 | 1570 |
| conductor | 18 | 3027 |
| config | 1 | 289 |
| contracts | 8 | 1113 |
| controller | 14 | 1561 |
| controllers | 7 | 2521 |
| costing | 7 | 883 |
| database | 3 | 1188 |
| datasource | 5 | 634 |
| deployment | 1 | 249 |
| diff | 1 | 261 |
| distributed | 1 | 194 |
| distributedstate | 9 | 1605 |
| docs | 2 | 408 |
| drainer | 1 | 205 |
| encryption | 10 | 2295 |
| errors | 2 | 264 |
| etl | 10 | 2519 |
| evalbroker | 2 | 263 |
| eventbus | 9 | 1363 |
| events | 6 | 1607 |
| export | 8 | 1077 |
| featurestore | 8 | 1068 |
| federation | 12 | 2244 |
| gatekeeper | 98 | 9358 |
| gis | 8 | 1542 |
| governance | 9 | 2065 |
| graphql | 3 | 418 |
| health | 3 | 641 |
| heartbeat | 1 | 158 |
| iam | 42 | 11996 |
| informer | 2 | 457 |
| integration | 14 | 3825 |
| jobs | 35 | 8811 |
| keyring | 1 | 161 |
| kubeplus | 6 | 12624 |
| lineage | 9 | 1600 |
| logging | 2 | 138 |
| mesh | 1 | 388 |
| metrics | 5 | 1071 |
| migrations | 1 | 137 |
| mlpipeline | 5 | 602 |
| models | 15 | 1279 |
| netintel | 16 | 7524 |
| notification | 5 | 482 |
| observability | 4 | 510 |
| output | 2 | 258 |
| performance | 2 | 479 |
| periodic | 2 | 352 |
| planner | 1 | 220 |
| platform | 40 | 11437 |
| policies | 16 | 6222 |
| quality | 12 | 2870 |
| query | 4 | 2252 |
| ratelimit | 2 | 377 |
| rbac | 8 | 2016 |
| reconciler | 6 | 1677 |
| repositories | 15 | 1664 |
| resources | 16 | 3296 |
| reviewflow | 2 | 4870 |
| rpcpool | 1 | 117 |
| runtime | 1 | 407 |
| scanner | 21 | 5456 |
| scheduler | 1 | 142 |
| schemaregistry | 9 | 2129 |
| security | 4 | 1224 |
| securitysiem | 2 | 446 |
| server | 2 | 1142 |
| serverboot | 1 | 109 |
| serviceregistry | 1 | 235 |
| services | 5 | 1254 |
| slo | 6 | 704 |
| snapshot | 1 | 116 |
| sqlfilter | 10 | 1564 |
| status | 1 | 384 |
| storage | 29 | 9143 |
| stream | 2 | 288 |
| streamanalytics | 8 | 1168 |
| streaming | 8 | 660 |
| template | 1 | 190 |
| tenant | 9 | 1007 |
| testutil | 2 | 157 |
| tracing | 10 | 1707 |
| transform | 3 | 501 |
| trivy | 7 | 797 |
| utils | 36 | 13215 |
| vectorplus | 2 | 5100 |
| versioning | 11 | 1362 |
| waitx | 20 | 2839 |
| webhooks | 9 | 777 |
| workflows | 2 | 908 |
| workqueue | 5 | 1335 |

## Frontend Template Coverage

Implemented frontend template pages and scripts include:

- Public and auth flows: dashboard, auth.
- Role views: admin, manager, system-manager.
- Domain dashboards: gis-dashboard, analytics-dashboard, cdc-etl-dashboard, netintel-dashboard, conductor-dashboard.
- Governance and operations: governance-dashboard, operations-center, version-lineage-dashboard, iam-admin, object-storage.
- Shared layout and styling: layout, responsive, platform-console styles.

Template files are present in frontend/templates and the frontend server routes expose the primary pages at runtime.

## Configuration and Environment Variables

Key vars used by current code paths:

- API_HOST
- API_PORT
- KEYCLOAK_HOST
- KEYCLOAK_PORT
- KEYCLOAK_REALM
- KEYCLOAK_CLIENT_ID
- KEYCLOAK_CLIENT_SECRET
- KEYCLOAK_ONLY_AUTH
- KEYCLOAK_USER_SYNC_ENABLED
- KEYCLOAK_ADMIN_REALM
- KEYCLOAK_ADMIN_CLIENT_ID
- KEYCLOAK_ADMIN_USERNAME
- KEYCLOAK_ADMIN_PASSWORD
- RATE_LIMIT_MAX_CALLS
- RATE_LIMIT_VALIDITY_MINUTES
- ETCD_HOST
- ETCD_PORT
- VALKEY_HOST
- VALKEY_PORT
- SAFEGATE_MAX_FILE_SIZE

### Reconciler Migration Flags

These flags control the Kubernetes-style reconcile loop migration.
All migration code phases are complete — these flags gate runtime activation.

Shadow mode (default: true — reconcilers run but don't affect production):

- RECONCILER_SHADOW_MODE (default: true)

Dual-write flags (handlers write to etcd AND call managers):

- DUAL_WRITE_VERSIONING
- DUAL_WRITE_AUDIT
- DUAL_WRITE_TRACING
- DUAL_WRITE_LINEAGE
- DUAL_WRITE_ENCRYPTION
- DUAL_WRITE_RBAC
- DUAL_WRITE_WEBHOOKS
- DUAL_WRITE_TENANT
- DUAL_WRITE_STREAMING
- DUAL_WRITE_EVENTBUS
- DUAL_WRITE_EXPORT
- DUAL_WRITE_BULK
- DUAL_WRITE_CONDUCTOR

Reconciler-authoritative flags (handlers write to etcd only, reconciler drives manager):

- RECONCILER_AUTHORITATIVE_VERSIONING
- RECONCILER_AUTHORITATIVE_AUDIT
- RECONCILER_AUTHORITATIVE_TRACING
- RECONCILER_AUTHORITATIVE_ENCRYPTION
- RECONCILER_AUTHORITATIVE_RBAC
- RECONCILER_AUTHORITATIVE_WEBHOOKS
- RECONCILER_AUTHORITATIVE_TENANT
- RECONCILER_AUTHORITATIVE_EVENTBUS
- RECONCILER_AUTHORITATIVE_EXPORT
- RECONCILER_AUTHORITATIVE_BULK
- RECONCILER_AUTHORITATIVE_CONDUCTOR

Activation order: shadow mode → dual-write → authoritative, one module at a time,
48 hours between each. Rollback: set any flag to false (instant, no deploy).

### Reconciler Health Endpoint

`GET /health/reconcilers` (no auth) returns:

```json
{
  "summary": { "status": "ok", "total": 18, "running": 17, "healthy": 17 },
  "reconcilers": [{ "module": "bulk", "running": true, "totalReconciles": 0 }, ...],
  "etcdKeySpace": [{ "prefix": "/axiomnizam/bulkoperations/", "keyCount": 0 }, ...]
}
```

Security recommendation:

- Rotate default credentials and secrets before production.
- Avoid committing real webhook URLs and admin credentials.

## Project Structure

Important folders:

- cmd/axiomnizam-server: backend server entrypoint
- cmd/axiomnizamctl: CLI implementation
- internal/auth: token validation and role checks
- internal/config: environment loading
- internal/database: connection initialization
- internal/handlers: API handlers and feature modules
- internal/apiserver: generic resource API server and extension route wiring
- internal/runtime: control-plane runtime
- internal/autopilot, internal/planner, internal/scheduler, internal/deployment, internal/drainer, internal/evalbroker, internal/heartbeat, internal/periodic, internal/serviceregistry, internal/snapshot: autonomous orchestration and dispatch primitives
- internal/apimachinery, internal/controller, internal/informer: API machinery, controller manager, and shared informer/cache primitives
- internal/* platform modules: bulk, eventbus, export, tenant, rbac, tracing, lineage, versioning, streaming, webhooks
- internal/kubeplus: admission, scheduler, and CRD extension modules
- internal/netintel/modes: mode manager and detectors
- internal/vectorplus: vector index and similarity metrics
- internal/reviewflow: staged review pipeline and quality checks
- internal/scripts: deterministic code generation helpers
- frontend/templates: dashboard pages and scripts
- examples: sample YAML and Postman collections

## Appendix: Feature-to-File Evidence Matrix

This appendix maps implemented features to concrete source evidence (module to route/command/template source path).

### Backend Feature Evidence

| Feature | Route Wiring Evidence | Module/Handler Evidence |
|---|---|---|
| Auth login and token lifecycle | main.go route registration for /auth/* | internal/handlers/auth_handler.go |
| GraphQL | main.go route registration for /api/graphql* | internal/handlers/graphql_handler.go |
| Health/status/distributed | main.go route registration for /health, /status, /distributed | internal/handlers/handlers.go |
| Dynamic SQL queries and stats | main.go route registration for /api/{db}/query, /schema, /logs, /stats | internal/handlers/dynamic_query_handler.go |
| Transformation rules and execution | main.go route registration for /api/transform/* | internal/handlers/transformation_handler.go |
| Control-plane resources | main.go route registration for /api/v1/namespaces/* and /api/v1/{apis,policies,workflows} | internal/handlers/resource_handler.go |
| Data source and job orchestration | main.go route registration for /api/v1/datasources* and /api/v1/jobs* | internal/handlers/datasource_handler.go, internal/handlers/job_handler.go |
| Platform services (bulk/eventbus/export/webhook/stream/tenant/rbac/versioning/lineage/tracing) | main.go route groups under /api/v1/* | internal/bulk/, internal/eventbus/, internal/export/, internal/webhooks/, internal/streaming/, internal/tenant/, internal/rbac/, internal/versioning/, internal/lineage/, internal/tracing/ |
| Conductor | main.go + internal/conductor route registration for /api/v1/conductor* and /ws/conductor | internal/conductor/ |
| GIS and analytics dashboards | main.go route groups /api/v1/gis*, /api/v1/analytics* | internal/handlers/gis_handler.go, internal/handlers/analytics_handler.go |
| ETL and CDC platform | main.go route groups /api/v1/etl*, /api/v1/cdc*, /api/v1/data-platform/overview | internal/handlers/cdc_etl_handler.go, internal/etl/, internal/cdc/ |
| API Builder runtime and scanning | main.go route group /api/v1/builder* and runtime /api/custom* | internal/handlers/api_builder_handler.go, internal/scanner/, internal/apiscanner/ |
| NetIntel and mode manager | main.go route group /api/v1/netintel* | internal/handlers/netintel_handler.go, internal/netintel/ |
| Kubeplus admission/scheduler/CRD | main.go route group /api/v1/kubeplus* | internal/kubeplus/ |
| Vector similarity and reviewflow | main.go route groups /api/v1/vectorplus* and /api/v1/reviewflow* | internal/vectorplus/, internal/reviewflow/ |
| IAM and OIDC | main.go IAM registration block and /iam* routes | internal/iam/ |
| Native object storage | main.go storage registration block under /api/v1/storage* | internal/storage/ |

### CLI Command Evidence

| CLI Area | Command Source Path |
|---|---|
| Root command | cmd/axiomnizamctl/root.go:22 |
| Login/logout/current-user | cmd/axiomnizamctl/auth.go:24 |
| API commands | cmd/axiomnizamctl/api.go:15 |
| API bank commands | cmd/axiomnizamctl/apibank.go:13 |
| Policy/workflow commands | cmd/axiomnizamctl/policy_workflow.go:15 |
| Datasource/job commands | cmd/axiomnizamctl/datasource_job.go:15 |
| Mesh commands | cmd/axiomnizamctl/mesh.go:18 |
| Tenant/RBAC/webhook/stream/export/bulk/versioning/trace/lineagex/incidents | cmd/axiomnizamctl/platform_commands.go:11 |
| Health/alerts/metrics/catalog/compliance/quality/lineage integration commands | cmd/axiomnizamctl/integration.go:17 |
| Config commands | cmd/axiomnizamctl/config.go:19 |
| Status command | cmd/axiomnizamctl/commands.go:9 |
| Events command | cmd/axiomnizamctl/events.go:10 |
| Diff command | cmd/axiomnizamctl/diff.go:14 |
| Completion command | cmd/axiomnizamctl/completion.go:9 |

### Frontend Route and Template Evidence

| Frontend Route Evidence | Template/Asset Evidence |
|---|---|
| frontend/main.go:125 (/) | frontend/templates/public-dashboard.html, frontend/templates/dashboard.js |
| frontend/main.go:126 (/admin) | frontend/templates/admin.html, frontend/templates/admin.js |
| frontend/main.go:127 (/system-manager) | frontend/templates/system-manager.html, frontend/templates/system-manager.js |
| frontend/main.go:128 (/manager) | frontend/templates/manager.html, frontend/templates/manager.js |
| frontend/main.go:129 (/gis) | frontend/templates/gis-dashboard.html, frontend/templates/gis-dashboard.js |
| frontend/main.go:130 (/analytics) | frontend/templates/analytics-dashboard.html, frontend/templates/analytics-dashboard.js |
| frontend/main.go:131 (/cdc-etl) | frontend/templates/cdc-etl-dashboard.html, frontend/templates/cdc-etl-dashboard.js |
| frontend/main.go:132 (/netintel) | frontend/templates/netintel-dashboard.html, frontend/templates/netintel-dashboard.js |
| frontend/main.go:133 (/governance) | frontend/templates/governance-dashboard.html, frontend/templates/governance-dashboard.js |
| frontend/main.go:134 (/operations-center) | frontend/templates/operations-center.html, frontend/templates/operations-center.js |
| frontend/main.go:135 (/lineage-version) | frontend/templates/version-lineage-dashboard.html, frontend/templates/version-lineage-dashboard.js |

## Troubleshooting

1. Login succeeds but privileged APIs return forbidden
- Confirm Keycloak roles exist and are assigned.
- Ensure token carries role in realm_access or resource_access.

2. Platform user created but cannot login
- Confirm KEYCLOAK_USER_SYNC_ENABLED is true.
- Validate Keycloak admin sync credentials and realm settings.

3. 401 on protected endpoints
- Check Authorization header format Bearer <token>.
- Check token status and rate-limit window.

4. Strict mode migration issues
- With KEYCLOAK_ONLY_AUTH=true, legacy demo/local fallback credentials are expected to fail.

5. NetIntel or dashboard write actions denied
- Verify caller role is admin or system-manager for write operations.

## License

See LICENSE.
