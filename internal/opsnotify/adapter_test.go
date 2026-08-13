package opsnotify

import (
	"context"
	"testing"
)

func TestAdapterAcceptsAlertmanagerPayloadWithoutDelivery(t *testing.T) {
	adapter := NewAdapter(Channel{ID: "ops-webhook", Kind: "alertmanager_webhook"})
	result, err := adapter.Deliver(context.Background(), AlertmanagerPayload{
		Receiver: "namros-webhook",
		Status:   "firing",
		Alerts: []AlertmanagerAlert{
			{Status: "firing", Labels: map[string]string{"alertname": "NamrosGatewayDown"}},
		},
	})
	if err != nil {
		t.Fatalf("Deliver() error = %v", err)
	}
	if result.Status != "not_delivered" || result.Delivered {
		t.Fatalf("result = %+v, want accepted capture without delivery", result)
	}
	if result.Alerts != 1 || len(result.Limitations) == 0 {
		t.Fatalf("result alert count/limitations = %+v", result)
	}
}
