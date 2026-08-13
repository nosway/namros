package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/nosway/namros/internal/opsnotify"
)

const deliveryHistoryDefaultLimit = 100

type deliveryHistory struct {
	mu         sync.Mutex
	limit      int
	deliveries []opsnotify.DeliveryResult
}

type deliveryHistorySnapshot struct {
	SchemaVersion string                     `json:"schema_version"`
	GeneratedAt   string                     `json:"generated_at"`
	Count         int                        `json:"count"`
	Deliveries    []opsnotify.DeliveryResult `json:"deliveries"`
}

func newDeliveryHistory(limit int) *deliveryHistory {
	if limit <= 0 {
		limit = deliveryHistoryDefaultLimit
	}
	return &deliveryHistory{limit: limit}
}

func (h *deliveryHistory) append(result opsnotify.DeliveryResult) {
	if h == nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.deliveries = append(h.deliveries, result)
	if len(h.deliveries) > h.limit {
		h.deliveries = h.deliveries[len(h.deliveries)-h.limit:]
	}
}

func (h *deliveryHistory) snapshot(now time.Time) deliveryHistorySnapshot {
	if h == nil {
		return deliveryHistorySnapshot{
			SchemaVersion: "namros.notification.deliveries.v1",
			GeneratedAt:   now.UTC().Format(time.RFC3339Nano),
		}
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	deliveries := append([]opsnotify.DeliveryResult(nil), h.deliveries...)
	return deliveryHistorySnapshot{
		SchemaVersion: "namros.notification.deliveries.v1",
		GeneratedAt:   now.UTC().Format(time.RFC3339Nano),
		Count:         len(deliveries),
		Deliveries:    deliveries,
	}
}

func newHTTPHandler(adapter *opsnotify.Adapter, history *deliveryHistory) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}` + "\n"))
	})
	mux.HandleFunc("/api/v1/notification/deliveries", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(history.snapshot(time.Now())); err != nil {
			log.Printf("encode delivery history failed: %v", err)
		}
	})
	mux.HandleFunc("/alertmanager/webhook", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		defer r.Body.Close()
		var payload opsnotify.AlertmanagerPayload
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		result, err := adapter.Deliver(r.Context(), payload)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		history.append(result)
		log.Printf("captured alertmanager webhook channel_id=%s status=%s alerts=%d delivered=%t", result.ChannelID, result.Status, result.Alerts, result.Delivered)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		if err := json.NewEncoder(w).Encode(result); err != nil {
			log.Printf("encode delivery result failed: %v", err)
		}
	})
	return mux
}

func main() {
	var listen string
	var channelID string
	var kind string
	var enabled bool
	flag.StringVar(&listen, "listen", "127.0.0.1:19120", "HTTP listen address for notification adapter")
	flag.StringVar(&channelID, "channel-id", "ops-webhook", "notification channel id")
	flag.StringVar(&kind, "kind", "alertmanager_webhook", "notification channel kind")
	flag.BoolVar(&enabled, "enabled", false, "mark channel enabled; provider delivery still requires a future provider implementation")
	flag.Parse()

	adapter := opsnotify.NewAdapter(opsnotify.Channel{ID: channelID, Kind: kind, Enabled: enabled})
	history := newDeliveryHistory(deliveryHistoryDefaultLimit)
	server := &http.Server{
		Addr:              listen,
		Handler:           newHTTPHandler(adapter, history),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("starting namros-notification-adapter listen=%s", listen)
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}
