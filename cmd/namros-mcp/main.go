package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/nosway/namros/internal/mcpops"
)

func main() {
	cfg := mcpops.DefaultConfig()
	fs := flag.NewFlagSet("namros-mcp", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	etcdEndpoints := strings.Join(cfg.EtcdEndpoints, ",")
	pdEndpoints := strings.Join(cfg.PDEndpoints, ",")

	fs.StringVar(&cfg.GatewayEndpoint, "gateway-endpoint", cfg.GatewayEndpoint, "gateway endpoint used for health/readiness probes")
	fs.StringVar(&cfg.AdminEndpoint, "admin-endpoint", cfg.AdminEndpoint, "admin/debug endpoint used for status and metrics collectors")
	fs.StringVar(&etcdEndpoints, "etcd-endpoints", etcdEndpoints, "comma-separated etcd endpoints for gateway registry reads")
	fs.StringVar(&cfg.EtcdRoot, "etcd-root", cfg.EtcdRoot, "etcd key prefix for gateway registry reads")
	fs.StringVar(&cfg.MetadataBackend, "metadata-backend", cfg.MetadataBackend, "metadata backend hint for operation plans")
	fs.StringVar(&cfg.MetadataPath, "metadata-path", cfg.MetadataPath, "metadata path hint for operation plans")
	fs.StringVar(&pdEndpoints, "pd-endpoints", pdEndpoints, "comma-separated TiKV PD endpoints hint for operation plans")
	fs.StringVar(&cfg.Keyspace, "keyspace", cfg.Keyspace, "TiKV keyspace hint for operation plans")
	fs.StringVar(&cfg.ReleaseReportDir, "release-report-dir", cfg.ReleaseReportDir, "directory containing release-readiness report artifacts")
	fs.StringVar(&cfg.CompatReportDir, "compat-report-dir", cfg.CompatReportDir, "directory containing compatibility report artifacts")
	fs.StringVar(&cfg.ChaosReportDir, "chaos-report-dir", cfg.ChaosReportDir, "directory containing multi-node chaos/soak report artifacts")
	fs.StringVar(&cfg.Mode, "mode", cfg.Mode, "MCP posture: observe or operate")
	fs.StringVar(&cfg.ApprovalPolicy, "approval-policy", cfg.ApprovalPolicy, "approval policy: dry-run, external-token, or local-confirmation")
	fs.StringVar(&cfg.OperationOutputDir, "operation-output-dir", cfg.OperationOutputDir, "directory for MCP incident bundles and operation records")
	fs.DurationVar(&cfg.HTTPTimeout, "http-timeout", cfg.HTTPTimeout, "HTTP and etcd collector timeout")

	if err := fs.Parse(os.Args[1:]); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 0 {
		fmt.Fprintf(os.Stderr, "namros-mcp: unexpected positional arguments: %v\n", fs.Args())
		os.Exit(2)
	}
	cfg.EtcdEndpoints = splitCommaList(etcdEndpoints)
	cfg.PDEndpoints = splitCommaList(pdEndpoints)
	cfg = cfg.Normalized()
	if cfg.Mode != mcpops.ModeObserve && cfg.Mode != mcpops.ModeOperate {
		fmt.Fprintf(os.Stderr, "namros-mcp: unsupported mode %q\n", cfg.Mode)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mcpops.RunStdio(ctx, cfg, os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "namros-mcp: %v\n", err)
		os.Exit(1)
	}
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
