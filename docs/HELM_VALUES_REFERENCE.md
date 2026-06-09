# Helm Chart — Values Reference

Complete reference for all configurable values in the `axiomnizam/axiomnizam` Helm chart.

---

## Quick Override Examples

```bash
# Override a single value
helm install axiomnizam axiomnizam/axiomnizam --set postgresql.password=strong-password

# Override multiple values
helm install axiomnizam axiomnizam/axiomnizam \
  --set postgresql.password=strong-password \
  --set iam.sysadminPassword=admin-password \
  --set image.tag=0.2.0

# Use a values file (recommended for production)
helm install axiomnizam axiomnizam/axiomnizam -f my-values.yaml
```

---

## Deployment

Controls the AxiomNizam application pod.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `replicaCount` | int | `1` | Number of pod replicas. Use >1 for HA (requires shared storage). |
| `image.repository` | string | `ghcr.io/shafiunmiraz0/axiomnizam` | Container image repository. |
| `image.tag` | string | `latest` | Container image tag. Pin to a version in production. |
| `image.pullPolicy` | string | `IfNotPresent` | When to pull the image: `Always`, `IfNotPresent`, `Never`. |
| `image.pullSecrets` | list | `[]` | Registry pull secrets for private registries. |

```yaml
image:
  repository: ghcr.io/shafiunmiraz0/axiomnizam
  tag: "0.2.0"
  pullPolicy: IfNotPresent
  pullSecrets:
    - name: ghcr-auth
```

---

## Service

Exposes the application within the cluster.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `service.type` | string | `ClusterIP` | Service type: `ClusterIP`, `NodePort`, `LoadBalancer`. |
| `service.port` | int | `8000` | Port exposed by the service. |

---

## Ingress

Optional external access via an ingress controller.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `ingress.enabled` | bool | `false` | Create an Ingress resource. |
| `ingress.className` | string | `nginx` | Ingress class name. |
| `ingress.annotations` | object | `{}` | Ingress annotations (e.g., cert-manager, rate-limit). |
| `ingress.hosts` | list | See below | Hostnames and path rules. |
| `ingress.tls` | list | `[]` | TLS termination configuration. |

```yaml
ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
  hosts:
    - host: axiomnizam.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: axiomnizam-tls
      hosts:
        - axiomnizam.example.com
```

---

## Resources

CPU and memory limits for the application container.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `resources.requests.cpu` | string | `250m` | Guaranteed CPU. |
| `resources.requests.memory` | string | `512Mi` | Guaranteed memory. |
| `resources.limits.cpu` | string | `2` | Maximum CPU. |
| `resources.limits.memory` | string | `2Gi` | Maximum memory. |

```yaml
resources:
  requests:
    cpu: 500m
    memory: 1Gi
  limits:
    cpu: "4"
    memory: 4Gi
```

---

## Persistence

PersistentVolumeClaims for application data. Each can be independently configured or disabled (falls back to `emptyDir`).

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `persistence.raft.enabled` | bool | `true` | Create PVC for Raft consensus data. |
| `persistence.raft.storageClass` | string | `""` | StorageClass (empty = cluster default). |
| `persistence.raft.accessMode` | string | `ReadWriteOnce` | PVC access mode. |
| `persistence.raft.size` | string | `5Gi` | PVC size. |
| `persistence.storage.enabled` | bool | `true` | Create PVC for object storage. |
| `persistence.storage.storageClass` | string | `""` | StorageClass. |
| `persistence.storage.accessMode` | string | `ReadWriteOnce` | PVC access mode. |
| `persistence.storage.size` | string | `10Gi` | PVC size. |
| `persistence.certs.enabled` | bool | `true` | Create PVC for TLS certificates. |
| `persistence.certs.storageClass` | string | `""` | StorageClass. |
| `persistence.certs.accessMode` | string | `ReadWriteOnce` | PVC access mode. |
| `persistence.certs.size` | string | `256Mi` | PVC size. |
| `persistence.queryLogs.enabled` | bool | `true` | Create PVC for query audit logs. |
| `persistence.queryLogs.storageClass` | string | `""` | StorageClass. |
| `persistence.queryLogs.accessMode` | string | `ReadWriteOnce` | PVC access mode. |
| `persistence.queryLogs.size` | string | `2Gi` | PVC size. |

