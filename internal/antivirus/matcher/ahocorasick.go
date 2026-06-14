// Package matcher implements Layer 2 of the AxiomNizam antivirus engine:
// byte-pattern matching using the Aho-Corasick multi-pattern search algorithm.
//
// # Algorithm overview
//
// Aho-Corasick builds a finite-state automaton from a dictionary of patterns
// and searches for all of them simultaneously in a single pass through the
// input. This is ClamAV's core detection mechanism.
//
//   - Build phase: O(Σ|patterns|) — insert all patterns into a trie, then
//     compute failure links via BFS. Done once at startup.
//   - Search phase: O(|input| + matches) — one byte at a time, following
//     transitions. Each byte costs exactly one state transition.
//
// # Design decisions
//
//   - Pure Go, zero dependencies — no cgo, no assembly, fully portable.
//   - The automaton is immutable after Build(). Concurrent Scan() calls are
//     safe without any locking.
//   - Patterns are stored as []byte to support arbitrary binary signatures
//     (not just UTF-8 text).
//   - Each match carries the original SignatureInfo metadata so that the
//     scan layer can produce fully-populated ThreatInfo results.
package matcher

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"fmt"
	"sync"
	"sync/atomic"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Signature Info
// ─────────────────────────────────────────────────────────────────────────────

// SignatureInfo describes a single byte-pattern signature. This is the
// metadata attached to each pattern in the automaton.
type SignatureInfo struct {
	// ID is a unique identifier for the signature (e.g. "NDB:Trojan.Emotet.1").
	ID string

	// Name is the threat name (e.g. "Trojan.Win32.Emotet.A").
	Name string

	// Pattern is the raw byte sequence to search for.
	Pattern []byte

	// Category classifies the threat family.
	Category antivirus.ThreatCategory

	// Severity indicates how dangerous the pattern is.
	Severity antivirus.ThreatSeverity

	// Confidence is the detection confidence (0.0–1.0). Byte-exact
	// signatures typically have 0.85–0.95 confidence (lower than hash
	// matches because a byte pattern could theoretically appear in
	// benign files).
	Confidence float64

	// Description is a human-readable explanation.
	Description string

	// Source indicates where the signature came from.
	Source string
}

// ─────────────────────────────────────────────────────────────────────────────
// Match
// ─────────────────────────────────────────────────────────────────────────────

// Match represents a single pattern match found in the input.
type Match struct {
	// Signature is the matched signature metadata.
	Signature *SignatureInfo

	// Offset is the byte offset in the input where the match ENDS.
	// The match starts at Offset - len(Signature.Pattern) + 1.
	Offset int64
}

// StartOffset returns the byte offset where the matched pattern begins.
func (m Match) StartOffset() int64 {
	return m.Offset - int64(len(m.Signature.Pattern)) + 1
}

// ─────────────────────────────────────────────────────────────────────────────
// Aho-Corasick Automaton
// ─────────────────────────────────────────────────────────────────────────────

// trieNode is a single node in the Aho-Corasick trie/automaton.
type trieNode struct {
	// children maps byte value → child node index. Using a map here
	// trades some speed for significantly lower memory when the
	// alphabet utilisation is sparse (typical for binary signatures).
	children map[byte]int

	// fail is the failure link — the longest proper suffix of the path
	// to this node that is also a prefix of some pattern.
	fail int

	// output holds indices into the Automaton.signatures slice for
	// patterns that end at this node (including via output links).
	output []int

	// depth is the length of the string from root to this node.
	depth int
}

// Automaton is an immutable Aho-Corasick multi-pattern matcher. After
// construction via NewBuilder().Build(), it is safe for concurrent use.
type Automaton struct {
	nodes      []trieNode
	signatures []SignatureInfo
	compiled   bool
}

// ─────────────────────────────────────────────────────────────────────────────
// Builder
// ─────────────────────────────────────────────────────────────────────────────

// Builder constructs an Automaton by inserting patterns and then compiling
// failure links.
type Builder struct {
	nodes      []trieNode
	signatures []SignatureInfo
}

// NewBuilder creates a new Aho-Corasick automaton builder.
func NewBuilder() *Builder {
	root := trieNode{
		children: make(map[byte]int),
		fail:     0,
		depth:    0,
	}
	return &Builder{
		nodes:      []trieNode{root},
		signatures: make([]SignatureInfo, 0, 256),
	}
}

