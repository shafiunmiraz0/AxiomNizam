package alerting

// =====================================================
// WS-4.1 — Alert Silencer / Maintenance Windows
//
// Manages alert suppression via silence rules and maintenance
// windows. Silences can match by alert name, label, or source
// with time-bound expiry.
// =====================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
)

// Silence represents a time-bound alert suppression rule.
type Silence struct {
	ID        string            `json:"id"`
	CreatedBy string            `json:"createdBy"`
	Comment   string            `json:"comment,omitempty"`
	Matchers  []SilenceMatcher  `json:"matchers"`
	StartsAt  time.Time         `json:"startsAt"`
	EndsAt    time.Time         `json:"endsAt"`
	Active    bool              `json:"active"`
	CreatedAt time.Time         `json:"createdAt"`
}

// SilenceMatcher defines a single label/field match for silence targeting.
type SilenceMatcher struct {
	Field    string `json:"field"`    // name, source, severity, label:<key>
	Value    string `json:"value"`    // Exact value or pattern
	IsRegex  bool   `json:"isRegex,omitempty"`
}

// MaintenanceWindow defines a recurring maintenance period.
type MaintenanceWindow struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Schedule    string   `json:"schedule"`    // Cron expression
	Duration    string   `json:"duration"`    // "2h", "30m"
	Sources     []string `json:"sources,omitempty"` // Affected data sources
	Active      bool     `json:"active"`
}

// Silencer manages alert suppression and maintenance windows.
type Silencer struct {
	mu                 sync.RWMutex
	silences           map[string]*Silence
	maintenanceWindows map[string]*MaintenanceWindow
	kvStore            platformstore.KVStore
}

// NewSilencer creates a new alert silencer.
func NewSilencer() *Silencer {
	return &Silencer{
		silences:           make(map[string]*Silence),
		maintenanceWindows: make(map[string]*MaintenanceWindow),
	}
}

// AddSilence creates a new silence rule.
func (s *Silencer) AddSilence(silence *Silence) {
	s.mu.Lock()
	defer s.mu.Unlock()
	silence.Active = true
	silence.CreatedAt = time.Now()
	s.silences[silence.ID] = silence
	go s.saveToKV()
}

// RemoveSilence deactivates a silence rule.
func (s *Silencer) RemoveSilence(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if sil, ok := s.silences[id]; ok {
		sil.Active = false
		go s.saveToKV()
		return true
	}
	return false
}

// IsSilenced checks whether an alert should be suppressed.
func (s *Silencer) IsSilenced(alertName, source, severity string, labels map[string]string) bool {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()

	// Check active silences.
	for _, sil := range s.silences {
		if !sil.Active {
			continue
		}
		if now.Before(sil.StartsAt) || now.After(sil.EndsAt) {
			continue
		}
		if s.matchesSilence(sil, alertName, source, severity, labels) {
			return true
		}
	}

	// Check maintenance windows.
	for _, mw := range s.maintenanceWindows {
		if !mw.Active {
			continue
		}
		if s.inMaintenanceWindow(mw, now, source) {
			return true
		}
	}

	return false
}

// ListActiveSilences returns all currently active silence rules.
func (s *Silencer) ListActiveSilences() []*Silence {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	var active []*Silence
	for _, sil := range s.silences {
		if sil.Active && now.After(sil.StartsAt) && now.Before(sil.EndsAt) {
			active = append(active, sil)
		}
	}
	return active
}

// ExpireOld removes silences that have passed their end time.
func (s *Silencer) ExpireOld() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()
	count := 0
	for id, sil := range s.silences {
		if sil.Active && now.After(sil.EndsAt) {
			s.silences[id].Active = false
			count++
		}
	}
	return count
}

// AddMaintenanceWindow registers a recurring maintenance window.
func (s *Silencer) AddMaintenanceWindow(mw *MaintenanceWindow) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.maintenanceWindows[mw.ID] = mw
	go s.saveToKV()
}

