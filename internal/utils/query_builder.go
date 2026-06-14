package utils

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"errors"
	"fmt"
	"strings"
	"sync"

	"gorm.io/gorm"
)

// QueryBuilder provides fluent interface for building dynamic SQL queries
type QueryBuilder struct {
	mu           sync.RWMutex
	db           *gorm.DB
	selectCols   []string
	table        string
	filters      []*FilterRule
	joins        []string
	orderBy      []string
	limit        int
	offset       int
	groupBy      []string
	aggregations []*Aggregation
	distinct     bool
	having       []string
	cteQueries   map[string]string
	lastQuery    string
	lastParams   []interface{}
}

// NewQueryBuilder creates a new query builder
func NewQueryBuilder(db *gorm.DB) *QueryBuilder {
	return &QueryBuilder{
		db:           db,
		selectCols:   []string{},
		filters:      make([]*FilterRule, 0),
		joins:        make([]string, 0),
		orderBy:      make([]string, 0),
		groupBy:      make([]string, 0),
		aggregations: make([]*Aggregation, 0),
		having:       make([]string, 0),
		cteQueries:   make(map[string]string),
		limit:        -1,
		offset:       0,
	}
}

// Select specifies columns to select
func (qb *QueryBuilder) Select(columns ...string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	if len(columns) == 0 {
		qb.selectCols = []string{"*"}
	} else {
		qb.selectCols = columns
	}
	return qb
}

// From specifies the table
func (qb *QueryBuilder) From(table string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	qb.table = table
	return qb
}

// Distinct adds DISTINCT keyword
func (qb *QueryBuilder) Distinct() *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	qb.distinct = true
	return qb
}

// AddFilter adds a filter rule
func (qb *QueryBuilder) AddFilter(filter *FilterRule) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	if filter != nil && filter.Validate() {
		qb.filters = append(qb.filters, filter)
	}
	return qb
}

// Where adds a simple WHERE condition
func (qb *QueryBuilder) Where(column string, operator string, value interface{}) *QueryBuilder {
	filter := NewFilterRule().
		SetColumn(column).
		SetOperator(operator).
		SetValue(value)

	return qb.AddFilter(filter)
}

// WhereIn adds WHERE column IN (values) condition
func (qb *QueryBuilder) WhereIn(column string, values []interface{}) *QueryBuilder {
	filter := NewFilterRule().
		SetColumn(column).
		SetOperator("IN").
		SetValues(values)

	return qb.AddFilter(filter)
}

// WhereNotIn adds WHERE column NOT IN (values) condition
func (qb *QueryBuilder) WhereNotIn(column string, values []interface{}) *QueryBuilder {
	filter := NewFilterRule().
		SetColumn(column).
		SetOperator("NOT IN").
		SetValues(values)

	return qb.AddFilter(filter)
}

// WhereBetween adds WHERE column BETWEEN start AND end condition
func (qb *QueryBuilder) WhereBetween(column string, start, end interface{}) *QueryBuilder {
	filter := NewFilterRule().
		SetColumn(column).
		SetOperator("BETWEEN").
		SetValue(start).
		SetValue2(end)

	return qb.AddFilter(filter)
}

// WhereNull adds WHERE column IS NULL condition
func (qb *QueryBuilder) WhereNull(column string) *QueryBuilder {
	filter := NewFilterRule().
		SetColumn(column).
		SetOperator("IS NULL").
		SetValue(nil)

	return qb.AddFilter(filter)
}

// WhereNotNull adds WHERE column IS NOT NULL condition
func (qb *QueryBuilder) WhereNotNull(column string) *QueryBuilder {
	filter := NewFilterRule().
		SetColumn(column).
		SetOperator("IS NOT NULL").
		SetValue(nil)

	return qb.AddFilter(filter)
}

// OrWhere adds OR condition (groups previous filters with OR)
func (qb *QueryBuilder) OrWhere(column string, operator string, value interface{}) *QueryBuilder {
	filter := NewFilterRule().
		SetColumn(column).
		SetOperator(operator).
		SetValue(value).
		SetLogicalOp("OR")

	return qb.AddFilter(filter)
}

// Join adds an INNER JOIN clause
func (qb *QueryBuilder) Join(table string, condition string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	joinClause := fmt.Sprintf("INNER JOIN %s ON %s", table, condition)
	qb.joins = append(qb.joins, joinClause)
	return qb
}

// LeftJoin adds a LEFT JOIN clause
func (qb *QueryBuilder) LeftJoin(table string, condition string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	joinClause := fmt.Sprintf("LEFT JOIN %s ON %s", table, condition)
	qb.joins = append(qb.joins, joinClause)
	return qb
}

