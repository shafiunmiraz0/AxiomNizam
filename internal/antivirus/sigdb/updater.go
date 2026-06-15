package sigdb

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ─────────────────────────────────────────────────────────────────────────────
// Updater
//
// Periodically polls a remote HTTP endpoint for new signature database
// versions. When a new version is detected, it downloads the update
// files and hot-reloads the Database.
//
// Update Protocol:
//
//  1. GET {UpdateURL}/version.json → returns RemoteVersion
//  2. Compare RemoteVersion.Version with local version
//  3. If different, download each file in RemoteVersion.Files
//  4. Write files to sigDir, update version.json
//  5. Trigger Database.Reload()
//
// Failure handling:
//   - All errors are logged but never fatal
//   - The engine continues with whatever signatures are loaded
//   - Retries happen at the next interval
// ─────────────────────────────────────────────────────────────────────────────

// RemoteVersion describes the available update from the remote server.
type RemoteVersion struct {
	Version   string       `json:"version"`
	Timestamp time.Time    `json:"timestamp"`
	Files     []RemoteFile `json:"files"`
}

// RemoteFile describes a single downloadable signature file.
type RemoteFile struct {
	Name     string `json:"name"`     // e.g. "hashes/main.hdb"
	URL      string `json:"url"`      // download URL
	SHA256   string `json:"sha256"`   // checksum for validation
	Size     int64  `json:"size"`     // expected file size
	Category string `json:"category"` // "hash", "pattern", "yara"
}

// Updater handles automatic signature database updates.
type Updater struct {
	db       *Database
	url      string
	interval time.Duration
	client   *http.Client

	mu        sync.Mutex
	running   bool
	cancel    context.CancelFunc
	lastCheck time.Time
	lastError string
	updates   int
}

// NewUpdater creates a new signature updater.
func NewUpdater(db *Database, updateURL string, interval time.Duration) *Updater {
	return &Updater{
		db:       db,
		url:      updateURL,
		interval: interval,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// Start begins the background update loop. Safe to call multiple times
// (subsequent calls are no-ops). Uses the provided context for cancellation.
func (u *Updater) Start(ctx context.Context) {
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.running || u.url == "" {
		return
	}

	childCtx, cancel := context.WithCancel(ctx)
	u.cancel = cancel
	u.running = true

	go u.loop(childCtx)
	logging.Z().Info(fmt.Sprintf("🛡️  updater: started (interval=%s, url=%s)", u.interval, u.url))
}

// Stop halts the background update loop.
func (u *Updater) Stop() {
	u.mu.Lock()
	defer u.mu.Unlock()

	if !u.running {
		return
	}

	if u.cancel != nil {
		u.cancel()
	}
	u.running = false
	logging.Z().Info(fmt.Sprintf("🛡️  updater: stopped"))
}

// loop is the main update polling loop.
func (u *Updater) loop(ctx context.Context) {
	// Perform an initial check after a short delay.
	select {
	case <-time.After(10 * time.Second):
	case <-ctx.Done():
		return
	}

	u.checkForUpdate(ctx)

	ticker := time.NewTicker(u.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			u.checkForUpdate(ctx)
		}
	}
}

// checkForUpdate performs a single update check cycle.
func (u *Updater) checkForUpdate(ctx context.Context) {
	u.mu.Lock()
	u.lastCheck = time.Now()
	u.mu.Unlock()

	// 1. Fetch remote version.
	remote, err := u.fetchVersion(ctx)
	if err != nil {
		u.setError(fmt.Sprintf("fetch version: %v", err))
		return
	}

	// 2. Compare with local.
	localVer := u.db.Stats().Version.Version
	if remote.Version == localVer {
		logging.Z().Info(fmt.Sprintf("🛡️  updater: signatures up to date (v%s)", localVer))
		u.clearError()
		return
	}

	logging.Z().Info(fmt.Sprintf("🛡️  updater: new version available: %s → %s", localVer, remote.Version))

	// 3. Download files.
	downloaded := 0
	for _, f := range remote.Files {
		if err := u.downloadFile(ctx, f); err != nil {
			u.setError(fmt.Sprintf("download %s: %v", f.Name, err))
			continue
		}
		downloaded++
	}

	if downloaded == 0 && len(remote.Files) > 0 {
		u.setError("all downloads failed")
		return
	}

	// 4. Write new version.json.
	u.db.WriteVersion(Version{
		Version:   remote.Version,
		UpdatedAt: time.Now(),
		Source:    "remote",
	})

	// 5. Reload database.
	ver, err := u.db.Reload()
	if err != nil {
		u.setError(fmt.Sprintf("reload: %v", err))
		return
	}

	u.mu.Lock()
	u.updates++
	u.mu.Unlock()
	u.clearError()

	logging.Z().Info(fmt.Sprintf("🛡️  updater: updated to v%s (hashes=%d patterns=%d yara=%d)",
		ver.Version, ver.HashCount, ver.PatternCount, ver.YARACount))
}

// fetchVersion retrieves the remote version manifest.
func (u *Updater) fetchVersion(ctx context.Context) (*RemoteVersion, error) {
	url := u.url + "/version.json"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "AxiomNizam-AV-Updater/1.0")

	resp, err := u.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, url)
	}

	var remote RemoteVersion
	if err := json.NewDecoder(resp.Body).Decode(&remote); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}

	return &remote, nil
}

