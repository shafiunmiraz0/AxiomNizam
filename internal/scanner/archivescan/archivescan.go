// Package archivescan provides the ArchiveScanner for the SafeGate pipeline.
// It detects zip bombs (via compression ratio and decompressed size analysis),
// path traversal attacks, executable files hidden inside archives, symlink
// bombs, and provides TAR/GZIP/BZ2 header-level analysis.
package archivescan

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"strings"

	"axiomnizam.bitbd.net/axiomnizam/internal/scanner"
)

// Scanner detects zip bombs, path traversal, symlink bombs, and malicious archives.
type Scanner struct {
	maxDepth        int
	maxDecompressed int64
	maxRatio        float64
	maxFiles        int
}

// NewScanner creates an ArchiveScanner with the given depth and size limits.
func NewScanner(maxDepth int, maxDecompressed int64) *Scanner {
	return &Scanner{maxDepth: maxDepth, maxDecompressed: maxDecompressed, maxRatio: 100, maxFiles: 10000}
}

// NewScannerWithLimits creates an ArchiveScanner with full configuration.
func NewScannerWithLimits(maxDepth int, maxDecompressed int64, maxRatio float64, maxFiles int) *Scanner {
	if maxRatio <= 0 {
		maxRatio = 100
	}
	if maxFiles <= 0 {
		maxFiles = 10000
	}
	return &Scanner{maxDepth: maxDepth, maxDecompressed: maxDecompressed, maxRatio: maxRatio, maxFiles: maxFiles}
}

func (s *Scanner) Name() string { return "archive_bomb_scanner" }

func (s *Scanner) Scan(_ context.Context, file *scanner.FileInfo) ([]scanner.Finding, error) {
	if !isArchive(file) {
		return nil, nil
	}

	var findings []scanner.Finding

	if isZipArchive(file) {
		f, err := s.analyzeZip(file.Content, 0)
		if err != nil {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityMedium,
				Description: "Failed to analyze zip archive", Details: err.Error(),
			})
		}
		findings = append(findings, f...)
	}

	if isTarArchive(file) {
		f := s.analyzeTar(file.Content)
		findings = append(findings, f...)
	}

	if isGzipArchive(file) {
		f := s.analyzeGzip(file.Content)
		findings = append(findings, f...)
	}

	if isBzip2Archive(file) {
		f := s.analyzeBzip2(file.Content)
		findings = append(findings, f...)
	}

	return findings, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// ZIP analysis
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) analyzeZip(data []byte, depth int) ([]scanner.Finding, error) {
	var findings []scanner.Finding

	if depth > s.maxDepth {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityHigh,
			Description: "Archive exceeds maximum nesting depth",
			Details:     fmt.Sprintf("Depth %d exceeds limit %d — possible zip bomb", depth, s.maxDepth),
		})
		return findings, nil
	}

	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, fmt.Errorf("failed to read zip: %w", err)
	}

	var totalUncompressed uint64
	var fileCount int

	for _, f := range reader.File {
		fileCount++
		totalUncompressed += f.UncompressedSize64

		if f.CompressedSize64 > 0 {
			ratio := float64(f.UncompressedSize64) / float64(f.CompressedSize64)
			if ratio > s.maxRatio {
				findings = append(findings, scanner.Finding{
					Scanner: s.Name(), Severity: scanner.SeverityCritical,
					Description: "Possible zip bomb detected",
					Details:     fmt.Sprintf("File %q has %.0f:1 compression ratio", f.Name, ratio),
				})
			}
		}

		if int64(totalUncompressed) > s.maxDecompressed {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityCritical,
				Description: "Archive decompressed size exceeds limit",
				Details:     fmt.Sprintf("Total %d bytes exceeds %d byte limit", totalUncompressed, s.maxDecompressed),
			})
			return findings, nil
		}

		// Path traversal
		if containsPathTraversal(f.Name) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityCritical,
				Description: "Archive contains path traversal",
				Details:     fmt.Sprintf("Entry %q contains directory traversal sequence", f.Name),
			})
		}

		// Executable detection
		if isExecutableExtension(strings.ToLower(f.Name)) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityHigh,
				Description: "Archive contains executable file",
				Details:     fmt.Sprintf("Found executable: %q", f.Name),
			})
		}

		// Symlink detection in zip (external attributes can indicate symlinks)
		if f.FileInfo().Mode()&0120000 == 0120000 {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityHigh,
				Description: "Archive contains symbolic link",
				Details:     fmt.Sprintf("Entry %q is a symlink — can escape archive boundary", f.Name),
			})
		}
	}

	if fileCount > s.maxFiles {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityHigh,
			Description: "Archive contains excessive files",
			Details:     fmt.Sprintf("%d files — possible resource exhaustion", fileCount),
		})
	}

	return findings, nil
}