```yaml
# Use a specific StorageClass for all PVCs
persistence:
  raft:
    storageClass: gp3-encrypted
    size: 10Gi
  storage:
    storageClass: gp3-encrypted
    size: 50Gi
```

---

## PostgreSQL

Bundled PostgreSQL StatefulSet. Disable to use an external database.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `postgresql.enabled` | bool | `true` | Deploy bundled PostgreSQL. |
| `postgresql.image` | string | `postgres:latest` | PostgreSQL image. |
| `postgresql.user` | string | `axiomnizam` | Database username. |
| `postgresql.password` | string | `changeme` | **Required.** Database password. |
| `postgresql.database` | string | `app_db` | Database name. |
| `postgresql.port` | int | `5432` | Database port. |
| `postgresql.storage.storageClass` | string | `""` | StorageClass. |
| `postgresql.storage.accessMode` | string | `ReadWriteOnce` | PVC access mode. |
| `postgresql.storage.size` | string | `10Gi` | Database PVC size. |
| `postgresql.resources.requests.cpu` | string | `100m` | CPU request. |
| `postgresql.resources.requests.memory` | string | `256Mi` | Memory request. |
| `postgresql.resources.limits.cpu` | string | `1` | CPU limit. |
| `postgresql.resources.limits.memory` | string | `1Gi` | Memory limit. |

```yaml
# Disable bundled PostgreSQL, use external
postgresql:
  enabled: false
config:
  postgresHost: rds-instance.amazonaws.com
  postgresPort: "5432"
  postgresSslmode: require
```

---

## TLS

Transport Layer Security configuration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `tls.enabled` | bool | `true` | Enable HTTPS on the server. |
| `tls.autoGenerate` | bool | `true` | Auto-generate self-signed certs (dev only). |
| `tls.certFile` | string | *(commented)* | Path to TLS certificate file. |
| `tls.keyFile` | string | *(commented)* | Path to TLS private key file. |

**Priority:** explicit cert/key > auto-generate > disabled.

```yaml
# Production: use cert-manager mounted certs
tls:
  enabled: true
  autoGenerate: false
  certFile: /data/certs/tls.crt
  keyFile: /data/certs/tls.key
```

---

## Public URLs

URLs used in CORS, redirects, and OIDC callbacks. **Must match your actual domain.**

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `public.frontendHostname` | string | `axiomnizam.local` | Frontend hostname. |
| `public.platformHostname` | string | `axiomnizam.local` | Platform API hostname. |
| `public.frontendUrl` | string | `https://axiomnizam.local` | Full frontend URL. |
| `public.platformUrl` | string | `https://axiomnizam.local` | Full platform URL. |
| `public.corsOrigins` | string | `https://axiomnizam.local` | Comma-separated CORS origins. |

```yaml
public:
  frontendHostname: app.example.com
  platformHostname: api.example.com
  frontendUrl: "https://app.example.com"
  platformUrl: "https://api.example.com"
  corsOrigins: "https://app.example.com,https://api.example.com"
```

---

## Application Config

Core server configuration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `config.apiPort` | string | `"8000"` | Server listen port. |
| `config.apiHost` | string | `"0.0.0.0"` | Server bind address. |
| `config.axiomnizamEnv` | string | `production` | Environment profile: `production`, `development`. |
| `config.securityGuardrailsMode` | string | `enforce` | `enforce` (block insecure defaults) or `audit` (log only). |
| `config.storageBackend` | string | `raft` | Persistence backend: `raft` (embedded) or `etcd`. |
| `config.trustedProxies` | string | `10.0.0.0/8,...` | Comma-separated trusted proxy CIDRs. `*` = trust none. |

---

## IAM

Identity and Access Management bootstrap configuration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `iam.sysadminEmail` | string | `sysadmin@axiomnizam.local` | **Required.** Bootstrap sysadmin email. |
| `iam.sysadminPassword` | string | `changeme-on-first-login` | **Required.** Bootstrap sysadmin password. |
| `iam.rsaPrivateKey` | string | `""` | RSA private key (PEM) for JWT signing. Mounted as file at `/secrets/iam/rsa_private_key.pem`. |
| `iam.demoJwtSecret` | string | `""` | Demo JWT fallback secret. Not for production. |

