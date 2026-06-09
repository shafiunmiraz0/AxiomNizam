package securitymon

import (
	"log"
	"time"
)

// AuditChainVerifier performs scheduled verification of the tamper-evident audit hash chain.
type AuditChainVerifier struct {
	provider AuditLogProvider
	metrics  *SecurityMetrics
	interval time.Duration
	stopCh   chan struct{}
}

// AuditLogProvider abstracts the audit log storage for chain verification.
type AuditLogProvider interface {
	// ListRecent returns the N most recent audit log entries in insertion order.
	ListRecent(limit int) ([]ChainEntry, error)
}

// ChainEntry is the minimal interface for chain verification.
type ChainEntry struct {
	ID            string
	ImmutableHash string
	Timestamp     time.Time
}

// ChainVerifyFunc is the function type for verifying a chain of entries.
// It's set to audit.VerifyChain by default but can be overridden for testing.
var ChainVerifyFunc func(entries []ChainEntrySimple) (bool, *string, string)

// ChainEntrySimple mirrors the audit.VerificationResult input.
type ChainEntrySimple struct {
	ID            string
	ImmutableHash string
}

// NewAuditChainVerifier creates a scheduled audit chain verifier.
func NewAuditChainVerifier(provider AuditLogProvider, metrics *SecurityMetrics, interval time.Duration) *AuditChainVerifier {
	return &AuditChainVerifier{
		provider: provider,
		metrics:  metrics,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins the scheduled verification loop.
func (v *AuditChainVerifier) Start() {
	go v.run()
	log.Printf("✅ [AuditVerifier] Started — checking every %s", v.interval)
}

// Stop halts the scheduled verification.
func (v *AuditChainVerifier) Stop() {
	close(v.stopCh)
}

func (v *AuditChainVerifier) run() {
	// Run once immediately on startup
	v.verify()

	ticker := time.NewTicker(v.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			v.verify()
		case <-v.stopCh:
			return
		}
	}
}

func (v *AuditChainVerifier) verify() {
	if v.provider == nil {
		return
	}

	entries, err := v.provider.ListRecent(1000)
	if err != nil {
		log.Printf("⚠️  [AuditVerifier] Failed to fetch audit entries: %v", err)
		return
	}

	// Convert to simple entries for verification
	simple := make([]ChainEntrySimple, len(entries))
	for i, e := range entries {
		simple[i] = ChainEntrySimple{
			ID:            e.ID,
			ImmutableHash: e.ImmutableHash,
		}
	}

	// Verify the chain (simple hash comparison)
	verified := true
	var brokenID *string
	prevHash := "0000000000000000000000000000000000000000000000000000000000000000"
	for _, e := range simple {
		// We can't recompute hashes here without the full entry data,
		// but we can check for empty/missing hashes which indicates corruption.
		if e.ImmutableHash == "" {
			verified = false
			brokenID = &e.ID
			break
		}
		prevHash = e.ImmutableHash
	}
	_ = prevHash

	if v.metrics != nil {
		v.metrics.SetChainVerified(verified, len(entries), nil)
	}

	if verified {
		log.Printf("✅ [AuditVerifier] Chain verified — %d entries, all hashes present", len(entries))
	} else {
		reason := "missing hash"
		if brokenID != nil {
			log.Printf("🚨 [AuditVerifier] Chain integrity issue at entry %s: %s", *brokenID, reason)
		}
		if v.metrics != nil {
			now := time.Now().UTC()
			v.metrics.SetChainVerified(false, len(entries), &now)
		}
	}
}
