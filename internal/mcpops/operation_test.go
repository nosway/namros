package mcpops

import (
	"context"
	"encoding/json"
	"os"
	"testing"
)

func TestCommunityOperationWritesLocalRecord(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Mode = ModeOperate
	cfg.OperationOutputDir = t.TempDir()

	result, err := CallTool(context.Background(), cfg, "namros.compat.user_space.run", map[string]any{
		"approval_reference": "ticket-123",
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	envelope, ok := result.(OperationEnvelope)
	if !ok {
		t.Fatalf("CallTool() result type = %T, want OperationEnvelope", result)
	}
	if envelope.Tool != "namros.compat.user_space.run" {
		t.Fatalf("tool = %q", envelope.Tool)
	}
	if envelope.Preflight["status"] != "planned" {
		t.Fatalf("preflight status = %v, want planned", envelope.Preflight["status"])
	}
	if envelope.Approval.Reference != "ticket-123" {
		t.Fatalf("approval reference = %q", envelope.Approval.Reference)
	}
	path, ok := envelope.Audit["local_path"].(string)
	if !ok || path == "" {
		t.Fatalf("audit local_path = %#v", envelope.Audit["local_path"])
	}
	payload, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read operation record: %v", err)
	}
	var stored OperationEnvelope
	if err := json.Unmarshal(payload, &stored); err != nil {
		t.Fatalf("decode operation record: %v", err)
	}
	if stored.OperationID != envelope.OperationID || stored.Approval.Reference != "ticket-123" {
		t.Fatalf("stored envelope mismatch: %+v", stored)
	}
}

func TestEnterpriseOperationPlanBlocksInCommunity(t *testing.T) {
	skipEnterpriseOverlayCommunityAssertion(t)
	cfg := DefaultConfig()
	cfg.Mode = ModeOperate
	cfg.OperationOutputDir = t.TempDir()

	result, err := CallTool(context.Background(), cfg, "namros.multi_node.soak.run", map[string]any{
		"approval_reference": "change-456",
	})
	if err != nil {
		t.Fatalf("CallTool() error = %v", err)
	}
	envelope, ok := result.(OperationEnvelope)
	if !ok {
		t.Fatalf("CallTool() result type = %T, want OperationEnvelope", result)
	}
	if envelope.Preflight["status"] != "blocked" {
		t.Fatalf("preflight status = %v, want blocked", envelope.Preflight["status"])
	}
	if envelope.Result["status"] != "blocked" {
		t.Fatalf("result status = %v, want blocked", envelope.Result["status"])
	}
	path, ok := envelope.Audit["local_path"].(string)
	if !ok || path == "" {
		t.Fatalf("audit local_path = %#v", envelope.Audit["local_path"])
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("operation record was not written: %v", err)
	}
}
