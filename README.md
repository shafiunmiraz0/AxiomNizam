<p align="center">
  <img src="axiomnizam-logo-minimal.svg" alt="AxiomNizam" width="140" />
</p>

<h1 align="center">AxiomNizam</h1>

<p align="center">
  <strong>Enterprise Data Control Plane</strong>
</p>

<p align="center">
  <a href="https://github.com/shafiunmiraz0/AxiomNizam/actions"><img alt="Build" src="https://img.shields.io/badge/build-passing-brightgreen" /></a>
  <a href="https://go.dev/"><img alt="Go" src="https://img.shields.io/badge/go-1.25-blue" /></a>
  <a href="https://github.com/shafiunmiraz0/AxiomNizam/blob/main/LICENSE"><img alt="License" src="https://img.shields.io/badge/license-proprietary-red" /></a>
  <a href="https://shafiunmiraz0.github.io/AxiomNizam/charts"><img alt="Helm" src="https://img.shields.io/badge/helm-v0.1.0-blueviolet" /></a>
</p>

---

AxiomNizam is a self-hosted data control plane built in Go. It provides IAM, object storage, file scanning, ETL/CDC pipelines, API management, and zero-trust security — all in a single binary with embedded Raft consensus.

## Key Features

- **IAM & Zero Trust** — JWT auth, RBAC, risk-based MFA (TOTP), session revocation, continuous verification, field-level encryption
- **Object Storage** — S3-compatible native storage with SafeGate 6-stage file scanner (metadata, MIME, SVG XSS, macros, archive bombs, native AV)
- **Data Platform** — API Builder, CSV upload, dashboard generation, GIS conversion, SQL assistant (OpenClaw/Ollama)
- **ETL/CDC Pipelines** — Declarative pipeline orchestration with K8s-style reconcilers
- **Gatekeeper 2FA** — TOTP enrollment, challenge/verify flow, trusted devices, backup codes, adaptive risk scoring
- **Embedded Raft** — Single-binary deployment with HashiCorp Raft consensus, no external etcd required
- **116 Internal Modules** — Catalog, quality, schema registry, governance, federation, feature store, ML pipelines, and more

## Codebase

| Metric | Count |
|--------|-------|
| Total code files | 1,159 |
| Total code lines | 307,541 |
| Go files | 1,073 |
| Go lines | 252,936 |
| Internal modules | 116 |
| Internal Go files | 1,022 |
| Internal Go lines | 238,100 |

## Quick Start

### Helm (recommended for K8s)

```bash
helm repo add axiomnizam https://shafiunmiraz0.github.io/AxiomNizam/charts
helm repo update

helm install axiomnizam axiomnizam/axiomnizam \
  -n axiomnizam --create-namespace \
  --set postgresql.password=<password> \
  --set iam.sysadminPassword=<password>
```

### Docker Compose

```bash
docker compose up -d --build
```

| Service | URL |
|---------|-----|
| API + Frontend | https://localhost:8000 |
| Keycloak | http://localhost:8080 |

### From Source

```bash
# Build
go build -o axiomnizam .

# Run (Raft mode, no etcd needed)
STORAGE_BACKEND=raft ./axiomnizam
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│                   Clients                        │
│         (Browser, CLI, API Consumers)            │
└──────────────────────┬──────────────────────────┘
                       │
┌──────────────────────▼──────────────────────────┐
│              Gin HTTP Server (:8000)              │
│  ┌─────────┐ ┌─────────┐ ┌────────┐ ┌────────┐ │
│  │  Auth   │ │  RBAC   │ │ Policy │ │  Rate  │ │
│  │  JWT    │ │  Engine │ │ Engine │ │ Limit  │ │
│  └────┬────┘ └────┬────┘ └───┬────┘ └────────┘ │
│       └───────────┼──────────┘                  │
│                   ▼                              │
│  ┌──────────────────────────────────────────┐   │
│  │           API Handlers (111 modules)      │   │
│  │  IAM · Storage · Jobs · CDC · ETL · GIS  │   │
│  │  Builder · Scanner · Gatekeeper · More   │   │
│  └──────────────────┬───────────────────────┘   │
│                     ▼                            │
│  ┌──────────────────────────────────────────┐   │
│  │     K8s-Style Reconcilers (30+)          │   │
│  │   GenericController + WorkQueue          │   │
│  └──────────────────┬───────────────────────┘   │
└─────────────────────┼───────────────────────────┘
                      ▼
┌─────────────────────────────────────────────────┐
│          Storage Backend (pluggable)             │
│   ┌──────────────┐   ┌─────────────────────┐   │
│   │ Embedded Raft │   │ External etcd       │   │
│   │ (default)     │   │ (optional)          │   │
│   └──────────────┘   └─────────────────────┘   │
│                    + PostgreSQL                  │
└─────────────────────────────────────────────────┘
```

