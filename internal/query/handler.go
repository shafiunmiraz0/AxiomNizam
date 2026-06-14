package query

// SECURITY (P0.5):
//
// This handler exposes an unrestricted-SQL surface which historically was the
// single largest injection risk in the platform. As of P0.5 every inbound
// query is passed through utils.SQLInjectionProtection.ValidateSQLInput
// before execution, and every non-SELECT statement additionally requires an
// authenticated caller with an admin-equivalent role.

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// adminishRoles lists role names considered privileged enough to run
// mutating SQL through the DynamicQuery endpoints.
var adminishRoles = map[string]struct{}{
	"admin":          {},
	"superadmin":     {},
	"platform-admin": {},
	"dba":            {},
}

// Handler handles dynamic SQL queries
type Handler struct {
	db            *gorm.DB
	logger        *QueryLogger
	sqlProtection *utils.SQLInjectionProtection
}

// NewHandler creates a new dynamic query handler
func NewHandler(db *gorm.DB, logger *QueryLogger) *Handler {
	return &Handler{
		db:            db,
		logger:        logger,
		sqlProtection: utils.NewSQLInjectionProtection(),
	}
}

// validateIncomingQuery runs the caller-supplied SQL text through the
// injection-protection engine before it is ever handed to the database.
func (h *Handler) validateIncomingQuery(query string) error {
	if h.sqlProtection == nil {
		h.sqlProtection = utils.NewSQLInjectionProtection()
	}
	return h.sqlProtection.ValidateSQLInput(query)
}

// requireAdminForMutation rejects the request unless the authenticated
// caller carries an admin-equivalent role.
func requireAdminForMutation(c *gin.Context) bool {
	rolesVal, ok := c.Get("roles")
	if !ok {
		c.JSON(http.StatusUnauthorized, models.Response{
			Status: "error",
			Error:  "authentication required for mutating queries",
		})
		return false
	}
	roles, _ := rolesVal.([]string)
	for _, r := range roles {
		if _, ok := adminishRoles[strings.ToLower(strings.TrimSpace(r))]; ok {
			return true
		}
	}
	c.JSON(http.StatusForbidden, models.Response{
		Status: "error",
		Error:  "admin role required for non-SELECT queries",
	})
	return false
}

// QueryRequest represents a dynamic query request
type QueryRequest struct {
	Query  string        `json:"query" binding:"required"`
	Params []interface{} `json:"params"`
}

// DynamicQuery handles GET requests with dynamic SQL queries
func (h *Handler) DynamicQuery(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	var query string
	var params []interface{}

	query = c.Query("q")
	if query == "" {
		var req QueryRequest
		if err := c.BindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, models.Response{
				Status: "error",
				Error:  "Missing query parameter. Use ?q=YOUR_QUERY or send JSON body with 'query' field",
			})
			return
		}
		query = req.Query
		params = req.Params
	} else {
		paramStr := c.Query("params")
		if paramStr != "" {
			paramParts := strings.Split(paramStr, ",")
			for _, p := range paramParts {
				params = append(params, strings.TrimSpace(p))
			}
		}
	}

	upperQuery := strings.ToUpper(strings.TrimSpace(query))
	if !isSelectQuery(upperQuery) {
		c.JSON(http.StatusForbidden, models.Response{
			Status: "error",
			Error:  "Only SELECT queries are allowed for GET requests. Use POST for INSERT/UPDATE/DELETE/CREATE",
		})
		return
	}

	if err := h.validateIncomingQuery(query); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "query rejected by SQL safety filter: " + err.Error(),
		})
		return
	}

	startTime := time.Now()
	defer func() {
		if h.logger != nil {
			duration := time.Since(startTime).Milliseconds()
			h.logger.LogQuery(QueryLog{
				Query:    query,
				Params:   ConvertParamsToStrings(params),
				Database: c.GetString("database"),
				User:     c.GetString("user_id"),
				Status:   "success",
				Duration: duration,
			})
		}
	}()

	var results []map[string]interface{}
	rows, err := h.db.Raw(query, params...).Rows()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Query execution failed: " + err.Error(),
		})
		return
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to get columns: " + err.Error(),
		})
		return
	}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Status: "error",
				Error:  "Failed to scan row: " + err.Error(),
			})
			return
		}

		entry := make(map[string]interface{})
		for i, col := range columns {
			var v interface{}
			val := values[i]
			b, ok := val.([]byte)
			if ok {
				v = string(b)
			} else {
				v = val
			}
			entry[col] = v
		}
		results = append(results, entry)
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Query executed successfully",
		Data:    results,
	})
}

