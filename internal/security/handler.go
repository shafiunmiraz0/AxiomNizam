package security

import (
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	certModeKubeadm = "kubeadm"
	certModeTLS     = "tls"
)

var kubeadmCertFileMap = map[string]string{
	"all":                      "",
	"apiserver":                "apiserver.crt",
	"apiserver-kubelet-client": "apiserver-kubelet-client.crt",
	"apiserver-etcd-client":    "apiserver-etcd-client.crt",
	"front-proxy-client":       "front-proxy-client.crt",
	"etcd-server":              "etcd/server.crt",
	"etcd-peer":                "etcd/peer.crt",
	"etcd-healthcheck-client":  "etcd/healthcheck-client.crt",
}

var defaultKubeadmManagedCerts = []string{
	"apiserver",
	"apiserver-kubelet-client",
	"apiserver-etcd-client",
	"front-proxy-client",
	"etcd-server",
	"etcd-peer",
	"etcd-healthcheck-client",
}

// Handler provides certificate expiry status and renewal actions.
type Handler struct {
	mode                 string
	monitorTargets       []string
	pkiDir               string
	managedCerts         []string
	renewalThresholdDays int
	renewCommand         string
}

// CertificateStatus describes one target certificate state.
type CertificateStatus struct {
	Mode               string    `json:"mode"`
	Target             string    `json:"target,omitempty"`
	CertName           string    `json:"cert_name,omitempty"`
	CertificatePath    string    `json:"certificate_path,omitempty"`
	Host               string    `json:"host,omitempty"`
	Port               string    `json:"port,omitempty"`
	Subject            string    `json:"subject,omitempty"`
	Issuer             string    `json:"issuer,omitempty"`
	SerialNumber       string    `json:"serial_number,omitempty"`
	NotBefore          time.Time `json:"not_before,omitempty"`
	NotAfter           time.Time `json:"not_after,omitempty"`
	DaysRemaining      int64     `json:"days_remaining"`
	Expired            bool      `json:"expired"`
	RenewalRecommended bool      `json:"renewal_recommended"`
	Error              string    `json:"error,omitempty"`
}

type renewCertificateRequest struct {
	Cert   string `json:"cert"`
	Target string `json:"target"`
	DryRun bool   `json:"dry_run"`
}

// NewHandler creates a certificate lifecycle handler.
func NewHandler() *Handler {
	mode := normalizeCertificateMode(os.Getenv("CERT_MANAGER_MODE"))
	threshold := parseIntEnv("CERT_RENEWAL_THRESHOLD_DAYS", 30)
	if threshold < 0 {
		threshold = 0
	}

	targets := parseMonitorTargets(os.Getenv("CERT_MONITOR_TARGETS"))
	pkiDir := strings.TrimSpace(os.Getenv("CERT_PKI_DIR"))
	if pkiDir == "" {
		pkiDir = "/etc/kubernetes/pki"
	}

	managedCerts := parseManagedCerts(os.Getenv("CERT_MANAGED_CERTS"))
	if len(managedCerts) == 0 {
		managedCerts = append([]string(nil), defaultKubeadmManagedCerts...)
	}

	return &Handler{
		mode:                 mode,
		monitorTargets:       targets,
		pkiDir:               pkiDir,
		managedCerts:         managedCerts,
		renewalThresholdDays: threshold,
		renewCommand:         strings.TrimSpace(os.Getenv("CERT_RENEW_COMMAND")),
	}
}

// GetCertificateStatus handles GET /api/admin/certificates/status.
// Optional query parameter: target=host[:port]
func (h *Handler) GetCertificateStatus(c *gin.Context) {
	if h.mode == certModeKubeadm {
		h.getKubeadmCertificateStatus(c)
		return
	}
	h.getTLSCertificateStatus(c)
}

