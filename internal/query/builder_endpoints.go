package query

import (
	"net/http"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/utils"
	"github.com/gin-gonic/gin"
)

// QueryBuilderRequest represents a query builder request
type QueryBuilderRequest struct {
	Table        string               `json:"table" binding:"required"`
	Select       []string             `json:"select"`
	Filters      []FilterCondition    `json:"filters"`
	Joins        []JoinCondition      `json:"joins"`
	GroupBy      []string             `json:"group_by"`
	Aggregations []AggregationRequest `json:"aggregations"`
	OrderBy      []OrderByClause      `json:"order_by"`
	Page         int                  `json:"page"`
	PageSize     int                  `json:"page_size"`
	Limit        int                  `json:"limit"`
	Offset       int                  `json:"offset"`
	Distinct     bool                 `json:"distinct"`
	CTE          map[string]string    `json:"cte"`
}

// FilterCondition represents a single filter
type FilterCondition struct {
	Column    string        `json:"column" binding:"required"`
	Operator  string        `json:"operator" binding:"required"`
	Value     interface{}   `json:"value"`
	Value2    interface{}   `json:"value2"`
	Values    []interface{} `json:"values"`
	LogicalOp string        `json:"logical_op"`
}

// JoinCondition represents a join
type JoinCondition struct {
	Type      string `json:"type"` // INNER, LEFT, RIGHT
	Table     string `json:"table" binding:"required"`
	Condition string `json:"condition" binding:"required"`
}

// OrderByClause represents order by condition
type OrderByClause struct {
	Column    string `json:"column" binding:"required"`
	Direction string `json:"direction"` // ASC or DESC
}

// AggregationRequest represents aggregation function
type AggregationRequest struct {
	Function string `json:"function" binding:"required"` // COUNT, SUM, AVG, MIN, MAX
	Column   string `json:"column"`
	Alias    string `json:"alias"`
}

// BuilderQuery handles query builder endpoint
func (h *Handler) BuilderQuery(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	var req QueryBuilderRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.Table == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Table name is required",
		})
		return
	}

	startTime := time.Now()

	qb := utils.NewQueryBuilder(h.db)
	qb.From(req.Table)

	if len(req.Select) > 0 {
		qb.Select(req.Select...)
	}

	if req.Distinct {
		qb.Distinct()
	}

	for _, filter := range req.Filters {
		filterRule := utils.NewFilterRule().
			SetColumn(filter.Column).
			SetOperator(filter.Operator)

		switch filter.Operator {
		case "IN", "NOT IN":
			filterRule.SetValues(filter.Values)
		case "BETWEEN":
			filterRule.SetValue(filter.Value).SetValue2(filter.Value2)
		default:
			filterRule.SetValue(filter.Value)
		}

		if filter.LogicalOp != "" {
			filterRule.SetLogicalOp(filter.LogicalOp)
		}

		qb.AddFilter(filterRule)
	}

	for _, join := range req.Joins {
		joinType := strings.ToUpper(join.Type)
		switch joinType {
		case "LEFT":
			qb.LeftJoin(join.Table, join.Condition)
		case "RIGHT":
			qb.RightJoin(join.Table, join.Condition)
		default:
			qb.Join(join.Table, join.Condition)
		}
	}

	if len(req.GroupBy) > 0 {
		qb.GroupBy(req.GroupBy...)
	}

	for _, agg := range req.Aggregations {
		aggObj := utils.NewAggregation(agg.Function, agg.Column, agg.Alias)
		qb.AddAggregation(aggObj)
	}

	for _, orderBy := range req.OrderBy {
		qb.OrderBy(orderBy.Column, orderBy.Direction)
	}

	if req.Page > 0 && req.PageSize > 0 {
		qb.Paginate(req.Page, req.PageSize)
	} else if req.Limit > 0 {
		qb.Limit(req.Limit)
		if req.Offset > 0 {
			qb.Offset(req.Offset)
		}
	}

	for name, cteQuery := range req.CTE {
		qb.WithCTE(name, cteQuery)
	}

	query, params, err := qb.Build()
	if err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Failed to build query: " + err.Error(),
		})
		return
	}

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

	results, err := qb.Execute()
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Query execution failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Query executed successfully",
		Data: map[string]interface{}{
			"results": results,
			"count":   len(results),
			"query":   query,
		},
	})
}

