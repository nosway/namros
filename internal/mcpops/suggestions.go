package mcpops

import "strings"

type RunbookSuggestion struct {
	Signal        string   `json:"signal"`
	Subsystem     string   `json:"subsystem"`
	RunbookIDs    []string `json:"runbook_ids"`
	SafeCommands  []string `json:"safe_commands,omitempty"`
	NeedsApproval []string `json:"needs_approval,omitempty"`
}

func SuggestRunbooks(signal string) []RunbookSuggestion {
	normalized := strings.ToLower(strings.TrimSpace(signal))
	if normalized == "" {
		return []RunbookSuggestion{{
			Signal:     "unknown",
			Subsystem:  "triage",
			RunbookIDs: []string{"mcp-operations"},
		}}
	}
	rules := []struct {
		contains string
		out      RunbookSuggestion
	}{
		{
			contains: "ready",
			out: RunbookSuggestion{
				Signal:       signal,
				Subsystem:    "gateway",
				RunbookIDs:   []string{"gateway-coordination", "mcp-operations"},
				SafeCommands: []string{"curl -fsS http://127.0.0.1:9000/healthz", "curl -fsS http://127.0.0.1:9000/readyz"},
			},
		},
		{
			contains: "registry",
			out: RunbookSuggestion{
				Signal:       signal,
				Subsystem:    "coordination",
				RunbookIDs:   []string{"gateway-coordination"},
				SafeCommands: []string{"make smoke-etcd-registry"},
			},
		},
		{
			contains: "metadata",
			out: RunbookSuggestion{
				Signal:        signal,
				Subsystem:     "metadata",
				RunbookIDs:    []string{"metadata-backup-restore", "release-readiness"},
				SafeCommands:  []string{"namros-admin status -metadata-backend tikv"},
				NeedsApproval: []string{"namros.metadata.backup.create"},
			},
		},
		{
			contains: "compat",
			out: RunbookSuggestion{
				Signal:        signal,
				Subsystem:     "compatibility",
				RunbookIDs:    []string{"s3-compatibility", "s3fs-linux"},
				SafeCommands:  []string{"make container-local-smoke"},
				NeedsApproval: []string{"namros.compat.user_space.run"},
			},
		},
		{
			contains: "release",
			out: RunbookSuggestion{
				Signal:        signal,
				Subsystem:     "release",
				RunbookIDs:    []string{"release-readiness"},
				SafeCommands:  []string{"make community-release-check"},
				NeedsApproval: []string{"namros.release.readiness.run"},
			},
		},
		{
			contains: "enterprise",
			out: RunbookSuggestion{
				Signal:     signal,
				Subsystem:  "edition",
				RunbookIDs: []string{"mcp-operations"},
			},
		},
	}
	for _, rule := range rules {
		if strings.Contains(normalized, rule.contains) {
			return []RunbookSuggestion{rule.out}
		}
	}
	return []RunbookSuggestion{{
		Signal:     signal,
		Subsystem:  "triage",
		RunbookIDs: []string{"mcp-operations", "release-readiness"},
	}}
}
