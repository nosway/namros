package auth

import "testing"

func TestAllowsAction(t *testing.T) {
	tests := []struct {
		name      string
		principal Principal
		action    string
		want      bool
	}{
		{
			name: "root allows any action",
			principal: Principal{
				Root: true,
			},
			action: ActionBypassGovernanceRetention,
			want:   true,
		},
		{
			name: "exact permission",
			principal: Principal{
				Permissions: []string{ActionBypassGovernanceRetention},
			},
			action: ActionBypassGovernanceRetention,
			want:   true,
		},
		{
			name: "service wildcard",
			principal: Principal{
				Permissions: []string{"s3:*"},
			},
			action: ActionBypassGovernanceRetention,
			want:   true,
		},
		{
			name: "global wildcard",
			principal: Principal{
				Permissions: []string{"*"},
			},
			action: ActionBypassGovernanceRetention,
			want:   true,
		},
		{
			name: "missing permission denies",
			principal: Principal{
				Permissions: []string{"s3:GetObject"},
			},
			action: ActionBypassGovernanceRetention,
			want:   false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := AllowsAction(tt.principal, tt.action); got != tt.want {
				t.Fatalf("AllowsAction() = %v, want %v", got, tt.want)
			}
		})
	}
}