// DynamicQueryWithBody handles POST requests with dynamic SQL queries
func (h *Handler) DynamicQueryWithBody(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	var req QueryRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body. Expected: {\"query\": \"SQL_QUERY\", \"params\": []}",
		})
		return
	}

	if req.Query == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Query cannot be empty",
		})
		return
	}

	upperQuery := strings.ToUpper(strings.TrimSpace(req.Query))
	if !isWriteOrSelectQuery(upperQuery) {
		c.JSON(http.StatusForbidden, models.Response{
			Status: "error",
			Error:  "Query type not allowed. Allowed: SELECT, INSERT, UPDATE, DELETE, CREATE, DROP, ALTER",
		})
		return
	}

	if err := h.validateIncomingQuery(req.Query); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "query rejected by SQL safety filter: " + err.Error(),
		})
		return
	}

	if isSelectQuery(upperQuery) {
		var results []map[string]interface{}
		rows, err := h.db.Raw(req.Query, req.Params...).Rows()
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Status: "error",
				Error:  "Query execution failed: " + err.Error(),
			})
			return
		}
		defer rows.Close()

		columns, err := rows.Columns()
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Status: "error",
				Error:  "Failed to get columns: " + err.Error(),
			})
			return
		}

		for rows.Next() {
			values := make([]interface{}, len(columns))
			valuePtrs := make([]interface{}, len(columns))
			for i := range columns {
				valuePtrs[i] = &values[i]
			}

			if err := rows.Scan(valuePtrs...); err != nil {
				c.JSON(http.StatusInternalServerError, models.Response{
					Status: "error",
					Error:  "Failed to scan row: " + err.Error(),
				})
				return
			}

			entry := make(map[string]interface{})
			for i, col := range columns {
				var v interface{}
				val := values[i]
				b, ok := val.([]byte)
				if ok {
					v = string(b)
				} else {
					v = val
				}
				entry[col] = v
			}
			results = append(results, entry)
		}

		c.JSON(http.StatusOK, models.Response{
			Status:  "ok",
			Message: "Query executed successfully",
			Data:    results,
		})
	} else {
		if !requireAdminForMutation(c) {
			return
		}
		result := h.db.Exec(req.Query, req.Params...)
		if result.Error != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Status: "error",
				Error:  "Query execution failed: " + result.Error.Error(),
			})
			return
		}

		c.JSON(http.StatusOK, models.Response{
			Status:  "ok",
			Message: "Query executed successfully",
			Data: map[string]interface{}{
				"rows_affected": result.RowsAffected,
			},
		})
	}
}

// BatchQueries handles batch queries via POST
func (h *Handler) BatchQueries(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	var requests []QueryRequest
	if err := c.BindJSON(&requests); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body. Expected array of {\"query\": \"SQL_QUERY\", \"params\": []}",
		})
		return
	}

	if len(requests) == 0 {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Requests array cannot be empty",
		})
		return
	}

	results := make([]map[string]interface{}, 0)
	for i, req := range requests {
		upperQuery := strings.ToUpper(strings.TrimSpace(req.Query))
		if !isWriteOrSelectQuery(upperQuery) {
			c.JSON(http.StatusForbidden, models.Response{
				Status: "error",
				Error:  "Query " + string(rune(i+1)) + " - Query type not allowed",
			})
			return
		}

		if err := h.validateIncomingQuery(req.Query); err != nil {
			c.JSON(http.StatusBadRequest, models.Response{
				Status: "error",
				Error:  fmt.Sprintf("Query %d rejected by SQL safety filter: %v", i+1, err),
			})
			return
		}

		if isSelectQuery(upperQuery) {
			rows, err := h.db.Raw(req.Query, req.Params...).Rows()
			if err != nil {
				c.JSON(http.StatusInternalServerError, models.Response{
					Status: "error",
					Error:  "Query " + string(rune(i+1)) + " failed: " + err.Error(),
				})
				return
			}
			defer rows.Close()

			columns, _ := rows.Columns()
			for rows.Next() {
				values := make([]interface{}, len(columns))
				valuePtrs := make([]interface{}, len(columns))
				for j := range columns {
					valuePtrs[j] = &values[j]
				}
				rows.Scan(valuePtrs...)

				entry := make(map[string]interface{})
				for j, col := range columns {
					var v interface{}
					val := values[j]
					b, ok := val.([]byte)
					if ok {
						v = string(b)
					} else {
						v = val
					}
					entry[col] = v
				}
				results = append(results, entry)
			}
		} else {
			if !requireAdminForMutation(c) {
				return
			}
			result := h.db.Exec(req.Query, req.Params...)
			if result.Error != nil {
				c.JSON(http.StatusInternalServerError, models.Response{
					Status: "error",
					Error:  "Query " + string(rune(i+1)) + " failed: " + result.Error.Error(),
				})
				return
			}
		}
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "All queries executed successfully",
		Data:    results,
	})
}

