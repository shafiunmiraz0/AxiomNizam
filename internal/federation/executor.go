package federation

// =====================================================
// WS-5.1 — Federated Query Parallel Executor
//
// Executes sub-queries against multiple datasources concurrently,
// collects results, and passes them to the merger for final assembly.
// =====================================================

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/platform/resilience"
)

// DataSourceExecutor abstracts executing SQL against a single datasource.
type DataSourceExecutor interface {
	Execute(ctx context.Context, dsRef, sql string, maxRows int64) (*ResultSet, error)
}

// ResultSet holds the output of a single sub-query execution.
type ResultSet struct {
	Columns  []ColumnMeta              `json:"columns"`
	Rows     []map[string]interface{}  `json:"rows"`
	RowCount int64                     `json:"rowCount"`
	Source   string                    `json:"source"`
	Duration time.Duration             `json:"duration"`
}

// ColumnMeta describes a result column.
type ColumnMeta struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SubQueryResult pairs a sub-query with its result or error.
type SubQueryResult struct {
	SubQuery SubQuery
	Result   *ResultSet
	Error    error
}

// QueryExecutor runs sub-queries in parallel across datasources.
type QueryExecutor struct {
	dsExecutor DataSourceExecutor
	maxParallel int
}

// NewQueryExecutor creates a new executor.
func NewQueryExecutor(dsExecutor DataSourceExecutor, maxParallel int) *QueryExecutor {
	if maxParallel <= 0 {
		maxParallel = 10
	}
	return &QueryExecutor{dsExecutor: dsExecutor, maxParallel: maxParallel}
}

// ExecuteAll runs all sub-queries concurrently and returns results.
func (e *QueryExecutor) ExecuteAll(ctx context.Context, subQueries []SubQuery, maxRows int64) ([]SubQueryResult, error) {
	if e.dsExecutor == nil {
		return nil, fmt.Errorf("executor: no datasource executor configured")
	}

	results := make([]SubQueryResult, len(subQueries))
	sem := make(chan struct{}, e.maxParallel)
	var wg sync.WaitGroup

	for i, sq := range subQueries {
		wg.Add(1)
		go func(idx int, query SubQuery) {
			defer wg.Done()

			// Respect context cancellation.
			select {
			case <-ctx.Done():
				results[idx] = SubQueryResult{
					SubQuery: query,
					Error:    ctx.Err(),
				}
				return
			case sem <- struct{}{}:
			}
			defer func() { <-sem }()

			rs, err := resilience.Do(ctx, resilience.Config{
				MaxAttempts:  2,
				InitialDelay: 200 * time.Millisecond,
				MaxDelay:     2 * time.Second,
				Name:         "subquery-" + query.DataSourceRef,
			}, func(ctx context.Context) (*ResultSet, error) {
				return e.dsExecutor.Execute(ctx, query.DataSourceRef, query.SQL, maxRows)
			})
			results[idx] = SubQueryResult{
				SubQuery: query,
				Result:   rs,
				Error:    err,
			}
		}(i, sq)
	}

	wg.Wait()

	// Check for errors.
	for _, r := range results {
		if r.Error != nil {
			return results, fmt.Errorf("sub-query against %s failed: %w", r.SubQuery.DataSourceRef, r.Error)
		}
	}

	return results, nil
}

// ExecuteSingle runs a single sub-query against one datasource.
func (e *QueryExecutor) ExecuteSingle(ctx context.Context, sq SubQuery, maxRows int64) (*ResultSet, error) {
	if e.dsExecutor == nil {
		return nil, fmt.Errorf("executor: no datasource executor configured")
	}
	return e.dsExecutor.Execute(ctx, sq.DataSourceRef, sq.SQL, maxRows)
}