func (h *Handler) getTLSCertificateStatus(c *gin.Context) {
	queryTarget := strings.TrimSpace(c.Query("target"))
	targets := make([]string, 0)

	if queryTarget != "" {
		targets = append(targets, queryTarget)
	} else {
		targets = append(targets, h.monitorTargets...)
	}

	if len(targets) == 0 {
		c.JSON(http.StatusBadRequest, MessageResponse{
			Error:   "no certificate target provided",
			Message: "set CERT_MONITOR_TARGETS in env or pass ?target=host[:port]",
		})
		return
	}

	statuses := make([]CertificateStatus, 0, len(targets))
	hasError := false
	for _, target := range targets {
		status := h.inspectTarget(target)
		if status.Error != "" {
			hasError = true
		}
		statuses = append(statuses, status)
	}

	c.JSON(http.StatusOK, TLSCertStatusResponse{
		Mode:                   h.mode,
		Status:                 "ok",
		ThresholdDays:          h.renewalThresholdDays,
		RenewCommandConfigured: h.renewCommand != "",
		HasError:               hasError,
		Certificates:           statuses,
	})
}

func (h *Handler) getKubeadmCertificateStatus(c *gin.Context) {
	queryCert := strings.TrimSpace(c.Query("cert"))
	certs, err := h.resolveKubeadmCertSelection(queryCert)
	if err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	statuses := make([]CertificateStatus, 0, len(certs))
	hasError := false
	for _, certName := range certs {
		status := h.inspectKubeadmCertificate(certName)
		if status.Error != "" {
			hasError = true
		}
		statuses = append(statuses, status)
	}

	c.JSON(http.StatusOK, KubeadmCertStatusResponse{
		Mode:                   h.mode,
		Status:                 "ok",
		PKIDir:                 h.pkiDir,
		ManagedCerts:           h.managedCerts,
		ThresholdDays:          h.renewalThresholdDays,
		RenewCommandConfigured: h.renewCommand != "",
		HasError:               hasError,
		Certificates:           statuses,
	})
}

// RenewCertificate handles POST /api/admin/certificates/renew.
// It executes CERT_RENEW_COMMAND and then returns refreshed cert status for the target.
func (h *Handler) RenewCertificate(c *gin.Context) {
	if h.mode == certModeKubeadm {
		h.renewKubeadmCertificate(c)
		return
	}
	h.renewTLSCertificate(c)
}

func (h *Handler) renewTLSCertificate(c *gin.Context) {
	if h.renewCommand == "" {
		c.JSON(http.StatusNotImplemented, MessageResponse{
			Error:   "renew command not configured",
			Message: "set CERT_RENEW_COMMAND in env to enable TLS target renewal",
		})
		return
	}

	var req renewCertificateRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, MessageResponse{Error: fmt.Sprintf("invalid request: %v", err)})
			return
		}
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		if len(h.monitorTargets) == 0 {
			c.JSON(http.StatusBadRequest, MessageResponse{
				Error:   "target is required",
				Message: "provide target in request body or configure CERT_MONITOR_TARGETS",
			})
			return
		}
		target = h.monitorTargets[0]
	}

	normalizedTarget, _, _, err := normalizeCertificateTarget(target)
	if err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	commandParts, err := buildRenewCommand(h.renewCommand, normalizedTarget)
	if err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	if req.DryRun {
		c.JSON(http.StatusOK, DryRunResponse{
			Status:  "ok",
			Message: "dry run: renewal command prepared",
			Target:  normalizedTarget,
			Command: commandParts,
		})
		return
	}

	cmd := exec.Command(commandParts[0], commandParts[1:]...)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		c.JSON(http.StatusInternalServerError, RenewErrorResponse{
			Error:   fmt.Sprintf("renewal command failed: %v", runErr),
			Target:  normalizedTarget,
			Command: commandParts,
			Output:  strings.TrimSpace(string(output)),
		})
		return
	}

	status := h.inspectTarget(normalizedTarget)
	resp := RenewSuccessResponse{
		Mode:              h.mode,
		Status:            "ok",
		Message:           "certificate renewal command executed",
		Target:            normalizedTarget,
		Command:           commandParts,
		Output:            strings.TrimSpace(string(output)),
		CertificateStatus: &status,
	}

	if status.Error != "" {
		resp.Message = "renew command executed, but certificate check failed"
	}

	c.JSON(http.StatusOK, resp)
}

