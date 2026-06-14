// Package snapshot provides Nomad-style HTTP endpoints for triggering,
// downloading, and restoring Raft state snapshots.
//
// GET  /api/v1/system/snapshot          — download a tar.gz backup
// POST /api/v1/system/snapshot/restore  — restore from an uploaded tar.gz
//
// These endpoints are only available when STORAGE_BACKEND=raft.
package snapshot

import (
	"archive/tar"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	platformstore "axiomnizam.bitbd.net/axiomnizam/internal/platform/store"
	"github.com/gin-gonic/gin"
)

// Handler provides HTTP endpoints for Raft snapshot operations.
type Handler struct {
	backend *platformstore.BackendManager
}

// NewHandler creates a snapshot handler backed by the given BackendManager.
func NewHandler(bm *platformstore.BackendManager) *Handler {
	return &Handler{backend: bm}
}

// SnapshotMeta describes a snapshot archive.
type SnapshotMeta struct {
	Timestamp  time.Time `json:"timestamp"`
	NodeID     string    `json:"node_id"`
	EntryCount int       `json:"entry_count"`
}

// SnapshotResponse is the JSON response for snapshot operations.
type SnapshotResponse struct {
	Status  string         `json:"status"`
	Message string         `json:"message,omitempty"`
	Meta    *SnapshotMeta  `json:"meta,omitempty"`
}

// Download creates an on-demand Raft snapshot and streams it as a tar.gz.
//
// GET /api/v1/system/snapshot
func (h *Handler) Download(c *gin.Context) {
	if !h.backend.IsRaft() {
		c.JSON(http.StatusNotImplemented, SnapshotResponse{
			Status:  "error",
			Message: "snapshot backup is only available in raft mode",
		})
		return
	}

	// Force a fresh snapshot.
	if err := h.backend.TriggerSnapshot(); err != nil {
		logging.Z().Error(fmt.Sprintf("snapshot: trigger failed: %v", err))
		c.JSON(http.StatusInternalServerError, SnapshotResponse{
			Status:  "error",
			Message: fmt.Sprintf("snapshot trigger failed: %v", err),
		})
		return
	}

	snapDir := h.backend.SnapshotDir()
	logDir := h.backend.LogDir()
	stableDir := h.backend.StableDir()
	dataDir := h.backend.DataDir()

	// Collect files to include in the archive.
	type archiveEntry struct {
		Path     string // on-disk path
		RelPath  string // path inside the archive
	}
	var entries []archiveEntry

	// Snapshot files.
	snapMeta := SnapshotMeta{Timestamp: time.Now().UTC()}
	if err := filepath.Walk(snapDir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(dataDir, path)
		entries = append(entries, archiveEntry{Path: path, RelPath: filepath.ToSlash(rel)})
		snapMeta.EntryCount++
		return nil
	}); err != nil {
		logging.Z().Error(fmt.Sprintf("snapshot: walk snapshots failed: %v", err))
	}

	// BoltDB log store.
	logBolt := filepath.Join(logDir, "raft-log.bolt")
	if _, err := os.Stat(logBolt); err == nil {
		rel, _ := filepath.Rel(dataDir, logBolt)
		entries = append(entries, archiveEntry{Path: logBolt, RelPath: filepath.ToSlash(rel)})
		snapMeta.EntryCount++
	}

	// BoltDB stable store.
	stableBolt := filepath.Join(stableDir, "raft-stable.bolt")
	if _, err := os.Stat(stableBolt); err == nil {
		rel, _ := filepath.Rel(dataDir, stableBolt)
		entries = append(entries, archiveEntry{Path: stableBolt, RelPath: filepath.ToSlash(rel)})
		snapMeta.EntryCount++
	}

	// Stream tar.gz to client.
	filename := fmt.Sprintf("axiomnizam-snapshot-%s.tar.gz", time.Now().UTC().Format("20060102-150405"))
	c.Header("Content-Type", "application/gzip")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	gz := gzip.NewWriter(c.Writer)
	defer gz.Close()
	tw := tar.NewWriter(gz)
	defer tw.Close()

	// Write snapshot-meta.json.
	metaJSON, _ := json.MarshalIndent(snapMeta, "", "  ")
	metaHeader := &tar.Header{
		Name:    "snapshot-meta.json",
		Mode:    0644,
		Size:    int64(len(metaJSON)),
		ModTime: snapMeta.Timestamp,
	}
	if err := tw.WriteHeader(metaHeader); err == nil {
		_, _ = tw.Write(metaJSON)
	}

	// Write all data files.
	for _, e := range entries {
		info, err := os.Stat(e.Path)
		if err != nil {
			continue
		}
		hdr := &tar.Header{
			Name:    e.RelPath,
			Mode:    0644,
			Size:    info.Size(),
			ModTime: info.ModTime(),
		}
		if err := tw.WriteHeader(hdr); err != nil {
			logging.Z().Warn(fmt.Sprintf("snapshot: tar header %s: %v", e.RelPath, err))
			continue
		}
		f, err := os.Open(e.Path)
		if err != nil {
			continue
		}
		_, _ = io.Copy(tw, f)
		f.Close()
	}

	logging.Z().Info(fmt.Sprintf("snapshot: downloaded (%d files)", snapMeta.EntryCount))
}