// ─────────────────────────────────────────────────────────────────────────────
// TAR analysis
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) analyzeTar(data []byte) []scanner.Finding {
	var findings []scanner.Finding

	tr := tar.NewReader(bytes.NewReader(data))
	var totalSize int64
	var fileCount int
	var symlinkCount int

	for {
		hdr, err := tr.Next()
		if err != nil {
			break // EOF or corrupt — stop
		}
		fileCount++
		totalSize += hdr.Size

		// Path traversal
		if containsPathTraversal(hdr.Name) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityCritical,
				Description: "TAR archive contains path traversal",
				Details:     fmt.Sprintf("Entry %q contains directory traversal sequence", hdr.Name),
			})
		}

		// Symlink detection
		if hdr.Typeflag == tar.TypeSymlink || hdr.Typeflag == tar.TypeLink {
			symlinkCount++
			// Check if symlink target escapes archive
			if containsPathTraversal(hdr.Linkname) {
				findings = append(findings, scanner.Finding{
					Scanner: s.Name(), Severity: scanner.SeverityCritical,
					Description: "TAR archive contains symlink with path traversal",
					Details:     fmt.Sprintf("Symlink %q -> %q escapes archive boundary", hdr.Name, hdr.Linkname),
				})
			}
		}

		// Executable detection
		if isExecutableExtension(strings.ToLower(hdr.Name)) {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityHigh,
				Description: "TAR archive contains executable file",
				Details:     fmt.Sprintf("Found executable: %q", hdr.Name),
			})
		}

		// Decompressed size check
		if totalSize > s.maxDecompressed {
			findings = append(findings, scanner.Finding{
				Scanner: s.Name(), Severity: scanner.SeverityCritical,
				Description: "TAR archive decompressed size exceeds limit",
				Details:     fmt.Sprintf("Total %d bytes exceeds %d byte limit", totalSize, s.maxDecompressed),
			})
			break
		}
	}

	// Symlink bomb: excessive symlinks suggest recursive link attack
	if symlinkCount > 50 {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityHigh,
			Description: "TAR archive contains excessive symlinks",
			Details:     fmt.Sprintf("%d symlinks detected — possible symlink bomb or link traversal attack", symlinkCount),
		})
	}

	if fileCount > s.maxFiles {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityHigh,
			Description: "TAR archive contains excessive files",
			Details:     fmt.Sprintf("%d files — possible resource exhaustion", fileCount),
		})
	}

	return findings
}

// ─────────────────────────────────────────────────────────────────────────────
// GZIP analysis (header + decompressed size estimation)
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) analyzeGzip(data []byte) []scanner.Finding {
	var findings []scanner.Finding

	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityMedium,
			Description: "Failed to analyze gzip archive",
			Details:     err.Error(),
		})
		return findings
	}
	defer gr.Close()

	// Read up to maxDecompressed+1 bytes to check for decompression bomb
	limited := io.LimitReader(gr, s.maxDecompressed+1)
	decompressed, err := io.ReadAll(limited)
	if err != nil && err != io.ErrUnexpectedEOF {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityMedium,
			Description: "Gzip decompression error",
			Details:     err.Error(),
		})
	}

	if int64(len(decompressed)) > s.maxDecompressed {
		ratio := float64(len(decompressed)) / float64(len(data))
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityCritical,
			Description: "Gzip decompressed size exceeds limit",
			Details:     fmt.Sprintf("Decompressed ≥%d bytes from %d bytes (%.0f:1 ratio) — possible gzip bomb", len(decompressed), len(data), ratio),
		})
	}

	// Check if the gzip contains a tar (common .tar.gz)
	if isTarData(decompressed) {
		findings = append(findings, s.analyzeTar(decompressed)...)
	}

	return findings
}

// ─────────────────────────────────────────────────────────────────────────────
// BZ2 analysis
// ─────────────────────────────────────────────────────────────────────────────

