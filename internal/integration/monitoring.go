package integration

import (
	"context"
	"fmt"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/apibanks"
	"axiomnizam.bitbd.net/axiomnizam/internal/mesh"
	"axiomnizam.bitbd.net/axiomnizam/internal/metrics"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// HealthStatus represents system health
type HealthStatus string

const (
	Healthy   HealthStatus = "healthy"
	Degraded  HealthStatus = "degraded"
	Unhealthy HealthStatus = "unhealthy"
)

// ComponentHealth represents health of a component
type ComponentHealth struct {
	Component   string                 `json:"component"`
	Status      HealthStatus           `json:"status"`
	LastChecked time.Time              `json:"lastChecked"`
	Details     map[string]interface{} `json:"details"`
	Error       string                 `json:"error,omitempty"`
}

// SystemHealth represents overall system health
type SystemHealth struct {
	Status     HealthStatus      `json:"status"`
	CheckedAt  time.Time         `json:"checkedAt"`
	Components []ComponentHealth `json:"components"`
	Uptime     time.Duration     `json:"uptime"`
	Summary    map[string]int    `json:"summary"`
}

// HealthMonitor monitors system health
type HealthMonitor struct {
	mu              sync.RWMutex
	startTime       time.Time
	lastCheckTimes  map[string]time.Time
	componentStatus map[string]ComponentHealth
	dataMesh        *mesh.DataMesh
	bankManager     *apibanks.APIBankManager
	metrics         *metrics.Metrics
	etcd            *clientv3.Client
	stateKey        string
}

type healthMonitorState struct {
	StartTime       time.Time                  `json:"startTime"`
	LastCheckTimes  map[string]time.Time       `json:"lastCheckTimes"`
	ComponentStatus map[string]ComponentHealth `json:"componentStatus"`
}

// NewHealthMonitor creates a new health monitor
func NewHealthMonitor() *HealthMonitor {
	hm := &HealthMonitor{
		startTime:       time.Now(),
		lastCheckTimes:  make(map[string]time.Time),
		componentStatus: make(map[string]ComponentHealth),
		dataMesh:        nil,
		bankManager:     nil,
		metrics:         nil,
		etcd:            integrationEtcdClient(),
		stateKey:        "integration:healthmonitor:state",
	}
	hm.loadState()
	return hm
}

func (hm *HealthMonitor) ConfigurePersistence(etcd *clientv3.Client) {
	hm.mu.Lock()
	hm.etcd = etcd
	if hm.stateKey == "" {
		hm.stateKey = "integration:healthmonitor:state"
	}
	hm.mu.Unlock()
	hm.loadState()
}

func (hm *HealthMonitor) loadState() {
	if hm.etcd == nil {
		return
	}

	var state healthMonitorState
	if !loadStateFromEtcd(hm.etcd, hm.stateKey, &state) {
		return
	}

	hm.mu.Lock()
	defer hm.mu.Unlock()
	if !state.StartTime.IsZero() {
		hm.startTime = state.StartTime
	}
	if state.LastCheckTimes != nil {
		hm.lastCheckTimes = state.LastCheckTimes
	}
	if state.ComponentStatus != nil {
		hm.componentStatus = state.ComponentStatus
	}
}

func (hm *HealthMonitor) persistStateLocked() {
	if hm.etcd == nil {
		return
	}

	saveStateToEtcd(hm.etcd, hm.stateKey, healthMonitorState{
		StartTime:       hm.startTime,
		LastCheckTimes:  hm.lastCheckTimes,
		ComponentStatus: hm.componentStatus,
	})
}

// CheckHealth performs a complete health check
func (hm *HealthMonitor) CheckHealth(ctx context.Context) *SystemHealth {
	hm.mu.Lock()
	defer hm.mu.Unlock()

	health := &SystemHealth{
		CheckedAt:  time.Now(),
		Uptime:     time.Since(hm.startTime),
		Components: make([]ComponentHealth, 0),
		Summary:    make(map[string]int),
	}

	// Check each component
	components := []string{"dataMesh", "apiBanks", "metrics", "eventSystem"}

	for _, comp := range components {
		switch comp {
		case "dataMesh":
			health.Components = append(health.Components, hm.checkDataMesh())
		case "apiBanks":
			health.Components = append(health.Components, hm.checkAPIBanks())
		case "metrics":
			health.Components = append(health.Components, hm.checkMetrics())
		case "eventSystem":
			health.Components = append(health.Components, hm.checkEventSystem())
		}
	}

	// Aggregate status
	healthyCount := 0
	degradedCount := 0
	unhealthyCount := 0

	for _, comp := range health.Components {
		hm.componentStatus[comp.Component] = comp
		hm.lastCheckTimes[comp.Component] = comp.LastChecked

		switch comp.Status {
		case Healthy:
			healthyCount++
		case Degraded:
			degradedCount++
		case Unhealthy:
			unhealthyCount++
		}
	}

	health.Summary["healthy"] = healthyCount
	health.Summary["degraded"] = degradedCount
	health.Summary["unhealthy"] = unhealthyCount

	// Determine overall status
	if unhealthyCount > 0 {
		health.Status = Unhealthy
	} else if degradedCount > 0 {
		health.Status = Degraded
	} else {
		health.Status = Healthy
	}

	hm.persistStateLocked()

	return health
}

// checkDataMesh checks data mesh component health
func (hm *HealthMonitor) checkDataMesh() ComponentHealth {
	comp := ComponentHealth{
		Component:   "dataMesh",
		LastChecked: time.Now(),
		Details:     make(map[string]interface{}),
	}

	if hm.dataMesh == nil {
		comp.Status = Unhealthy
		comp.Error = "data mesh not initialized"
		return comp
	}

	domains := hm.dataMesh.ListDomains()
	comp.Details["domainCount"] = len(domains)

	productCount := 0
	for _, domain := range domains {
		productCount += len(domain.DataProducts)
	}
	comp.Details["productCount"] = productCount

	subscriptionCount := 0
	for _, domain := range domains {
		for _, product := range domain.DataProducts {
			subscriptionCount += len(product.Subscriptions)
		}
	}
	comp.Details["subscriptionCount"] = subscriptionCount

	if len(domains) > 0 && productCount > 0 {
		comp.Status = Healthy
	} else {
		comp.Status = Degraded
	}

	return comp
}

// checkAPIBanks checks API bank component health
func (hm *HealthMonitor) checkAPIBanks() ComponentHealth {
	comp := ComponentHealth{
		Component:   "apiBanks",
		LastChecked: time.Now(),
		Details:     make(map[string]interface{}),
	}

	if hm.bankManager == nil {
		comp.Status = Unhealthy
		comp.Error = "bank manager not initialized"
		return comp
	}

	banks := hm.bankManager.ListBanks()
	comp.Details["bankCount"] = len(banks)

	apiCount := 0
	for _, bank := range banks {
		apiCount += len(bank.APIs)
	}
	comp.Details["apiCount"] = apiCount

	if len(banks) > 0 && apiCount > 0 {
		comp.Status = Healthy
	} else {
		comp.Status = Degraded
	}

	return comp
}

// checkMetrics checks metrics component health
func (hm *HealthMonitor) checkMetrics() ComponentHealth {
	comp := ComponentHealth{
		Component:   "metrics",
		LastChecked: time.Now(),
		Details:     make(map[string]interface{}),
	}

	if hm.metrics == nil {
		comp.Status = Unhealthy
		comp.Error = "metrics not initialized"
		return comp
	}

	comp.Status = Healthy
	comp.Details["recording"] = "active"

	return comp
}

// checkEventSystem checks event system health
func (hm *HealthMonitor) checkEventSystem() ComponentHealth {
	comp := ComponentHealth{
		Component:   "eventSystem",
		LastChecked: time.Now(),
		Details:     make(map[string]interface{}),
	}

	comp.Status = Healthy
	comp.Details["recording"] = "active"

	return comp
}

// PlatformMetricsCollector collects platform metrics
type PlatformMetricsCollector struct {
	mu          sync.RWMutex
	dataMesh    *mesh.DataMesh
	bankManager *apibanks.APIBankManager
	auditor     *ComplianceAuditor
}

// NewPlatformMetricsCollector creates a metrics collector
func NewPlatformMetricsCollector() *PlatformMetricsCollector {
	return &PlatformMetricsCollector{
		dataMesh:    nil,
		bankManager: nil,
		auditor:     nil,
	}
}

// PlatformMetrics represents platform metrics
type PlatformMetrics struct {
	Timestamp          time.Time              `json:"timestamp"`
	DataMeshMetrics    map[string]interface{} `json:"dataMeshMetrics"`
	APIBankMetrics     map[string]interface{} `json:"apiBankMetrics"`
	ComplianceMetrics  map[string]interface{} `json:"complianceMetrics"`
	PerformanceMetrics map[string]interface{} `json:"performanceMetrics"`
}

// CollectMetrics collects all platform metrics
func (pmc *PlatformMetricsCollector) CollectMetrics(ctx context.Context) *PlatformMetrics {
	metrics := &PlatformMetrics{
		Timestamp:          time.Now(),
		DataMeshMetrics:    make(map[string]interface{}),
		APIBankMetrics:     make(map[string]interface{}),
		ComplianceMetrics:  make(map[string]interface{}),
		PerformanceMetrics: make(map[string]interface{}),
	}

	// Data mesh metrics
	domains := pmc.dataMesh.ListDomains()
	metrics.DataMeshMetrics["domainCount"] = len(domains)
	metrics.DataMeshMetrics["averageDomainSize"] = calcAvgDomainSize(domains)

	// API bank metrics
	banks := pmc.bankManager.ListBanks()
	metrics.APIBankMetrics["bankCount"] = len(banks)
	metrics.APIBankMetrics["totalAPIs"] = countTotalAPIs(banks)

	// Compliance metrics
	report := pmc.auditor.GenerateReport(AuditFilter{})
	metrics.ComplianceMetrics["totalOperations"] = report.TotalOperations
	metrics.ComplianceMetrics["successRate"] = float64(report.SuccessfulOps) / float64(report.TotalOperations)
	metrics.ComplianceMetrics["denialRate"] = float64(report.DeniedOps) / float64(report.TotalOperations)

	// Performance metrics
	metrics.PerformanceMetrics["memoryUsage"] = "tracking"
	metrics.PerformanceMetrics["operationsPerSecond"] = "monitoring"

	return metrics
}

// calcAvgDomainSize calculates average domain size
func calcAvgDomainSize(domains []*mesh.DataDomain) float64 {
	if len(domains) == 0 {
		return 0
	}

	total := 0
	for _, d := range domains {
		total += len(d.DataProducts)
	}

	return float64(total) / float64(len(domains))
}

// countTotalAPIs counts total APIs
func countTotalAPIs(banks []*apibanks.APIBank) int {
	total := 0
	for _, b := range banks {
		total += len(b.APIs)
	}
	return total
}

// AlertManager manages system alerts
type AlertManager struct {
	mu               sync.RWMutex
	alerts           []Alert
	maxAlerts        int
	healthMonitor    *HealthMonitor
	metricsCollector *PlatformMetricsCollector
	etcd             *clientv3.Client
	stateKey         string
}

type alertManagerState struct {
	Alerts    []Alert `json:"alerts"`
	MaxAlerts int     `json:"maxAlerts"`
}

// Alert represents a system alert
type Alert struct {
	ID        string                 `json:"id"`
	Timestamp time.Time              `json:"timestamp"`
	Severity  string                 `json:"severity"` // "info", "warning", "critical"
	Component string                 `json:"component"`
	Message   string                 `json:"message"`
	Details   map[string]interface{} `json:"details"`
}

// NewAlertManager creates alert manager
func NewAlertManager(maxAlerts int) *AlertManager {
	am := &AlertManager{
		alerts:           make([]Alert, 0, maxAlerts),
		maxAlerts:        maxAlerts,
		healthMonitor:    NewHealthMonitor(),
		metricsCollector: NewPlatformMetricsCollector(),
		etcd:             integrationEtcdClient(),
		stateKey:         "integration:alerts:state",
	}
	am.loadState()
	return am
}

func (am *AlertManager) ConfigurePersistence(etcd *clientv3.Client) {
	am.mu.Lock()
	am.etcd = etcd
	if am.stateKey == "" {
		am.stateKey = "integration:alerts:state"
	}
	if am.healthMonitor != nil {
		am.healthMonitor.ConfigurePersistence(etcd)
	}
	am.mu.Unlock()
	am.loadState()
}

func (am *AlertManager) loadState() {
	if am.etcd == nil {
		return
	}

	var state alertManagerState
	if !loadStateFromEtcd(am.etcd, am.stateKey, &state) {
		return
	}

	am.mu.Lock()
	defer am.mu.Unlock()
	if state.Alerts != nil {
		am.alerts = state.Alerts
	}
	if state.MaxAlerts > 0 {
		am.maxAlerts = state.MaxAlerts
	}
}

func (am *AlertManager) persistStateLocked() {
	if am.etcd == nil {
		return
	}

	saveStateToEtcd(am.etcd, am.stateKey, alertManagerState{
		Alerts:    am.alerts,
		MaxAlerts: am.maxAlerts,
	})
}

// GenerateAlerts generates alerts based on system state
func (am *AlertManager) GenerateAlerts(ctx context.Context) []Alert {
	am.mu.Lock()
	defer am.mu.Unlock()

	health := am.healthMonitor.CheckHealth(ctx)

	newAlerts := make([]Alert, 0)

	// Alert for unhealthy components
	for _, comp := range health.Components {
		if comp.Status == Unhealthy {
			alert := Alert{
				ID:        fmt.Sprintf("alert_%d", time.Now().UnixNano()),
				Timestamp: time.Now(),
				Severity:  "critical",
				Component: comp.Component,
				Message:   fmt.Sprintf("Component %s is unhealthy: %s", comp.Component, comp.Error),
				Details:   comp.Details,
			}
			newAlerts = append(newAlerts, alert)
		}
	}

	// Add new alerts
	am.alerts = append(am.alerts, newAlerts...)

	// Keep only recent alerts
	if len(am.alerts) > am.maxAlerts {
		am.alerts = am.alerts[len(am.alerts)-am.maxAlerts:]
	}
	am.persistStateLocked()

	return newAlerts
}

// GetActiveAlerts returns active alerts
func (am *AlertManager) GetActiveAlerts() []Alert {
	am.mu.RLock()
	defer am.mu.RUnlock()

	return am.alerts
}

// GlobalHealthMonitor is the package-level health monitor
var GlobalHealthMonitor = NewHealthMonitor()

// GlobalPlatformMetricsCollector is the package-level metrics collector
var GlobalPlatformMetricsCollector = NewPlatformMetricsCollector()

// GlobalAlertManager is the package-level alert manager
var GlobalAlertManager = NewAlertManager(1000)
