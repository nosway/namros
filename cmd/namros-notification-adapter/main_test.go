package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/nosway/namros/internal/opsnotify"
)

func TestDeliveryHistoryEndpointRecordsWebhook(t *testing.T) {
	adapter := opsnotify.NewAdapter(opsnotify.Channel{ID: "ops-webhook", Kind: "alertmanager_webhook"})
	history := newDeliveryHistory(10)
	handler := newHTTPHandler(adapter, history)

	payload := []byte(`{
	  "receiver": "namros-webhook",
	  "status": "firing",
	  "alerts": [
	    {
	      "status": "firing",
	      "labels": {"alertname": "NamrosGatewayDown"},
	      "annotations": {"summary": "sample gateway down"}
	    }
	  ]
	}`)
	postReq := httptest.NewRequest(http.MethodPost, "/alertmanager/webhook", bytes.NewReader(payload))
	postReq.Header.Set("Content-Type", "application/json")
	postResp := httptest.NewRecorder()
	handler.ServeHTTP(postResp, postReq)
	if postResp.Code != http.StatusAccepted {
		t.Fatalf("POST webhook status = %d, want %d", postResp.Code, http.StatusAccepted)
	}

	historyReq := httptest.NewRequest(http.MethodGet, "/api/v1/notification/deliveries", nil)
	historyResp := httptest.NewRecorder()
	handler.ServeHTTP(historyResp, historyReq)
	var snapshot deliveryHistorySnapshot
	if err := json.NewDecoder(historyResp.Body).Decode(&snapshot); err != nil {
		t.Fatalf("decode history failed: %v", err)
	}
	if snapshot.SchemaVersion != "namros.notification.deliveries.v1" || snapshot.Count != 1 {
		t.Fatalf("snapshot = %+v, want one delivery", snapshot)
	}
	if got := snapshot.Deliveries[0]; got.Alerts != 1 || got.ChannelID != "ops-webhook" {
		t.Fatalf("delivery = %+v, want captured ops webhook alert", got)
	}
}

func TestDeliveryHistoryLimitKeepsMostRecent(t *testing.T) {
	history := newDeliveryHistory(1)
	history.append(opsnotify.DeliveryResult{ChannelID: "old", Alerts: 1})
	history.append(opsnotify.DeliveryResult{ChannelID: "new", Alerts: 2})

	snapshot := history.snapshot(time.Now())
	if snapshot.Count != 1 {
		t.Fatalf("snapshot count = %d, want 1", snapshot.Count)
	}
	if got := snapshot.Deliveries[0].ChannelID; got != "new" {
		t.Fatalf("kept channel = %q, want new", got)
	}
}
