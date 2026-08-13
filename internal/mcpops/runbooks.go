package mcpops

type Runbook struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Path        string `json:"path"`
	HTMLPath    string `json:"html_path,omitempty"`
	Description string `json:"description"`
}

func Runbooks() []Runbook {
	return []Runbook{
		{
			ID:          "s3-compatibility",
			Title:       "S3 compatibility",
			Path:        "docs/html-docs/s3-client-compatibility-guide.html",
			HTMLPath:    "docs/html-docs/s3-client-compatibility-guide.html",
			Description: "AWS CLI, MinIO client, rclone, and s3fs-fuse compatibility checks.",
		},
		{
			ID:          "gateway-coordination",
			Title:       "Gateway coordination",
			Path:        "docs/html-docs/etcd-ha-cluster-install-operations-guide.html",
			HTMLPath:    "docs/html-docs/etcd-ha-cluster-install-operations-guide.html",
			Description: "Active-active gateway registry, heartbeat, readiness, and etcd lease behavior.",
		},
		{
			ID:          "metadata-backup-restore",
			Title:       "Metadata backup and restore",
			Path:        "docs/html-docs/upgrade-release-operations-guide.html",
			HTMLPath:    "docs/html-docs/upgrade-release-operations-guide.html",
			Description: "Metadata export/import preflight, restore safety checks, and conflict handling.",
		},
		{
			ID:          "s3fs-linux",
			Title:       "Linux s3fs-fuse compatibility",
			Path:        "docs/html-docs/s3-client-compatibility-guide.html",
			HTMLPath:    "docs/html-docs/s3-client-compatibility-guide.html",
			Description: "Linux host s3fs-fuse mount/read/write/list/rename/multipart procedure.",
		},
		{
			ID:          "release-readiness",
			Title:       "Release readiness",
			Path:        "docs/html-docs/upgrade-release-operations-guide.html",
			HTMLPath:    "docs/html-docs/upgrade-release-operations-guide.html",
			Description: "Release gate artifact, compatibility matrix, and readiness target set.",
		},
		{
			ID:          "mcp-operations",
			Title:       "MCP operations",
			Path:        "docs/html-docs/mcp-operations-guide.html",
			HTMLPath:    "docs/html-docs/mcp-operations-guide.html",
			Description: "MCP resources, read-only tools, approval envelope, and incident workflows.",
		},
	}
}

func RunbookIndex() map[string]any {
	return map[string]any{
		"schema_version": "namros.mcp.runbooks.v1",
		"runbooks":       Runbooks(),
	}
}
