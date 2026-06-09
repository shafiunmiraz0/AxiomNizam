# Helm Chart — Packaging & Release Guide

This document covers how to build, package, and publish the AxiomNizam Helm chart.

---

## Prerequisites

| Tool | Install |
|------|---------|
| Helm 3.x | https://helm.sh/docs/intro/install/ |
| Git | https://git-scm.com/ |
| Docker | https://docs.docker.com/get-docker/ |
| GitHub CLI (`gh`) | https://cli.github.com/ (optional, for GHCR login) |

---

## Chart Location

```
helm/axiomnizam/
├── Chart.yaml              # Chart metadata (name, version, appVersion)
├── values.yaml             # Default configuration values
├── .helmignore
└── templates/
    ├── _helpers.tpl         # Template helpers
    ├── configmap.yaml       # Non-secret env vars
    ├── secret.yaml          # Secrets (Postgres, Gatekeeper, RSA key)
    ├── deployment.yaml      # App Deployment
    ├── service.yaml         # ClusterIP Service
    ├── ingress.yaml         # Optional Ingress
    ├── postgres.yaml        # PostgreSQL StatefulSet + Service + PVC
    ├── pvc.yaml             # App PVCs (raft, storage, certs, query-logs)
    ├── certificate.yaml     # Optional cert-manager Certificates
    ├── networkpolicy.yaml   # Optional NetworkPolicies
    └── NOTES.txt            # Post-install instructions
```

---

## Step 0 — Build & Push the Docker Image

The Helm chart references a pre-built container image. The Dockerfile packages everything — the Go binary, frontend templates, and CLI tool — into a self-contained image. No external packages are needed at runtime.

### What's inside the image

The multi-stage Dockerfile:

1. **Build stage** (`golang:1.25`) — compiles `axiomnizam` and `axiomnizamctl` binaries
2. **Runtime stage** (`debian:bookworm-slim`) — copies binaries + frontend templates, creates data dirs, runs as non-root user

The image contains:
- `/app/axiomnizam` — main server binary
- `/usr/local/bin/axiomnizamctl` — CLI tool
- `/app/internal/frontend/templates/` — HTML/JS/CSS frontend assets
- `/data/certs/`, `/data/storage/`, `/data/raft/`, `/data/query_logs/` — data directories

### Build

```bash
# Build with a version tag
docker build -t ghcr.io/shafiunmiraz0/axiomnizam:0.1.0 .

# Tag as latest too
docker tag ghcr.io/shafiunmiraz0/axiomnizam:0.1.0 ghcr.io/shafiunmiraz0/axiomnizam:latest
```

### Push to GitHub Container Registry (GHCR)

```bash
# Login (use a PAT with write:packages scope, or use `gh`)
echo $GITHUB_TOKEN | docker login ghcr.io -u shafiunmiraz0 --password-stdin

# Push both tags
docker push ghcr.io/shafiunmiraz0/axiomnizam:0.1.0
docker push ghcr.io/shafiunmiraz0/axiomnizam:latest
```

### Push to Docker Hub (alternative)

```bash
docker login
docker tag ghcr.io/shafiunmiraz0/axiomnizam:0.1.0 shafiunmiraz0/axiomnizam:0.1.0
docker push shafiunmiraz0/axiomnizam:0.1.0
```

### Push to any private registry

```bash
docker login registry.example.com
docker build -t registry.example.com/axiomnizam/axiomnizam:0.1.0 .
docker push registry.example.com/axiomnizam/axiomnizam:0.1.0
```

### Update chart defaults (optional)

If you always use the same registry, update `helm/axiomnizam/values.yaml`:

```yaml
image:
  repository: ghcr.io/shafiunmiraz0/axiomnizam
  tag: 0.1.0
  pullPolicy: IfNotPresent
```

Or override at install time:

```bash
helm install axiomnizam axiomnizam/axiomnizam \
  --set image.repository=ghcr.io/shafiunmiraz0/axiomnizam \
  --set image.tag=0.1.0
```

