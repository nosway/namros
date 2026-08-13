package opsnotify

import (
	"context"
	"errors"
	"strings"
	"time"
)

type Channel struct {
	ID      string `json:"id"`
	Kind    string `json:"kind"`
	Enabled bool   `json:"enabled"`
}

type AlertmanagerPayload struct {
	Receiver string              `json:"receiver"`
	Status   string              `json:"status"`
	Alerts   []AlertmanagerAlert `json:"alerts"`
}

type AlertmanagerAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
	EndsAt      time.Time         `json:"endsAt"`
}

type DeliveryResult struct {
	SchemaVersion string    `json:"schema_version"`
	GeneratedAt   string    `json:"generated_at"`
	Status        string    `json:"status"`
	ChannelID     string    `json:"channel_id"`
	Kind          string    `json:"kind"`
	Alerts        int       `json:"alerts"`
	Delivered     bool      `json:"delivered"`
	Limitations   []string  `json:"limitations,omitempty"`
	ReceivedAt    time.Time `json:"received_at"`
}

type Adapter struct {
	channel Channel
}

func NewAdapter(channel Channel) *Adapter {
	channel.ID = strings.TrimSpace(channel.ID)
	if channel.ID == "" {
		channel.ID = "ops-webhook"
	}
	channel.Kind = strings.TrimSpace(channel.Kind)
	if channel.Kind == "" {
		channel.Kind = "alertmanager_webhook"
	}
	return &Adapter{channel: channel}
}

func (a *Adapter) Deliver(_ context.Context, payload AlertmanagerPayload) (DeliveryResult, error) {
	if a == nil {
		return DeliveryResult{}, errors.New("nil notification adapter")
	}
	now := time.Now().UTC()
	result := DeliveryResult{
		SchemaVersion: "namros.notification.delivery.v1",
		GeneratedAt:   now.Format(time.RFC3339Nano),
		Status:        "not_delivered",
		ChannelID:     a.channel.ID,
		Kind:          a.channel.Kind,
		Alerts:        len(payload.Alerts),
		Delivered:     false,
		ReceivedAt:    now,
	}
	if !a.channel.Enabled {
		result.Limitations = []string{"Notification channel is disabled; payload was accepted but not delivered."}
		return result, nil
	}
	result.Status = "accepted"
	result.Delivered = false
	result.Limitations = []string{"Provider delivery is not implemented in this build slice; use Alertmanager webhook capture for validation."}
	return result, nil
}
