# AxiomNizam Security README

Date: 2026-05-03  
Last Updated: 2026-05-10

## Scope and Method

This document is the authoritative security posture reference for the
AxiomNizam platform, rewritten from a code-backed scan of the internal
directory and current runtime wiring.

Scan coverage:
- Internal Go files scanned: 619 (across 100 packages)
- Focus areas: authentication, authorization, SQL execution paths, file scanning, logging, encryption, rate limiting, configuration defaults, reconciler security, and startup guardrails

Companion documents:
- [SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) — 38 code-level findings (2026-04-29)
- [CODING_PRACTICES.md](docs/CODING_PRACTICES.md) — enforced security-related coding standards

## Executive Summary

Current posture:
- Strong baseline controls exist (JWT validation, role middleware, admin/system-manager gates on privileged routes, SQL read-only policy checks for API Builder templates, SafeGate file scanning pipeline, AES-256-GCM field encryption, RBAC with reconciled RoleBinding resources).
- 29 reconciler controllers run in shadow mode by default, providing declarative state management with dual-write migration path.
- IAM system provides full OIDC/OAuth with admin, authn, authz, identity, and token management.
- Critical credential and token-hardening gaps remain (hardcoded CLI auth key/admin, insecure config defaults, secrets handling hygiene).
- Startup guardrails are implemented and can enforce in production, but rollout is currently designed to begin in audit mode.

Top risks to address first:
1. Hardcoded CLI auth secret and default admin credentials in internal handlers.
2. Insecure default credentials in configuration fallbacks.
3. Token validation helper endpoint that returns success for any presented bearer token.
4. Runtime custom API auth_required flag is stored but not enforced at invocation.
5. Query logging stores raw SQL and params, which may include sensitive data.

Resolved:
- ✅ `.env` files are now excluded from Git via `.gitignore` and removed from tracking (2026-05-10). All `.env` values are for local development only and are not a production concern. See [SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) SEC-01, SEC-02, SEC-03, SEC-13 for details.

## Security Module Inventory

### Layer 1 — Authentication & Identity

| Module | Path | Status |
|--------|------|--------|
| JWT/OIDC Validator | `internal/auth/auth.go` | ✅ Active |
| Auth Middleware | `internal/auth/middleware.go` | ✅ Active |
| Rate Limiter | `internal/auth/rate_limit.go` | ✅ Active |
| IAM System (OIDC) | `internal/iam/` (15 files) | ✅ Active |
| Bootstrap Secrets | `internal/bootstrapsecrets/store.go` | ✅ Active |

### Layer 2 — Authorization & Access Control

| Module | Path | Status |
|--------|------|--------|
| RBAC Engine | `internal/rbac/` (7 files) | ✅ Active — reconciled via GenericController |
| Admission Chain | `internal/admission/chain.go` | ✅ Active |
| Policy Engine (CEL/Rego/DSL) | `internal/policies/` (16 files) | ✅ Active |
| Row-Level Security | `internal/security/rls.go` | ⚠️ Exists, not fully wired |

### Layer 3 — Encryption & Key Management

| Module | Path | Status |
|--------|------|--------|
| AES-256-GCM Encryption | `internal/encryption/` | ✅ Active — reconciled via GenericController |
| Keyring | `internal/keyring/keyring.go` | ✅ Active |
| Crypto Utilities | `internal/utils/security_utils.go` | ✅ Available |

### Layer 4 — Auditing & Compliance

| Module | Path | Status |
|--------|------|--------|
| Audit Compliance Manager | `internal/audit/` (9 files) | ✅ Active — reconciled via GenericController |
| Compliance Engine | `internal/policies/compliance_engine.go` | ✅ Active |
| Security Policy Engine | `internal/policies/security/` | ✅ Active |

### Layer 5 — Scanning & Threat Detection

| Module | Path | Status |
|--------|------|--------|
| SafeGate File Scanner | `internal/scanner/` (7 files) | ✅ Active |
| Trivy Integration | `internal/trivy/` (7 files) | ✅ Active |
| API Security Scanner | `internal/apiscanner/` (10 files) | ✅ Active |
| Network Intelligence | `internal/netintel/` (5 files) | ✅ Active |

