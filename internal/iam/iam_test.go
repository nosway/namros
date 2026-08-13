package iam

import (
	"strings"
	"testing"

	"github.com/nosway/namros/internal/auth"
)

func TestValidateMappingSpec(t *testing.T) {
	report := ValidateMappingSpec(MappingSpec{
		Providers: []ProviderSpec{{
			ProviderID:   "okta",
			Issuer:       "https://issuer.example",
			Audience:     "namros",
			SubjectClaim: "sub",
			GroupClaim:   "groups",
		}},
		Bindings: []BindingSpec{{
			Groups:  []string{"storage-admins"},
			Actions: []string{"s3:GetObject"},
		}},
	})
	if !report.Valid {
		t.Fatalf("ValidateMappingSpec valid = false errors=%v", report.Errors)
	}
	if !report.RequiresEnterprise {
		t.Fatal("mapping report did not mark Enterprise requirement")
	}
}

func TestPrincipalFromClaims(t *testing.T) {
	principal, err := PrincipalFromClaims(MappingSpec{
		Providers: []ProviderSpec{{
			ProviderID:          "oidc",
			Issuer:              "https://issuer.example",
			Audience:            "namros",
			TenantID:            "tenant-a",
			SubjectClaim:        "sub",
			GroupClaim:          "groups",
			RoleClaim:           "roles",
			SessionIDClaim:      "sid",
			SourceIdentityClaim: "email",
			PolicyVersion:       "policy-v1",
		}},
	}, "oidc", ClaimSet{
		"sub":    "user-123",
		"groups": []any{"storage-admins", "auditors"},
		"roles":  "writer,reader",
		"sid":    "session-1",
		"email":  "user@example.com",
	})
	if err != nil {
		t.Fatalf("PrincipalFromClaims() error = %v", err)
	}
	if principal.Subject != "user-123" || principal.TenantID != "tenant-a" || principal.SessionID != "session-1" {
		t.Fatalf("principal = %+v", principal)
	}
	if len(principal.Groups) != 2 || len(principal.Roles) != 2 {
		t.Fatalf("principal groups/roles = %+v", principal)
	}
}

func TestSimulatePolicy(t *testing.T) {
	policy, err := auth.ParsePolicyDocument(strings.NewReader(`{
		"Statement": {
			"Sid": "AllowReaders",
			"Effect": "Allow",
			"Principal": "reader",
			"Action": "s3:GetObject",
			"Resource": "arn:aws:s3:::bucket/*"
		}
	}`))
	if err != nil {
		t.Fatalf("ParsePolicyDocument() error = %v", err)
	}
	result, err := SimulatePolicy(PolicySimulationRequest{
		Principal: auth.Principal{Subject: "reader"},
		Action:    "s3:GetObject",
		Resource:  "arn:aws:s3:::bucket/key",
		Policy:    &policy,
	})
	if err != nil {
		t.Fatalf("SimulatePolicy() error = %v", err)
	}
	if !result.Allowed || result.Decision.StatementSid != "AllowReaders" {
		t.Fatalf("simulation result = %+v", result)
	}
}