func (h *Handler) renewKubeadmCertificate(c *gin.Context) {
	var req renewCertificateRequest
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, MessageResponse{Error: fmt.Sprintf("invalid request: %v", err)})
			return
		}
	}

	certName := strings.TrimSpace(req.Cert)
	if certName == "" {
		// Backward compatibility with previous UI/CLI payload key.
		certName = strings.TrimSpace(req.Target)
	}
	if certName == "" {
		certName = "all"
	}

	commandParts, err := h.buildKubeadmRenewCommand(certName)
	if err != nil {
		c.JSON(http.StatusBadRequest, MessageResponse{Error: err.Error()})
		return
	}

	if req.DryRun {
		c.JSON(http.StatusOK, DryRunResponse{
			Mode:    h.mode,
			Status:  "ok",
			Message: "dry run: kubeadm renewal command prepared",
			Cert:    certName,
			Command: commandParts,
		})
		return
	}

	cmd := exec.Command(commandParts[0], commandParts[1:]...)
	output, runErr := cmd.CombinedOutput()
	if runErr != nil {
		message := fmt.Sprintf("renewal command failed: %v", runErr)
		if errors.Is(runErr, exec.ErrNotFound) {
			message = "kubeadm command not found; install kubeadm or set CERT_RENEW_COMMAND"
		}
		c.JSON(http.StatusInternalServerError, RenewErrorResponse{
			Mode:    h.mode,
			Error:   message,
			Cert:    certName,
			Command: commandParts,
			Output:  strings.TrimSpace(string(output)),
		})
		return
	}

	certs, selErr := h.resolveKubeadmCertSelection(certName)
	if selErr != nil {
		certs = []string{certName}
	}

	statuses := make([]CertificateStatus, 0, len(certs))
	for _, cName := range certs {
		statuses = append(statuses, h.inspectKubeadmCertificate(cName))
	}

	c.JSON(http.StatusOK, RenewSuccessResponse{
		Mode:         h.mode,
		Status:       "ok",
		Message:      "kubeadm certificate renewal command executed",
		Cert:         certName,
		Command:      commandParts,
		Output:       strings.TrimSpace(string(output)),
		Certificates: statuses,
	})
}

func (h *Handler) inspectTarget(target string) CertificateStatus {
	normalizedTarget, host, port, err := normalizeCertificateTarget(target)
	if err != nil {
		return CertificateStatus{Mode: h.mode, Target: strings.TrimSpace(target), Error: err.Error()}
	}

	status := CertificateStatus{
		Mode:   h.mode,
		Target: normalizedTarget,
		Host:   host,
		Port:   port,
	}

	tlsCfg := &tls.Config{
		ServerName:         host,
		InsecureSkipVerify: true,
	}

	dialer := &net.Dialer{Timeout: 8 * time.Second}
	conn, err := tls.DialWithDialer(dialer, "tcp", normalizedTarget, tlsCfg)
	if err != nil {
		status.Error = fmt.Sprintf("tls connection failed: %v", err)
		status.DaysRemaining = -1
		return status
	}
	defer conn.Close()

	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		status.Error = "no peer certificate returned"
		status.DaysRemaining = -1
		return status
	}

	leaf := state.PeerCertificates[0]
	now := time.Now()
	remaining := leaf.NotAfter.Sub(now)
	daysRemaining := int64(remaining.Hours() / 24)
	if remaining < 0 {
		daysRemaining = -1
	}

	status.Subject = leaf.Subject.String()
	status.Issuer = leaf.Issuer.String()
	status.SerialNumber = leaf.SerialNumber.String()
	status.NotBefore = leaf.NotBefore
	status.NotAfter = leaf.NotAfter
	status.DaysRemaining = daysRemaining
	status.Expired = now.After(leaf.NotAfter)
	status.RenewalRecommended = status.Expired || (!status.Expired && daysRemaining <= int64(h.renewalThresholdDays))

	return status
}