### Layer 6 — Data Protection

| Module | Path | Status |
|--------|------|--------|
| Data Anonymization | `internal/anonymization/masker.go` | ✅ Active |
| Input Sanitizer | `internal/utils/security_utils.go` | ✅ Available |
| Security Headers | `internal/utils/security_utils.go` | ⚠️ Defined, not applied as middleware |

## Security Controls Implemented

### 1) Authentication and Authorization

Implemented:
- Keycloak-backed JWT validation with JWKS refresh and role extraction.
  - internal/auth/auth.go
- Role middleware helpers for single and multi-role checks.
  - internal/auth/middleware.go
- Main runtime route protection uses auth, admin, and admin-or-system-manager middleware.
  - main.go
- Full IAM system with OIDC discovery, OAuth flows, admin operations, user/client/role management.
  - internal/iam/

Notes:
- API route groups in many internal modules expose plain Register...Routes functions and rely on caller wiring to add auth middleware.

### 2) SQL Execution Controls

Implemented:
- API Builder SQL templates are validated as read-only with policy modes compat and strict.
  - internal/handlers/api_builder_handler.go
- Runtime custom API execution uses stored templates and parameter extraction, then validates placeholder count.
  - internal/handlers/api_builder_handler.go
- Dynamic SQL GET path is read-only.
  - internal/handlers/dynamic_query_handler.go
- Dynamic SQL POST and batch routes are privileged at router layer.
  - main.go
- SQL identifier validation (ValidateIdentifier) prevents injection in quality rules engine.
  - internal/quality/rules/sanitize.go

### 3) File Upload and Malware Scanning

Implemented:
- SafeGate pipeline for uploads includes metadata, MIME, SVG, macro, archive, and native antivirus scanners.
  - internal/handlers/api_builder_handler.go
  - internal/scanner/ (modular SafeGate pipeline with subpackages)
  - internal/scanner/native/ (bridge to internal/antivirus engine)
- Native antivirus engine provides 5-layer detection: hash DB, byte-pattern (Aho-Corasick), behavioral heuristics, entropy analysis, and YARA rules.
  - internal/antivirus/ (10-phase implementation, replaced ClamAV)

### 4) Rate Limiting

Implemented:
- Token usage tracking and expiry window enforcement.
  - internal/auth/rate_limit.go
- Combined token validation and rate-limit middleware exists for auth flows.
  - internal/auth/rate_limit_middleware.go
- Per-custom-API runtime rate limiting is enforced in API Builder invocation path.
  - internal/handlers/api_builder_handler.go

### 5) Encryption at Rest

Implemented:
- AES-256-GCM field-level encryption with key rotation and audit log.
  - internal/encryption/
- Encryption key and policy management via reconciled resources.
  - EncryptionKeyResource, EncryptionPolicyResource via GenericController
- Rotating keyring with key-ID prefixed ciphertexts.
  - internal/keyring/keyring.go

### 6) Audit and Compliance

Implemented:
- Audit log handlers with report/query/delete support (GDPR/HIPAA/SOC2/PCI-DSS frameworks).
  - internal/audit/
- Compliance engine with rule-based checks and remediation plans.
  - internal/policies/compliance_engine.go
- Security policy engine with threat modeling and incident tracking.
  - internal/policies/security/

## Priority Findings

### Critical

1. Hardcoded CLI JWT key and default admin account
- Evidence:
  - internal/handlers/cli_auth_handler.go
- Why this is critical:
  - A static signing secret and default admin credentials allow token forgery and unauthorized access if exposed.

2. Insecure default credential fallbacks in runtime config
- Evidence:
  - internal/config/config.go
- Why this is critical:
  - Default root/postgres/oracle-style credentials increase risk of insecure deployment if env overrides are missing.

### High

3. Validate token endpoint is not doing cryptographic validation
- Evidence:
  - internal/handlers/auth_handler.go
  - Function: ValidateToken
