// Package yara implements Layer 4 of the AxiomNizam antivirus engine:
// a lightweight, pure-Go YARA-compatible rule matcher for community
// threat intelligence rules.
//
// # Supported YARA Subset
//
// This is NOT a full YARA implementation. We support the core subset
// that covers ~90% of community detection rules:
//
//   - meta: block (author, description, severity, etc.)
//   - strings: text strings ($s = "text") and hex strings ($h = { AA BB })
//   - condition: basic boolean logic:
//     "any of them", "all of them", "$s1", "$s1 and $s2",
//     "$s1 or $s2", "N of them", "N of ($s*)"
//
// Features NOT supported (would require full libyara):
//   - Regular expressions in strings
//   - String modifiers (nocase, wide, fullword, xor) — partial support
//   - File-size / entry-point / import conditions
//   - For-loops and set operations
//   - Modules (pe, elf, math, etc.)
package yara

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"bufio"
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Rule
// ─────────────────────────────────────────────────────────────────────────────

// Rule is a parsed YARA rule.
type Rule struct {
	Name      string            `json:"name"`
	Tags      []string          `json:"tags,omitempty"`
	Meta      map[string]string `json:"meta,omitempty"`
	Strings   []RuleString      `json:"strings"`
	Condition string            `json:"condition"`
}

// RuleString is a single string definition within a rule.
type RuleString struct {
	ID      string // e.g. "$s1"
	Value   []byte // decoded bytes to search for
	IsHex   bool   // true if defined as hex { AA BB }
	Nocase  bool   // case-insensitive match
}

// ─────────────────────────────────────────────────────────────────────────────
// Match Result
// ─────────────────────────────────────────────────────────────────────────────

// RuleMatch represents a rule that matched a file.
type RuleMatch struct {
	Rule           *Rule
	MatchedStrings []string // IDs of strings that matched
}

// ─────────────────────────────────────────────────────────────────────────────
// Rule Set
// ─────────────────────────────────────────────────────────────────────────────

// RuleSet holds a collection of compiled rules ready for matching.
type RuleSet struct {
	rules []Rule
}

// NewRuleSet creates an empty rule set.
func NewRuleSet() *RuleSet {
	return &RuleSet{}
}

// AddRule adds a pre-parsed rule to the set.
func (rs *RuleSet) AddRule(r Rule) {
	rs.rules = append(rs.rules, r)
}

// RuleCount returns the number of rules in the set.
func (rs *RuleSet) RuleCount() int {
	return len(rs.rules)
}

// Match evaluates all rules against the given data and returns matches.
func (rs *RuleSet) Match(data []byte) []RuleMatch {
	if len(rs.rules) == 0 || len(data) == 0 {
		return nil
	}

	var matches []RuleMatch

	for i := range rs.rules {
		rule := &rs.rules[i]

		// Find which strings match.
		matched := matchStrings(rule.Strings, data)

		// Evaluate condition.
		if evaluateCondition(rule.Condition, rule.Strings, matched) {
			ids := make([]string, 0, len(matched))
			for id := range matched {
				ids = append(ids, id)
			}
			matches = append(matches, RuleMatch{
				Rule:           rule,
				MatchedStrings: ids,
			})
		}
	}

	return matches
}

// matchStrings searches for all rule strings in the data.
// Returns a map of string-ID → true for strings that were found.
func matchStrings(strs []RuleString, data []byte) map[string]bool {
	result := make(map[string]bool, len(strs))
	for _, s := range strs {
		if len(s.Value) == 0 {
			continue
		}
		if s.Nocase {
			if bytes.Contains(bytes.ToLower(data), bytes.ToLower(s.Value)) {
				result[s.ID] = true
			}
		} else {
			if bytes.Contains(data, s.Value) {
				result[s.ID] = true
			}
		}
	}
	return result
}

// evaluateCondition evaluates a simplified YARA condition.
func evaluateCondition(cond string, strs []RuleString, matched map[string]bool) bool {
	cond = strings.TrimSpace(cond)
	if cond == "" {
		return false
	}

	// "any of them" — at least one string matched.
	if cond == "any of them" {
		return len(matched) > 0
	}

	// "all of them" — every string matched.
	if cond == "all of them" {
		for _, s := range strs {
			if !matched[s.ID] {
				return false
			}
		}
		return len(strs) > 0
	}

	// "N of them" — at least N strings matched.
	if strings.HasSuffix(cond, " of them") {
		nStr := strings.TrimSuffix(cond, " of them")
		n, err := strconv.Atoi(strings.TrimSpace(nStr))
		if err == nil {
			return len(matched) >= n
		}
	}

	// "N of ($s*)" — N of strings matching a prefix.
	if strings.Contains(cond, " of (") {
		return evalNOfPrefix(cond, strs, matched)
	}

	// Boolean expressions with "and" / "or".
	if strings.Contains(cond, " and ") {
		parts := strings.Split(cond, " and ")
		for _, part := range parts {
			if !evalSingleTerm(strings.TrimSpace(part), matched) {
				return false
			}
		}
		return true
	}

	if strings.Contains(cond, " or ") {
		parts := strings.Split(cond, " or ")
		for _, part := range parts {
			if evalSingleTerm(strings.TrimSpace(part), matched) {
				return true
			}
		}
		return false
	}

	// Single variable reference: "$s1"
	return evalSingleTerm(cond, matched)
}

