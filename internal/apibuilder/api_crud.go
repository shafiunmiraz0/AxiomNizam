package apibuilder

import (
	"fmt"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"example.com/axiomnizam/internal/logging"
	"example.com/axiomnizam/internal/sqlfilter"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"go.uber.org/zap"
)

func (h *APIBuilderHandler) ListAPIs(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	category := c.Query("category")
	status := c.Query("status")
	apiType := strings.ToLower(strings.TrimSpace(c.Query("api_type")))

	result := make([]*CustomAPI, 0, len(h.customAPIs))
	for _, api := range h.customAPIs {
		currentType := strings.ToLower(strings.TrimSpace(api.APIType))
		if currentType == "" {
			currentType = "rest"
		}
		if apiType != "" && currentType != apiType {
			continue
		}
		if category != "" && api.Category != category {
			continue
		}
		if status != "" && api.Status != status {
			continue
		}
		result = append(result, api)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })

	c.JSON(http.StatusOK, gin.H{"status": "success", "count": len(result), "apis": result})
}

func (h *APIBuilderHandler) GetAPI(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	id := c.Param("id")
	api, ok := h.customAPIs[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "api not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "api": api})
}

func (h *APIBuilderHandler) CreateAPI(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	var req struct {
		APIType        string            `json:"api_type"`
		Name           string            `json:"name" binding:"required"`
		Method         string            `json:"method"`
		Path           string            `json:"path"`
		SQLTemplate    string            `json:"sql_template"`
		SQLPolicyMode  string            `json:"sql_policy_mode"`
		GraphQLQuery   string            `json:"graphql_query"`
		GraphQLOpName  string            `json:"graphql_operation_name"`
		Description    string            `json:"description"`
		Category       string            `json:"category"`
		SourceDatabase string            `json:"source_database"`
		SourceServer   string            `json:"source_server"`
		AuthRequired   bool              `json:"auth_required"`
		RateLimit      int               `json:"rate_limit"`
		RequestSchema  *SchemaDefinition `json:"request_schema"`
		ResponseSchema *SchemaDefinition `json:"response_schema"`
		MockResponse   interface{}       `json:"mock_response"`
		Headers        map[string]string `json:"headers"`
		QueryParams    []ParamDef        `json:"query_params"`
		CacheEnabled   bool              `json:"cache_enabled"`
		CacheTTL       int               `json:"cache_ttl"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	apiType := strings.ToLower(strings.TrimSpace(req.APIType))
	if apiType == "" {
		apiType = "rest"
	}
	if apiType != "rest" && apiType != "graphql" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "api_type must be rest or graphql"})
		return
	}

	policyMode, policyModeValid := normalizeSQLPolicyMode(req.SQLPolicyMode)
	if !policyModeValid {
		c.JSON(http.StatusBadRequest, gin.H{"error": "sql_policy_mode must be compat or strict"})
		return
	}
	if policyMode == "" {
		policyMode = "compat"
	}

	method := strings.ToUpper(strings.TrimSpace(req.Method))
	path := strings.TrimSpace(req.Path)

	if apiType == "rest" {
		if method == "" || path == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "method and path are required for REST APIs"})
			return
		}
		if method != "GET" && method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "method must be GET, POST, PUT, DELETE, or PATCH"})
			return
		}

		if existing := h.findDuplicateRESTEndpointLocked(method, path, ""); existing != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":             "endpoint already exists",
				"method":            strings.ToUpper(strings.TrimSpace(method)),
				"path":              normalizeBuilderRuntimePath(path),
				"existing_api_id":   existing.ID,
				"existing_api_name": existing.Name,
			})
			return
		}

		sqlTemplate := strings.TrimSpace(req.SQLTemplate)
		if sqlTemplate == "" && strings.TrimSpace(req.SourceDatabase) != "" {
			legacyTemplate, _ := extractQueryFromMock(req.MockResponse)
			sqlTemplate = strings.TrimSpace(legacyTemplate)
		}
		if strings.TrimSpace(req.SourceDatabase) != "" && sqlTemplate == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sql_template is required when source_database is set"})
			return
		}
		if sqlTemplate != "" && !isStrictReadOnlyQuery(sqlTemplate, policyMode) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sql_template must be read-only (SELECT/WITH/SHOW/DESCRIBE/EXPLAIN/INSERT)"})
			return
		}
		req.SQLTemplate = sqlTemplate
	} else {
		if method == "" {
			method = "POST"
		}
		if path == "" {
			path = "/api/graphql"
		}
		if strings.TrimSpace(req.GraphQLQuery) == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "graphql_query is required for GraphQL APIs"})
			return
		}

		if existing := h.findDuplicateGraphQLEndpointLocked(method, path, ""); existing != nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":             "endpoint already exists",
				"api_type":          "graphql",
				"method":            strings.ToUpper(strings.TrimSpace(method)),
				"path":              normalizeBuilderRuntimePath(path),
				"existing_api_id":   existing.ID,
				"existing_api_name": existing.Name,
			})
			return
		}
	}

	id := "api-" + uuid.New().String()[:8]
	now := time.Now()

	ttl := req.CacheTTL
	if req.CacheEnabled && ttl <= 0 {
		ttl = 300 // default 5 minutes
	}

	api := &CustomAPI{
		ID:             id,
		APIType:        apiType,
		Name:           req.Name,
		Method:         method,
		Path:           path,
		SQLTemplate:    strings.TrimSpace(req.SQLTemplate),
		SQLPolicyMode:  policyMode,
		GraphQLQuery:   strings.TrimSpace(req.GraphQLQuery),
		GraphQLOpName:  strings.TrimSpace(req.GraphQLOpName),
		Description:    req.Description,
		Category:       req.Category,
		SourceDatabase: strings.TrimSpace(req.SourceDatabase),
		SourceServer:   strings.TrimSpace(req.SourceServer),
		AuthRequired:   req.AuthRequired,
		RateLimit:      req.RateLimit,
		CacheEnabled:   req.CacheEnabled,
		CacheTTL:       ttl,
		RequestSchema:  req.RequestSchema,
		ResponseSchema: req.ResponseSchema,
		MockResponse:   req.MockResponse,
		Headers:        req.Headers,
		QueryParams:    req.QueryParams,
		Status:         "active",
		CreatedBy:      "admin",
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	h.customAPIs[id] = api
	h.apiData[id] = make([]map[string]interface{}, 0)
	h.persistStateLocked()

	c.JSON(http.StatusCreated, gin.H{"status": "success", "api": api})
}

func (h *APIBuilderHandler) UpdateAPI(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := c.Param("id")
	api, ok := h.customAPIs[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "api not found"})
		return
	}
	if strings.TrimSpace(api.APIType) == "" {
		api.APIType = "rest"
	}
	originalAPIType := strings.ToLower(strings.TrimSpace(api.APIType))
	if originalAPIType == "" {
		originalAPIType = "rest"
	}
	originalEndpointSignature := ""
	if originalAPIType == "rest" {
		originalEndpointSignature = normalizeRESTEndpointSignature(api.Method, api.Path)
	} else if originalAPIType == "graphql" {
		originalEndpointSignature = normalizeGraphQLEndpointSignature(api.Method, api.Path)
	}

	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if v, ok := req["name"].(string); ok && v != "" {
		api.Name = v
	}
	if v, ok := req["api_type"].(string); ok {
		vt := strings.ToLower(strings.TrimSpace(v))
		if vt != "" && vt != "rest" && vt != "graphql" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "api_type must be rest or graphql"})
			return
		}
		if vt != "" {
			api.APIType = vt
		}
	}
	if v, ok := req["method"].(string); ok && strings.TrimSpace(v) != "" {
		method := strings.ToUpper(strings.TrimSpace(v))
		if method != "GET" && method != "POST" && method != "PUT" && method != "DELETE" && method != "PATCH" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "method must be GET, POST, PUT, DELETE, or PATCH"})
			return
		}
		api.Method = method
	}
	if v, ok := req["path"].(string); ok && strings.TrimSpace(v) != "" {
		api.Path = strings.TrimSpace(v)
	}
	if v, ok := req["endpoint"].(string); ok && strings.TrimSpace(v) != "" {
		api.Path = strings.TrimSpace(v)
	}
	if v, ok := req["description"].(string); ok {
		api.Description = v
	}
	if v, ok := req["status"].(string); ok {
		api.Status = v
	}
	if v, ok := req["category"].(string); ok {
		api.Category = v
	}
	if v, ok := req["source_database"].(string); ok {
		api.SourceDatabase = strings.TrimSpace(v)
	}
	if v, ok := req["source_server"].(string); ok {
		api.SourceServer = strings.TrimSpace(v)
	}
	if v, ok := req["graphql_query"].(string); ok {
		api.GraphQLQuery = strings.TrimSpace(v)
	}
	if v, ok := req["graphql_operation_name"].(string); ok {
		api.GraphQLOpName = strings.TrimSpace(v)
	}
	if v, ok := req["sql_policy_mode"].(string); ok {
		mode, valid := normalizeSQLPolicyMode(v)
		if !valid {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sql_policy_mode must be compat or strict"})
			return
		}
		if mode == "" {
			mode = "compat"
		}
		api.SQLPolicyMode = mode
	}
	if v, ok := req["sql_template"].(string); ok {
		template := strings.TrimSpace(v)
		if template != "" && !isStrictReadOnlyQuery(template, resolveSQLPolicyMode(api.SQLPolicyMode)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sql_template must be read-only (SELECT/WITH/SHOW/DESCRIBE/EXPLAIN/INSERT)"})
			return
		}
		api.SQLTemplate = template
	}
	if v, ok := req["auth_required"].(bool); ok {
		api.AuthRequired = v
	}
	if v, ok := req["rate_limit"].(float64); ok {
		api.RateLimit = int(v)
	}
	if v, ok := req["cache_enabled"].(bool); ok {
		api.CacheEnabled = v
		if !v {
			api.cachedResult = nil
		}
	}
	if v, ok := req["cache_ttl"].(float64); ok {
		api.CacheTTL = int(v)
	}
	currentAPIType := strings.ToLower(strings.TrimSpace(api.APIType))
	if currentAPIType == "" {
		currentAPIType = "rest"
		api.APIType = "rest"
	}

	if currentAPIType == "graphql" {
		if strings.TrimSpace(api.Path) == "" {
			api.Path = "/api/graphql"
		}
		if strings.TrimSpace(api.Method) == "" {
			api.Method = "POST"
		}
	} else if strings.TrimSpace(api.SourceDatabase) != "" {
		template := strings.TrimSpace(api.SQLTemplate)
		if template == "" {
			legacyTemplate, _ := extractQueryFromMock(api.MockResponse)
			template = strings.TrimSpace(legacyTemplate)
		}
		if template == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sql_template is required when source_database is set"})
			return
		}
		if !isStrictReadOnlyQuery(template, resolveSQLPolicyMode(api.SQLPolicyMode)) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "sql_template must be read-only (SELECT/WITH/SHOW/DESCRIBE/EXPLAIN/INSERT)"})
			return
		}
		api.SQLTemplate = template
	}

	if currentAPIType == "rest" && strings.TrimSpace(api.Method) != "" && strings.TrimSpace(api.Path) != "" {
		currentEndpointSignature := normalizeRESTEndpointSignature(api.Method, api.Path)
		endpointChanged := originalAPIType != "rest" || currentEndpointSignature != originalEndpointSignature
		if endpointChanged {
			if existing := h.findDuplicateRESTEndpointLocked(api.Method, api.Path, api.ID); existing != nil {
				c.JSON(http.StatusConflict, gin.H{
					"error":             "endpoint already exists",
					"method":            strings.ToUpper(strings.TrimSpace(api.Method)),
					"path":              normalizeBuilderRuntimePath(api.Path),
					"existing_api_id":   existing.ID,
					"existing_api_name": existing.Name,
				})
				return
			}
		}
	}

	if currentAPIType == "graphql" && strings.TrimSpace(api.Path) != "" {
		currentEndpointSignature := normalizeGraphQLEndpointSignature(api.Method, api.Path)
		endpointChanged := originalAPIType != "graphql" || currentEndpointSignature != originalEndpointSignature
		if endpointChanged {
			if existing := h.findDuplicateGraphQLEndpointLocked(api.Method, api.Path, api.ID); existing != nil {
				c.JSON(http.StatusConflict, gin.H{
					"error":             "endpoint already exists",
					"api_type":          "graphql",
					"method":            strings.ToUpper(strings.TrimSpace(api.Method)),
					"path":              normalizeBuilderRuntimePath(api.Path),
					"existing_api_id":   existing.ID,
					"existing_api_name": existing.Name,
				})
				return
			}
		}
	}

	api.UpdatedAt = time.Now()
	h.persistStateLocked()

	c.JSON(http.StatusOK, gin.H{"status": "success", "api": api})
}

func (h *APIBuilderHandler) DeleteAPI(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := c.Param("id")
	if _, ok := h.customAPIs[id]; !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "api not found"})
		return
	}
	delete(h.customAPIs, id)
	delete(h.apiData, id)
	h.persistStateLocked()

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "api deleted"})
}

// TestAPI executes a mock call against a custom API and returns the mock response
func (h *APIBuilderHandler) TestAPI(c *gin.Context) {
	h.mu.Lock()
	defer h.mu.Unlock()

	id := c.Param("id")
	api, ok := h.customAPIs[id]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "api not found"})
		return
	}
	api.HitCount++

	// Cache check
	if api.CacheEnabled && api.cachedResult != nil {
		ttl := time.Duration(api.CacheTTL) * time.Second
		if ttl <= 0 {
			ttl = 300 * time.Second
		}
		if time.Since(api.cachedAt) < ttl {
			c.JSON(http.StatusOK, gin.H{
				"status":    "success",
				"api_id":    api.ID,
				"api_type":  api.APIType,
				"method":    api.Method,
				"path":      api.Path,
				"operation": api.GraphQLOpName,
				"cached":    true,
				"cache_ttl": api.CacheTTL,
				"response":  api.cachedResult,
			})
			return
		}
	}

	var response interface{}
	if api.MockResponse != nil {
		response = api.MockResponse
	} else {
		response = gin.H{
			"message": "API endpoint active, no mock response configured",
			"data":    h.apiData[id],
		}
	}

	// Store in cache if enabled
	if api.CacheEnabled {
		api.cachedResult = response
		api.cachedAt = time.Now()
	}

	c.JSON(http.StatusOK, gin.H{
		"status":    "success",
		"api_id":    api.ID,
		"api_type":  api.APIType,
		"method":    api.Method,
		"path":      api.Path,
		"operation": api.GraphQLOpName,
		"cached":    false,
		"cache_ttl": api.CacheTTL,
		"response":  response,
	})
}

// InvokeCustomAPI executes runtime calls for builder-created REST APIs.
// Routes are mounted under /api/custom/* and resolved against saved API definitions.
func (h *APIBuilderHandler) InvokeCustomAPI(c *gin.Context) {
	method := strings.ToUpper(strings.TrimSpace(c.Request.Method))
	path := strings.TrimSpace(c.Request.URL.Path)

	h.mu.Lock()
	defer h.mu.Unlock()

	pathMatches := make([]*CustomAPI, 0)
	for _, candidate := range h.customAPIs {
		if strings.ToLower(strings.TrimSpace(candidate.APIType)) != "rest" {
			continue
		}
		if !matchBuilderPath(candidate.Path, path) {
			continue
		}
		pathMatches = append(pathMatches, candidate)
	}

	if len(pathMatches) == 0 {
		c.JSON(http.StatusNotFound, gin.H{
			"error":            "custom api not found",
			"requested_path":   path,
			"requested_method": method,
		})
		return
	}

	var api *CustomAPI
	allowedMethods := make([]string, 0)
	for _, candidate := range pathMatches {
		candidateMethod := strings.ToUpper(strings.TrimSpace(candidate.Method))
		allowedMethods = append(allowedMethods, candidateMethod)
		if candidateMethod == method {
			api = candidate
			break
		}
	}

	if api == nil {
		c.JSON(http.StatusMethodNotAllowed, gin.H{
			"error":            "method not allowed for custom api",
			"requested_path":   path,
			"requested_method": method,
			"allowed_methods":  allowedMethods,
		})
		return
	}

	if strings.ToLower(strings.TrimSpace(api.Status)) != "active" {
		c.JSON(http.StatusForbidden, gin.H{"error": "custom api is not active"})
		return
	}

	if allowed, retryAfter := enforceCustomAPIRateLimit(c, api); !allowed {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"error":               "custom api rate limit exceeded",
			"api_id":              api.ID,
			"rate_limit_per_min":  api.RateLimit,
			"retry_after_seconds": retryAfter,
		})
		return
	}

	api.HitCount++

	if api.CacheEnabled && api.cachedResult != nil {
		ttl := time.Duration(api.CacheTTL) * time.Second
		if ttl <= 0 {
			ttl = 300 * time.Second
		}
		if time.Since(api.cachedAt) < ttl {
			writeCustomAPISuccess(c, normalizeCustomAPISuccessData(api.cachedResult))
			return
		}
	}

	query := resolveStoredSQLTemplate(api)
	params := make([]interface{}, 0)
	if query != "" {
		if !isStrictReadOnlyQuery(query, resolveSQLPolicyMode(api.SQLPolicyMode)) {
			c.JSON(http.StatusForbidden, gin.H{"error": "custom api sql_template must be read-only"})
			return
		}

		placeholderCount := countSQLPlaceholders(query)
		if placeholderCount > 0 {
			paramDefs := api.QueryParams
			if len(paramDefs) > placeholderCount {
				paramDefs = paramDefs[:placeholderCount]
			}
			if len(paramDefs) > 0 && len(paramDefs) < placeholderCount {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":                    "insufficient query_params definitions for sql_template placeholders",
					"required_placeholders":    placeholderCount,
					"defined_query_parameters": len(paramDefs),
				})
				return
			}

			extractedParams, paramErr := extractRuntimeParamsFromRequest(c, method, paramDefs)
			if paramErr != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": paramErr.Error()})
				return
			}
			params = extractedParams
			if len(params) != placeholderCount {
				c.JSON(http.StatusBadRequest, gin.H{
					"error":               "parameter count does not match sql_template placeholders",
					"expected_parameters": placeholderCount,
					"received_parameters": len(params),
				})
				return
			}
		}
	}

	var response interface{}
	if query != "" {
		// Prefer source_server (specific server key) over source_database (db type)
		lookupKey := strings.ToLower(strings.TrimSpace(api.SourceServer))
		if lookupKey == "" {
			lookupKey = strings.ToLower(strings.TrimSpace(api.SourceDatabase))
		}
		dbConn := h.db[lookupKey]
		if lookupKey == "" || dbConn == nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":           "source database is not configured for this custom api",
				"source_server":   api.SourceServer,
				"source_database": api.SourceDatabase,
			})
			return
		}

		rows, err := dbConn.Raw(query, params...).Rows()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "query execution failed",
				"detail": err.Error(),
			})
			return
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":  "failed to get result columns",
				"detail": err.Error(),
			})
			return
		}

		result := make([]map[string]interface{}, 0)
		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range columns {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"error":  "failed to scan result row",
					"detail": err.Error(),
				})
				return
			}

			entry := make(map[string]interface{})
			for i, col := range columns {
				val := values[i]
				if b, ok := val.([]byte); ok {
					entry[col] = string(b)
				} else {
					entry[col] = val
				}
			}
			result = append(result, entry)
		}

		response = result
	} else if api.MockResponse != nil {
		response = api.MockResponse
	} else {
		response = h.apiData[api.ID]
	}

	response = normalizeCustomAPISuccessData(response)

	if api.CacheEnabled {
		api.cachedResult = response
		api.cachedAt = time.Now()
	}

	writeCustomAPISuccess(c, response)
}

func writeCustomAPISuccess(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, customAPISuccessEnvelope{
		Message:      "success",
		ResponseCode: http.StatusOK,
		Success:      true,
		Data:         data,
	})
}

func normalizeCustomAPISuccessData(raw interface{}) interface{} {
	switch typed := raw.(type) {
	case gin.H:
		if data, ok := typed["data"]; ok {
			return data
		}
	case map[string]interface{}:
		if data, ok := typed["data"]; ok {
			return data
		}
	}

	return raw
}

func matchBuilderPath(pattern, actual string) bool {
	pattern = normalizeBuilderRuntimePath(pattern)
	actual = normalizeBuilderRuntimePath(actual)

	if pattern == actual {
		return true
	}

	patternParts := strings.Split(strings.Trim(pattern, "/"), "/")
	actualParts := strings.Split(strings.Trim(actual, "/"), "/")

	if len(patternParts) != len(actualParts) {
		return false
	}

	for i := 0; i < len(patternParts); i++ {
		segment := strings.TrimSpace(patternParts[i])
		if strings.HasPrefix(segment, ":") {
			continue
		}
		if segment != strings.TrimSpace(actualParts[i]) {
			return false
		}
	}

	return true
}

func normalizeBuilderRuntimePath(path string) string {
	normalized := "/" + strings.Trim(strings.TrimSpace(path), "/")
	if normalized == "/api/custom" {
		return "/"
	}
	if strings.HasPrefix(normalized, "/api/custom/") {
		trimmed := strings.TrimPrefix(normalized, "/api/custom")
		if trimmed == "" {
			return "/"
		}
		return "/" + strings.Trim(strings.TrimSpace(trimmed), "/")
	}
	return normalized
}

func normalizeRESTEndpointSignature(method, path string) string {
	return strings.ToUpper(strings.TrimSpace(method)) + " " + normalizeBuilderRuntimePath(path)
}

func normalizeGraphQLEndpointSignature(method, path string) string {
	normalizedMethod := strings.ToUpper(strings.TrimSpace(method))
	if normalizedMethod == "" {
		normalizedMethod = "POST"
	}

	normalizedPath := strings.TrimSpace(path)
	if normalizedPath == "" {
		normalizedPath = "/api/graphql"
	}

	return normalizedMethod + " " + normalizeBuilderRuntimePath(normalizedPath)
}

func (h *APIBuilderHandler) findDuplicateRESTEndpointLocked(method, path, excludeID string) *CustomAPI {
	targetSignature := normalizeRESTEndpointSignature(method, path)
	for _, candidate := range h.customAPIs {
		if candidate == nil {
			continue
		}

		candidateType := strings.ToLower(strings.TrimSpace(candidate.APIType))
		if candidateType == "" {
			candidateType = "rest"
		}
		if candidateType != "rest" {
			continue
		}
		if excludeID != "" && candidate.ID == excludeID {
			continue
		}

		if normalizeRESTEndpointSignature(candidate.Method, candidate.Path) == targetSignature {
			return candidate
		}
	}

	return nil
}

func (h *APIBuilderHandler) findDuplicateGraphQLEndpointLocked(method, path, excludeID string) *CustomAPI {
	targetSignature := normalizeGraphQLEndpointSignature(method, path)
	for _, candidate := range h.customAPIs {
		if candidate == nil {
			continue
		}

		candidateType := strings.ToLower(strings.TrimSpace(candidate.APIType))
		if candidateType != "graphql" {
			continue
		}
		if excludeID != "" && candidate.ID == excludeID {
			continue
		}

		if normalizeGraphQLEndpointSignature(candidate.Method, candidate.Path) == targetSignature {
			return candidate
		}
	}

	return nil
}

func extractQueryFromMock(mock interface{}) (string, []interface{}) {
	m, ok := mock.(map[string]interface{})
	if !ok {
		return "", nil
	}

	query, _ := m["query"].(string)
	paramsRaw, _ := m["params"].([]interface{})
	if paramsRaw == nil {
		return query, nil
	}

	params := make([]interface{}, 0, len(paramsRaw))
	for _, p := range paramsRaw {
		params = append(params, p)
	}
	return query, params
}

func resolveStoredSQLTemplate(api *CustomAPI) string {
	if api == nil {
		return ""
	}
	template := strings.TrimSpace(api.SQLTemplate)
	if template != "" {
		return template
	}
	legacyTemplate, _ := extractQueryFromMock(api.MockResponse)
	return strings.TrimSpace(legacyTemplate)
}

func extractRuntimeParamsFromRequest(c *gin.Context, method string, defs []ParamDef) ([]interface{}, error) {
	bodyParamsByName, bodyParamsList, err := extractRuntimeBodyParams(c, method)
	if err != nil {
		return nil, err
	}

	if len(defs) == 0 {
		if len(bodyParamsList) > 0 {
			return bodyParamsList, nil
		}
		return make([]interface{}, 0), nil
	}

	params := make([]interface{}, 0, len(defs))
	missing := make([]string, 0)

	for _, def := range defs {
		name := strings.TrimSpace(def.Name)
		if name == "" {
			continue
		}

		rawFromQuery := strings.TrimSpace(c.Query(name))
		if rawFromQuery != "" {
			parsed, parseErr := parseParamValue(def.Type, rawFromQuery)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid value for query parameter %q: %v", name, parseErr)
			}
			params = append(params, parsed)
			continue
		}

		if bodyParamsByName != nil {
			if raw, ok := bodyParamsByName[name]; ok {
				parsed, parseErr := parseParamValue(def.Type, raw)
				if parseErr != nil {
					return nil, fmt.Errorf("invalid value for parameter %q: %v", name, parseErr)
				}
				params = append(params, parsed)
				continue
			}
		}

		if strings.TrimSpace(def.Default) != "" {
			parsed, parseErr := parseParamValue(def.Type, def.Default)
			if parseErr != nil {
				return nil, fmt.Errorf("invalid default value for parameter %q: %v", name, parseErr)
			}
			params = append(params, parsed)
			continue
		}

		if def.Required {
			missing = append(missing, name)
			continue
		}

		params = append(params, nil)
	}

	if len(missing) > 0 {
		return nil, fmt.Errorf("missing required query parameters: %s", strings.Join(missing, ", "))
	}

	return params, nil
}

func extractRuntimeBodyParams(c *gin.Context, method string) (map[string]interface{}, []interface{}, error) {
	if method == "GET" || c.Request.ContentLength == 0 {
		return nil, nil, nil
	}

	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		return nil, nil, fmt.Errorf("invalid request body: expected JSON parameter object")
	}

	if rawParams, ok := body["params"]; ok {
		switch typed := rawParams.(type) {
		case map[string]interface{}:
			return typed, nil, nil
		case []interface{}:
			return nil, typed, nil
		default:
			return nil, nil, fmt.Errorf("invalid params field: expected object or array")
		}
	}

	return body, nil, nil
}

func parseParamValue(paramType string, raw interface{}) (interface{}, error) {
	t := strings.ToLower(strings.TrimSpace(paramType))
	if t == "" || t == "string" {
		return fmt.Sprintf("%v", raw), nil
	}

	switch t {
	case "int", "integer":
		switch v := raw.(type) {
		case float64:
			return int64(v), nil
		case int:
			return int64(v), nil
		case int32:
			return int64(v), nil
		case int64:
			return v, nil
		case string:
			parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
			if err != nil {
				return nil, err
			}
			return parsed, nil
		default:
			parsed, err := strconv.ParseInt(strings.TrimSpace(fmt.Sprintf("%v", raw)), 10, 64)
			if err != nil {
				return nil, err
			}
			return parsed, nil
		}
	case "number", "float", "decimal":
		switch v := raw.(type) {
		case float64:
			return v, nil
		case int:
			return float64(v), nil
		case int32:
			return float64(v), nil
		case int64:
			return float64(v), nil
		case string:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
			if err != nil {
				return nil, err
			}
			return parsed, nil
		default:
			parsed, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprintf("%v", raw)), 64)
			if err != nil {
				return nil, err
			}
			return parsed, nil
		}
	case "bool", "boolean":
		switch v := raw.(type) {
		case bool:
			return v, nil
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(v))
			if err != nil {
				return nil, err
			}
			return parsed, nil
		default:
			parsed, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprintf("%v", raw)))
			if err != nil {
				return nil, err
			}
			return parsed, nil
		}
	default:
		return raw, nil
	}
}

func countSQLPlaceholders(query string) int {
	return strings.Count(query, "?")
}

func normalizeSQLPolicyMode(raw string) (string, bool) {
	mode := strings.ToLower(strings.TrimSpace(raw))
	if mode == "" {
		return "", true
	}
	if mode == "compat" || mode == "strict" {
		return mode, true
	}
	return "", false
}

func resolveSQLPolicyMode(apiMode string) string {
	if normalized, ok := normalizeSQLPolicyMode(apiMode); ok && normalized != "" {
		return normalized
	}
	if normalized, ok := normalizeSQLPolicyMode(os.Getenv("BUILDER_SQL_POLICY_MODE")); ok && normalized != "" {
		return normalized
	}
	return "compat"
}

func isStrictReadOnlyQuery(query string, policyMode string) bool {
	trimmed := strings.TrimSpace(query)
	if trimmed == "" {
		return false
	}

	mode := resolveSQLPolicyMode(policyMode)
	filter := sqlfilter.New(sqlfilter.PolicyMode(mode))

	// Run injection detection — reject high/critical risk queries.
	injection := sqlfilter.DetectInjection(trimmed)
	if injection.Risk == sqlfilter.RiskHigh || injection.Risk == sqlfilter.RiskCritical {
		logging.Z().Warn("SQL injection pattern detected in API template",
			zap.String("risk", string(injection.Risk)),
			zap.Int("score", injection.Score),
			zap.String("suggestion", injection.Suggestion),
		)
		return false
	}

	return filter.IsReadOnly(trimmed)
}

func enforceCustomAPIRateLimit(c *gin.Context, api *CustomAPI) (bool, int) {
	limit := api.RateLimit
	if limit <= 0 {
		return true, 0
	}

	if api.rateBuckets == nil {
		api.rateBuckets = make(map[string]*apiRuntimeRateBucket)
	}

	callerKey := strings.TrimSpace(c.GetString("token"))
	if callerKey == "" {
		callerKey = "ip:" + strings.TrimSpace(c.ClientIP())
	}

	now := time.Now().UTC()
	for key, bucket := range api.rateBuckets {
		if bucket == nil || now.Sub(bucket.WindowStart) > 5*time.Minute {
			delete(api.rateBuckets, key)
		}
	}

	bucket, exists := api.rateBuckets[callerKey]
	if !exists || bucket == nil || now.Sub(bucket.WindowStart) >= time.Minute {
		bucket = &apiRuntimeRateBucket{WindowStart: now, Count: 1}
		api.rateBuckets[callerKey] = bucket
		remaining := limit - 1
		if remaining < 0 {
			remaining = 0
		}
		c.Header("X-API-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-API-RateLimit-Remaining", strconv.Itoa(remaining))
		return true, 0
	}

	if bucket.Count >= limit {
		retryAfter := int((time.Minute - now.Sub(bucket.WindowStart)).Seconds())
		if retryAfter < 1 {
			retryAfter = 1
		}
		c.Header("X-API-RateLimit-Limit", strconv.Itoa(limit))
		c.Header("X-API-RateLimit-Remaining", "0")
		c.Header("Retry-After", strconv.Itoa(retryAfter))
		return false, retryAfter
	}

	bucket.Count++
	remaining := limit - bucket.Count
	if remaining < 0 {
		remaining = 0
	}
	c.Header("X-API-RateLimit-Limit", strconv.Itoa(limit))
	c.Header("X-API-RateLimit-Remaining", strconv.Itoa(remaining))
	return true, 0
}

func (h *APIBuilderHandler) GetSummary(c *gin.Context) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	apiTypeFilter := strings.ToLower(strings.TrimSpace(c.Query("api_type")))
	active, inactive, draft := 0, 0, 0
	byCategory := map[string]int{}
	byMethod := map[string]int{}
	byAPIType := map[string]int{}
	var totalHits int64

	for _, api := range h.customAPIs {
		currentType := strings.ToLower(strings.TrimSpace(api.APIType))
		if currentType == "" {
			currentType = "rest"
		}
		if apiTypeFilter != "" && currentType != apiTypeFilter {
			continue
		}
		switch api.Status {
		case "active":
			active++
		case "inactive":
			inactive++
		case "draft":
			draft++
		}
		byCategory[api.Category]++
		byMethod[api.Method]++
		byAPIType[currentType]++
		totalHits += api.HitCount
	}

	totalAPIs := active + inactive + draft

	c.JSON(http.StatusOK, gin.H{
		"status":            "success",
		"total_apis":        totalAPIs,
		"active":            active,
		"inactive":          inactive,
		"draft":             draft,
		"total_hits":        totalHits,
		"by_category":       byCategory,
		"by_method":         byMethod,
		"by_api_type":       byAPIType,
		"total_csv_uploads": len(h.csvUploads),
		"total_conversions": len(h.conversions),
	})
}