// RightJoin adds a RIGHT JOIN clause
func (qb *QueryBuilder) RightJoin(table string, condition string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	joinClause := fmt.Sprintf("RIGHT JOIN %s ON %s", table, condition)
	qb.joins = append(qb.joins, joinClause)
	return qb
}

// GroupBy adds GROUP BY clause
func (qb *QueryBuilder) GroupBy(columns ...string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	qb.groupBy = append(qb.groupBy, columns...)
	return qb
}

// AddAggregation adds an aggregation function
func (qb *QueryBuilder) AddAggregation(agg *Aggregation) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	if agg != nil && agg.Validate() {
		qb.aggregations = append(qb.aggregations, agg)
	}
	return qb
}

// Count adds COUNT aggregation
func (qb *QueryBuilder) Count(column string, alias string) *QueryBuilder {
	agg := NewAggregation("COUNT", column, alias)
	return qb.AddAggregation(agg)
}

// Sum adds SUM aggregation
func (qb *QueryBuilder) Sum(column string, alias string) *QueryBuilder {
	agg := NewAggregation("SUM", column, alias)
	return qb.AddAggregation(agg)
}

// Avg adds AVG aggregation
func (qb *QueryBuilder) Avg(column string, alias string) *QueryBuilder {
	agg := NewAggregation("AVG", column, alias)
	return qb.AddAggregation(agg)
}

// Min adds MIN aggregation
func (qb *QueryBuilder) Min(column string, alias string) *QueryBuilder {
	agg := NewAggregation("MIN", column, alias)
	return qb.AddAggregation(agg)
}

// Max adds MAX aggregation
func (qb *QueryBuilder) Max(column string, alias string) *QueryBuilder {
	agg := NewAggregation("MAX", column, alias)
	return qb.AddAggregation(agg)
}

// Having adds HAVING clause for aggregations
func (qb *QueryBuilder) Having(condition string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	qb.having = append(qb.having, condition)
	return qb
}

// OrderBy adds ORDER BY clause
func (qb *QueryBuilder) OrderBy(column string, direction string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	if direction == "" {
		direction = "ASC"
	}
	direction = strings.ToUpper(direction)
	if direction != "ASC" && direction != "DESC" {
		direction = "ASC"
	}

	qb.orderBy = append(qb.orderBy, fmt.Sprintf("%s %s", column, direction))
	return qb
}

// Limit sets LIMIT clause
func (qb *QueryBuilder) Limit(limit int) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	qb.limit = limit
	return qb
}

// Offset sets OFFSET clause
func (qb *QueryBuilder) Offset(offset int) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	qb.offset = offset
	return qb
}

// Paginate sets LIMIT and OFFSET for pagination
func (qb *QueryBuilder) Paginate(page int, pageSize int) *QueryBuilder {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	offset := (page - 1) * pageSize
	return qb.Limit(pageSize).Offset(offset)
}

// WithCTE adds a Common Table Expression (CTE)
func (qb *QueryBuilder) WithCTE(name string, query string) *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	qb.cteQueries[name] = query
	return qb
}

