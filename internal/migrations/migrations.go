package migrations

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// RunMigrations executes all database migrations
func RunMigrations(db *gorm.DB) error {
	// Audit migrations
	if err := db.AutoMigrate(&models.AuditLogModel{}); err != nil {
		return err
	}

	// Tenant migrations
	if err := db.AutoMigrate(&models.TenantModel{}, &models.TenantMemberModel{}, &models.TenantQuotaModel{}); err != nil {
		return err
	}

	// Job migrations
	if err := db.AutoMigrate(&models.JobModel{}, &models.JobLogModel{}); err != nil {
		return err
	}

	// Streaming migrations
	if err := db.AutoMigrate(&models.StreamModel{}, &models.StreamSubscriptionModel{}); err != nil {
		return err
	}

	// Bulk operation migrations
	if err := db.AutoMigrate(&models.BulkOperationModel{}, &models.BulkResultModel{}); err != nil {
		return err
	}

	// Versioning migrations
	if err := db.AutoMigrate(&models.ResourceVersionModel{}, &models.VersionSnapshotModel{}); err != nil {
		return err
	}

	// Webhook migrations
	if err := db.AutoMigrate(&models.WebhookModel{}, &models.WebhookDeliveryLogModel{}); err != nil {
		return err
	}

	// Event bus migrations
	if err := db.AutoMigrate(
		&models.EventModel{},
		&models.TopicModel{},
		&models.SubscriptionModel{},
		&models.DeadLetterEventModel{},
	); err != nil {
		return err
	}

	// Tracing migrations (traces before spans due to FK constraint)
	if err := db.AutoMigrate(&models.TraceModel{}); err != nil {
		return err
	}
	if err := db.AutoMigrate(&models.SpanModel{}, &models.ServiceMetricsModel{}); err != nil {
		return err
	}

	// Export migrations
	if err := db.AutoMigrate(&models.ExportJobModel{}, &models.ExportResultModel{}, &models.ExportTemplateModel{}); err != nil {
		return err
	}

	// Lineage migrations
	if err := db.AutoMigrate(
		&models.LineageNodeModel{},
		&models.LineageEdgeModel{},
		&models.LineageProcessModel{},
	); err != nil {
		return err
	}

	// Encryption migrations
	if err := db.AutoMigrate(
		&models.EncryptionKeyModel{},
		&models.EncryptionPolicyModel{},
		&models.KeyRotationModel{},
		&models.EncryptionAuditLogModel{},
	); err != nil {
		return err
	}

	// RBAC migrations
	if err := db.AutoMigrate(
		&models.RoleModel{},
		&models.RoleBindingModel{},
		&models.PermissionModel{},
		&models.AccessRequestModel{},
	); err != nil {
		return err
	}

	// Create indexes for performance
	if err := createIndexes(db); err != nil {
		return err
	}

	return nil
}

// indexDef defines a model and its corresponding index creation SQL.
type indexDef struct {
	model interface{}
	sql   string
}

// createIndexes creates additional performance indexes
func createIndexes(db *gorm.DB) error {
	indexes := []indexDef{
		{&models.AuditLogModel{}, "CREATE INDEX IF NOT EXISTS idx_audit_tenant_action ON audit_logs(tenant_id, action_type, created_at)"},
		{&models.JobModel{}, "CREATE INDEX IF NOT EXISTS idx_job_tenant_status ON jobs(tenant_id, status, created_at)"},
		{&models.StreamModel{}, "CREATE INDEX IF NOT EXISTS idx_stream_tenant_status ON streams(tenant_id, status, created_at)"},
		{&models.BulkOperationModel{}, "CREATE INDEX IF NOT EXISTS idx_bulk_tenant_status ON bulk_operations(tenant_id, status, created_at)"},
		{&models.ResourceVersionModel{}, "CREATE INDEX IF NOT EXISTS idx_version_resource ON resource_versions(tenant_id, resource_type, resource_id, version_number DESC)"},
		{&models.WebhookModel{}, "CREATE INDEX IF NOT EXISTS idx_webhook_tenant ON webhooks(tenant_id, active, created_at)"},
		{&models.EventModel{}, "CREATE INDEX IF NOT EXISTS idx_event_topic ON events(tenant_id, topic, created_at DESC)"},
		{&models.SubscriptionModel{}, "CREATE INDEX IF NOT EXISTS idx_subscription_topic ON subscriptions(tenant_id, topic, status)"},
		{&models.TraceModel{}, "CREATE INDEX IF NOT EXISTS idx_trace_service ON traces(tenant_id, service, start_time DESC)"},
		{&models.SpanModel{}, "CREATE INDEX IF NOT EXISTS idx_span_trace ON spans(trace_id, start_time ASC)"},
		{&models.ExportJobModel{}, "CREATE INDEX IF NOT EXISTS idx_export_tenant_status ON export_jobs(tenant_id, status, created_at DESC)"},
		{&models.LineageEdgeModel{}, "CREATE INDEX IF NOT EXISTS idx_lineage_edges ON lineage_edges(tenant_id, source_node_id, target_node_id)"},
		{&models.EncryptionKeyModel{}, "CREATE INDEX IF NOT EXISTS idx_encryption_key_tenant ON encryption_keys(tenant_id, status, created_at DESC)"},
		{&models.RoleModel{}, "CREATE INDEX IF NOT EXISTS idx_role_tenant_level ON roles(tenant_id, level DESC)"},
		{&models.RoleBindingModel{}, "CREATE INDEX IF NOT EXISTS idx_binding_subject ON role_bindings(tenant_id, subject_id)"},
		{&models.AccessRequestModel{}, "CREATE INDEX IF NOT EXISTS idx_access_request_subject ON access_requests(tenant_id, subject_id, status)"},
	}

	for _, idx := range indexes {
		if err := db.Model(idx.model).Exec(idx.sql).Error; err != nil {
			return err
		}
	}

	return nil
}
