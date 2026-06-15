package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// TracingRepository interface for tracing operations
type TracingRepository interface {
	CreateTrace(trace *models.TraceModel) error
	GetTrace(id string) (*models.TraceModel, error)
	ListTraces(tenantID, service string, limit, offset int) ([]*models.TraceModel, error)
	CreateSpan(span *models.SpanModel) error
	GetSpan(id string) (*models.SpanModel, error)
	ListSpans(traceID string) ([]*models.SpanModel, error)
	CreateServiceMetrics(metrics *models.ServiceMetricsModel) error
	GetServiceMetrics(tenantID, service string) (*models.ServiceMetricsModel, error)
	UpdateServiceMetrics(metrics *models.ServiceMetricsModel) error
}

// TracingRepositoryImpl implements TracingRepository
type TracingRepositoryImpl struct {
	db *gorm.DB
}

// NewTracingRepository creates tracing repository
func NewTracingRepository(db *gorm.DB) TracingRepository {
	return &TracingRepositoryImpl{db: db}
}

// CreateTrace creates trace
func (r *TracingRepositoryImpl) CreateTrace(trace *models.TraceModel) error {
	return r.db.Create(trace).Error
}

// GetTrace retrieves trace by ID
func (r *TracingRepositoryImpl) GetTrace(id string) (*models.TraceModel, error) {
	var trace models.TraceModel
	err := r.db.Preload("Spans").Where("id = ?", id).First(&trace).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("trace not found")
	}
	return &trace, err
}

// ListTraces lists traces
func (r *TracingRepositoryImpl) ListTraces(tenantID, service string, limit, offset int) ([]*models.TraceModel, error) {
	var traces []*models.TraceModel
	query := r.db.Preload("Spans").Where("tenant_id = ?", tenantID)
	if service != "" {
		query = query.Where("service = ?", service)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("start_time DESC").Find(&traces).Error
	return traces, err
}

// CreateSpan creates span
func (r *TracingRepositoryImpl) CreateSpan(span *models.SpanModel) error {
	return r.db.Create(span).Error
}

// GetSpan retrieves span by ID
func (r *TracingRepositoryImpl) GetSpan(id string) (*models.SpanModel, error) {
	var span models.SpanModel
	err := r.db.Where("id = ?", id).First(&span).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("span not found")
	}
	return &span, err
}

// ListSpans lists spans for trace
func (r *TracingRepositoryImpl) ListSpans(traceID string) ([]*models.SpanModel, error) {
	var spans []*models.SpanModel
	err := r.db.Where("trace_id = ?", traceID).Order("start_time ASC").Find(&spans).Error
	return spans, err
}

// CreateServiceMetrics creates service metrics
func (r *TracingRepositoryImpl) CreateServiceMetrics(metrics *models.ServiceMetricsModel) error {
	return r.db.Create(metrics).Error
}

// GetServiceMetrics retrieves service metrics
func (r *TracingRepositoryImpl) GetServiceMetrics(tenantID, service string) (*models.ServiceMetricsModel, error) {
	var metrics models.ServiceMetricsModel
	err := r.db.Where("tenant_id = ? AND service = ?", tenantID, service).First(&metrics).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("metrics not found")
	}
	return &metrics, err
}

// UpdateServiceMetrics updates service metrics
func (r *TracingRepositoryImpl) UpdateServiceMetrics(metrics *models.ServiceMetricsModel) error {
	return r.db.Save(metrics).Error
}