// TableSchema returns the schema of a table
func (h *Handler) TableSchema(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	tableName := c.Query("table")
	if tableName == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Table name required. Use ?table=table_name",
		})
		return
	}

	type Column struct {
		Field   string
		Type    string
		Null    string
		Key     string
		Default interface{}
		Extra   string
	}

	var columns []Column
	dbType := h.db.Dialector.Name()
	var query string

	switch dbType {
	case "mysql":
		query = "SELECT COLUMN_NAME as Field, COLUMN_TYPE as Type, IS_NULLABLE as Null, COLUMN_KEY as Key, COLUMN_DEFAULT as Default, EXTRA as Extra FROM INFORMATION_SCHEMA.COLUMNS WHERE TABLE_NAME = ? ORDER BY ORDINAL_POSITION"
		h.db.Raw(query, tableName).Scan(&columns)
	case "postgres":
		query = "SELECT column_name as Field, data_type as Type, is_nullable as Null FROM information_schema.columns WHERE table_name = $1"
		h.db.Raw(query, tableName).Scan(&columns)
	default:
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Schema query not supported for this database type",
		})
		return
	}

	if len(columns) == 0 {
		c.JSON(http.StatusNotFound, models.Response{
			Status: "error",
			Error:  "Table not found or has no columns",
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Table schema retrieved successfully",
		Data:    columns,
	})
}

// isSelectQuery checks if query is a SELECT query
func isSelectQuery(upperQuery string) bool {
	return strings.HasPrefix(upperQuery, "SELECT") ||
		strings.HasPrefix(upperQuery, "WITH") ||
		strings.HasPrefix(upperQuery, "SHOW") ||
		strings.HasPrefix(upperQuery, "DESCRIBE") ||
		strings.HasPrefix(upperQuery, "DESC") ||
		strings.HasPrefix(upperQuery, "EXPLAIN")
}

// isWriteOrSelectQuery checks if query is allowed (SELECT or write operations)
func isWriteOrSelectQuery(upperQuery string) bool {
	allowedPrefixes := []string{
		"SELECT", "WITH", "SHOW", "DESCRIBE", "DESC", "EXPLAIN",
		"INSERT", "UPDATE", "DELETE", "CREATE", "DROP", "ALTER",
		"TRUNCATE", "REPLACE",
	}
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(upperQuery, prefix) {
			return true
		}
	}
	return false
}

// ValidateQuerySafety validates SQL query safety
func ValidateQuerySafety(query string) bool {
	dangerousKeywords := []string{
		"DROP DATABASE",
		"DROP SCHEMA",
	}
	upperQuery := strings.ToUpper(query)
	for _, keyword := range dangerousKeywords {
		if strings.Contains(upperQuery, keyword) {
			return false
		}
	}
	return true
}

// ConvertParamsToStrings converts interface{} params to strings for logging
func ConvertParamsToStrings(params []interface{}) []string {
	var result []string
	for _, p := range params {
		result = append(result, fmt.Sprintf("%v", p))
	}
	return result
}

// GetQueryLogs handles GET /api/{db}/logs endpoint
func (h *Handler) GetQueryLogs(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	database := c.Query("database")
	if database == "" {
		database = "all"
	}

	limitStr := c.Query("limit")
	limit := 100
	if limitStr != "" {
		fmt.Sscanf(limitStr, "%d", &limit)
	}
	if limit > 1000 {
		limit = 1000
	}

	logs, err := h.logger.GetQueryLogs(database, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to retrieve logs: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Query logs retrieved successfully",
		Data:    logs,
	})
}

// GetQueryStats handles GET /api/{db}/stats endpoint
func (h *Handler) GetQueryStats(c *gin.Context) {
	if h.logger == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Query logging not available",
		})
		return
	}

	database := c.Query("database")
	if database == "" {
		database = "all"
	}

	stats, err := h.logger.GetQueryStats(database)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to retrieve stats: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Query statistics retrieved successfully",
		Data:    stats,
	})
}