func (h *Handler) inspectKubeadmCertificate(certName string) CertificateStatus {
	status := CertificateStatus{
		Mode:     h.mode,
		CertName: certName,
	}

	certPath, err := resolveKubeadmCertificatePath(certName, h.pkiDir)
	if err != nil {
		status.Error = err.Error()
		status.DaysRemaining = -1
		return status
	}
	status.CertificatePath = certPath

	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		status.Error = fmt.Sprintf("failed to read certificate: %v", err)
		status.DaysRemaining = -1
		return status
	}

	cert, err := parseLeafCertificateFromPEM(pemBytes)
	if err != nil {
		status.Error = fmt.Sprintf("failed to parse certificate: %v", err)
		status.DaysRemaining = -1
		return status
	}

	now := time.Now()
	remaining := cert.NotAfter.Sub(now)
	daysRemaining := int64(remaining.Hours() / 24)
	if remaining < 0 {
		daysRemaining = -1
	}

	status.Subject = cert.Subject.String()
	status.Issuer = cert.Issuer.String()
	status.SerialNumber = cert.SerialNumber.String()
	status.NotBefore = cert.NotBefore
	status.NotAfter = cert.NotAfter
	status.DaysRemaining = daysRemaining
	status.Expired = now.After(cert.NotAfter)
	status.RenewalRecommended = status.Expired || (!status.Expired && daysRemaining <= int64(h.renewalThresholdDays))

	return status
}

func (h *Handler) resolveKubeadmCertSelection(raw string) ([]string, error) {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" || v == "all" {
		certs := append([]string(nil), h.managedCerts...)
		sort.Strings(certs)
		return certs, nil
	}

	parts := strings.Split(v, ",")
	seen := make(map[string]struct{}, len(parts))
	selected := make([]string, 0, len(parts))

	for _, p := range parts {
		name := strings.ToLower(strings.TrimSpace(p))
		if name == "" {
			continue
		}
		if _, exists := kubeadmCertFileMap[name]; !exists || name == "all" {
			return nil, fmt.Errorf("unsupported certificate '%s' (supported: %s)", name, strings.Join(h.managedCerts, ", "))
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		selected = append(selected, name)
	}

	if len(selected) == 0 {
		return nil, fmt.Errorf("certificate name is required")
	}

	sort.Strings(selected)
	return selected, nil
}

func (h *Handler) buildKubeadmRenewCommand(certName string) ([]string, error) {
	cert := strings.ToLower(strings.TrimSpace(certName))
	if cert == "" {
		cert = "all"
	}

	if cert != "all" {
		if _, ok := kubeadmCertFileMap[cert]; !ok {
			return nil, fmt.Errorf("unsupported certificate '%s' (supported: %s, all)", cert, strings.Join(h.managedCerts, ", "))
		}
	}

	if h.renewCommand == "" {
		return []string{"kubeadm", "certs", "renew", cert}, nil
	}

	return buildRenewCommand(h.renewCommand, cert)
}

func resolveKubeadmCertificatePath(certName, pkiDir string) (string, error) {
	name := strings.ToLower(strings.TrimSpace(certName))
	relPath, exists := kubeadmCertFileMap[name]
	if !exists || relPath == "" {
		return "", fmt.Errorf("unknown kubeadm certificate '%s'", certName)
	}

	if strings.TrimSpace(pkiDir) == "" {
		pkiDir = "/etc/kubernetes/pki"
	}

	return filepath.Join(pkiDir, filepath.FromSlash(relPath)), nil
}