// Build constructs and returns the SQL query
func (qb *QueryBuilder) Build() (string, []interface{}, error) {
	qb.mu.RLock()
	defer qb.mu.RUnlock()

	if qb.table == "" {
		return "", nil, errors.New("table name is required")
	}

	var query strings.Builder
	var params []interface{}

	// CTE (WITH clause)
	if len(qb.cteQueries) > 0 {
		query.WriteString("WITH ")
		var cteList []string
		for name, cteQuery := range qb.cteQueries {
			cteList = append(cteList, fmt.Sprintf("%s AS (%s)", name, cteQuery))
		}
		query.WriteString(strings.Join(cteList, ", "))
		query.WriteString(" ")
	}

	// SELECT
	query.WriteString("SELECT ")
	if qb.distinct {
		query.WriteString("DISTINCT ")
	}

	// Select columns
	if len(qb.selectCols) == 0 {
		query.WriteString("*")
	} else {
		query.WriteString(strings.Join(qb.selectCols, ", "))
	}

	// Add aggregations to SELECT
	if len(qb.aggregations) > 0 {
		query.WriteString(", ")
		var aggCols []string
		for _, agg := range qb.aggregations {
			aggStr := agg.String()
			aggCols = append(aggCols, aggStr)
		}
		query.WriteString(strings.Join(aggCols, ", "))
	}

	// FROM
	query.WriteString(" FROM ")
	query.WriteString(qb.table)

	// JOINs
	for _, join := range qb.joins {
		query.WriteString(" ")
		query.WriteString(join)
	}

	// WHERE filters
	if len(qb.filters) > 0 {
		query.WriteString(" WHERE ")
		filterQueries := make([]string, 0)
		var currentLogicalOp string

		for i, filter := range qb.filters {
			filterSQL, filterParams := filter.ToSQL()

			if i == 0 {
				filterQueries = append(filterQueries, filterSQL)
				currentLogicalOp = "AND"
			} else {
				if filter.LogicalOp == "OR" {
					currentLogicalOp = "OR"
				}
				filterQueries = append(filterQueries, fmt.Sprintf("%s %s", currentLogicalOp, filterSQL))
				currentLogicalOp = "AND"
			}

			params = append(params, filterParams...)
		}

		query.WriteString(strings.Join(filterQueries, " "))
	}

	// GROUP BY
	if len(qb.groupBy) > 0 {
		query.WriteString(" GROUP BY ")
		query.WriteString(strings.Join(qb.groupBy, ", "))
	}

	// HAVING
	if len(qb.having) > 0 {
		query.WriteString(" HAVING ")
		query.WriteString(strings.Join(qb.having, " AND "))
	}

	// ORDER BY
	if len(qb.orderBy) > 0 {
		query.WriteString(" ORDER BY ")
		query.WriteString(strings.Join(qb.orderBy, ", "))
	}

	// LIMIT and OFFSET
	if qb.limit > 0 {
		query.WriteString(fmt.Sprintf(" LIMIT %d", qb.limit))
	}
	if qb.offset > 0 {
		query.WriteString(fmt.Sprintf(" OFFSET %d", qb.offset))
	}

	finalQuery := query.String()
	qb.lastQuery = finalQuery
	qb.lastParams = params

	return finalQuery, params, nil
}

// Execute executes the built query and returns results
func (qb *QueryBuilder) Execute() ([]map[string]interface{}, error) {
	query, params, err := qb.Build()
	if err != nil {
		return nil, err
	}

	logging.Z().Info(fmt.Sprintf("Executing query: %s with params: %v", query, params))

	var results []map[string]interface{}
	rows, err := qb.db.Raw(query, params...).Rows()
	if err != nil {
		return nil, fmt.Errorf("query execution failed: %w", err)
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, fmt.Errorf("failed to get columns: %w", err)
	}

	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("failed to scan row: %w", err)
		}

		entry := make(map[string]interface{})
		for i, col := range columns {
			var v interface{}
			val := values[i]
			if b, ok := val.([]byte); ok {
				v = string(b)
			} else {
				v = val
			}
			entry[col] = v
		}
		results = append(results, entry)
	}

	return results, nil
}

// ExecuteCount executes query and returns row count
func (qb *QueryBuilder) ExecuteCount() (int64, error) {
	query, params, err := qb.Build()
	if err != nil {
		return 0, err
	}

	var count int64
	if err := qb.db.Raw(query, params...).Scan(&count).Error; err != nil {
		return 0, fmt.Errorf("count execution failed: %w", err)
	}

	return count, nil
}

// GetLastQuery returns the last built query
func (qb *QueryBuilder) GetLastQuery() string {
	qb.mu.RLock()
	defer qb.mu.RUnlock()

	return qb.lastQuery
}

// GetLastParams returns the last query parameters
func (qb *QueryBuilder) GetLastParams() []interface{} {
	qb.mu.RLock()
	defer qb.mu.RUnlock()

	return qb.lastParams
}

// Reset resets the query builder
func (qb *QueryBuilder) Reset() *QueryBuilder {
	qb.mu.Lock()
	defer qb.mu.Unlock()

	qb.selectCols = []string{}
	qb.table = ""
	qb.filters = make([]*FilterRule, 0)
	qb.joins = make([]string, 0)
	qb.orderBy = make([]string, 0)
	qb.groupBy = make([]string, 0)
	qb.aggregations = make([]*Aggregation, 0)
	qb.distinct = false
	qb.having = make([]string, 0)
	qb.cteQueries = make(map[string]string)
	qb.limit = -1
	qb.offset = 0
	qb.lastQuery = ""
	qb.lastParams = nil

	return qb
}

// ================================
// Filter Rule Implementation
// ================================

// FilterRule represents a WHERE condition
type FilterRule struct {
	Column        string
	Operator      string
	Value         interface{}
	Value2        interface{}
	Values        []interface{}
	LogicalOp     string // "AND" or "OR"
	CaseSensitive bool
}

// NewFilterRule creates a new filter rule
func NewFilterRule() *FilterRule {
	return &FilterRule{
		LogicalOp:     "AND",
		CaseSensitive: true,
	}
}