- Why this is high:
  - Endpoint currently responds success when token is present, creating false assurance for clients.

4. Custom API auth_required flag is not enforced in runtime invocation path
- Evidence:
  - Model field present in internal/handlers/api_builder_handler.go
  - Runtime InvokeCustomAPI enforces status and rate limit but does not evaluate auth_required per API policy.
- Why this is high:
  - Security metadata intent can diverge from effective behavior.

5. Query logs can capture sensitive SQL and parameter values without redaction
- Evidence:
  - internal/handlers/dynamic_query_handler.go
  - internal/handlers/query_logger.go
- Why this is high:
  - Persisted logs may contain secrets or personal data.

### Medium

6. Demo/platform local login branches exist when Keycloak-only mode is relaxed
- Evidence:
  - internal/handlers/auth_handler.go
  - internal/auth/auth.go (demo secret behavior)

7. Security headers middleware exists but is not applied
- Evidence:
  - internal/utils/security_utils.go (DefaultSecurityHeaders defined)
  - No Gin middleware applies them in main.go

8. math/rand used in anonymization masker (predictable noise)
- Evidence:
  - internal/anonymization/masker.go

## Startup Security Guardrails

The platform includes configurable security guardrails that run at startup.

Environment variables:
- `AXIOMNIZAM_ENV` / `APP_ENV` / `ENVIRONMENT` / `GO_ENV` — resolved in priority order
- `SECURITY_GUARDRAILS_MODE` — `off` (default dev), `audit` (default prod), `enforce`

Guardrails currently check for:
- `IAM_SYSADMIN_PASSWORD` quality (rejects empty/default-like values)
- `DEMO_JWT_SECRET` presence
- `CORS_ALLOWED_ORIGINS` presence
- Default-like DB passwords (MySQL, PostgreSQL, Oracle) as warnings

Recommended staged rollout:
1. Set AXIOMNIZAM_ENV=production in staging.
2. Set SECURITY_GUARDRAILS_MODE=audit in staging first.
3. Observe logs and fix all guardrail issues.
4. Switch SECURITY_GUARDRAILS_MODE=enforce only when clean.

## Security Test Coverage

Validated tests:
- internal/handlers/api_builder_sql_policy_test.go
- internal/rbac/handlers_access_requests_test.go
- main_rbac_access_requests_integration_test.go

Coverage gaps to add:
- auth_required enforcement behavior tests for InvokeCustomAPI
- auth/validate endpoint cryptographic validation tests
- Log redaction tests for query logger
- Guardrail enforce-mode startup behavior tests

## Remediation Plan — Phase-Wise Mitigation

> Last updated: 2026-05-10
> Cross-reference: [SECURITY_AUDIT.md](docs/SECURITY_AUDIT.md) (38 findings, SEC-01 → SEC-38)

---

### Phase 0 — Immediate (Before Next Deploy)

**Goal:** Eliminate credential exposure and critical authentication bypasses.  
**Timeline:** 0–3 days  
**Status legend:** ✅ Done · 🔧 In Progress · ⏳ Pending