// AddPattern inserts a signature's byte pattern into the trie. Duplicate
// patterns (same bytes) are allowed — each gets its own output entry.
func (b *Builder) AddPattern(sig SignatureInfo) {
	if len(sig.Pattern) == 0 {
		return
	}

	current := 0 // start at root

	for _, ch := range sig.Pattern {
		next, exists := b.nodes[current].children[ch]
		if !exists {
			// Create new node.
			next = len(b.nodes)
			b.nodes = append(b.nodes, trieNode{
				children: make(map[byte]int),
				fail:     0,
				depth:    b.nodes[current].depth + 1,
			})
			b.nodes[current].children[ch] = next
		}
		current = next
	}

	// Record the signature index in the output of the final node.
	sigIdx := len(b.signatures)
	b.signatures = append(b.signatures, sig)
	b.nodes[current].output = append(b.nodes[current].output, sigIdx)
}

// PatternCount returns the number of patterns added so far.
func (b *Builder) PatternCount() int {
	return len(b.signatures)
}

// Build compiles the trie into an Aho-Corasick automaton by computing
// failure links and output chains via BFS. After this call, the returned
// Automaton is immutable and safe for concurrent search.
func (b *Builder) Build() *Automaton {
	if len(b.signatures) == 0 {
		return &Automaton{
			nodes:      b.nodes,
			signatures: b.signatures,
			compiled:   true,
		}
	}

	// BFS to compute failure links.
	// Level 1 nodes (direct children of root) have fail → root.
	queue := make([]int, 0, len(b.nodes))

	for _, childIdx := range b.nodes[0].children {
		b.nodes[childIdx].fail = 0 // fail to root
		queue = append(queue, childIdx)
	}

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		for ch, childIdx := range b.nodes[current].children {
			queue = append(queue, childIdx)

			// Walk up failure links to find the longest proper suffix
			// that has a transition on byte `ch`.
			f := b.nodes[current].fail
			for f != 0 && b.nodes[f].children[ch] == 0 {
				_, exists := b.nodes[f].children[ch]
				if exists {
					break
				}
				f = b.nodes[f].fail
			}

			if target, exists := b.nodes[f].children[ch]; exists && target != childIdx {
				b.nodes[childIdx].fail = target
			} else {
				b.nodes[childIdx].fail = 0
			}

			// Merge output from the failure target (output links).
			failNode := b.nodes[childIdx].fail
			if len(b.nodes[failNode].output) > 0 {
				b.nodes[childIdx].output = append(
					b.nodes[childIdx].output,
					b.nodes[failNode].output...,
				)
			}
		}
	}

	return &Automaton{
		nodes:      b.nodes,
		signatures: b.signatures,
		compiled:   true,
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Search
// ─────────────────────────────────────────────────────────────────────────────

// Search scans the entire input and returns all matches. The input is
// processed in a single pass — O(|input| + matches).
func (a *Automaton) Search(data []byte) []Match {
	if !a.compiled || len(a.signatures) == 0 {
		return nil
	}

	var matches []Match
	state := 0 // start at root

	for i, b := range data {
		state = a.nextState(state, b)

		// Collect outputs at this state.
		if len(a.nodes[state].output) > 0 {
			for _, sigIdx := range a.nodes[state].output {
				matches = append(matches, Match{
					Signature: &a.signatures[sigIdx],
					Offset:    int64(i),
				})
			}
		}
	}

	return matches
}

// SearchFirst returns the first match found, or nil if no patterns match.
// More efficient than Search() when you only need to know IF a match exists.
func (a *Automaton) SearchFirst(data []byte) *Match {
	if !a.compiled || len(a.signatures) == 0 {
		return nil
	}

	state := 0

	for i, b := range data {
		state = a.nextState(state, b)

		if len(a.nodes[state].output) > 0 {
			m := Match{
				Signature: &a.signatures[a.nodes[state].output[0]],
				Offset:    int64(i),
			}
			return &m
		}
	}

	return nil
}

// nextState computes the transition from the current state on byte b,
// following failure links as needed.
func (a *Automaton) nextState(current int, b byte) int {
	for current != 0 {
		if next, exists := a.nodes[current].children[b]; exists {
			return next
		}
		current = a.nodes[current].fail
	}

	// At root — either take the child transition or stay at root.
	if next, exists := a.nodes[0].children[b]; exists {
		return next
	}
	return 0
}

// Stats returns statistics about the compiled automaton.
func (a *Automaton) Stats() AutomatonStats {
	return AutomatonStats{
		NodeCount:      len(a.nodes),
		PatternCount:   len(a.signatures),
		Compiled:       a.compiled,
		EstMemoryBytes: a.estimateMemory(),
	}
}

// AutomatonStats holds information about the automaton's size.
type AutomatonStats struct {
	NodeCount      int   `json:"nodeCount"`
	PatternCount   int   `json:"patternCount"`
	Compiled       bool  `json:"compiled"`
	EstMemoryBytes int64 `json:"estMemoryBytes"`
}

// estimateMemory provides a rough memory estimate for the automaton.
func (a *Automaton) estimateMemory() int64 {
	var mem int64
	for _, n := range a.nodes {
		// map overhead: ~8 bytes per entry + bucket overhead
		mem += int64(len(n.children)) * 16
		mem += int64(len(n.output)) * 8
		mem += 48 // struct overhead
	}
	// Signatures.
	for _, s := range a.signatures {
		mem += int64(len(s.Pattern))
		mem += int64(len(s.Name))
		mem += int64(len(s.ID))
		mem += 128 // struct overhead
	}
	return mem
}

// ─────────────────────────────────────────────────────────────────────────────
// Scan Layer
// ─────────────────────────────────────────────────────────────────────────────

// Layer implements antivirus.ScanLayer for byte-pattern matching.
type Layer struct {
	mu sync.RWMutex

	// automaton is the compiled Aho-Corasick automaton.
	automaton *Automaton

	// stats
	scans   atomic.Int64
	matches atomic.Int64
}

// NewLayer creates a pattern matcher scan layer from a pre-built automaton.
func NewLayer(automaton *Automaton) *Layer {
	if automaton == nil {
		automaton = NewBuilder().Build()
	}
	return &Layer{automaton: automaton}
}

// Name returns the layer identifier.
func (l *Layer) Name() string { return "pattern" }

// Scan searches the file content for malware byte patterns using the
// Aho-Corasick automaton. Safe for concurrent use.
func (l *Layer) Scan(target *antivirus.ScanTarget) ([]antivirus.ThreatInfo, error) {
	if len(target.Content) == 0 {
		return nil, nil
	}

	l.scans.Add(1)

	l.mu.RLock()
	ac := l.automaton
	l.mu.RUnlock()

	if ac == nil || !ac.compiled || len(ac.signatures) == 0 {
		return nil, nil
	}

	rawMatches := ac.Search(target.Content)
	if len(rawMatches) == 0 {
		return nil, nil
	}

	l.matches.Add(int64(len(rawMatches)))

	// Deduplicate: if the same signature matches multiple times, keep
	// only the first occurrence.
	seen := make(map[string]struct{}, len(rawMatches))
	threats := make([]antivirus.ThreatInfo, 0, len(rawMatches))

	for _, m := range rawMatches {
		if _, dup := seen[m.Signature.ID]; dup {
			continue
		}
		seen[m.Signature.ID] = struct{}{}

		confidence := m.Signature.Confidence
		if confidence == 0 {
			confidence = 0.85 // default for byte-pattern matches
		}

		threats = append(threats, antivirus.ThreatInfo{
			Name:        m.Signature.Name,
			Category:    m.Signature.Category,
			Severity:    m.Signature.Severity,
			Layer:       antivirus.LayerPattern,
			Description: m.Signature.Description,
			Signature:   m.Signature.ID,
			Confidence:  confidence,
			Offset:      m.StartOffset(),
			Metadata: map[string]string{
				"source":        m.Signature.Source,
				"patternLength": fmt.Sprintf("%d", len(m.Signature.Pattern)),
				"endOffset":     fmt.Sprintf("%d", m.Offset),
			},
		})
	}

	if len(threats) > 0 {
		logging.Z().Info(fmt.Sprintf("🛡️  pattern: %d signature(s) matched in %q", len(threats), target.Filename))
	}

	return threats, nil
}

// Reload atomically replaces the automaton with a new one. This is used
// when the signature database is updated.
func (l *Layer) Reload(automaton *Automaton) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.automaton = automaton
}

// LayerStats holds runtime statistics for the pattern matcher.
type LayerStats struct {
	TotalScans   int64          `json:"totalScans"`
	TotalMatches int64          `json:"totalMatches"`
	Automaton    AutomatonStats `json:"automaton"`
}

// Stats returns a snapshot of pattern matcher statistics.
func (l *Layer) Stats() LayerStats {
	l.mu.RLock()
	ac := l.automaton
	l.mu.RUnlock()

	var acStats AutomatonStats
	if ac != nil {
		acStats = ac.Stats()
	}

	return LayerStats{
		TotalScans:   l.scans.Load(),
		TotalMatches: l.matches.Load(),
		Automaton:    acStats,
	}
}
