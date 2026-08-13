package main

import (
	"context"
	"encoding/json"
	"flag"
	"log"
	"os"
	"strings"
	"time"

	"github.com/nosway/namros/internal/mcpops"
	"github.com/nosway/namros/internal/opsreport"
	"github.com/nosway/namros/internal/sbsops"
)

func main() {
	var scope string
	var adminEndpoints string
	var dataEndpoints string
	var volumeIDs string
	var namrbdEndpoint string
	var namrbdTimeout time.Duration
	var incidentBundle bool
	var incidentLabel string
	var gatewayEndpoint string
	var adminEndpoint string
	var operationOutputDir string
	var etcdEndpoints string
	var etcdRoot string
	mcpCfg := mcpops.DefaultConfig()
	flag.StringVar(&scope, "scope", "cluster", "report scope label")
	flag.StringVar(&adminEndpoints, "sbs-admin-endpoints", "", "comma-separated SBS admin endpoints")
	flag.StringVar(&dataEndpoints, "sbs-data-endpoints", "", "comma-separated SBS data endpoints")
	flag.StringVar(&volumeIDs, "sbs-volume-ids", "", "comma-separated SBS volume ids")
	flag.StringVar(&namrbdEndpoint, "namrbd-sbs-observability-endpoint", "", "NAMRBD SBS read-only observability endpoint or base URL")
	flag.DurationVar(&namrbdTimeout, "namrbd-sbs-observability-timeout", sbsops.DefaultNAMRBDTimeout(), "NAMRBD SBS observability request timeout")
	flag.BoolVar(&incidentBundle, "incident-bundle", false, "write an incident support bundle and print the bundle envelope")
	flag.StringVar(&incidentLabel, "incident-label", "", "optional incident bundle label")
	flag.StringVar(&gatewayEndpoint, "gateway-endpoint", mcpCfg.GatewayEndpoint, "gateway endpoint used for incident bundle health probes")
	flag.StringVar(&adminEndpoint, "admin-endpoint", mcpCfg.AdminEndpoint, "admin endpoint used for incident bundle debug probes")
	flag.StringVar(&operationOutputDir, "operation-output-dir", mcpCfg.OperationOutputDir, "directory for incident bundle output")
	flag.StringVar(&etcdEndpoints, "etcd-endpoints", strings.Join(mcpCfg.EtcdEndpoints, ","), "comma-separated etcd endpoints used for incident bundle gateway registry snapshots")
	flag.StringVar(&etcdRoot, "etcd-root", mcpCfg.EtcdRoot, "etcd gateway registry root used for incident bundle snapshots")
	flag.Parse()

	if incidentBundle {
		mcpCfg.GatewayEndpoint = gatewayEndpoint
		mcpCfg.AdminEndpoint = adminEndpoint
		mcpCfg.OperationOutputDir = operationOutputDir
		mcpCfg.EtcdEndpoints = splitComma(etcdEndpoints)
		mcpCfg.EtcdRoot = etcdRoot
		out, err := mcpops.WriteIncidentBundle(context.Background(), mcpCfg, incidentLabel)
		if err != nil {
			log.Fatal(err)
		}
		writeJSON(out)
		return
	}

	collector := sbsops.NewCollector(sbsops.Config{
		ClusterID:                      scope,
		AdminEndpoints:                 splitComma(adminEndpoints),
		DataEndpoints:                  splitComma(dataEndpoints),
		VolumeIDs:                      splitComma(volumeIDs),
		NAMRBDSBSObservabilityEndpoint: namrbdEndpoint,
		NAMRBDSBSObservabilityTimeout:  namrbdTimeout,
	})
	report := opsreport.Build(opsreport.Input{
		Scope: scope,
		SBS:   collector.Snapshot(nil),
	})
	writeJSON(report)
}

func writeJSON(value any) {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		log.Fatal(err)
	}
}

func splitComma(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