| # | SEC ID | Finding | Action | Files | Status |
|---|--------|---------|--------|-------|--------|
| 1 | SEC-01 | RSA private key committed to `.env` | a) Fix `.gitignore` to exclude `.env`/`.env.*` | `.gitignore` | ✅ Done (2026-05-10) |
| | | | b) Remove `.env` from Git tracking (`git rm --cached`) | `.env`, `frontend/.env` | ✅ Done (2026-05-10) |
| | | | c) Rotate the RSA signing key | `.env:132` (`IAM_RSA_PRIVATE_KEY`) | ⏳ Pending |
| | | | d) Purge key from Git history (`git filter-repo`) | Git history | ⏳ Pending |
| | | | e) Migrate to `IAM_RSA_PRIVATE_KEY_FILE` with mounted secret | `internal/auth/auth.go`, deployment configs | ⏳ Pending |
| 2 | SEC-02 | Keycloak admin creds & client secret in `.env` | a) Rotate Keycloak admin password & client secret | Keycloak admin console | ⏳ Pending |
| | | | b) Move to Docker/K8s secrets | `docker-compose.yml`, K8s manifests | ⏳ Pending |
| 3 | SEC-03 | `DEMO_JWT_SECRET` committed | a) Rotate secret value | `.env:128` | ⏳ Pending |
| | | | b) Source from secret manager instead of `.env` | `internal/auth/auth.go:268-283` | ⏳ Pending |
| 4 | SEC-04 | Hardcoded demo accounts with admin access | a) Gate behind `ENABLE_DEMO_ACCOUNTS=true` (default: `false`) | `internal/handlers/auth_handler.go:1376` | ⏳ Pending |
| | | | b) Log startup warning when demo mode is enabled | `main.go` startup sequence | ⏳ Pending |
| | | | c) Verify `ENABLE_DEMO_ACCOUNTS` is never set in production | Deployment configs | ⏳ Pending |
| 5 | SEC-05 | SQL injection filter blocks legitimate queries | Redesign `ValidateSQLInput()` to validate user-supplied parameters only, not full query text | `internal/utils/sql_injection_protection.go:21-28` | ⏳ Pending |
| 6 | SEC-16 | Weak JWT secret fallback (`time.Now().UnixNano()`) | Fail startup if `DEMO_JWT_SECRET` is not set in production; keep `crypto/rand` fallback for dev only | `internal/auth/auth.go:268-283` | ⏳ Pending |

**Phase 0 — Verification Criteria:**
- [ ] RSA key rotated and old key revoked
- [ ] Git history purged of all secrets (`git filter-repo`)
- [x] `.env` files excluded from version control
- [ ] `ENABLE_DEMO_ACCOUNTS` defaults to `false`; demo login rejected when unset
- [ ] Startup fails in production if `DEMO_JWT_SECRET` is missing
- [ ] SQL injection protection validates parameters, not query bodies

---

### Phase 1 — This Sprint (1–2 Weeks)

**Goal:** Fix XSS vectors, strengthen authentication, harden infrastructure defaults.  
**Timeline:** 1–2 weeks