// BuilderQueryWithPagination handles paginated query builder requests
func (h *Handler) BuilderQueryWithPagination(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	var req QueryBuilderRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request body: " + err.Error(),
		})
		return
	}

	if req.Table == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Table name is required",
		})
		return
	}

	if req.Page < 1 {
		req.Page = 1
	}
	if req.PageSize < 1 {
		req.PageSize = 10
	}

	startTime := time.Now()

	qb := utils.NewQueryBuilder(h.db)
	qb.From(req.Table)

	if len(req.Select) > 0 {
		qb.Select(req.Select...)
	}

	if req.Distinct {
		qb.Distinct()
	}

	for _, filter := range req.Filters {
		filterRule := utils.NewFilterRule().
			SetColumn(filter.Column).
			SetOperator(filter.Operator)

		switch filter.Operator {
		case "IN", "NOT IN":
			filterRule.SetValues(filter.Values)
		case "BETWEEN":
			filterRule.SetValue(filter.Value).SetValue2(filter.Value2)
		default:
			filterRule.SetValue(filter.Value)
		}

		if filter.LogicalOp != "" {
			filterRule.SetLogicalOp(filter.LogicalOp)
		}

		qb.AddFilter(filterRule)
	}

	for _, join := range req.Joins {
		joinType := strings.ToUpper(join.Type)
		switch joinType {
		case "LEFT":
			qb.LeftJoin(join.Table, join.Condition)
		case "RIGHT":
			qb.RightJoin(join.Table, join.Condition)
		default:
			qb.Join(join.Table, join.Condition)
		}
	}

	if len(req.GroupBy) > 0 {
		qb.GroupBy(req.GroupBy...)
	}

	for _, agg := range req.Aggregations {
		aggObj := utils.NewAggregation(agg.Function, agg.Column, agg.Alias)
		qb.AddAggregation(aggObj)
	}

	for _, orderBy := range req.OrderBy {
		qb.OrderBy(orderBy.Column, orderBy.Direction)
	}

	result, err := qb.ExecuteWithPagination(req.Page, req.PageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Query execution failed: " + err.Error(),
		})
		return
	}

	if h.logger != nil {
		duration := time.Since(startTime).Milliseconds()
		query, params, _ := qb.Build()
		h.logger.LogQuery(QueryLog{
			Query:    query,
			Params:   ConvertParamsToStrings(params),
			Database: c.GetString("database"),
			User:     c.GetString("user_id"),
			Status:   "success",
			Duration: duration,
		})
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Query executed successfully",
		Data:    result,
	})
}

// TableSchemaRequest represents a table schema request
type TableSchemaRequest struct {
	Table string `json:"table" binding:"required"`
}

// GetTableSchema handles schema introspection requests
func (h *Handler) GetTableSchema(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	var req TableSchemaRequest
	if err := c.BindJSON(&req); err != nil {
		tableName := c.Query("table")
		if tableName == "" {
			c.JSON(http.StatusBadRequest, models.Response{
				Status: "error",
				Error:  "Table name is required",
			})
			return
		}
		req.Table = tableName
	}

	qb := utils.NewQueryBuilder(h.db)
	schema, err := qb.ScanTableSchema(req.Table)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to scan table schema: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Table schema retrieved successfully",
		Data:    schema,
	})
}

// AdvancedFilterRequest for complex filtering scenarios
type AdvancedFilterRequest struct {
	Table        string           `json:"table" binding:"required"`
	Filters      []FilterRuleJSON `json:"filters"`
	OrderBy      []OrderByClause  `json:"order_by"`
	Page         int              `json:"page"`
	PageSize     int              `json:"page_size"`
	IncludeCount bool             `json:"include_count"`
}

// FilterRuleJSON is JSON representation of filter rule
type FilterRuleJSON struct {
	Column        string        `json:"column" binding:"required"`
	Operator      string        `json:"operator" binding:"required"`
	Value         interface{}   `json:"value"`
	Value2        interface{}   `json:"value2"`
	Values        []interface{} `json:"values"`
	LogicalOp     string        `json:"logical_op"`
	CaseSensitive bool          `json:"case_sensitive"`
}

