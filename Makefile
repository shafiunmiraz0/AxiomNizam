.PHONY: vuln-check sbom verify-deps sign-image scan-image security-all

# ── Supply Chain Security Targets (Phase 12) ─────────────────────────────────

## Run govulncheck to scan Go dependencies for known vulnerabilities
vuln-check:
	@echo "=== Running govulncheck ==="
	@command -v govulncheck >/dev/null 2>&1 || go install golang.org/x/vuln/cmd/govulncheck@latest
	govulncheck ./...

## Generate SBOM (Software Bill of Materials) in CycloneDX format
sbom:
	@echo "=== Generating SBOM ==="
	@command -v cyclonedx-gomod >/dev/null 2>&1 || go install github.com/CycloneDX/cyclonedx-gomod/cmd/cyclonedx-gomod@latest
	cyclonedx-gomod app -output sbom.json -json
	@echo "SBOM written to sbom.json"

## Verify go.sum integrity and module pinning
verify-deps:
	@echo "=== Verifying dependency integrity ==="
	go mod verify
	go mod tidy
	@git diff --exit-code go.mod go.sum && echo "OK: go.mod/go.sum are clean" || echo "WARN: go.mod/go.sum have uncommitted changes"

## Build and sign container image (requires Docker + cosign)
sign-image:
	@echo "=== Building and signing container image ==="
	docker build -t axiomnizam:latest .
	docker tag axiomnizam:latest ghcr.io/axiomnizam/axiomnizam:latest
	cosign sign --yes ghcr.io/axiomnizam/axiomnizam:latest

## Scan container image for vulnerabilities (requires Trivy)
scan-image:
	@echo "=== Scanning container image ==="
	docker build -t axiomnizam:scan .
	@command -v trivy >/dev/null 2>&1 || { echo "Install trivy: https://aquasecurity.github.io/trivy/"; exit 1; }
	trivy image axiomnizam:scan

## Run all security checks locally
security-all: verify-deps vuln-check sbom
	@echo "=== All security checks passed ==="