func (s *Scanner) analyzeBzip2(data []byte) []scanner.Finding {
	var findings []scanner.Finding

	br := bzip2.NewReader(bytes.NewReader(data))

	// Read up to maxDecompressed+1 bytes to check for decompression bomb
	limited := io.LimitReader(br, s.maxDecompressed+1)
	decompressed, err := io.ReadAll(limited)
	if err != nil && err != io.ErrUnexpectedEOF {
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityMedium,
			Description: "Bzip2 decompression error",
			Details:     err.Error(),
		})
	}

	if int64(len(decompressed)) > s.maxDecompressed {
		ratio := float64(len(decompressed)) / float64(len(data))
		findings = append(findings, scanner.Finding{
			Scanner: s.Name(), Severity: scanner.SeverityCritical,
			Description: "Bzip2 decompressed size exceeds limit",
			Details:     fmt.Sprintf("Decompressed ≥%d bytes from %d bytes (%.0f:1 ratio) — possible bzip2 bomb", len(decompressed), len(data), ratio),
		})
	}

	// Check if the bzip2 contains a tar (common .tar.bz2)
	if isTarData(decompressed) {
		findings = append(findings, s.analyzeTar(decompressed)...)
	}

	return findings
}

// ─────────────────────────────────────────────────────────────────────────────
// Detection helpers
// ─────────────────────────────────────────────────────────────────────────────

func isArchive(file *scanner.FileInfo) bool {
	ext := strings.ToLower(file.Extension)
	switch ext {
	case ".zip", ".rar", ".7z", ".tar", ".gz", ".tgz", ".bz2", ".xz",
		".docx", ".xlsx", ".pptx", ".docm", ".xlsm", ".pptm", ".jar":
		return true
	}
	mime := strings.ToLower(file.MIMEType)
	return strings.Contains(mime, "zip") || strings.Contains(mime, "rar") ||
		strings.Contains(mime, "7z") || strings.Contains(mime, "compressed") ||
		strings.Contains(mime, "gzip") || strings.Contains(mime, "bzip2") ||
		strings.Contains(mime, "tar")
}

func isZipArchive(file *scanner.FileInfo) bool {
	if len(file.Content) >= 4 &&
		file.Content[0] == 0x50 && file.Content[1] == 0x4B &&
		file.Content[2] == 0x03 && file.Content[3] == 0x04 {
		return true
	}
	ext := strings.ToLower(file.Extension)
	switch ext {
	case ".zip", ".docx", ".xlsx", ".pptx", ".docm", ".xlsm", ".pptm", ".jar":
		return true
	}
	return false
}

func isTarArchive(file *scanner.FileInfo) bool {
	ext := strings.ToLower(file.Extension)
	if ext == ".tar" {
		return true
	}
	// Check for TAR magic bytes at offset 257: "ustar"
	return isTarData(file.Content)
}

func isTarData(data []byte) bool {
	if len(data) >= 263 {
		magic := string(data[257:262])
		return magic == "ustar"
	}
	return false
}

func isGzipArchive(file *scanner.FileInfo) bool {
	ext := strings.ToLower(file.Extension)
	if ext == ".gz" || ext == ".tgz" {
		return true
	}
	// Gzip magic bytes: 1F 8B
	return len(file.Content) >= 2 && file.Content[0] == 0x1F && file.Content[1] == 0x8B
}

func isBzip2Archive(file *scanner.FileInfo) bool {
	ext := strings.ToLower(file.Extension)
	if ext == ".bz2" {
		return true
	}
	// BZ2 magic bytes: BZ (0x42 0x5A)
	return len(file.Content) >= 3 && file.Content[0] == 0x42 && file.Content[1] == 0x5A && file.Content[2] == 0x68
}

// containsPathTraversal checks for directory traversal patterns in a path.
func containsPathTraversal(name string) bool {
	normalized := strings.ReplaceAll(name, "\\", "/")
	return strings.Contains(normalized, "../") ||
		strings.Contains(normalized, "/..") ||
		strings.HasPrefix(normalized, "/") ||
		name == ".." ||
		strings.HasPrefix(name, "..")
}

func isExecutableExtension(name string) bool {
	exts := []string{".exe", ".bat", ".cmd", ".com", ".msi", ".scr", ".pif",
		".sh", ".bash", ".ps1", ".vbs", ".vbe", ".js", ".wsh", ".wsf",
		".dll", ".sys", ".cpl", ".hta", ".inf", ".reg"}
	for _, ext := range exts {
		if strings.HasSuffix(name, ext) {
			return true
		}
	}
	return false
}