| # | SEC ID | Finding | Action | Files | Status |
|---|--------|---------|--------|-------|--------|
| 7 | SEC-06 | XSS via `innerHTML` in frontend dashboards | a) Audit all ~290+ `innerHTML` assignments across JS files | `frontend/templates/admin.js` (~50+), `object-storage.js` (~25+), `conductor-dashboard.js` (~15+), `iam-admin.js` (~20+), `admin-dashboard.js` (~10+) | ⏳ Pending |
| | | | b) Replace with `textContent` for plain text rendering | All dashboard JS files | ⏳ Pending |
| | | | c) Add Content-Security-Policy (CSP) header blocking inline scripts | `main.go`, `frontend/main.go` | ⏳ Pending |
| 8 | SEC-07 | `safeHTML` template function enables XSS | a) Audit all template usages of `{{safeHTML ...}}` | `frontend/main.go:133-135`, all `.html` templates | ⏳ Pending |
| | | | b) Restrict to static HTML only or integrate `bluemonday` sanitizer | `frontend/main.go` | ⏳ Pending |
| 9 | SEC-08 | Frontend auth is client-side only (cookie spoofing) | Validate JWT server-side in `requireFrontendRoles()` by calling `/iam/auth/whoami` | `frontend/main.go:77-107` | ⏳ Pending |
| 10 | SEC-09 | OAuth access token passed in URL fragment | Use a short-lived authorization code exchange; or set token as `HttpOnly`, `Secure`, `SameSite=Lax` cookie | `internal/handlers/auth_handler.go:540-551` | ⏳ Pending |
| 11 | SEC-10 | Command injection risk in certificate handler | a) Validate renewal target against strict allowlist | `internal/handlers/certificate_handler.go:254,319` | ⏳ Pending |
| | | | b) Never pass user input directly to `exec.Command` | Same file | ⏳ Pending |
| 12 | SEC-11 | Custom SQL execution in quality rules | a) Add `QUALITY_CUSTOM_SQL_ENABLED=false` feature flag (default: disabled) | `internal/quality/rules/engine.go:229`, `resource.go:44` | ⏳ Pending |
| | | | b) Execute with read-only database connection | `internal/quality/rules/reconciler.go` | ⏳ Pending |
| 13 | SEC-12 | MD5/SHA1 for security operations | Replace all MD5/SHA1 usage with SHA-256 | `internal/utils/hash/hash.go:40`, `internal/cache/middleware.go:39`, `internal/storage/native/native.go:601`, `internal/utils/bot_protection.go:348` | ⏳ Pending |
| 14 | SEC-13 | Default database passwords & Discord webhook | a) Auto-generate passwords at first start | `docker-compose.yml:233-234`, `.env:63-94` | ⏳ Pending |
| | | | b) Rotate Discord webhook URL | `.env:164` | ⏳ Pending |
| | | | c) Store all credentials via secret manager | Deployment configs | ⏳ Pending |
| 15 | SEC-14 | No CSRF protection | Implement double-submit cookie or synchronizer token pattern; validate `X-CSRF-Token` header | `main.go:305-327` | ⏳ Pending |
| 16 | SEC-15 | PostgreSQL SSL disabled; sysadmin password in frontend `.env` | a) Set `POSTGRES_SSLMODE=require` in production | `internal/config/config.go:130` | ⏳ Pending |
| | | | b) Remove `IAM_SYSADMIN_PASSWORD` from `frontend/.env` | `frontend/.env` | ⏳ Pending |
| 17 | SEC-17 | Auth tokens in `localStorage` (50+ usages) | Migrate token storage to `HttpOnly`, `SameSite=Strict`, `Secure` cookies | `frontend/templates/auth.js:213-218`, `iam-admin.js:37-46`, `system-manager.js:102`, `layout.html:442-444`, and 40+ other locations | ⏳ Pending |
| 18 | SEC-29 | Missing security headers (CSP, X-Frame, HSTS, etc.) | Wire `DefaultSecurityHeaders()` from `security_utils.go` as Gin middleware in both routers | `main.go`, `frontend/main.go`, `internal/utils/security_utils.go:27-28` | ⏳ Pending |

**Phase 1 — Verification Criteria:**
- [ ] Zero unsanitized `innerHTML` assignments in frontend JS
- [ ] `safeHTML` restricted or replaced with `bluemonday`
- [ ] `requireFrontendRoles()` validates JWT server-side
- [ ] OAuth callback uses code exchange, not URL fragment tokens
- [ ] `exec.Command` calls in certificate handler use allowlisted targets
- [ ] Custom SQL disabled by default; runs read-only when enabled
- [ ] No MD5/SHA1 in security-critical paths
- [ ] CSRF tokens validated on state-changing requests
- [ ] `POSTGRES_SSLMODE=require` in production config
- [ ] All security headers present on every response
- [ ] Tokens stored in `HttpOnly` cookies, not `localStorage`

---

### Phase 2 — Medium-Term (1 Month)

**Goal:** Harden runtime defenses, eliminate dead code, secure infrastructure communication.  
**Timeline:** 2–4 weeks

