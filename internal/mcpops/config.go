package mcpops

import (
	"net/http"
	"strings"
	"time"

	"github.com/nosway/namros/internal/config"
)

const (
	ModeObserve = "observe"
	ModeOperate = "operate"

	ApprovalPolicyDryRun            = "dry-run"
	ApprovalPolicyExternalToken     = "external-token"
	ApprovalPolicyLocalConfirmation = "local-confirmation"
)

type Config struct {
	GatewayEndpoint    string
	AdminEndpoint      string
	EtcdEndpoints      []string
	EtcdRoot           string
	MetadataBackend    string
	MetadataPath       string
	PDEndpoints        []string
	Keyspace           string
	ReleaseReportDir   string
	CompatReportDir    string
	ChaosReportDir     string
	OperationOutputDir string
	Mode               string
	ApprovalPolicy     string
	HTTPTimeout        time.Duration
	HTTPClient         *http.Client
}

func DefaultConfig() Config {
	return Config{
		GatewayEndpoint:    "http://127.0.0.1:9000",
		AdminEndpoint:      "http://127.0.0.1:9000",
		EtcdEndpoints:      append([]string(nil), config.DefaultEtcdEndpoints...),
		EtcdRoot:           config.DefaultGatewayRegistryPrefix,
		MetadataBackend:    config.DefaultMetadataBackend,
		MetadataPath:       config.DefaultMetadataPath,
		PDEndpoints:        append([]string(nil), config.DefaultTiKVPDEndpoints...),
		Keyspace:           "namros",
		ReleaseReportDir:   "release-reports",
		CompatReportDir:    "compat-reports",
		ChaosReportDir:     "chaos-reports",
		OperationOutputDir: ".namros/mcp-operations",
		Mode:               ModeObserve,
		ApprovalPolicy:     ApprovalPolicyDryRun,
		HTTPTimeout:        3 * time.Second,
	}
}

func (c Config) Normalized() Config {
	c.GatewayEndpoint = trimEndpoint(c.GatewayEndpoint)
	c.AdminEndpoint = trimEndpoint(c.AdminEndpoint)
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode == "" {
		c.Mode = ModeObserve
	}
	c.ApprovalPolicy = strings.ToLower(strings.TrimSpace(c.ApprovalPolicy))
	if c.ApprovalPolicy == "" {
		c.ApprovalPolicy = ApprovalPolicyDryRun
	}
	if c.HTTPTimeout <= 0 {
		c.HTTPTimeout = 3 * time.Second
	}
	c.EtcdRoot = strings.TrimSpace(c.EtcdRoot)
	if c.EtcdRoot == "" {
		c.EtcdRoot = config.DefaultGatewayRegistryPrefix
	}
	return c
}

func trimEndpoint(value string) string {
	value = strings.TrimSpace(value)
	return strings.TrimRight(value, "/")
}

func splitCommaList(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
