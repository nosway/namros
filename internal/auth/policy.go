package auth

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
)

type PolicyDocument struct {
	Version    string            `json:"Version,omitempty"`
	Statements []PolicyStatement `json:"Statement"`
}

type PolicyStatement struct {
	Sid        string   `json:"Sid,omitempty"`
	Effect     string   `json:"Effect"`
	Principals []string `json:"Principal"`
	Actions    []string `json:"Action"`
	Resources  []string `json:"Resource"`
}

func ParsePolicyDocument(r io.Reader) (PolicyDocument, error) {
	var raw rawPolicyDocument
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&raw); err != nil {
		return PolicyDocument{}, err
	}
	policy, err := raw.normalize()
	if err != nil {
		return PolicyDocument{}, err
	}
	if len(policy.Statements) == 0 {
		return PolicyDocument{}, errors.New("policy must include at least one statement")
	}
	return policy, nil
}

func (p PolicyDocument) Allows(principal Principal, action, resource string) bool {
	return p.Decide(principal, action, resource).Allowed
}

func (p PolicyDocument) Decide(principal Principal, action, resource string) PolicyDecision {
	allowed := false
	var allowSid string
	for _, statement := range p.Statements {
		if !statement.matches(principal, action, resource) {
			continue
		}
		if strings.EqualFold(statement.Effect, "Deny") {
			return PolicyDecision{
				Allowed:       false,
				Source:        "policy_document",
				Reason:        "explicit deny",
				Action:        strings.TrimSpace(action),
				Resource:      strings.TrimSpace(resource),
				StatementSid:  statement.Sid,
				Principal:     principal.Clone(),
				PolicyVersion: principal.PolicyVersion,
			}
		}
		if strings.EqualFold(statement.Effect, "Allow") {
			allowed = true
			allowSid = statement.Sid
		}
	}
	if allowed {
		return PolicyDecision{
			Allowed:       true,
			Source:        "policy_document",
			Reason:        "allow matched",
			Action:        strings.TrimSpace(action),
			Resource:      strings.TrimSpace(resource),
			StatementSid:  allowSid,
			Principal:     principal.Clone(),
			PolicyVersion: principal.PolicyVersion,
		}
	}
	return PolicyDecision{
		Allowed:       false,
		Source:        "policy_document",
		Reason:        "no matching allow",
		Action:        strings.TrimSpace(action),
		Resource:      strings.TrimSpace(resource),
		Principal:     principal.Clone(),
		PolicyVersion: principal.PolicyVersion,
	}
}

func (s PolicyStatement) matches(principal Principal, action, resource string) bool {
	if !principalMatches(s.Principals, principal) {
		return false
	}
	if !matchesAny(s.Actions, action) {
		return false
	}
	return matchesAny(s.Resources, resource)
}

func principalMatches(principals []string, principal Principal) bool {
	candidates := principalMatchCandidates(principal)
	for _, candidate := range principals {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" {
			return true
		}
		for _, value := range candidates {
			if candidate != "" && candidate == value {
				return true
			}
		}
	}
	return false
}

func principalMatchCandidates(principal Principal) []string {
	values := []string{
		principal.AccessKeyID,
		principal.TenantID,
		principal.DisplayName,
		principal.Subject,
		principal.SessionID,
		principal.ExternalIssuer,
		principal.SourceIdentity,
	}
	values = append(values, principal.Groups...)
	values = append(values, principal.Roles...)
	return compactStrings(values)
}

func matchesAny(patterns []string, value string) bool {
	for _, pattern := range patterns {
		if actionMatches(pattern, value) {
			return true
		}
	}
	return false
}

type rawPolicyDocument struct {
	Version   string          `json:"Version"`
	Statement json.RawMessage `json:"Statement"`
}

type rawPolicyStatement struct {
	Sid       string          `json:"Sid"`
	Effect    string          `json:"Effect"`
	Principal json.RawMessage `json:"Principal"`
	Action    json.RawMessage `json:"Action"`
	Resource  json.RawMessage `json:"Resource"`
}

func (r rawPolicyDocument) normalize() (PolicyDocument, error) {
	rawStatements, err := parseRawStatements(r.Statement)
	if err != nil {
		return PolicyDocument{}, err
	}
	statements := make([]PolicyStatement, 0, len(rawStatements))
	for _, raw := range rawStatements {
		statement, err := raw.normalize()
		if err != nil {
			return PolicyDocument{}, err
		}
		statements = append(statements, statement)
	}
	return PolicyDocument{
		Version:    r.Version,
		Statements: statements,
	}, nil
}

func parseRawStatements(raw json.RawMessage) ([]rawPolicyStatement, error) {
	if len(raw) == 0 {
		return nil, errors.New("policy statement is required")
	}
	var statements []rawPolicyStatement
	if err := json.Unmarshal(raw, &statements); err == nil {
		return statements, nil
	}
	var statement rawPolicyStatement
	if err := json.Unmarshal(raw, &statement); err != nil {
		return nil, err
	}
	return []rawPolicyStatement{statement}, nil
}

func (r rawPolicyStatement) normalize() (PolicyStatement, error) {
	effect := strings.TrimSpace(r.Effect)
	if !strings.EqualFold(effect, "Allow") && !strings.EqualFold(effect, "Deny") {
		return PolicyStatement{}, fmt.Errorf("policy statement effect must be Allow or Deny")
	}
	principals, err := parsePrincipalValues(r.Principal)
	if err != nil {
		return PolicyStatement{}, err
	}
	if len(principals) == 0 {
		return PolicyStatement{}, errors.New("policy statement principal is required")
	}
	actions, err := parseStringOrStringSlice(r.Action, "action")
	if err != nil {
		return PolicyStatement{}, err
	}
	resources, err := parseStringOrStringSlice(r.Resource, "resource")
	if err != nil {
		return PolicyStatement{}, err
	}
	return PolicyStatement{
		Sid:        r.Sid,
		Effect:     canonicalEffect(effect),
		Principals: principals,
		Actions:    actions,
		Resources:  resources,
	}, nil
}

func canonicalEffect(effect string) string {
	if strings.EqualFold(effect, "Deny") {
		return "Deny"
	}
	return "Allow"
}

func parsePrincipalValues(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, errors.New("policy statement principal is required")
	}
	values, err := parseStringOrStringSlice(raw, "principal")
	if err == nil {
		return values, nil
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(raw, &object); err != nil {
		return nil, err
	}
	var out []string
	for _, value := range object {
		values, err := parseStringOrStringSlice(value, "principal")
		if err != nil {
			return nil, err
		}
		out = append(out, values...)
	}
	return compactStrings(out), nil
}

func parseStringOrStringSlice(raw json.RawMessage, field string) ([]string, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("policy statement %s is required", field)
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		return compactStrings([]string{single}), nil
	}
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		return nil, err
	}
	values := compactStrings(many)
	if len(values) == 0 {
		return nil, fmt.Errorf("policy statement %s is required", field)
	}
	return values, nil
}

func compactStrings(in []string) []string {
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}
