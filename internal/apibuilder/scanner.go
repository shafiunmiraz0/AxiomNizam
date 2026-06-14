package apibuilder

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apiscanner"
	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func (h *APIBuilderHandler) ScanFile(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required: " + err.Error()})
		return
	}
	defer file.Close()

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read file: " + err.Error()})
		return
	}

	// Build SHA256
	hash := sha256.Sum256(fileBytes)
	sha := fmt.Sprintf("%x", hash)

	// Build FileInfo for scanner
	claimedType := header.Header.Get("Content-Type")
	info := &scanner.FileInfo{
		Filename:  header.Filename,
		Extension: strings.ToLower(filepath.Ext(header.Filename)),
		MIMEType:  claimedType,
		Size:      int64(len(fileBytes)),
		SHA256:    sha,
		Content:   fileBytes,
	}

	result := h.scanOrch.Scan(info)

	record := &FileScanRecord{
		ID:        "scan-" + uuid.New().String()[:8],
		Filename:  header.Filename,
		FileSize:  int64(len(fileBytes)),
		SHA256:    sha,
		Safe:      result.Safe,
		Findings:  result.Findings,
		ScannedAt: time.Now(),
	}

	h.mu.Lock()
	h.scanRecords[record.ID] = record
	h.persistStateLocked()
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"status":   "success",
		"scan":     record,
		"safe":     result.Safe,
		"findings": len(result.Findings),
	})
}

// ListScans returns all file scan records
func (h *APIBuilderHandler) ListScans(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*FileScanRecord, 0, len(h.scanRecords))
	for _, r := range h.scanRecords {
		result = append(result, r)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].ScannedAt.After(result[j].ScannedAt) })

	c.JSON(http.StatusOK, gin.H{"status": "success", "count": len(result), "scans": result})
}

// GetScan returns a single file scan report by id
func (h *APIBuilderHandler) GetScan(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "scan id is required"})
		return
	}

	record, ok := h.scanRecords[id]
	if !ok || record == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "scan report not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "scan": record})
}

// GetScannerHealth returns the scanner pipeline health status and metrics.
func (h *APIBuilderHandler) GetScannerHealth(c *gin.Context) {
	includeMetrics := c.Query("metrics") == "true"
	health := h.scanOrch.Health(includeMetrics)
	c.JSON(http.StatusOK, gin.H{
		"status":  "success",
		"health":  health,
	})
}

// ===================================================================
// API Scanner Reports
// ===================================================================

type apiScanRunRequest struct {
	ScanType           string            `json:"scan_type"`
	Target             string            `json:"target"`
	Method             string            `json:"method"`
	Headers            map[string]string `json:"headers"`
	Body               string            `json:"body"`
	TimeoutSeconds     int               `json:"timeout_seconds"`
	RetryCount         int               `json:"retry_count"`
	RetryBackoffMS     int               `json:"retry_backoff_ms"`
	InsecureSkipVerify bool              `json:"insecure_skip_verify"`
	AuthHeader         string            `json:"auth_header"`
	AuthValue          string            `json:"auth_value"`
	MaxPaths           int               `json:"max_paths"`
	MaxSubdomains      int               `json:"max_subdomains"`
	MaxHints           int               `json:"max_hints"`
	Schemes            []string          `json:"schemes"`
	IncludeScanIDs     []string          `json:"include_scan_ids"`
	ExcludeScanIDs     []string          `json:"exclude_scan_ids"`
}

type apiScanBulkDeleteRequest struct {
	IDs []string `json:"ids"`
	All bool     `json:"all"`
}

type sqlAssistantChatRequest struct {
	Message    string `json:"message"`
	Dialect    string `json:"dialect"`
	SchemaHint string `json:"schema_hint"`
	Context    string `json:"context"`
}

type sqlAssistantSuggestion struct {
	Title string `json:"title"`
	SQL   string `json:"sql"`
	Notes string `json:"notes,omitempty"`
}

type sqlAssistantChatResponse struct {
	Provider    string                   `json:"provider"`
	Reply       string                   `json:"reply"`
	Suggestions []sqlAssistantSuggestion `json:"suggestions"`
	Warning     string                   `json:"warning,omitempty"`
}