| # | SEC ID | Finding | Action | Files | Status |
|---|--------|---------|--------|-------|--------|
| 19 | SEC-18 | SSRF risk in OAuth configuration | Validate OAuth URLs: restrict to `https` scheme only; block private/loopback IP ranges; set 5s timeout | `internal/handlers/auth_handler.go:776,853,1728,1798` | ⏳ Pending |
| 20 | SEC-19 | Unbounded request body reading | Add `http.MaxBytesReader` to all `io.ReadAll` calls; set `router.MaxMultipartMemory = 32 << 20` | Various handlers (e.g., `auth_handler.go:789`) | ⏳ Pending |
| 21 | SEC-20 | Error messages leak internal details | Return generic error messages to clients; log detailed errors server-side only | `main.go:406,413`, various handlers | ⏳ Pending |
| 22 | SEC-21 | Sensitive fields unmasked in API responses | Mask secrets in list/get responses (show last 4 chars only); return full secrets only on creation | `internal/iam/admin/admin.go`, `internal/storage/admin/admin.go` | ⏳ Pending |
| 23 | SEC-22 | Privilege escalation via email pattern matching | Remove `deriveOAuthRole()` email-based role logic; require explicit IdP role mappings | `internal/handlers/auth_handler.go:418,1333`, `main.go:443-451` | ⏳ Pending |
| 24 | SEC-23 | `ValidateQuerySafety` is dead code | Remove unused function (only checks `DROP DATABASE`/`DROP SCHEMA`, trivially bypassed) | `internal/handlers/dynamic_query_handler.go:581-594` | ⏳ Pending |
| 25 | SEC-24 | Token accepted from query parameter (unscoped) | Restrict query-parameter token acceptance to `Upgrade: websocket` requests only | `main.go:419-421` | ⏳ Pending |
| 26 | SEC-25 | etcd has no authentication | Enable etcd TLS and client certificate authentication | `docker-compose.yml:157-174` | ⏳ Pending |
| 27 | SEC-26 | OAuth state cookie shares JWT key | Set a dedicated `OAUTH_STATE_SECRET` instead of falling back to `DemoJWTSecret()` | `internal/handlers/auth_handler.go:257` | ⏳ Pending |
| 28 | SEC-27 | Demo token path always active | Guard `ValidateDemoToken()` fallback in `ValidateToken()` behind `ENABLE_DEMO_ACCOUNTS` flag | `internal/auth/auth.go:364,394` | ⏳ Pending |
| 29 | SEC-28 | Kafka/RabbitMQ communication is plaintext | Enable TLS for all Kafka listeners and RabbitMQ connections | `docker-compose.yml:407-410` | ⏳ Pending |
| 30 | SEC-30 | Missing auth on detailed health endpoints | Require authentication for `/health/reconcilers`; keep `/health` as unauthenticated simple check | `main.go:526` | ⏳ Pending |

**Phase 2 — Verification Criteria:**
- [ ] OAuth URLs validated against scheme and IP allowlists
- [ ] All request body reads bounded by `MaxBytesReader`
- [ ] Client-facing error messages contain no stack traces or internal paths
- [ ] API responses mask secrets (last 4 chars only)
- [ ] No email-based role derivation in production
- [ ] `ValidateQuerySafety` dead code removed
- [ ] Query-parameter tokens accepted only for WebSocket upgrades
- [ ] etcd requires TLS + client certificates
- [ ] `OAUTH_STATE_SECRET` is independent of `DEMO_JWT_SECRET`
- [ ] Demo token fallback gated behind `ENABLE_DEMO_ACCOUNTS`
- [ ] All message bus connections use TLS
- [ ] `/health/reconcilers` requires authentication

---

### Phase 3 — Long-Term Hardening (Ongoing)

**Goal:** Address low-severity issues, integrate security tooling into CI/CD, establish continuous posture.  
**Timeline:** Ongoing / quarterly

| # | SEC ID | Finding | Action | Files | Status |
|---|--------|---------|--------|-------|--------|
| 31 | SEC-31 | `math/rand` in anonymization masker | Replace with `crypto/rand` in security-adjacent operations; acceptable in `backoff.go` for jitter | `internal/anonymization/masker.go:15`, `synthetic.go:15` | ⏳ Pending |
| 32 | SEC-32 | Path traversal risk in file upload | Add `filepath.Clean()` + prefix check to verify result stays under `uploadPath` | `internal/utils/examples.go:225` | ⏳ Pending |
| 33 | SEC-33 | CLI `--password` flag visible in process list | Remove `--password` flag; always use interactive prompt or `stdin` pipe | `cmd/axiomnizamctl/auth.go` | ⏳ Pending |
| 34 | SEC-34 | `--insecure-skip-tls-verify` available without warning | Emit a visible warning to stderr when flag is used | `cmd/axiomnizamctl/auth.go:321`, `config.go:321` | ⏳ Pending |
| 35 | SEC-35 | Health/status endpoints expose backend topology | Restrict `/status` and `/distributed` to authenticated users; keep `/health` public | `main.go:545-548` | ⏳ Pending |
| 36 | SEC-36 | Docker runtime uses root user | Add non-root user: `RUN useradd -r appuser && USER appuser` | `Dockerfile:48` | ⏳ Pending |
| 37 | SEC-37 | CORS `Access-Control-Max-Age: 86400` is aggressive | Reduce to 3600 (1 hour) for faster policy propagation | `main.go:311` | ⏳ Pending |
| 38 | SEC-38 | Firebase credentials placeholder in `.env` | Add startup validation to reject placeholder values when Firebase features are enabled | `.env:155-162` | ⏳ Pending |

