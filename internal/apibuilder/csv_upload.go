package apibuilder

import (
	"bytes"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/xuri/excelize/v2"
)

func (h *APIBuilderHandler) UploadCSV(c *gin.Context) {
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "file required: " + err.Error()})
		return
	}
	defer file.Close()

	// Read file bytes for multi-format parsing
	fileBytes, err := io.ReadAll(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read file: " + err.Error()})
		return
	}

	ext := strings.ToLower(filepath.Ext(header.Filename))

	// Run SafeGate scanner pipeline on the uploaded file
	claimedType := header.Header.Get("Content-Type")
	scanInfo := &scanner.FileInfo{
		Filename:  header.Filename,
		Extension: ext,
		MIMEType:  claimedType,
		Size:      int64(len(fileBytes)),
		SHA256:    fmt.Sprintf("%x", sha256.Sum256(fileBytes)),
		Content:   fileBytes,
	}
	scanResult := h.scanOrch.Scan(scanInfo)
	if !scanResult.Safe {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":    "File failed security scan — upload rejected",
			"safe":     false,
			"findings": scanResult.Findings,
		})
		return
	}

	var headers []string
	var dataRows [][]string

	switch ext {
	case ".csv":
		reader := csv.NewReader(bytes.NewReader(fileBytes))
		reader.LazyQuotes = true
		reader.TrimLeadingSpace = true
		records, err := reader.ReadAll()
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid CSV: " + err.Error()})
			return
		}
		if len(records) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "CSV must have header row and at least one data row"})
			return
		}
		headers = records[0]
		dataRows = records[1:]

	case ".json":
		var jsonData []map[string]interface{}
		if err := json.Unmarshal(fileBytes, &jsonData); err != nil {
			// Try object with data array
			var wrapper map[string]interface{}
			if err2 := json.Unmarshal(fileBytes, &wrapper); err2 != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON: must be an array of objects or an object with a data array"})
				return
			}
			// Look for array fields
			found := false
			for _, v := range wrapper {
				if arr, ok := v.([]interface{}); ok && len(arr) > 0 {
					for _, item := range arr {
						if obj, ok := item.(map[string]interface{}); ok {
							jsonData = append(jsonData, obj)
						}
					}
					found = true
					break
				}
			}
			if !found || len(jsonData) == 0 {
				c.JSON(http.StatusBadRequest, gin.H{"error": "JSON must contain an array of objects"})
				return
			}
		}
		if len(jsonData) == 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "JSON array is empty"})
			return
		}
		// Extract headers from all keys
		keySet := map[string]bool{}
		for _, obj := range jsonData {
			for k := range obj {
				keySet[k] = true
			}
		}
		for k := range keySet {
			headers = append(headers, k)
		}
		sort.Strings(headers)
		// Convert to string rows
		for _, obj := range jsonData {
			row := make([]string, len(headers))
			for i, h := range headers {
				if v, ok := obj[h]; ok {
					row[i] = fmt.Sprintf("%v", v)
				}
			}
			dataRows = append(dataRows, row)
		}

	case ".xlsx", ".xls":
		f, err := excelize.OpenReader(bytes.NewReader(fileBytes))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid Excel file: " + err.Error()})
			return
		}
		defer f.Close()
		sheetName := f.GetSheetName(0)
		if sheetName == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Excel file has no sheets"})
			return
		}
		rows, err := f.GetRows(sheetName)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "failed to read Excel sheet: " + err.Error()})
			return
		}
		if len(rows) < 2 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "Excel sheet must have header row and at least one data row"})
			return
		}
		headers = rows[0]
		dataRows = rows[1:]

	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unsupported file type: " + ext + ". Supported: .csv, .json, .xlsx, .xls"})
		return
	}

	// Analyze column types
	colTypes := analyzeColumnTypes(headers, dataRows)

	// Check for geo data
	hasGeo := false
	for _, ct := range colTypes {
		if ct == "geo_lat" || ct == "geo_lng" || ct == "geo_name" {
			hasGeo = true
			break
		}
	}

	// Build sample data (first 10 rows)
	sampleSize := 10
	if len(dataRows) < sampleSize {
		sampleSize = len(dataRows)
	}
	sampleData := make([]map[string]interface{}, sampleSize)
	for i := 0; i < sampleSize; i++ {
		row := map[string]interface{}{}
		for j, hdr := range headers {
			if j < len(dataRows[i]) {
				row[hdr] = dataRows[i][j]
			}
		}
		sampleData[i] = row
	}

	id := "csv-" + uuid.New().String()[:8]
	now := time.Now()

	fileType := "csv"
	if ext == ".json" {
		fileType = "json"
	} else if ext == ".xlsx" || ext == ".xls" {
		fileType = "xlsx"
	}

	upload := &CSVUpload{
		ID:          id,
		Filename:    header.Filename,
		FileType:    fileType,
		Rows:        len(dataRows),
		Columns:     len(headers),
		ColumnNames: headers,
		ColumnTypes: colTypes,
		SampleData:  sampleData,
		HasGeoData:  hasGeo,
		Status:      "analyzed",
		CreatedAt:   now,
	}

	// Reconstruct records in CSV format for internal storage
	records := make([][]string, 0, len(dataRows)+1)
	records = append(records, headers)
	records = append(records, dataRows...)

	h.mu.Lock()
	h.csvUploads[id] = upload
	h.csvData[id] = records
	h.persistStateLocked()
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{
		"status":          "success",
		"upload":          upload,
		"message":         fmt.Sprintf("%s file analyzed. Call POST /generate-dashboard to create an analytics dashboard.", strings.ToUpper(fileType)),
		"can_convert_gis": hasGeo,
		"scan_safe":       scanResult.Safe,
		"scan_findings":   len(scanResult.Findings),
	})
}