func (r *apiScanRunRequest) normalize() (string, string, map[string]string, time.Duration, time.Duration, error) {
	target := strings.TrimSpace(r.Target)
	if target == "" {
		return "", "", nil, 0, 0, fmt.Errorf("target is required")
	}

	scanType := strings.ToLower(strings.TrimSpace(r.ScanType))
	if scanType == "" {
		scanType = "runtime"
	}

	if r.TimeoutSeconds <= 0 {
		r.TimeoutSeconds = 30
	}
	if r.RetryBackoffMS <= 0 {
		r.RetryBackoffMS = 1000
	}
	if r.MaxPaths <= 0 {
		r.MaxPaths = 64
	}
	if r.MaxSubdomains <= 0 {
		r.MaxSubdomains = 32
	}
	if r.MaxHints <= 0 {
		r.MaxHints = 48
	}

	headers := r.Headers
	if headers == nil {
		headers = map[string]string{}
	}

	timeout := time.Duration(r.TimeoutSeconds) * time.Second
	retryBackoff := time.Duration(r.RetryBackoffMS) * time.Millisecond
	return scanType, target, headers, timeout, retryBackoff, nil
}

func buildRuntimeAPIScanReport(ctx context.Context, req apiScanRunRequest, target string, headers map[string]string, timeout time.Duration, retryBackoff time.Duration, report *APIScanReport) error {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	report.Method = method
	authHeader := strings.TrimSpace(req.AuthHeader)
	authValue := strings.TrimSpace(req.AuthValue)
	if authHeader == "" && authValue == "" {
		for key, value := range headers {
			if strings.EqualFold(strings.TrimSpace(key), "Authorization") {
				authHeader = "Authorization"
				authValue = strings.TrimSpace(value)
				break
			}
		}
	}

	engine := apiscanner.NewEngine()
	result, err := engine.Scan(ctx, apiscanner.ScanRequest{
		Endpoint: apiscanner.Endpoint{
			URL:     target,
			Method:  method,
			Body:    req.Body,
			Headers: headers,
		},
		Timeout:            timeout,
		RetryCount:         req.RetryCount,
		RetryBackoff:       retryBackoff,
		InsecureSkipVerify: req.InsecureSkipVerify,
		AuthHeader:         authHeader,
		AuthValue:          authValue,
		Format:             apiscanner.FormatJSON,
	})
	if err != nil {
		return fmt.Errorf("runtime API scan failed: %w", err)
	}

	report.Summary = result.Summary
	report.Result = result
	return nil
}

func buildDiscoverAPIScanReport(ctx context.Context, req apiScanRunRequest, target string, headers map[string]string, timeout time.Duration, report *APIScanReport) error {
	result, err := apiscanner.DiscoverAPI(ctx, apiscanner.DiscoverRequest{
		Target:             target,
		Headers:            headers,
		Timeout:            timeout,
		InsecureSkipVerify: req.InsecureSkipVerify,
		MaxPaths:           req.MaxPaths,
		IncludeIDs:         req.IncludeScanIDs,
		ExcludeIDs:         req.ExcludeScanIDs,
	})
	if err != nil {
		return fmt.Errorf("API discovery failed: %w", err)
	}

	report.Summary = result.Summary
	report.Result = result
	return nil
}

func buildDiscoverDomainScanReport(ctx context.Context, req apiScanRunRequest, target string, headers map[string]string, timeout time.Duration, report *APIScanReport) error {
	result, err := apiscanner.DiscoverDomain(ctx, apiscanner.DiscoverDomainRequest{
		Target:             target,
		Headers:            headers,
		Timeout:            timeout,
		InsecureSkipVerify: req.InsecureSkipVerify,
		MaxSubdomains:      req.MaxSubdomains,
		MaxHints:           req.MaxHints,
		Schemes:            req.Schemes,
		IncludeIDs:         req.IncludeScanIDs,
		ExcludeIDs:         req.ExcludeScanIDs,
	})
	if err != nil {
		return fmt.Errorf("domain discovery failed: %w", err)
	}

	report.Summary = result.Summary
	report.Result = result
	return nil
}