// Restore accepts an uploaded tar.gz and replaces the Raft state.
//
// POST /api/v1/system/snapshot/restore
func (h *Handler) Restore(c *gin.Context) {
	if !h.backend.IsRaft() {
		c.JSON(http.StatusNotImplemented, SnapshotResponse{
			Status:  "error",
			Message: "snapshot restore is only available in raft mode",
		})
		return
	}

	// Only the leader can restore.
	if !h.backend.GetRaftIsLeader() {
		c.JSON(http.StatusConflict, SnapshotResponse{
			Status:  "error",
			Message: "snapshot restore must be performed on the Raft leader",
		})
		return
	}

	// Safety: refuse if there are peers (multi-node cluster).
	peers, err := h.backend.GetRaftPeers()
	if err == nil && len(peers) > 1 {
		c.JSON(http.StatusConflict, SnapshotResponse{
			Status:  "error",
			Message: "snapshot restore is disabled on multi-node clusters to prevent data loss",
		})
		return
	}

	file, _, err := c.Request.FormFile("snapshot")
	if err != nil {
		c.JSON(http.StatusBadRequest, SnapshotResponse{
			Status:  "error",
			Message: "missing 'snapshot' file in multipart upload",
		})
		return
	}
	defer file.Close()

	snapDir := h.backend.SnapshotDir()
	logDir := h.backend.LogDir()
	stableDir := h.backend.StableDir()

	// Extract the archive to a temp directory first.
	tmpDir, err := os.MkdirTemp("", "axiomnizam-restore-*")
	if err != nil {
		c.JSON(http.StatusInternalServerError, SnapshotResponse{
			Status:  "error",
			Message: fmt.Sprintf("temp dir: %v", err),
		})
		return
	}
	defer os.RemoveAll(tmpDir)

	var meta SnapshotMeta
	filesExtracted := 0

	gz, err := gzip.NewReader(file)
	if err != nil {
		c.JSON(http.StatusBadRequest, SnapshotResponse{
			Status:  "error",
			Message: fmt.Sprintf("invalid gzip: %v", err),
		})
		return
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			c.JSON(http.StatusBadRequest, SnapshotResponse{
				Status:  "error",
				Message: fmt.Sprintf("tar read error: %v", err),
			})
			return
		}

		// Sanitize path to prevent directory traversal.
		cleanName := filepath.Clean(hdr.Name)
		if strings.Contains(cleanName, "..") {
			c.JSON(http.StatusBadRequest, SnapshotResponse{
				Status:  "error",
				Message: fmt.Sprintf("invalid path in archive: %s", hdr.Name),
			})
			return
		}

		if cleanName == "snapshot-meta.json" {
			data := make([]byte, hdr.Size)
			if _, err := io.ReadFull(tr, data); err == nil {
				_ = json.Unmarshal(data, &meta)
			}
			continue
		}

		target := filepath.Join(tmpDir, cleanName)
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			continue
		}

		outFile, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
		if err != nil {
			continue
		}
		if _, err := io.Copy(outFile, tr); err != nil {
			outFile.Close()
			continue
		}
		outFile.Close()
		filesExtracted++
	}

	if filesExtracted == 0 {
		c.JSON(http.StatusBadRequest, SnapshotResponse{
			Status:  "error",
			Message: "archive contained no data files",
		})
		return
	}

	// Shut down Raft before replacing files.
	if err := h.backend.RaftServer.Shutdown(); err != nil {
		logging.Z().Error(fmt.Sprintf("snapshot: raft shutdown failed: %v", err))
		// Continue anyway — the state is being replaced.
	}

	var restoreErrors []string

	// Replace snapshot directory.
	if err := replaceDir(snapDir, filepath.Join(tmpDir, "snapshots")); err != nil {
		logging.Z().Error(fmt.Sprintf("snapshot: replace snapshots failed: %v", err))
		restoreErrors = append(restoreErrors, fmt.Sprintf("snapshots: %v", err))
	}

	// Replace log bolt.
	logSrc := filepath.Join(tmpDir, "logs", "raft-log.bolt")
	if _, err := os.Stat(logSrc); err == nil {
		_ = os.MkdirAll(logDir, 0755)
		if err := copyFile(logSrc, filepath.Join(logDir, "raft-log.bolt")); err != nil {
			logging.Z().Error(fmt.Sprintf("snapshot: copy log bolt failed: %v", err))
			restoreErrors = append(restoreErrors, fmt.Sprintf("log bolt: %v", err))
		}
	}

	// Replace stable bolt.
	stableSrc := filepath.Join(tmpDir, "stable", "raft-stable.bolt")
	if _, err := os.Stat(stableSrc); err == nil {
		_ = os.MkdirAll(stableDir, 0755)
		if err := copyFile(stableSrc, filepath.Join(stableDir, "raft-stable.bolt")); err != nil {
			logging.Z().Error(fmt.Sprintf("snapshot: copy stable bolt failed: %v", err))
			restoreErrors = append(restoreErrors, fmt.Sprintf("stable bolt: %v", err))
		}
	}

	if len(restoreErrors) > 0 {
		logging.Z().Error(fmt.Sprintf("snapshot: restore completed with errors: %v", restoreErrors))
		c.JSON(http.StatusInternalServerError, SnapshotResponse{
			Status:  "partial_failure",
			Message: fmt.Sprintf("restore completed with errors: %s — restart required", strings.Join(restoreErrors, "; ")),
			Meta:    &meta,
		})
		return
	}

	logging.Z().Info(fmt.Sprintf("snapshot: restored (%d files, meta=%+v)", filesExtracted, meta))

	c.JSON(http.StatusOK, SnapshotResponse{
		Status:  "success",
		Message: "snapshot restored — restart required to re-join the cluster",
		Meta:    &meta,
	})
}

// replaceDir removes dst and moves src to dst.
func replaceDir(dst, src string) error {
	_ = os.RemoveAll(dst)
	return os.Rename(src, dst)
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