```yaml
# Production: provide RSA key
iam:
  sysadminEmail: admin@example.com
  sysadminPassword: <strong-password>
  rsaPrivateKey: |
    -----BEGIN RSA PRIVATE KEY-----
    MIIEpAIBAAKCAQEA...
    -----END RSA PRIVATE KEY-----
```

---

## SafeGate / Antivirus

File scanning and malware detection.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `antivirus.enabled` | bool | `true` | Enable the antivirus engine. |
| `antivirus.workers` | int | `4` | Concurrent scan goroutines. |
| `antivirus.maxFileSize` | int | `104857600` | Max file size to scan (bytes). Default: 100MB. |
| `antivirus.cacheSize` | int | `100000` | LRU scan result cache entries. |
| `antivirus.quarantineAction` | string | `tag` | Action on threat detection: `tag`, `delete`, `move`. |
| `safegate.maxFileSize` | int | `104857600` | SafeGate orchestrator max file size. |

---

## Rate Limiting

API rate limiting configuration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `rateLimit.maxCalls` | int | `500` | Max API calls per token window. |
| `rateLimit.validityMinutes` | int | `5` | Rate limit window duration (minutes). |

---

## Gatekeeper 2FA

Multi-factor authentication and risk engine.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `gatekeeper.totpIssuer` | string | `AxiomNizam` | TOTP issuer name shown in authenticator apps. |
| `gatekeeper.encryptionKey` | string | *(auto-gen)* | AES-256 key for encrypting TOTP secrets. Auto-generated if empty. |
| `gatekeeper.hmacKey` | string | *(auto-gen)* | HMAC key for integrity verification. Auto-generated if empty. |

---

## Optional: Keycloak

External Identity Provider integration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `keycloak.enabled` | bool | `false` | Enable Keycloak integration. |
| `keycloak.url` | string | `keycloak` | Keycloak service hostname. |
| `keycloak.host` | string | `keycloak` | Keycloak host. |
| `keycloak.port` | string | `"8080"` | Keycloak port. |
| `keycloak.realm` | string | `axiomnizam` | Keycloak realm. |
| `keycloak.clientId` | string | `axiomnizam-backend` | Backend OIDC client ID. |
| `keycloak.clientSecret` | string | `""` | **Required if enabled.** Backend client secret. |
| `keycloak.adminUsername` | string | `admin` | Keycloak admin username. |
| `keycloak.adminPassword` | string | `admin` | Keycloak admin password. |

---

## Optional: OpenClaw SQL Assistant

AI-powered SQL assistant integration.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `openclaw.enabled` | bool | `false` | Enable SQL assistant. |
| `openclaw.host` | string | `openclaw-gateway` | Gateway hostname. |
| `openclaw.port` | string | `"18789"` | Gateway port. |
| `openclaw.token` | string | `""` | Authentication token. |
| `openclaw.model` | string | `ollama/tinyllama` | LLM model reference. |
| `openclaw.timeoutSeconds` | string | `"90"` | Request timeout. |

---

## Optional: RabbitMQ

Message queue for async workflows.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `rabbitmq.enabled` | bool | `false` | Enable RabbitMQ. |
| `rabbitmq.url` | string | `""` | Full AMQP connection string. |
| `rabbitmq.user` | string | `axiomnizam` | RabbitMQ username. |
| `rabbitmq.password` | string | `""` | RabbitMQ password. |
| `rabbitmq.vhost` | string | `/axiom` | RabbitMQ virtual host. |

---

## Optional: Kafka

Event streaming platform.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `kafka.enabled` | bool | `false` | Enable Kafka. |
| `kafka.brokers` | string | `kafka:9092` | Comma-separated broker list. |

---

## Network & Security

Kubernetes-native security features.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `networkPolicy.enabled` | bool | `false` | Create NetworkPolicies for micro-segmentation. |
| `certManager.enabled` | bool | `false` | Create cert-manager Certificate resources. |
| `certManager.issuerName` | string | `axiomnizam-ca-issuer` | Certificate issuer name. |
| `certManager.issuerKind` | string | `Issuer` | Issuer type: `Issuer` or `ClusterIssuer`. |
| `discord.webhookUrl` | string | `""` | Discord webhook for threat notifications. |

