package platform

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/bulk"
	"axiomnizam.bitbd.net/axiomnizam/internal/database"
	"axiomnizam.bitbd.net/axiomnizam/internal/eventbus"
	exportpkg "axiomnizam.bitbd.net/axiomnizam/internal/export"
	"axiomnizam.bitbd.net/axiomnizam/internal/lineage"
	"axiomnizam.bitbd.net/axiomnizam/internal/rbac"
	"axiomnizam.bitbd.net/axiomnizam/internal/streaming"
	"axiomnizam.bitbd.net/axiomnizam/internal/tenant"
	"axiomnizam.bitbd.net/axiomnizam/internal/tracing"
	"axiomnizam.bitbd.net/axiomnizam/internal/versioning"
	"axiomnizam.bitbd.net/axiomnizam/internal/webhooks"
)

// Managers bundles persistent platform manager implementations used by API handlers.
type Managers struct {
	Bulk     bulk.BulkManager
	EventBus eventbus.EventBusManager
	Export   exportpkg.ExportManager
	Stream   streaming.StreamManager
	Webhook  webhooks.WebhookManager
	Tenant   tenant.TenantManager
	RBAC     rbac.RBACManager
	Version  versioning.VersionManager
	Lineage  lineage.LineageManager
	Tracing  tracing.TracingManager
}

// NewManagers creates persistent platform managers.
// When etcd is available, uses etcd-backed persistence.
// When etcd is nil (e.g. STORAGE_BACKEND=raft), uses in-memory-only
// persistence (state is not persisted across restarts but managers
// still function).
func NewManagers(conns *database.Connections) (*Managers, error) {
	store := newPlatformStateStore(conns, "axiomnizam")

	return &Managers{
		Bulk:     newPersistentBulkManager(store),
		EventBus: newPersistentEventBusManager(store),
		Export:   &exportManagerAdapter{base: newPersistentExportCoreManager(store)},
		Stream:   newPersistentStreamManager(store),
		Webhook:  newPersistentWebhookManager(store),
		Tenant:   newPersistentTenantManager(store),
		RBAC:     &rbacManagerAdapter{base: newPersistentRBACCoreManager(store)},
		Version:  newPersistentVersionManager(store),
		Lineage:  &lineageManagerAdapter{base: newPersistentLineageCoreManager(store)},
		Tracing:  &tracingManagerAdapter{base: newPersistentTracingCoreManager(store)},
	}, nil
}