// SetColumn sets the column name
func (fr *FilterRule) SetColumn(column string) *FilterRule {
	fr.Column = column
	return fr
}

// SetOperator sets the comparison operator
func (fr *FilterRule) SetOperator(operator string) *FilterRule {
	fr.Operator = strings.ToUpper(operator)
	return fr
}

// SetValue sets the filter value
func (fr *FilterRule) SetValue(value interface{}) *FilterRule {
	fr.Value = value
	return fr
}

// SetValue2 sets second value (for BETWEEN)
func (fr *FilterRule) SetValue2(value interface{}) *FilterRule {
	fr.Value2 = value
	return fr
}

// SetValues sets multiple values (for IN)
func (fr *FilterRule) SetValues(values []interface{}) *FilterRule {
	fr.Values = values
	return fr
}

// SetLogicalOp sets logical operator (AND/OR)
func (fr *FilterRule) SetLogicalOp(op string) *FilterRule {
	fr.LogicalOp = strings.ToUpper(op)
	return fr
}

// SetCaseSensitive sets case sensitivity
func (fr *FilterRule) SetCaseSensitive(sensitive bool) *FilterRule {
	fr.CaseSensitive = sensitive
	return fr
}

// Validate checks if filter is valid
func (fr *FilterRule) Validate() bool {
	if fr.Column == "" {
		return false
	}

	if fr.Operator == "" {
		return false
	}

	// Check if operator requires values
	nullOps := map[string]bool{"IS NULL": true, "IS NOT NULL": true}
	if !nullOps[fr.Operator] {
		if fr.Operator == "IN" || fr.Operator == "NOT IN" {
			if len(fr.Values) == 0 {
				return false
			}
		} else if fr.Operator == "BETWEEN" {
			if fr.Value == nil || fr.Value2 == nil {
				return false
			}
		} else {
			if fr.Value == nil {
				return false
			}
		}
	}

	return true
}

// ToSQL converts filter rule to SQL
func (fr *FilterRule) ToSQL() (string, []interface{}) {
	var sql string
	var params []interface{}

	switch fr.Operator {
	case "=", "!=", "<>", ">", ">=", "<", "<=", "LIKE", "NOT LIKE":
		sql = fmt.Sprintf("%s %s ?", fr.Column, fr.Operator)
		params = append(params, fr.Value)

	case "IN":
		placeholders := make([]string, len(fr.Values))
		for i := range fr.Values {
			placeholders[i] = "?"
			params = append(params, fr.Values[i])
		}
		sql = fmt.Sprintf("%s IN (%s)", fr.Column, strings.Join(placeholders, ", "))

	case "NOT IN":
		placeholders := make([]string, len(fr.Values))
		for i := range fr.Values {
			placeholders[i] = "?"
			params = append(params, fr.Values[i])
		}
		sql = fmt.Sprintf("%s NOT IN (%s)", fr.Column, strings.Join(placeholders, ", "))

	case "BETWEEN":
		sql = fmt.Sprintf("%s BETWEEN ? AND ?", fr.Column)
		params = append(params, fr.Value, fr.Value2)

	case "IS NULL":
		sql = fmt.Sprintf("%s IS NULL", fr.Column)

	case "IS NOT NULL":
		sql = fmt.Sprintf("%s IS NOT NULL", fr.Column)

	default:
		sql = fmt.Sprintf("%s = ?", fr.Column)
		params = append(params, fr.Value)
	}

	return sql, params
}

// ================================
// Aggregation Implementation
// ================================

// Aggregation represents an aggregation function
type Aggregation struct {
	Function string
	Column   string
	Alias    string
}

// NewAggregation creates a new aggregation
func NewAggregation(function string, column string, alias string) *Aggregation {
	return &Aggregation{
		Function: strings.ToUpper(function),
		Column:   column,
		Alias:    alias,
	}
}

// Validate checks if aggregation is valid
func (a *Aggregation) Validate() bool {
	return a.Function != "" && (a.Column != "" || a.Function == "COUNT(*)")
}

// String returns SQL representation
func (a *Aggregation) String() string {
	var col string
	if a.Column == "*" || a.Column == "" {
		col = "*"
	} else {
		col = a.Column
	}

	sql := fmt.Sprintf("%s(%s)", a.Function, col)
	if a.Alias != "" {
		sql = fmt.Sprintf("%s AS %s", sql, a.Alias)
	}

	return sql
}

// ================================
// Pagination Helper
// ================================

// PaginationResult holds paginated query results
type PaginationResult struct {
	Data        []map[string]interface{} `json:"data"`
	CurrentPage int                      `json:"current_page"`
	PageSize    int                      `json:"page_size"`
	Total       int64                    `json:"total"`
	TotalPages  int64                    `json:"total_pages"`
	HasMore     bool                     `json:"has_more"`
}