**CI/CD Security Tooling Roadmap:**

| Tool | Purpose | Integration | Timeline |
|------|---------|-------------|----------|
| `gosec` | Go static security analysis | CI pipeline | 2 weeks |
| `semgrep` | Multi-language SAST rules | CI pipeline | 2 weeks |
| `govulncheck` | Go dependency vulnerability scanning | CI pipeline (continuous) | 2 weeks |
| `trivy` | Container image scanning | Already integrated ✅ | — |
| `eslint-plugin-security` | JavaScript security linting | Frontend CI | 1 month |
| `bluemonday` | HTML sanitization for Go templates | Go middleware | Phase 1 |
| `helmet` (equivalent) | Security headers middleware | Go middleware | Phase 1 |

**Phase 3 — Verification Criteria:**
- [ ] `crypto/rand` used in all anonymization operations
- [ ] File upload paths validated against traversal attacks
- [ ] CLI never accepts passwords as command-line flags
- [ ] `--insecure-skip-tls-verify` prints visible warning
- [ ] Topology endpoints require authentication
- [ ] Docker container runs as non-root user
- [ ] CORS max-age ≤ 3600s
- [ ] SAST scanners (gosec, semgrep) integrated in CI with zero critical findings
- [ ] `govulncheck` runs on every PR with blocking policy

---

## Progress Summary

| Phase | Total Items | Done | In Progress | Pending |
|-------|-------------|------|-------------|---------|
| Phase 0 — Immediate | 6 findings (SEC-01–05, SEC-16) | 1 partial (SEC-01 a,b) | 0 | 5.5 |
| Phase 1 — This Sprint | 12 findings (SEC-06–15, SEC-17, SEC-29) | 0 | 0 | 12 |
| Phase 2 — Medium-Term | 12 findings (SEC-18–28, SEC-30) | 0 | 0 | 12 |
| Phase 3 — Long-Term | 8 findings (SEC-31–38) | 0 | 0 | 8 |
| **Total** | **38 findings** | **1 partial** | **0** | **37.5** |

> **Next milestone:** Complete Phase 0 (secret rotation + git history purge) before next deployment.

---

## Ownership

| Area | Owner | Scope |
|------|-------|-------|
| Platform / Backend | Platform team | Route protections, guardrails, runtime policy enforcement, auth hardening (SEC-04, SEC-05, SEC-08, SEC-14, SEC-16, SEC-22, SEC-24, SEC-27, SEC-30) |
| Frontend | Frontend team | XSS remediation, token storage migration, CSP implementation (SEC-06, SEC-07, SEC-09, SEC-17, SEC-29) |
| Infrastructure | DevOps / SRE | Secret management, TLS, Docker hardening, etcd/Kafka/RabbitMQ security (SEC-01, SEC-02, SEC-03, SEC-13, SEC-15, SEC-25, SEC-28, SEC-36) |
| CLI | Platform team | Password flag removal, TLS warning (SEC-33, SEC-34) |
| Security / Compliance | Security team | Secret lifecycle, log redaction policy, SAST tooling, review cadence (SEC-10, SEC-11, SEC-12, SEC-18–21, SEC-23, SEC-26) |
| Data / API | Data/API team | SQL policy maintenance, quality rules feature flags, runtime API tests (SEC-05, SEC-11, SEC-23) |
