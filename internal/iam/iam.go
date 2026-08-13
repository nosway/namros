package iam

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/edition"
)

type MappingSpec struct {
	SchemaVersion string         `json:"schema_version"`
	Providers     []ProviderSpec `json:"providers"`
	Bindings      []BindingSpec  `json:"bindings,omitempty"`
}

type ProviderSpec struct {
	ProviderID          string `json:"provider_id"`
	Issuer              string `json:"issuer"`
	Audience            string `json:"audience"`
	TenantID            string `json:"tenant_id,omitempty"`
	SubjectClaim        string `json:"subject_claim"`
	GroupClaim          string `json:"group_claim,omitempty"`
	RoleClaim           string `json:"role_claim,omitempty"`
	SessionIDClaim      string `json:"session_id_claim,omitempty"`
	SourceIdentityClaim string `json:"source_identity_claim,omitempty"`
	PolicyVersion       string `json:"policy_version,omitempty"`
}

type BindingSpec struct {
	BindingID string   `json:"binding_id,omitempty"`
	TenantID  string   `json:"tenant_id,omitempty"`
	Bucket    string   `json:"bucket,omitempty"`
	Prefix    string   `json:"prefix,omitempty"`
	Subjects  []string `json:"subjects,omitempty"`
	Groups    []string `json:"groups,omitempty"`
	Roles     []string `json:"roles,omitempty"`
	Actions   []string `json:"actions,omitempty"`
}

type MappingValidationReport struct {
	SchemaVersion      string   `json:"schema_version"`
	Valid              bool     `json:"valid"`
	RequiresEnterprise bool     `json:"requires_enterprise"`
	MinimumEdition     string   `json:"minimum_edition"`
	Providers          int      `json:"providers"`
	Bindings           int      `json:"bindings"`
	Errors             []string `json:"errors,omitempty"`
	Warnings           []string `json:"warnings,omitempty"`
}

type ClaimSet map[string]any

type PrincipalInspectOutput struct {
	SchemaVersion      string         `json:"schema_version"`
	GeneratedAt        string         `json:"generated_at"`
	Principal          auth.Principal `json:"principal"`
	Session            SessionSummary `json:"session"`
	RequiresEnterprise bool           `json:"requires_enterprise"`
	MinimumEdition     string         `json:"minimum_edition,omitempty"`
}

type SessionSummary struct {
	SessionID      string   `json:"session_id,omitempty"`
	Subject        string   `json:"subject,omitempty"`
	Groups         []string `json:"groups,omitempty"`
	Roles          []string `json:"roles,omitempty"`
	ExternalIssuer string   `json:"external_issuer,omitempty"`
	PolicyVersion  string   `json:"policy_version,omitempty"`
	SourceIdentity string   `json:"source_identity,omitempty"`
	SessionType    string   `json:"session_type"`
}

type TemporaryCredentialEnvelope struct {
	SchemaVersion      string              `json:"schema_version"`
	AccessKeyID        string              `json:"access_key_id,omitempty"`
	Expiration         string              `json:"expiration,omitempty"`
	SessionPolicy      auth.PolicyDocument `json:"session_policy,omitempty"`
	SourceIdentity     string              `json:"source_identity,omitempty"`
	AuditCorrelationID string              `json:"audit_correlation_id,omitempty"`
	Principal          auth.Principal      `json:"principal"`
	MinimumEdition     string              `json:"minimum_edition"`
}

type PolicySimulationRequest struct {
	Principal auth.Principal
	Action    string
	Resource  string
	Policy    *auth.PolicyDocument
}

type PolicySimulationResult struct {
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   string              `json:"generated_at"`
	Action        string              `json:"action"`
	Resource      string              `json:"resource"`
	Allowed       bool                `json:"allowed"`
	Decision      auth.PolicyDecision `json:"decision"`
	Engine        string              `json:"engine"`
}

func ParseMappingSpec(r io.Reader) (MappingSpec, error) {
	var spec MappingSpec
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&spec); err != nil {
		return MappingSpec{}, err
	}
	return spec, nil
}

func ValidateMappingSpec(spec MappingSpec) MappingValidationReport {
	report := MappingValidationReport{
		SchemaVersion:      "namros.iam.mapping_validation.v1",
		RequiresEnterprise: true,
		MinimumEdition:     edition.Enterprise,
		Providers:          len(spec.Providers),
		Bindings:           len(spec.Bindings),
	}
	seenProviders := map[string]bool{}
	for i, provider := range spec.Providers {
		prefix := fmt.Sprintf("providers[%d]", i)
		if strings.TrimSpace(provider.ProviderID) == "" {
			report.Errors = append(report.Errors, prefix+".provider_id is required")
		}
		if seenProviders[provider.ProviderID] {
			report.Errors = append(report.Errors, prefix+".provider_id is duplicated")
		}
		seenProviders[provider.ProviderID] = true
		if strings.TrimSpace(provider.Issuer) == "" {
			report.Errors = append(report.Errors, prefix+".issuer is required")
		}
		if strings.TrimSpace(provider.Audience) == "" {
			report.Errors = append(report.Errors, prefix+".audience is required")
		}
		if strings.TrimSpace(provider.SubjectClaim) == "" {
			report.Errors = append(report.Errors, prefix+".subject_claim is required")
		}
		if strings.TrimSpace(provider.GroupClaim) == "" && strings.TrimSpace(provider.RoleClaim) == "" {
			report.Warnings = append(report.Warnings, prefix+" has no group_claim or role_claim; mapping will only identify subjects")
		}
	}
	if len(spec.Providers) == 0 {
		report.Errors = append(report.Errors, "at least one provider is required")
	}
	for i, binding := range spec.Bindings {
		prefix := fmt.Sprintf("bindings[%d]", i)
		if len(compact(binding.Subjects)) == 0 && len(compact(binding.Groups)) == 0 && len(compact(binding.Roles)) == 0 {
			report.Errors = append(report.Errors, prefix+" must include at least one subject, group, or role")
		}
		if len(compact(binding.Actions)) == 0 {
			report.Errors = append(report.Errors, prefix+".actions is required")
		}
	}
	report.Valid = len(report.Errors) == 0
	return report
}