// ExecuteWithPagination executes query with pagination
func (qb *QueryBuilder) ExecuteWithPagination(page int, pageSize int) (*PaginationResult, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 10
	}

	// Get total count first (without limit/offset)
	countBuilder := NewQueryBuilder(qb.db)
	countBuilder.From(qb.table)
	countBuilder.filters = qb.filters
	countBuilder.joins = qb.joins
	countBuilder.groupBy = qb.groupBy

	countQuery, countParams, _ := countBuilder.Build()
	countQuery = fmt.Sprintf("SELECT COUNT(*) FROM (%s) AS total_count", countQuery)

	var total int64
	if err := qb.db.Raw(countQuery, countParams...).Scan(&total).Error; err != nil {
		return nil, fmt.Errorf("count query failed: %w", err)
	}

	// Apply pagination
	qb.Paginate(page, pageSize)

	// Execute paginated query
	data, err := qb.Execute()
	if err != nil {
		return nil, err
	}

	totalPages := (total + int64(pageSize) - 1) / int64(pageSize)
	hasMore := int64(page) < totalPages

	return &PaginationResult{
		Data:        data,
		CurrentPage: page,
		PageSize:    pageSize,
		Total:       total,
		TotalPages:  totalPages,
		HasMore:     hasMore,
	}, nil
}

// ================================
// Table Schema Scanner
// ================================

// TableSchema holds table structure information
type TableSchema struct {
	TableName string
	Columns   []*ColumnInfo
	Indexes   []*IndexInfo
}

// ColumnInfo holds column details
type ColumnInfo struct {
	Name            string
	Type            string
	Nullable        bool
	IsPrimaryKey    bool
	IsAutoIncrement bool
	DefaultValue    interface{}
}

// IndexInfo holds index details
type IndexInfo struct {
	Name      string
	Columns   []string
	IsUnique  bool
	IsPrimary bool
}

// ScanTableSchema scans table schema from database
func (qb *QueryBuilder) ScanTableSchema(tableName string) (*TableSchema, error) {
	dbType := qb.db.Dialector.Name()

	schema := &TableSchema{
		TableName: tableName,
		Columns:   make([]*ColumnInfo, 0),
		Indexes:   make([]*IndexInfo, 0),
	}
	_ = schema // Keep for future schema processing

	switch dbType {
	case "mysql":
		return qb.scanMySQLSchema(tableName)
	case "postgres":
		return qb.scanPostgresSchema(tableName)
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}
}

// scanMySQLSchema scans MySQL table schema
func (qb *QueryBuilder) scanMySQLSchema(tableName string) (*TableSchema, error) {
	var columns []*ColumnInfo

	query := `SELECT 
		COLUMN_NAME, COLUMN_TYPE, IS_NULLABLE, COLUMN_KEY, EXTRA, COLUMN_DEFAULT
		FROM INFORMATION_SCHEMA.COLUMNS 
		WHERE TABLE_NAME = ? 
		ORDER BY ORDINAL_POSITION`

	rows, err := qb.db.Raw(query, tableName).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name, colType, nullable, key, extra string
		var defaultVal interface{}

		rows.Scan(&name, &colType, &nullable, &key, &extra, &defaultVal)

		col := &ColumnInfo{
			Name:            name,
			Type:            colType,
			Nullable:        nullable == "YES",
			IsPrimaryKey:    key == "PRI",
			IsAutoIncrement: strings.Contains(extra, "auto_increment"),
			DefaultValue:    defaultVal,
		}

		columns = append(columns, col)
	}

	return &TableSchema{
		TableName: tableName,
		Columns:   columns,
	}, nil
}

// scanPostgresSchema scans PostgreSQL table schema
func (qb *QueryBuilder) scanPostgresSchema(tableName string) (*TableSchema, error) {
	var columns []*ColumnInfo

	query := `SELECT 
		column_name, data_type, is_nullable, column_default
		FROM information_schema.columns 
		WHERE table_name = $1 
		ORDER BY ordinal_position`

	rows, err := qb.db.Raw(query, tableName).Rows()
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var name, dataType, nullable string
		var defaultVal interface{}

		rows.Scan(&name, &dataType, &nullable, &defaultVal)

		col := &ColumnInfo{
			Name:         name,
			Type:         dataType,
			Nullable:     nullable == "YES",
			DefaultValue: defaultVal,
		}

		columns = append(columns, col)
	}

	return &TableSchema{
		TableName: tableName,
		Columns:   columns,
	}, nil
}
