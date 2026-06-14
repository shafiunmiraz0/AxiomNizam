package bootstrap

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/audit"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/cache"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/contracts"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/metrics"
)

// Provider implements contracts.Provider for dependency injection.
type Provider struct {
	enrollmentSvc contracts.EnrollmentService
	challengeSvc  contracts.ChallengeService
	factorSvc     contracts.FactorService
	policySvc     contracts.PolicyService
	riskSvc       contracts.RiskService
	deviceSvc     contracts.TrustedDeviceService
	backupSvc     contracts.BackupCodeService
	auditLog      *audit.Logger
	coll          *metrics.Collector
	cacheSvc      cache.Store
}

// NewProvider creates a new Provider.
func NewProvider(
	enrollmentSvc contracts.EnrollmentService,
	challengeSvc contracts.ChallengeService,
	factorSvc contracts.FactorService,
	policySvc contracts.PolicyService,
	riskSvc contracts.RiskService,
	deviceSvc contracts.TrustedDeviceService,
	backupSvc contracts.BackupCodeService,
	auditLog *audit.Logger,
	coll *metrics.Collector,
	cacheSvc cache.Store,
) *Provider {
	return &Provider{
		enrollmentSvc: enrollmentSvc,
		challengeSvc:  challengeSvc,
		factorSvc:     factorSvc,
		policySvc:     policySvc,
		riskSvc:       riskSvc,
		deviceSvc:     deviceSvc,
		backupSvc:     backupSvc,
		auditLog:      auditLog,
		coll:          coll,
		cacheSvc:      cacheSvc,
	}
}

func (p *Provider) EnrollmentService() contracts.EnrollmentService { return p.enrollmentSvc }
func (p *Provider) ChallengeService() contracts.ChallengeService   { return p.challengeSvc }
func (p *Provider) FactorService() contracts.FactorService         { return p.factorSvc }
func (p *Provider) PolicyService() contracts.PolicyService         { return p.policySvc }
func (p *Provider) RiskService() contracts.RiskService             { return p.riskSvc }
func (p *Provider) TrustedDeviceService() contracts.TrustedDeviceService { return p.deviceSvc }
func (p *Provider) BackupCodeService() contracts.BackupCodeService { return p.backupSvc }
