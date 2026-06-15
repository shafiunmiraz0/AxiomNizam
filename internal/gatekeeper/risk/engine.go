package risk

import (
	"context"
	"net"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// Engine scores authentication requests based on risk signals.
type Engine struct {
	scorer Scorer
}

// Scorer provides risk scoring logic.
type Scorer interface {
	Score(ctx context.Context, signals *Signals) (int, error)
}

// Signals represents risk indicators for an authentication event.
type Signals struct {
	// Device/Location signals
	IsNewDevice       bool
	NewBrowser        bool
	VPNDetected       bool
	IPAddress         string
	GeoLocation       string
	DeviceFingerprint string

	// Temporal signals
	UnusualTimeOfDay  bool
	LastAuthTime      time.Time
	DaysSinceLastAuth int

	// Behavioral signals
	UnusualActivity  bool
	SuspiciousLogin  bool
	FrequentFailures bool
	FailureCount     int
	FailureWindow    time.Duration

	// Network signals
	ASNChange     bool
	GeoDifference int // km from last known location
	IPReputation  int // 0-100
	DatacenterIP  bool

	// User signals
	AccountAge      time.Duration
	HighPrivilegeOp bool
	SensitiveAction bool
}

// NewEngine creates a new risk scoring engine.
func NewEngine(s Scorer) *Engine {
	return &Engine{
		scorer: s,
	}
}

// Score evaluates risk signals and returns a risk score (0-100).
func (e *Engine) Score(ctx context.Context, signals *Signals) (int, error) {
	if e.scorer == nil {
		return 0, nil // No scorer = no risk
	}
	return e.scorer.Score(ctx, signals)
}

// ScoreAuthentication calculates a risk score for an authentication attempt.
// Implements contracts.RiskService.
func (e *Engine) ScoreAuthentication(ctx context.Context, userID models.UserID, ipAddress string) (int, error) {
	signals := &Signals{
		IPAddress: ipAddress,
	}
	return e.Score(ctx, signals)
}

// IsHighRisk returns true if the risk score exceeds the threshold (70).
// Implements contracts.RiskService.
func (e *Engine) IsHighRisk(ctx context.Context, score int) bool {
	return score >= 70
}

// DefaultScorer provides a simple risk scoring implementation.
type DefaultScorer struct{}

// Score calculates a cumulative risk score.
func (d *DefaultScorer) Score(ctx context.Context, signals *Signals) (int, error) {
	score := 0

	// Device/Location risk (up to 40 points)
	if signals.IsNewDevice {
		score += 10
	}
	if signals.NewBrowser {
		score += 5
	}
	if signals.VPNDetected {
		score += 10
	}
	if signals.DatacenterIP {
		score += 5
	}

	// Geographic signals (up to 30 points)
	if signals.ASNChange {
		score += 10
	}
	if signals.GeoDifference > 1000 { // > 1000km
		score += 15
	} else if signals.GeoDifference > 500 {
		score += 10
	}

	// IP reputation (up to 20 points)
	if signals.IPReputation > 70 {
		score += 20
	} else if signals.IPReputation > 50 {
		score += 10
	}

	// Behavioral risk (up to 30 points)
	if signals.UnusualTimeOfDay {
		score += 5
	}
	if signals.DaysSinceLastAuth > 90 {
		score += 5
	}
	if signals.UnusualActivity {
		score += 10
	}
	if signals.SuspiciousLogin {
		score += 15
	}

	// Failure tracking (up to 25 points)
	if signals.FailureCount > 3 {
		score += 15
	} else if signals.FailureCount > 1 {
		score += 5
	}

	// Account maturity (reduce score for established accounts)
	if signals.AccountAge > 365*24*time.Hour {
		score = score - 5 // Reduce by 5 for accounts older than 1 year
		if score < 0 {
			score = 0
		}
	}

	// Privilege escalation risk (up to 25 points)
	if signals.HighPrivilegeOp {
		score += 15
	}
	if signals.SensitiveAction {
		score += 10
	}

	// Cap score at 100
	if score > 100 {
		score = 100
	}

	return score, nil
}

// RiskLevel categorizes a numeric score into a risk level.
type RiskLevel int

const (
	RiskLevelLow      RiskLevel = iota // 0-30
	RiskLevelMedium                    // 31-60
	RiskLevelHigh                      // 61-80
	RiskLevelCritical                  // 81-100
)

// LevelForScore converts a numeric score to a risk level.
func LevelForScore(score int) RiskLevel {
	if score <= 30 {
		return RiskLevelLow
	}
	if score <= 60 {
		return RiskLevelMedium
	}
	if score <= 80 {
		return RiskLevelHigh
	}
	return RiskLevelCritical
}

// IsPrivateIP checks if an IP address is private (RFC 1918, etc).
func IsPrivateIP(ip string) bool {
	privateRanges := []string{
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
		"127.0.0.0/8",
		"::1/128",
		"fc00::/7",
	}

	addr := net.ParseIP(ip)
	if addr == nil {
		return false
	}

	for _, cidr := range privateRanges {
		_, net, err := net.ParseCIDR(cidr)
		if err != nil {
			continue
		}
		if net.Contains(addr) {
			return true
		}
	}

	return false
}