// evalSingleTerm evaluates a single condition term (a string variable reference).
func evalSingleTerm(term string, matched map[string]bool) bool {
	term = strings.TrimSpace(term)
	if strings.HasPrefix(term, "$") {
		return matched[term]
	}
	// "any of them" nested.
	if term == "any of them" {
		return len(matched) > 0
	}
	return false
}

// evalNOfPrefix handles "N of ($prefix*)" conditions.
func evalNOfPrefix(cond string, strs []RuleString, matched map[string]bool) bool {
	// Parse "N of ($prefix*)"
	parts := strings.SplitN(cond, " of (", 2)
	if len(parts) != 2 {
		return false
	}

	n, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return false
	}

	prefixPart := strings.TrimSuffix(strings.TrimSpace(parts[1]), ")")
	prefixPart = strings.TrimSuffix(prefixPart, "*")
	prefix := strings.TrimSpace(prefixPart)

	count := 0
	for _, s := range strs {
		if strings.HasPrefix(s.ID, prefix) && matched[s.ID] {
			count++
		}
	}
	return count >= n
}

// ─────────────────────────────────────────────────────────────────────────────
// Parser
// ─────────────────────────────────────────────────────────────────────────────

// ParseRule parses a YARA rule from its text representation.
func ParseRule(text string) (*Rule, error) {
	scanner := bufio.NewScanner(strings.NewReader(text))
	rule := &Rule{Meta: make(map[string]string)}

	state := "init" // init → meta → strings → condition

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}

		switch state {
		case "init":
			if strings.HasPrefix(line, "rule ") {
				header := strings.TrimPrefix(line, "rule ")
				header = strings.TrimSuffix(header, "{")
				header = strings.TrimSpace(header)

				// Parse tags: "rule name : tag1 tag2"
				if colonIdx := strings.Index(header, ":"); colonIdx >= 0 {
					rule.Name = strings.TrimSpace(header[:colonIdx])
					tagStr := strings.TrimSpace(header[colonIdx+1:])
					if tagStr != "" {
						rule.Tags = strings.Fields(tagStr)
					}
				} else {
					rule.Name = strings.TrimSpace(header)
				}
			}
			if strings.HasPrefix(line, "meta:") {
				state = "meta"
			} else if strings.HasPrefix(line, "strings:") {
				state = "strings"
			} else if strings.HasPrefix(line, "condition:") {
				state = "condition"
				// Condition may be on the same line.
				after := strings.TrimPrefix(line, "condition:")
				after = strings.TrimSpace(after)
				if after != "" {
					rule.Condition = after
				}
			}

		case "meta":
			if strings.HasPrefix(line, "strings:") {
				state = "strings"
				continue
			}
			if strings.HasPrefix(line, "condition:") {
				state = "condition"
				after := strings.TrimPrefix(line, "condition:")
				after = strings.TrimSpace(after)
				if after != "" {
					rule.Condition = after
				}
				continue
			}
			if line == "}" {
				continue
			}
			// Parse meta key = "value" or key = number.
			if eqIdx := strings.Index(line, "="); eqIdx >= 0 {
				key := strings.TrimSpace(line[:eqIdx])
				val := strings.TrimSpace(line[eqIdx+1:])
				val = strings.Trim(val, "\"")
				rule.Meta[key] = val
			}

		case "strings":
			if strings.HasPrefix(line, "condition:") {
				state = "condition"
				after := strings.TrimPrefix(line, "condition:")
				after = strings.TrimSpace(after)
				if after != "" {
					rule.Condition = after
				}
				continue
			}
			if line == "}" {
				continue
			}
			// Parse string definitions.
			if rs, err := parseRuleString(line); err == nil {
				rule.Strings = append(rule.Strings, rs)
			}

		case "condition":
			if line == "}" {
				continue
			}
			// Accumulate condition (may span multiple lines).
			if rule.Condition != "" {
				rule.Condition += " " + line
			} else {
				rule.Condition = line
			}
		}
	}

	if rule.Name == "" {
		return nil, fmt.Errorf("no rule name found")
	}
	if rule.Condition == "" {
		return nil, fmt.Errorf("rule %q has no condition", rule.Name)
	}

	return rule, nil
}