// ApplyAdvancedFilter applies advanced filtering to query
func (h *Handler) ApplyAdvancedFilter(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	var req AdvancedFilterRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request: " + err.Error(),
		})
		return
	}

	if req.Table == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Table name is required",
		})
		return
	}

	qb := utils.NewQueryBuilder(h.db)
	qb.From(req.Table)

	for _, f := range req.Filters {
		filterRule := utils.NewFilterRule().
			SetColumn(f.Column).
			SetOperator(f.Operator).
			SetCaseSensitive(f.CaseSensitive)

		switch f.Operator {
		case "IN", "NOT IN":
			filterRule.SetValues(f.Values)
		case "BETWEEN":
			filterRule.SetValue(f.Value).SetValue2(f.Value2)
		default:
			filterRule.SetValue(f.Value)
		}

		if f.LogicalOp != "" {
			filterRule.SetLogicalOp(f.LogicalOp)
		}

		qb.AddFilter(filterRule)
	}

	for _, order := range req.OrderBy {
		qb.OrderBy(order.Column, order.Direction)
	}

	if req.Page > 0 && req.PageSize > 0 {
		qb.Paginate(req.Page, req.PageSize)
	}

	startTime := time.Now()

	var result interface{}
	var err error

	if req.Page > 0 && req.PageSize > 0 {
		result, err = qb.ExecuteWithPagination(req.Page, req.PageSize)
	} else {
		data, execErr := qb.Execute()
		if execErr != nil {
			err = execErr
		} else {
			result = map[string]interface{}{
				"data":  data,
				"count": len(data),
			}
		}
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Query execution failed: " + err.Error(),
		})
		return
	}

	if h.logger != nil {
		duration := time.Since(startTime).Milliseconds()
		query, params, _ := qb.Build()
		h.logger.LogQuery(QueryLog{
			Query:    query,
			Params:   ConvertParamsToStrings(params),
			Database: c.GetString("database"),
			User:     c.GetString("user_id"),
			Status:   "success",
			Duration: duration,
		})
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Advanced filter applied successfully",
		Data:    result,
	})
}

// ScanIfNotPresentRequest checks if table exists, creates if not
type ScanIfNotPresentRequest struct {
	TableName   string `json:"table_name" binding:"required"`
	CreateQuery string `json:"create_query"`
}

// ScanIfNotPresent checks if table exists, creates with provided query if not
func (h *Handler) ScanIfNotPresent(c *gin.Context) {
	if h.db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  "Database not connected",
		})
		return
	}

	var req ScanIfNotPresentRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid request: " + err.Error(),
		})
		return
	}

	qb := utils.NewQueryBuilder(h.db)
	schema, err := qb.ScanTableSchema(req.TableName)

	if err == nil && schema != nil && len(schema.Columns) > 0 {
		c.JSON(http.StatusOK, models.Response{
			Status:  "ok",
			Message: "Table already exists",
			Data: map[string]interface{}{
				"exists": true,
				"schema": schema,
			},
		})
		return
	}

	if req.CreateQuery == "" {
		c.JSON(http.StatusNotFound, models.Response{
			Status: "error",
			Error:  "Table not found and no creation query provided",
		})
		return
	}

	upperQuery := strings.ToUpper(strings.TrimSpace(req.CreateQuery))
	if !strings.HasPrefix(upperQuery, "CREATE TABLE") {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid query. Must be CREATE TABLE statement",
		})
		return
	}

	if result := h.db.Exec(req.CreateQuery); result.Error != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to create table: " + result.Error.Error(),
		})
		return
	}

	schema, err = qb.ScanTableSchema(req.TableName)
	if err != nil {
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  "Failed to scan created table: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Table created successfully",
		Data: map[string]interface{}{
			"created": true,
			"schema":  schema,
		},
	})
}

// GetSampleQuery returns example query builder request
func (h *Handler) GetSampleQuery(c *gin.Context) {
	sampleRequest := QueryBuilderRequest{
		Table:    "users",
		Select:   []string{"id", "name", "email"},
		Distinct: false,
		Filters: []FilterCondition{
			{
				Column:    "status",
				Operator:  "=",
				Value:     "active",
				LogicalOp: "AND",
			},
			{
				Column:    "created_at",
				Operator:  ">=",
				Value:     "2024-01-01",
				LogicalOp: "AND",
			},
		},
		OrderBy:  []OrderByClause{{Column: "name", Direction: "ASC"}},
		Page:     1,
		PageSize: 20,
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Sample query builder request",
		Data:    sampleRequest,
	})
}