func parseLeafCertificateFromPEM(pemBytes []byte) (*x509.Certificate, error) {
	remaining := pemBytes
	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, err
			}
			return cert, nil
		}
		remaining = rest
	}

	return nil, fmt.Errorf("no certificate PEM block found")
}

func parseMonitorTargets(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}

	entries := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(entries))
	result := make([]string, 0, len(entries))

	for _, item := range entries {
		normalized, _, _, err := normalizeCertificateTarget(item)
		if err != nil {
			logging.Z().Warn("skipping invalid CERT_MONITOR_TARGETS entry", zap.String("entry", strings.TrimSpace(item)), zap.Error(err))
			continue
		}
		if _, exists := seen[normalized]; exists {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	return result
}

func parseManagedCerts(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string(nil), defaultKubeadmManagedCerts...)
	}

	entries := strings.Split(raw, ",")
	seen := make(map[string]struct{}, len(entries))
	result := make([]string, 0, len(entries))

	for _, item := range entries {
		name := strings.ToLower(strings.TrimSpace(item))
		if name == "" || name == "all" {
			continue
		}
		if _, ok := kubeadmCertFileMap[name]; !ok {
			logging.Z().Warn("skipping invalid CERT_MANAGED_CERTS entry", zap.String("entry", name))
			continue
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result = append(result, name)
	}

	if len(result) == 0 {
		return append([]string(nil), defaultKubeadmManagedCerts...)
	}

	return result
}

func normalizeCertificateMode(raw string) string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == certModeTLS {
		return certModeTLS
	}
	return certModeKubeadm
}

func normalizeCertificateTarget(raw string) (target string, host string, port string, err error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", "", "", fmt.Errorf("target cannot be empty")
	}

	v = strings.TrimPrefix(v, "https://")
	v = strings.TrimPrefix(v, "http://")
	if slash := strings.Index(v, "/"); slash >= 0 {
		v = v[:slash]
	}

	host = v
	port = "443"
	if strings.Contains(v, ":") {
		parsedHost, parsedPort, splitErr := net.SplitHostPort(v)
		if splitErr != nil {
			return "", "", "", fmt.Errorf("invalid target '%s': expected host[:port]", strings.TrimSpace(raw))
		}
		host = parsedHost
		port = parsedPort
	}

	host = strings.TrimSpace(host)
	port = strings.TrimSpace(port)
	if host == "" {
		return "", "", "", fmt.Errorf("target host cannot be empty")
	}

	portNum, convErr := strconv.Atoi(port)
	if convErr != nil || portNum < 1 || portNum > 65535 {
		return "", "", "", fmt.Errorf("invalid target port '%s'", port)
	}

	target = net.JoinHostPort(host, port)
	return target, host, port, nil
}

func buildRenewCommand(rawCommand, value string) ([]string, error) {
	command := strings.TrimSpace(rawCommand)
	if command == "" {
		return nil, fmt.Errorf("CERT_RENEW_COMMAND is empty")
	}

	parts := strings.Fields(command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("CERT_RENEW_COMMAND has no executable")
	}

	hasTargetPlaceholder := strings.Contains(command, "{{target}}")
	hasCertPlaceholder := strings.Contains(command, "{{cert}}")
	for i := range parts {
		parts[i] = strings.ReplaceAll(parts[i], "{{target}}", value)
		parts[i] = strings.ReplaceAll(parts[i], "{{cert}}", value)
	}

	if !hasTargetPlaceholder && !hasCertPlaceholder {
		parts = append(parts, value)
	}

	return parts, nil
}

func parseIntEnv(key string, fallback int) int {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}

	v, err := strconv.Atoi(value)
	if err != nil {
		logging.Z().Warn("invalid integer env var", zap.String("key", key), zap.Error(err), zap.Int("fallback", fallback))
		return fallback
	}
	return v
}