### Image pull secrets (private registries)

If your registry requires authentication, create a pull secret:

```bash
kubectl create secret docker-registry ghcr-auth \
  -n axiomnizam \
  --docker-server=ghcr.io \
  --docker-username=shafiunmiraz0 \
  --docker-password=$GITHUB_TOKEN
```

Then set in `values.yaml` or at install time:

```bash
helm install axiomnizam axiomnizam/axiomnizam \
  --set image.pullSecrets[0].name=ghcr-auth
```

---

## Step 1 — Bump the Version

Edit `helm/axiomnizam/Chart.yaml`:

```yaml
version: 0.2.0        # Chart version (SemVer)
appVersion: "1.0.1"    # App version (matches your release tag)
```

**Versioning rules:**
- `version` — chart packaging version (bump when templates change)
- `appVersion` — application release version (bump when the app image changes)
- Both must be valid SemVer (`MAJOR.MINOR.PATCH`)

---

## Step 2 — Validate the Chart

```bash
# Lint — checks for template errors, missing values, bad practices
helm lint ./helm/axiomnizam

# Dry-run — renders all templates without installing
helm template test ./helm/axiomnizam \
  --set postgresql.password=test \
  --set iam.sysadminPassword=test

# Dry-run against a live cluster (optional)
helm install test ./helm/axiomnizam \
  --namespace axiomnizam-test --create-namespace \
  --set postgresql.password=test \
  --set iam.sysadminPassword=test \
  --dry-run --debug
```

Fix any errors before proceeding.

---

## Step 3 — Package the Chart

```bash
# Package creates a .tgz archive
helm package ./helm/axiomnizam --destination /tmp/helm-packages

# Output:
#   /tmp/helm-packages/axiomnizam-0.2.0.tgz
```

---

## Step 4 — Publish to GitHub Pages (Helm Repo)

The Helm repo is hosted on the `gh-pages` branch via GitHub Pages.

### First-time setup (already done)

```bash
# Create orphan branch (one-time)
git checkout --orphan gh-pages
git rm -rf .
mkdir -p charts
cp /tmp/helm-packages/axiomnizam-0.1.0.tgz charts/
helm repo index charts/ --url https://shafiunmiraz0.github.io/AxiomNizam/charts
git add charts/
git commit -m "Helm chart v0.1.0"
git push origin gh-pages
git checkout main
```

Then enable GitHub Pages:
1. Go to https://github.com/shafiunmiraz0/AxiomNizam/settings/pages
2. Source: **Deploy from a branch**
3. Branch: **gh-pages**, folder: **/ (root)**
4. Click **Save**

### Release a new version

```bash
# 1. Make sure the chart is packaged
helm package ./helm/axiomnizam --destination /tmp/helm-packages

# 2. Switch to gh-pages
git checkout gh-pages

# 3. Copy the new .tgz into charts/
cp /tmp/helm-packages/axiomnizam-0.2.0.tgz charts/

# 4. Regenerate index (use --merge to keep old entries)
helm repo index charts/ \
  --url https://shafiunmiraz0.github.io/AxiomNizam/charts \
  --merge charts/index.yaml

# 5. Commit and push
git add charts/
git commit -m "chart v0.2.0"
git push origin gh-pages

# 6. Switch back to your working branch
git checkout miraz-zero-trust
```

### Remove an old version

```bash
git checkout gh-pages
rm charts/axiomnizam-0.1.0.tgz
helm repo index charts/ \
  --url https://shafiunmiraz0.github.io/AxiomNizam/charts \
  --merge charts/index.yaml
git add -A && git commit -m "remove chart v0.1.0" && git push
git checkout miraz-zero-trust
```

---

## Step 5 — Verify the Repo

```bash
# Add the repo
helm repo add axiomnizam https://shafiunmiraz0.github.io/AxiomNizam/charts
helm repo update

# Search for the chart
helm search repo axiomnizam

# Expected output:
# NAME                    CHART VERSION   APP VERSION   DESCRIPTION
# axiomnizam/axiomnizam   0.2.0           1.0.1         AxiomNizam — Enterprise Data Control Plane
```

