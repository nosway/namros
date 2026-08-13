package mcpops

import "testing"

func TestSuggestRunbooks(t *testing.T) {
	tests := []struct {
		name      string
		signal    string
		subsystem string
		runbookID string
	}{
		{name: "gateway readiness", signal: "gateway is not ready", subsystem: "gateway", runbookID: "gateway-coordination"},
		{name: "metadata", signal: "metadata backend unavailable", subsystem: "metadata", runbookID: "metadata-backup-restore"},
		{name: "compatibility", signal: "compatibility smoke failed", subsystem: "compatibility", runbookID: "s3-compatibility"},
		{name: "fallback", signal: "unexpected failure", subsystem: "triage", runbookID: "mcp-operations"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := SuggestRunbooks(tt.signal)
			if len(got) != 1 {
				t.Fatalf("SuggestRunbooks() len = %d, want 1", len(got))
			}
			if got[0].Subsystem != tt.subsystem {
				t.Fatalf("Subsystem = %q, want %q", got[0].Subsystem, tt.subsystem)
			}
			if !containsString(got[0].RunbookIDs, tt.runbookID) {
				t.Fatalf("RunbookIDs = %v, want %q", got[0].RunbookIDs, tt.runbookID)
			}
		})
	}
}

func TestRedactRemovesSecretLikeFields(t *testing.T) {
	value := map[string]any{
		"authorization": "AWS4-HMAC-SHA256 secret",
		"nested": map[string]any{
			"root_secret_access_key": "very-secret",
			"safe":                   "visible",
		},
	}
	got := Redact(value).(map[string]any)
	if got["authorization"] != redacted {
		t.Fatalf("authorization = %v, want redacted", got["authorization"])
	}
	nested := got["nested"].(map[string]any)
	if nested["root_secret_access_key"] != redacted {
		t.Fatalf("root_secret_access_key = %v, want redacted", nested["root_secret_access_key"])
	}
	if nested["safe"] != "visible" {
		t.Fatalf("safe = %v, want visible", nested["safe"])
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