## Configuration

Core environment variables (see [docs/HELM_VALUES_REFERENCE.md](docs/HELM_VALUES_REFERENCE.md) for full list):

| Variable | Default | Description |
|----------|---------|-------------|
| `API_PORT` | `8000` | Server port |
| `STORAGE_BACKEND` | `raft` | `raft` (embedded) or `etcd` |
| `TLS_ENABLED` | `true` | Enable HTTPS |
| `TLS_AUTO_GENERATE` | `true` | Auto-generate self-signed certs |
| `POSTGRES_HOST` | `localhost` | PostgreSQL host |
| `POSTGRES_PASSWORD` | — | **Required** |
| `IAM_SYSADMIN_EMAIL` | — | **Required** bootstrap admin email |
| `IAM_SYSADMIN_PASSWORD` | — | **Required** bootstrap admin password |

All config is via environment variables or `.env` file. See `.env.example` for the full list.

## Project Structure

```
├── main.go                          # Server entry point
├── cmd/
│   ├── axiomnizam-server/           # Server CLI
│   └── axiomnizamctl/               # CLI tool
├── internal/
│   ├── iam/                         # Identity & Access Management
│   ├── gatekeeper/                  # 2FA (TOTP, risk engine, challenges)
│   ├── storage/                     # Object storage + SafeGate scanner
│   ├── scanner/                     # 6-stage file scanning pipeline
│   ├── antivirus/                   # Native AV engine
│   ├── apibuilder/                  # API Builder + SQL assistant
│   ├── frontend/                    # HTML/JS/CSS dashboards
│   ├── platform/                    # Raft, controllers, store, GC
│   ├── encryption/                  # AES-256-GCM, auto-encrypt, KMS
│   ├── rbac/                        # Role-based access control
│   ├── auth/                        # JWT validation
│   ├── apigateway/                  # Rate limiting, API keys, validation
│   ├── securitymon/                 # Anomaly detection, threat response
│   └── ...                          # 111 modules total
├── helm/axiomnizam/                 # Helm chart
├── docs/                            # Documentation
├── Dockerfile                       # Multi-stage build
└── docker-compose.yml               # Local dev stack
```

## Security

AxiomNizam implements zero-trust architecture across 19 phases:

- RSA-256 JWT signatures with JTI revocation
- Risk-based authentication with IP/device fingerprinting
- TOTP MFA with step-up challenges on sensitive operations
- RBAC + policy engine with IP/time conditions
- TLS everywhere (auto-generated or cert-manager)
- Pre-commit file scanning (malicious objects never written)
- Field-level AES-256-GCM encryption via struct tags
- Hash-chain sealed audit logs in PostgreSQL
- K8s NetworkPolicies for micro-segmentation

See [docs/ZERO_TRUST_ARCHITECTURE.md](docs/ZERO_TRUST_ARCHITECTURE.md) for the full security model.

## Documentation

| Document | Description |
|----------|-------------|
| [HELM_RELEASE_GUIDE.md](docs/HELM_RELEASE_GUIDE.md) | How to package and publish the Helm chart |
| [HELM_VALUES_REFERENCE.md](docs/HELM_VALUES_REFERENCE.md) | All configurable Helm values |
| [ZERO_TRUST_ARCHITECTURE.md](docs/ZERO_TRUST_ARCHITECTURE.md) | Security model and implementation |
| [RAFT_STORAGE_GUIDE.md](docs/RAFT_STORAGE_GUIDE.md) | Embedded Raft storage operations |
| [SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) | Security audit findings |
| [MODULE_ENRICHMENT_PLAN.md](docs/MODULE_ENRICHMENT_PLAN.md) | Module standardization plan |
| [CODING_PRACTICES.md](docs/CODING_PRACTICES.md) | Code standards reference |

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-change`)
3. Commit your changes (`git commit -m "Add my feature"`)
4. Push to the branch (`git push origin feature/my-change`)
5. Open a Pull Request

## License

Proprietary. See [LICENSE](LICENSE) for details.