---

## Installing the Chart

### Minimal (dev)

```bash
helm install axiomnizam axiomnizam/axiomnizam \
  -n axiomnizam --create-namespace \
  --set postgresql.password=changeme \
  --set iam.sysadminPassword=changeme
```

### Production (with ingress + network policies)

```bash
cat > my-values.yaml <<EOF
postgresql:
  password: <strong-password>

iam:
  sysadminEmail: admin@example.com
  sysadminPassword: <strong-password>
  rsaPrivateKey: |
    -----BEGIN RSA PRIVATE KEY-----
    ...
    -----END RSA PRIVATE KEY-----

tls:
  enabled: true
  autoGenerate: false
  certFile: /data/certs/tls.crt
  keyFile: /data/certs/tls.key

public:
  frontendHostname: axiomnizam.example.com
  platformHostname: axiomnizam.example.com
  frontendUrl: "https://axiomnizam.example.com"
  platformUrl: "https://axiomnizam.example.com"
  corsOrigins: "https://axiomnizam.example.com"

ingress:
  enabled: true
  className: nginx
  hosts:
    - host: axiomnizam.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: axiomnizam-tls
      hosts:
        - axiomnizam.example.com

networkPolicy:
  enabled: true

certManager:
  enabled: true
  issuerName: letsencrypt-prod
  issuerKind: ClusterIssuer
EOF

helm install axiomnizam axiomnizam/axiomnizam \
  -n axiomnizam --create-namespace \
  -f my-values.yaml
```

### External PostgreSQL (disable bundled)

```bash
helm install axiomnizam axiomnizam/axiomnizam \
  -n axiomnizam --create-namespace \
  --set postgresql.enabled=false \
  --set config.postgresHost=my-pg.example.com \
  --set postgresql.password=<password> \
  --set iam.sysadminPassword=<password>
```

---

## Upgrading

```bash
# Update repo
helm repo update

# Upgrade
helm upgrade axiomnizam axiomnizam/axiomnizam \
  -n axiomnizam \
  -f my-values.yaml

# Rollback if needed
helm rollback axiomnizam 1 -n axiomnizam
```

---

## Uninstalling

```bash
helm uninstall axiomnizam -n axiomnizam

# PVCs are NOT deleted automatically — delete manually if needed
kubectl delete pvc -l app.kubernetes.io/instance=axiomnizam -n axiomnizam
```

---

## OCI Registry (Alternative)

Instead of GitHub Pages, you can push to any OCI-compatible registry (GHCR, Docker Hub, ECR, ACR):

```bash
# Login to GHCR
echo $GITHUB_TOKEN | helm registry login ghcr.io -u USERNAME --password-stdin

# Package and push
helm package ./helm/axiomnizam --destination /tmp/helm-packages
helm push /tmp/helm-packages/axiomnizam-0.2.0.tgz oci://ghcr.io/shafiunmiraz0/charts

# Install from OCI
helm install axiomnizam oci://ghcr.io/shafiunmiraz0/charts/axiomnizam \
  --version 0.2.0 \
  -n axiomnizam --create-namespace
```

---

## Quick Reference

| Command | Purpose |
|---------|---------|
| `helm lint ./helm/axiomnizam` | Validate chart |
| `helm template test ./helm/axiomnizam` | Render templates locally |
| `helm package ./helm/axiomnizam` | Create .tgz archive |
| `helm repo index charts/ --url <URL>` | Generate/merge index.yaml |
| `helm repo add axiomnizam <URL>` | Add repo locally |
| `helm search repo axiomnizam` | List available chart versions |
| `helm install axiomnizam axiomnizam/axiomnizam` | Install |
| `helm upgrade axiomnizam axiomnizam/axiomnizam` | Upgrade |
| `helm rollback axiomnizam <REV>` | Rollback |
| `helm uninstall axiomnizam` | Remove release |