```yaml
# Production: enable network policies + cert-manager
networkPolicy:
  enabled: true
certManager:
  enabled: true
  issuerName: letsencrypt-prod
  issuerKind: ClusterIssuer
```

---

## Pod Scheduling & Security

Kubernetes pod scheduling and security context.

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `nodeSelector` | object | `{}` | Node selection constraints. |
| `tolerations` | list | `[]` | Pod tolerations for tainted nodes. |
| `affinity` | object | `{}` | Pod affinity/anti-affinity rules. |
| `podAnnotations` | object | `{}` | Additional annotations on pods. |
| `podSecurityContext.runAsUser` | int | `1000` | UID to run the container as. |
| `podSecurityContext.runAsGroup` | int | `1000` | GID to run the container as. |
| `podSecurityContext.fsGroup` | int | `1000` | Filesystem group for volume mounts. |
| `securityContext.runAsNonRoot` | bool | `true` | Enforce non-root container execution. |
| `securityContext.readOnlyRootFilesystem` | bool | `false` | Mount root filesystem as read-only. |
| `securityContext.allowPrivilegeEscalation` | bool | `false` | Prevent privilege escalation. |

```yaml
# Schedule on specific nodes
nodeSelector:
  node-role.kubernetes.io/worker: ""

tolerations:
  - key: "dedicated"
    operator: "Equal"
    value: "axiomnizam"
    effect: "NoSchedule"

affinity:
  podAntiAffinity:
    preferredDuringSchedulingIgnoredDuringExecution:
      - weight: 100
        podAffinityTerm:
          labelSelector:
            matchExpressions:
              - key: app.kubernetes.io/name
                operator: In
                values:
                  - axiomnizam
          topologyKey: kubernetes.io/hostname
```

---

## Full Production Example

```yaml
# my-production-values.yaml
replicaCount: 2

image:
  repository: ghcr.io/shafiunmiraz0/axiomnizam
  tag: "0.2.0"
  pullSecrets:
    - name: ghcr-auth

service:
  type: ClusterIP
  port: 8000

ingress:
  enabled: true
  className: nginx
  annotations:
    cert-manager.io/cluster-issuer: letsencrypt-prod
    nginx.ingress.kubernetes.io/proxy-body-size: "100m"
  hosts:
    - host: axiomnizam.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: axiomnizam-tls
      hosts:
        - axiomnizam.example.com

resources:
  requests:
    cpu: 500m
    memory: 1Gi
  limits:
    cpu: "4"
    memory: 4Gi

persistence:
  raft:
    storageClass: gp3-encrypted
    size: 10Gi
  storage:
    storageClass: gp3-encrypted
    size: 50Gi
  certs:
    storageClass: gp3-encrypted
    size: 256Mi
  queryLogs:
    storageClass: gp3-encrypted
    size: 5Gi

postgresql:
  enabled: true
  password: <strong-password>
  storage:
    storageClass: gp3-encrypted
    size: 20Gi
  resources:
    requests:
      cpu: 250m
      memory: 512Mi
    limits:
      cpu: "2"
      memory: 2Gi

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

config:
  axiomnizamEnv: production
  securityGuardrailsMode: enforce
  storageBackend: raft

iam:
  sysadminEmail: admin@example.com
  sysadminPassword: <strong-password>
  rsaPrivateKey: |
    -----BEGIN RSA PRIVATE KEY-----
    ...
    -----END RSA PRIVATE KEY-----

antivirus:
  enabled: true
  workers: 8
  maxFileSize: 209715200  # 200MB
  quarantineAction: tag

networkPolicy:
  enabled: true

certManager:
  enabled: true
  issuerName: letsencrypt-prod
  issuerKind: ClusterIssuer

podSecurityContext:
  runAsUser: 1000
  runAsGroup: 1000
  fsGroup: 1000

securityContext:
  runAsNonRoot: true
  allowPrivilegeEscalation: false
```
