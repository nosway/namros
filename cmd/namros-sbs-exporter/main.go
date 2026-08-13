package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/nosway/namros/internal/sbsops"
)

func main() {
	var listen string
	var clusterID string
	var adminEndpoints string
	var dataEndpoints string
	var volumeIDs string
	var namrbdEndpoint string
	var namrbdTimeout time.Duration
	flag.StringVar(&listen, "listen", "127.0.0.1:19110", "HTTP listen address for SBS operations exporter")
	flag.StringVar(&clusterID, "cluster-id", "", "SBS cluster id label shown in JSON status")
	flag.StringVar(&adminEndpoints, "sbs-admin-endpoints", "", "comma-separated SBS admin endpoints")
	flag.StringVar(&dataEndpoints, "sbs-data-endpoints", "", "comma-separated SBS data endpoints")
	flag.StringVar(&volumeIDs, "sbs-volume-ids", "", "comma-separated SBS volume ids")
	flag.StringVar(&namrbdEndpoint, "namrbd-sbs-observability-endpoint", "", "NAMRBD SBS read-only observability endpoint or base URL")
	flag.DurationVar(&namrbdTimeout, "namrbd-sbs-observability-timeout", sbsops.DefaultNAMRBDTimeout(), "NAMRBD SBS observability request timeout")
	flag.Parse()

	collector := sbsops.NewCollector(sbsops.Config{
		ClusterID:                      clusterID,
		AdminEndpoints:                 splitComma(adminEndpoints),
		DataEndpoints:                  splitComma(dataEndpoints),
		VolumeIDs:                      splitComma(volumeIDs),
		NAMRBDSBSObservabilityEndpoint: namrbdEndpoint,
		NAMRBDSBSObservabilityTimeout:  namrbdTimeout,
	})
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
	})
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		if err := sbsops.WritePrometheus(w, collector.Snapshot(r.Context())); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/api/v1/sbs/cluster", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, collector.Snapshot(r.Context()))
	})
	server := &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("starting namros-sbs-exporter listen=%s", listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
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

func writeJSON(w http.ResponseWriter, snapshot sbsops.Snapshot) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(snapshot); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