func (h *APIBuilderHandler) ScanAPI(c *gin.Context) {
	var req apiScanRunRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "details": err.Error()})
		return
	}

	scanType, target, headers, timeout, retryBackoff, err := req.normalize()
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	report := &APIScanReport{
		ID:        "api-report-" + uuid.New().String()[:8],
		ScanType:  scanType,
		Target:    target,
		CreatedAt: time.Now(),
	}

	ctx := c.Request.Context()
	buildErr := error(nil)
	switch scanType {
	case "runtime", "api", "scan-api":
		buildErr = buildRuntimeAPIScanReport(ctx, req, target, headers, timeout, retryBackoff, report)
	case "discover-api", "api-discovery":
		buildErr = buildDiscoverAPIScanReport(ctx, req, target, headers, timeout, report)
	case "discover-domain", "domain-discovery":
		buildErr = buildDiscoverDomainScanReport(ctx, req, target, headers, timeout, report)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported scan_type", "supported": []string{"runtime", "discover-api", "discover-domain"}})
		return
	}

	if buildErr != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": buildErr.Error()})
		return
	}

	h.mu.Lock()
	h.apiScanReports[report.ID] = report
	h.persistStateLocked()
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"status": "success", "report": report})
}

func (h *APIBuilderHandler) ListAPIScanReports(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	reports := make([]*APIScanReport, 0, len(h.apiScanReports))
	for _, report := range h.apiScanReports {
		reports = append(reports, report)
	}
	sort.Slice(reports, func(i, j int) bool { return reports[i].CreatedAt.After(reports[j].CreatedAt) })

	c.JSON(http.StatusOK, gin.H{"status": "success", "count": len(reports), "reports": reports})
}

func (h *APIBuilderHandler) GetAPIScanReport(c *gin.Context) {
	id := c.Param("id")

	h.mu.RLock()
	report, ok := h.apiScanReports[id]
	h.mu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "API scan report not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "report": report})
}

func (h *APIBuilderHandler) DeleteAPIScanReport(c *gin.Context) {
	id := strings.TrimSpace(c.Param("id"))
	if id == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "report id is required"})
		return
	}

	h.mu.Lock()
	if _, ok := h.apiScanReports[id]; !ok {
		h.mu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"error": "API scan report not found"})
		return
	}
	delete(h.apiScanReports, id)
	h.persistStateLocked()
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "API scan report deleted", "id": id})
}

func (h *APIBuilderHandler) BulkDeleteAPIScanReports(c *gin.Context) {
	var req apiScanBulkDeleteRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request payload", "details": err.Error()})
		return
	}

	normalizedIDs := make([]string, 0)
	if !req.All {
		normalizedIDs = normalizeAPIScanReportIDs(req.IDs)
		if len(normalizedIDs) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "ids are required when all=false"})
			return
		}
	}

	h.mu.Lock()
	defer h.mu.Unlock()

	var deletedIDs []string
	if req.All {
		deletedIDs = h.deleteAllAPIScanReportsLocked()
	} else {
		deletedIDs = h.deleteAPIScanReportsByIDsLocked(normalizedIDs)
	}

	if len(deletedIDs) > 0 {
		h.persistStateLocked()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          "success",
		"deleted_count":   len(deletedIDs),
		"deleted_ids":     deletedIDs,
		"total_remaining": len(h.apiScanReports),
	})
}

func normalizeAPIScanReportIDs(rawIDs []string) []string {
	normalized := make([]string, 0, len(rawIDs))
	seen := make(map[string]struct{}, len(rawIDs))
	for _, rawID := range rawIDs {
		id := strings.TrimSpace(rawID)
		if id == "" {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		normalized = append(normalized, id)
	}
	return normalized
}

func (h *APIBuilderHandler) deleteAllAPIScanReportsLocked() []string {
	deletedIDs := make([]string, 0, len(h.apiScanReports))
	for id := range h.apiScanReports {
		deletedIDs = append(deletedIDs, id)
	}
	h.apiScanReports = make(map[string]*APIScanReport)
	return deletedIDs
}

func (h *APIBuilderHandler) deleteAPIScanReportsByIDsLocked(ids []string) []string {
	deletedIDs := make([]string, 0, len(ids))
	for _, id := range ids {
		if _, ok := h.apiScanReports[id]; !ok {
			continue
		}
		delete(h.apiScanReports, id)
		deletedIDs = append(deletedIDs, id)
	}
	return deletedIDs
}