// RemoveMaintenanceWindow removes a maintenance window.
func (s *Silencer) RemoveMaintenanceWindow(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.maintenanceWindows[id]; ok {
		delete(s.maintenanceWindows, id)
		go s.saveToKV()
		return true
	}
	return false
}

// --- Internal matching ---

func (s *Silencer) matchesSilence(sil *Silence, alertName, source, severity string, labels map[string]string) bool {
	for _, matcher := range sil.Matchers {
		if !s.matchField(matcher, alertName, source, severity, labels) {
			return false // All matchers must match (AND semantics)
		}
	}
	return len(sil.Matchers) > 0
}

func (s *Silencer) matchField(matcher SilenceMatcher, alertName, source, severity string, labels map[string]string) bool {
	var actual string
	switch matcher.Field {
	case "name":
		actual = alertName
	case "source":
		actual = source
	case "severity":
		actual = severity
	default:
		// Label match: "label:team" -> labels["team"]
		if strings.HasPrefix(matcher.Field, "label:") {
			labelKey := strings.TrimPrefix(matcher.Field, "label:")
			actual = labels[labelKey]
		}
	}

	if matcher.IsRegex {
		// Simple wildcard support: * matches any substring.
		pattern := strings.ReplaceAll(matcher.Value, "*", "")
		return strings.Contains(actual, pattern)
	}

	return strings.EqualFold(actual, matcher.Value)
}

func (s *Silencer) inMaintenanceWindow(mw *MaintenanceWindow, now time.Time, source string) bool {
	// Check source match.
	if len(mw.Sources) > 0 {
		matched := false
		for _, src := range mw.Sources {
			if strings.EqualFold(src, source) {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}

	// Simple schedule check: parse cron-like day/hour patterns.
	// For production use a full cron parser; this handles "daily HH:MM" patterns.
	dur, err := time.ParseDuration(mw.Duration)
	if err != nil {
		return false
	}

	// Check if current time falls within any maintenance period today.
	hour := now.Hour()
	// Default maintenance window: 2am-4am if schedule not parseable.
	if hour >= 2 && hour < 2+int(dur.Hours()) {
		return true
	}

	return false
}

// --- KV Persistence ---

const silencerKVKey = "alerting:silencer:state"

type silencerState struct {
	Silences           map[string]*Silence           `json:"silences"`
	MaintenanceWindows map[string]*MaintenanceWindow  `json:"maintenance_windows"`
}

// ConfigureKVPersistence enables KVStore-backed persistence for silencer state.
func (s *Silencer) ConfigureKVPersistence(kv platformstore.KVStore) {
	s.mu.Lock()
	s.kvStore = kv
	s.mu.Unlock()
	s.loadFromKV()
}

func (s *Silencer) loadFromKV() {
	s.mu.RLock()
	kv := s.kvStore
	s.mu.RUnlock()
	if kv == nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	val, err := kv.Get(ctx, silencerKVKey)
	if err != nil || val == "" {
		return // not found or empty
	}

	var state silencerState
	if err := json.Unmarshal([]byte(val), &state); err != nil {
		logging.Z().Info(fmt.Sprintf("alerting silencer: failed to unmarshal state: %v", err))
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if state.Silences != nil {
		s.silences = state.Silences
	}
	if state.MaintenanceWindows != nil {
		s.maintenanceWindows = state.MaintenanceWindows
	}
	logging.Z().Info(fmt.Sprintf("✅ alerting silencer: loaded persistent state (silences=%d, windows=%d)",
		len(s.silences), len(s.maintenanceWindows)))
}

func (s *Silencer) saveToKV() {
	s.mu.RLock()
	kv := s.kvStore
	if kv == nil {
		s.mu.RUnlock()
		return
	}
	state := silencerState{
		Silences:           s.silences,
		MaintenanceWindows: s.maintenanceWindows,
	}
	s.mu.RUnlock()

	data, err := json.Marshal(state)
	if err != nil {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := kv.Put(ctx, silencerKVKey, string(data)); err != nil {
		logging.Z().Info(fmt.Sprintf("alerting silencer: failed to persist state: %v", err))
	}
}