// parseRuleString parses a single string definition line.
func parseRuleString(line string) (RuleString, error) {
	// Format: $identifier = "text" [nocase]
	// Format: $identifier = { AA BB CC }

	eqIdx := strings.Index(line, "=")
	if eqIdx < 0 {
		return RuleString{}, fmt.Errorf("no = in string definition")
	}

	id := strings.TrimSpace(line[:eqIdx])
	if !strings.HasPrefix(id, "$") {
		return RuleString{}, fmt.Errorf("string ID must start with $")
	}

	rest := strings.TrimSpace(line[eqIdx+1:])
	nocase := false

	// Check for "nocase" modifier.
	if strings.HasSuffix(rest, "nocase") {
		nocase = true
		rest = strings.TrimSpace(strings.TrimSuffix(rest, "nocase"))
	}

	// Hex string: { AA BB CC DD }
	if strings.HasPrefix(rest, "{") && strings.HasSuffix(rest, "}") {
		hexStr := rest[1 : len(rest)-1]
		hexStr = strings.TrimSpace(hexStr)
		hexStr = strings.ReplaceAll(hexStr, " ", "")
		hexStr = strings.ReplaceAll(hexStr, "\t", "")

		val, err := hex.DecodeString(hexStr)
		if err != nil {
			return RuleString{}, fmt.Errorf("invalid hex: %w", err)
		}
		return RuleString{ID: id, Value: val, IsHex: true, Nocase: nocase}, nil
	}

	// Text string: "content"
	if strings.HasPrefix(rest, "\"") && strings.HasSuffix(rest, "\"") {
		text := rest[1 : len(rest)-1]
		// Handle basic escape sequences.
		text = strings.ReplaceAll(text, "\\n", "\n")
		text = strings.ReplaceAll(text, "\\r", "\r")
		text = strings.ReplaceAll(text, "\\t", "\t")
		text = strings.ReplaceAll(text, "\\\\", "\\")
		text = strings.ReplaceAll(text, "\\\"", "\"")
		return RuleString{ID: id, Value: []byte(text), IsHex: false, Nocase: nocase}, nil
	}

	return RuleString{}, fmt.Errorf("unrecognized string format: %s", rest)
}

// ─────────────────────────────────────────────────────────────────────────────
// File Loader
// ─────────────────────────────────────────────────────────────────────────────

// LoadFile parses a YARA rule file (may contain multiple rules).
func LoadFile(path string) ([]Rule, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %q: %w", path, err)
	}
	defer f.Close()

	return Load(f)
}

// Load parses YARA rules from a reader.
func Load(r io.Reader) ([]Rule, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	return ParseRules(string(data))
}

// ParseRules splits a YARA source into individual rules and parses each.
func ParseRules(source string) ([]Rule, error) {
	var rules []Rule
	var current strings.Builder
	depth := 0

	scanner := bufio.NewScanner(strings.NewReader(source))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "rule ") {
			current.Reset()
			depth = 0
		}

		current.WriteString(line)
		current.WriteString("\n")

		depth += strings.Count(trimmed, "{") - strings.Count(trimmed, "}")

		if depth <= 0 && current.Len() > 0 && strings.Contains(current.String(), "rule ") {
			rule, err := ParseRule(current.String())
			if err == nil {
				rules = append(rules, *rule)
			}
			current.Reset()
		}
	}

	return rules, scanner.Err()
}

// LoadFromDir loads all .yar and .yara files from a directory.
func LoadFromDir(rs *RuleSet, dir string) (int, []error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			logging.Z().Info(fmt.Sprintf("🛡️  yara: rule directory %q does not exist, skipping", dir))
			return 0, nil
		}
		return 0, []error{err}
	}

	loaded := 0
	var errs []error

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(entry.Name()))
		if ext != ".yar" && ext != ".yara" {
			continue
		}

		path := filepath.Join(dir, entry.Name())
		rules, err := LoadFile(path)
		if err != nil {
			errs = append(errs, fmt.Errorf("load %q: %w", entry.Name(), err))
			continue
		}

		for _, r := range rules {
			rs.AddRule(r)
		}
		loaded += len(rules)
		logging.Z().Info(fmt.Sprintf("🛡️  yara: loaded %d rules from %s", len(rules), entry.Name()))
	}

	return loaded, errs
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer — ScanLayer implementation
// ─────────────────────────────────────────────────────────────────────────────