// downloadFile downloads a single signature file to the sig directory.
func (u *Updater) downloadFile(ctx context.Context, f RemoteFile) error {
	destPath := filepath.Join(u.db.SigDir(), f.Name)

	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return fmt.Errorf("mkdir: %w", err)
	}

	// Download to a temp file first, then rename (atomic).
	tmpPath := destPath + ".tmp"
	defer os.Remove(tmpPath)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, f.URL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "AxiomNizam-AV-Updater/1.0")

	resp, err := u.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	defer out.Close()

	// Limit download size to 50MB.
	limited := io.LimitReader(resp.Body, 50*1024*1024)
	written, err := io.Copy(out, limited)
	if err != nil {
		return fmt.Errorf("write: %w", err)
	}

	out.Close()

	// Validate size if specified.
	if f.Size > 0 && written != f.Size {
		return fmt.Errorf("size mismatch: expected %d, got %d", f.Size, written)
	}

	// Rename temp file to final location.
	if err := os.Rename(tmpPath, destPath); err != nil {
		return fmt.Errorf("rename: %w", err)
	}

	logging.Z().Info(fmt.Sprintf("🛡️  updater: downloaded %s (%d bytes)", f.Name, written))
	return nil
}

// setError records the last error.
func (u *Updater) setError(msg string) {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.lastError = msg
	logging.Z().Info(fmt.Sprintf("⚠️  updater: %s", msg))
}

// clearError clears the last error.
func (u *Updater) clearError() {
	u.mu.Lock()
	defer u.mu.Unlock()
	u.lastError = ""
}

// ─────────────────────────────────────────────────────────────────────────────
// Stats
// ─────────────────────────────────────────────────────────────────────────────

// UpdaterStats holds runtime statistics for the updater.
type UpdaterStats struct {
	Running    bool      `json:"running"`
	URL        string    `json:"url"`
	Interval   string    `json:"interval"`
	LastCheck  time.Time `json:"lastCheck"`
	LastError  string    `json:"lastError,omitempty"`
	Updates    int       `json:"updates"`
}

// Stats returns a snapshot of updater statistics.
func (u *Updater) Stats() UpdaterStats {
	u.mu.Lock()
	defer u.mu.Unlock()
	return UpdaterStats{
		Running:   u.running,
		URL:       u.url,
		Interval:  u.interval.String(),
		LastCheck: u.lastCheck,
		LastError: u.lastError,
		Updates:   u.updates,
	}
}

// ForceCheck triggers an immediate update check. Blocks until complete.
func (u *Updater) ForceCheck(ctx context.Context) {
	u.checkForUpdate(ctx)
}