func PrincipalFromClaims(spec MappingSpec, providerID string, claims ClaimSet) (auth.Principal, error) {
	provider, ok := findProvider(spec, providerID)
	if !ok {
		return auth.Principal{}, fmt.Errorf("provider %q not found", providerID)
	}
	subject := claimString(claims, provider.SubjectClaim)
	if subject == "" {
		return auth.Principal{}, fmt.Errorf("subject claim %q not found", provider.SubjectClaim)
	}
	tenantID := strings.TrimSpace(provider.TenantID)
	if tenantID == "" {
		tenantID = "external"
	}
	return auth.Principal{
		TenantID:       tenantID,
		DisplayName:    subject,
		Subject:        subject,
		Groups:         claimStringSlice(claims, provider.GroupClaim),
		Roles:          claimStringSlice(claims, provider.RoleClaim),
		SessionID:      claimString(claims, provider.SessionIDClaim),
		ExternalIssuer: provider.Issuer,
		PolicyVersion:  provider.PolicyVersion,
		SourceIdentity: claimString(claims, provider.SourceIdentityClaim),
	}, nil
}

func InspectPrincipal(principal auth.Principal) PrincipalInspectOutput {
	sessionType := "access_key"
	requiresEnterprise := false
	if principal.ExternalIssuer != "" || principal.Subject != "" || len(principal.Groups) > 0 || len(principal.Roles) > 0 {
		sessionType = "external_iam"
		requiresEnterprise = true
	}
	return PrincipalInspectOutput{
		SchemaVersion: "namros.iam.principal_inspect.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Principal:     principal.Clone(),
		Session: SessionSummary{
			SessionID:      principal.SessionID,
			Subject:        principal.Subject,
			Groups:         cloneStrings(principal.Groups),
			Roles:          cloneStrings(principal.Roles),
			ExternalIssuer: principal.ExternalIssuer,
			PolicyVersion:  principal.PolicyVersion,
			SourceIdentity: principal.SourceIdentity,
			SessionType:    sessionType,
		},
		RequiresEnterprise: requiresEnterprise,
		MinimumEdition:     edition.Enterprise,
	}
}

func BuildTemporaryCredentialEnvelope(principal auth.Principal, expiresAt time.Time, sessionPolicy auth.PolicyDocument, auditCorrelationID string) TemporaryCredentialEnvelope {
	expiration := ""
	if !expiresAt.IsZero() {
		expiration = expiresAt.UTC().Format(time.RFC3339Nano)
	}
	return TemporaryCredentialEnvelope{
		SchemaVersion:      "namros.iam.temporary_credential.v1",
		AccessKeyID:        principal.AccessKeyID,
		Expiration:         expiration,
		SessionPolicy:      sessionPolicy,
		SourceIdentity:     principal.SourceIdentity,
		AuditCorrelationID: auditCorrelationID,
		Principal:          principal.Clone(),
		MinimumEdition:     edition.Enterprise,
	}
}

func SimulatePolicy(req PolicySimulationRequest) (PolicySimulationResult, error) {
	action := strings.TrimSpace(req.Action)
	if action == "" {
		return PolicySimulationResult{}, errors.New("action is required")
	}
	resource := strings.TrimSpace(req.Resource)
	if resource == "" {
		return PolicySimulationResult{}, errors.New("resource is required")
	}
	decision := auth.EvaluatePrincipalAction(req.Principal, action, resource)
	engine := "principal_permissions"
	if req.Policy != nil {
		decision = req.Policy.Decide(req.Principal, action, resource)
		engine = "policy_document"
	}
	return PolicySimulationResult{
		SchemaVersion: "namros.iam.policy_simulation.v1",
		GeneratedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Action:        action,
		Resource:      resource,
		Allowed:       decision.Allowed,
		Decision:      decision,
		Engine:        engine,
	}, nil
}

func findProvider(spec MappingSpec, providerID string) (ProviderSpec, bool) {
	for _, provider := range spec.Providers {
		if provider.ProviderID == providerID {
			return provider, true
		}
	}
	return ProviderSpec{}, false
}

func claimString(claims ClaimSet, name string) string {
	if strings.TrimSpace(name) == "" {
		return ""
	}
	value, ok := claims[name]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func claimStringSlice(claims ClaimSet, name string) []string {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	value, ok := claims[name]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return compact(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, fmt.Sprint(item))
		}
		return compact(out)
	case string:
		return compact(strings.Split(typed, ","))
	default:
		return compact([]string{fmt.Sprint(typed)})
	}
}

func compact(in []string) []string {
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func cloneStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
