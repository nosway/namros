package auth

import (
	"context"
	"strings"
)

const (
	ActionBypassGovernanceRetention = "s3:BypassGovernanceRetention"
)

type Principal struct {
	TenantID       string   `json:"tenant_id,omitempty"`
	AccessKeyID    string   `json:"access_key_id,omitempty"`
	DisplayName    string   `json:"display_name,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Groups         []string `json:"groups,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	SessionID      string   `json:"session_id,omitempty"`
	ExternalIssuer string   `json:"external_issuer,omitempty"`
	PolicyVersion  string   `json:"policy_version,omitempty"`
	SourceIdentity string   `json:"source_identity,omitempty"`
	Root           bool     `json:"root"`
	Permissions    []string `json:"permissions,omitempty"`
}

type contextKey struct{}
type decisionContextKey struct{}

type PolicyDecision struct {
	Allowed       bool      `json:"allowed"`
	Source        string    `json:"source"`
	Reason        string    `json:"reason,omitempty"`
	Action        string    `json:"action,omitempty"`
	Resource      string    `json:"resource,omitempty"`
	StatementSid  string    `json:"statement_sid,omitempty"`
	Principal     Principal `json:"principal"`
	PolicyVersion string    `json:"policy_version,omitempty"`
}

func WithPrincipal(ctx context.Context, principal Principal) context.Context {
	return context.WithValue(ctx, contextKey{}, principal)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(contextKey{}).(Principal)
	return principal, ok
}

func WithPolicyDecision(ctx context.Context, decision PolicyDecision) context.Context {
	return context.WithValue(ctx, decisionContextKey{}, decision)
}

func PolicyDecisionFromContext(ctx context.Context) (PolicyDecision, bool) {
	decision, ok := ctx.Value(decisionContextKey{}).(PolicyDecision)
	return decision, ok
}

func AllowsAction(principal Principal, action string) bool {
	return EvaluatePrincipalAction(principal, action, "").Allowed
}

func EvaluatePrincipalAction(principal Principal, action, resource string) PolicyDecision {
	if principal.Root {
		return PolicyDecision{
			Allowed:       true,
			Source:        "root_principal",
			Reason:        "root principal allows all actions",
			Action:        strings.TrimSpace(action),
			Resource:      strings.TrimSpace(resource),
			Principal:     principal.Clone(),
			PolicyVersion: principal.PolicyVersion,
		}
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return PolicyDecision{
			Allowed:       false,
			Source:        "principal_permissions",
			Reason:        "empty action",
			Resource:      strings.TrimSpace(resource),
			Principal:     principal.Clone(),
			PolicyVersion: principal.PolicyVersion,
		}
	}
	for _, permission := range principal.Permissions {
		if actionMatches(permission, action) {
			return PolicyDecision{
				Allowed:       true,
				Source:        "principal_permissions",
				Reason:        "permission matched",
				Action:        action,
				Resource:      strings.TrimSpace(resource),
				Principal:     principal.Clone(),
				PolicyVersion: principal.PolicyVersion,
			}
		}
	}
	return PolicyDecision{
		Allowed:       false,
		Source:        "principal_permissions",
		Reason:        "no matching permission",
		Action:        action,
		Resource:      strings.TrimSpace(resource),
		Principal:     principal.Clone(),
		PolicyVersion: principal.PolicyVersion,
	}
}

func (p Principal) Clone() Principal {
	out := p
	out.Groups = cloneStrings(p.Groups)
	out.Roles = cloneStrings(p.Roles)
	out.Permissions = cloneStrings(p.Permissions)
	return out
}

func actionMatches(permission, action string) bool {
	permission = strings.TrimSpace(permission)
	if permission == "" {
		return false
	}
	if permission == "*" {
		return true
	}
	if strings.EqualFold(permission, action) {
		return true
	}
	if strings.HasSuffix(permission, "*") {
		prefix := strings.TrimSuffix(permission, "*")
		return strings.HasPrefix(strings.ToLower(action), strings.ToLower(prefix))
	}
	return false
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