func (h *APIBuilderHandler) ListCSVUploads(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	result := make([]*CSVUpload, 0, len(h.csvUploads))
	for _, u := range h.csvUploads {
		result = append(result, u)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].CreatedAt.After(result[j].CreatedAt) })

	c.JSON(http.StatusOK, gin.H{"status": "success", "count": len(result), "uploads": result})
}

func (h *APIBuilderHandler) GetCSVUpload(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	id := c.Param("id")
	u, ok := h.csvUploads[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "upload not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "upload": u})
}

func (h *APIBuilderHandler) DeleteCSVUpload(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := c.Param("id")
	if _, ok := h.csvUploads[id]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "upload not found"})
		return
	}
	delete(h.csvUploads, id)
	delete(h.csvData, id)
	h.persistStateLocked()

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "upload deleted"})
}

// GenerateDashboard creates an analytics dashboard from an uploaded CSV
func (h *APIBuilderHandler) GenerateDashboard(c *gin.Context) {
	id := c.Param("id")

	h.mu.RLock()
	upload, ok := h.csvUploads[id]
	rawData, hasRaw := h.csvData[id]
	h.mu.RUnlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "upload not found"})
		return
	}
	if !hasRaw || len(rawData) < 2 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "no data available for dashboard generation"})
		return
	}

	headers := rawData[0]
	dataRows := rawData[1:]

	// Generate dashboard with auto-detected widgets
	dashID := "csv-dash-" + uuid.New().String()[:8]
	now := time.Now()
	widgets := generateWidgetsFromCSV(headers, upload.ColumnTypes, dataRows)

	dashboard := &AnalyticsDashboard{
		ID:          dashID,
		Name:        "CSV: " + upload.Filename,
		Description: fmt.Sprintf("Auto-generated from %s (%d rows, %d columns)", upload.Filename, upload.Rows, upload.Columns),
		Category:    "csv-import",
		Widgets:     widgets,
		Filters:     generateFiltersFromCSV(headers, upload.ColumnTypes),
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Register in analytics handler
	h.analyticsHandler.mu.Lock()
	h.analyticsHandler.dashboards[dashID] = dashboard
	h.analyticsHandler.mu.Unlock()

	// Update upload record
	h.mu.Lock()
	upload.DashboardID = dashID
	upload.Status = "dashboard_created"
	h.generatedDashboards[dashID] = dashboard
	h.persistStateLocked()
	h.mu.Unlock()

	c.JSON(http.StatusCreated, gin.H{
		"status":       "success",
		"dashboard_id": dashID,
		"dashboard":    dashboard,
		"message":      fmt.Sprintf("Dashboard created with %d auto-generated widgets", len(widgets)),
	})
}

// ===================================================================
// Dashboard <-> GIS Conversion
// ===================================================================

// AnalyzeConversion checks if a dashboard can be converted to GIS or vice versa
