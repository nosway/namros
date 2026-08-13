package mcpops

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxArtifactPreviewBytes = 64 * 1024

type ArtifactSummary struct {
	SchemaVersion string `json:"schema_version"`
	Kind          string `json:"kind"`
	Directory     string `json:"directory"`
	Found         bool   `json:"found"`
	Path          string `json:"path,omitempty"`
	SizeBytes     int64  `json:"size_bytes,omitempty"`
	ModifiedAt    string `json:"modified_at,omitempty"`
	Preview       string `json:"preview,omitempty"`
	Truncated     bool   `json:"truncated,omitempty"`
	Error         string `json:"error,omitempty"`
}

type ReportIndex struct {
	SchemaVersion string            `json:"schema_version"`
	GeneratedAt   string            `json:"generated_at"`
	Reports       []ArtifactSummary `json:"reports"`
}

func LatestArtifact(kind, dir string) ArtifactSummary {
	out := ArtifactSummary{
		SchemaVersion: "namros.mcp.artifact.v1",
		Kind:          kind,
		Directory:     dir,
	}
	if strings.TrimSpace(dir) == "" {
		out.Error = "directory is not configured"
		return out
	}
	info, err := os.Stat(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return out
		}
		out.Error = err.Error()
		return out
	}
	if !info.IsDir() {
		out.Error = "path is not a directory"
		return out
	}
	files, err := artifactFiles(dir)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	if len(files) == 0 {
		return out
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].ModifiedAt.After(files[j].ModifiedAt)
	})
	latest := files[0]
	out.Found = true
	out.Path = latest.Path
	out.SizeBytes = latest.Size
	out.ModifiedAt = latest.ModifiedAt.UTC().Format(time.RFC3339Nano)
	preview, truncated, err := artifactPreview(latest.Path, latest.Size)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Preview = RedactText(preview)
	out.Truncated = truncated
	return out
}

func BuildReportIndex(cfg Config) ReportIndex {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	return ReportIndex{
		SchemaVersion: "namros.mcp.report_index.v1",
		GeneratedAt:   now,
		Reports: []ArtifactSummary{
			LatestArtifact("compatibility", cfg.CompatReportDir),
			LatestArtifact("release", cfg.ReleaseReportDir),
		},
	}
}

type artifactFile struct {
	Path       string
	Size       int64
	ModifiedAt time.Time
}

func artifactFiles(dir string) ([]artifactFile, error) {
	var files []artifactFile
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		files = append(files, artifactFile{
			Path:       path,
			Size:       info.Size(),
			ModifiedAt: info.ModTime(),
		})
		return nil
	})
	return files, err
}

func artifactPreview(path string, size int64) (string, bool, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", false, err
	}
	defer file.Close()
	limit := int64(maxArtifactPreviewBytes)
	if size < limit {
		limit = size
	}
	buf := make([]byte, limit)
	n, err := file.Read(buf)
	if err != nil && n == 0 {
		return "", false, err
	}
	return string(buf[:n]), size > int64(n), nil
}