// Layer implements antivirus.ScanLayer for YARA rule matching.
type Layer struct {
	mu      sync.RWMutex
	ruleset *RuleSet
	scans   atomic.Int64
	matches atomic.Int64
}

// NewLayer creates a YARA scan layer with the given rule set.
func NewLayer(rs *RuleSet) *Layer {
	if rs == nil {
		rs = NewRuleSet()
	}
	return &Layer{ruleset: rs}
}

// Name returns the layer identifier.
func (l *Layer) Name() string { return "yara" }

// Scan evaluates all YARA rules against the target file.
func (l *Layer) Scan(target *antivirus.ScanTarget) ([]antivirus.ThreatInfo, error) {
	if len(target.Content) == 0 {
		return nil, nil
	}

	l.scans.Add(1)

	l.mu.RLock()
	rs := l.ruleset
	l.mu.RUnlock()

	if rs == nil || rs.RuleCount() == 0 {
		return nil, nil
	}

	ruleMatches := rs.Match(target.Content)
	if len(ruleMatches) == 0 {
		return nil, nil
	}

	l.matches.Add(int64(len(ruleMatches)))

	threats := make([]antivirus.ThreatInfo, 0, len(ruleMatches))
	for _, rm := range ruleMatches {
		category := antivirus.CategoryGeneric
		severity := antivirus.SeverityMedium
		confidence := 0.80
		desc := fmt.Sprintf("YARA rule matched: %s", rm.Rule.Name)

		// Extract metadata.
		if v, ok := rm.Rule.Meta["category"]; ok {
			category = inferYARACategory(v)
		}
		if v, ok := rm.Rule.Meta["severity"]; ok {
			severity = antivirus.ThreatSeverity(v)
		}
		if v, ok := rm.Rule.Meta["confidence"]; ok {
			if c, err := strconv.ParseFloat(v, 64); err == nil {
				confidence = c
			}
		}
		if v, ok := rm.Rule.Meta["description"]; ok {
			desc = v
		}

		meta := map[string]string{
			"matchedStrings": strings.Join(rm.MatchedStrings, ", "),
			"ruleTags":       strings.Join(rm.Rule.Tags, ", "),
		}
		for k, v := range rm.Rule.Meta {
			if k != "category" && k != "severity" && k != "confidence" && k != "description" {
				meta["meta_"+k] = v
			}
		}

		threats = append(threats, antivirus.ThreatInfo{
			Name:        rm.Rule.Name,
			Category:    category,
			Severity:    severity,
			Layer:       antivirus.LayerYARA,
			Description: desc,
			Confidence:  confidence,
			Signature:   fmt.Sprintf("yara:%s", rm.Rule.Name),
			Metadata:    meta,
		})
	}

	logging.Z().Info(fmt.Sprintf("🛡️  yara: %d rule(s) matched in %q", len(threats), target.Filename))
	return threats, nil
}

// Reload atomically replaces the rule set.
func (l *Layer) Reload(rs *RuleSet) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.ruleset = rs
}

// Stats returns runtime statistics.
type Stats struct {
	TotalScans   int64 `json:"totalScans"`
	TotalMatches int64 `json:"totalMatches"`
	RuleCount    int   `json:"ruleCount"`
}

// Stats returns a snapshot of YARA layer statistics.
func (l *Layer) Stats() Stats {
	l.mu.RLock()
	rs := l.ruleset
	l.mu.RUnlock()

	rc := 0
	if rs != nil {
		rc = rs.RuleCount()
	}
	return Stats{
		TotalScans:   l.scans.Load(),
		TotalMatches: l.matches.Load(),
		RuleCount:    rc,
	}
}

// inferYARACategory maps YARA metadata category strings to ThreatCategory.
func inferYARACategory(cat string) antivirus.ThreatCategory {
	lower := strings.ToLower(cat)
	mapping := map[string]antivirus.ThreatCategory{
		"trojan":      antivirus.CategoryTrojan,
		"ransomware":  antivirus.CategoryRansomware,
		"worm":        antivirus.CategoryWorm,
		"exploit":     antivirus.CategoryExploit,
		"backdoor":    antivirus.CategoryBackdoor,
		"cryptominer": antivirus.CategoryCryptominer,
		"miner":       antivirus.CategoryCryptominer,
		"webshell":    antivirus.CategoryWebshell,
		"dropper":     antivirus.CategoryDropper,
		"rootkit":     antivirus.CategoryRootkit,
		"adware":      antivirus.CategoryAdware,
		"spyware":     antivirus.CategorySpyware,
		"packer":      antivirus.CategoryPacker,
	}
	if c, ok := mapping[lower]; ok {
		return c
	}
	return antivirus.CategoryGeneric
}
