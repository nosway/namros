package auth

import (
	"strings"
	"testing"
)

func TestPolicyDocumentAllows(t *testing.T) {
	policy, err := ParsePolicyDocument(strings.NewReader(`{
		"Version": "2012-10-17",
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": {"AWS": "ak-bypass"},
				"Action": "s3:BypassGovernanceRetention",
				"Resource": "arn:aws:s3:::locked-bucket/*"
			}
		]
	}`))
	if err != nil {
		t.Fatalf("ParsePolicyDocument() error = %v", err)
	}
	if !policy.Allows(Principal{AccessKeyID: "ak-bypass"}, ActionBypassGovernanceRetention, "arn:aws:s3:::locked-bucket/object.txt") {
		t.Fatal("policy did not allow matching principal/action/resource")
	}
	if policy.Allows(Principal{AccessKeyID: "ak-other"}, ActionBypassGovernanceRetention, "arn:aws:s3:::locked-bucket/object.txt") {
		t.Fatal("policy allowed non-matching principal")
	}
	if policy.Allows(Principal{AccessKeyID: "ak-bypass"}, "s3:DeleteObject", "arn:aws:s3:::locked-bucket/object.txt") {
		t.Fatal("policy allowed non-matching action")
	}
	if policy.Allows(Principal{AccessKeyID: "ak-bypass"}, ActionBypassGovernanceRetention, "arn:aws:s3:::other/object.txt") {
		t.Fatal("policy allowed non-matching resource")
	}
}

func TestPolicyDocumentDenyOverridesAllow(t *testing.T) {
	policy, err := ParsePolicyDocument(strings.NewReader(`{
		"Statement": [
			{
				"Effect": "Allow",
				"Principal": "*",
				"Action": "s3:*",
				"Resource": "arn:aws:s3:::locked-bucket/*"
			},
			{
				"Effect": "Deny",
				"Principal": {"AWS": "ak-bypass"},
				"Action": "s3:BypassGovernanceRetention",
				"Resource": "arn:aws:s3:::locked-bucket/private/*"
			}
		]
	}`))
	if err != nil {
		t.Fatalf("ParsePolicyDocument() error = %v", err)
	}
	if !policy.Allows(Principal{AccessKeyID: "ak-bypass"}, ActionBypassGovernanceRetention, "arn:aws:s3:::locked-bucket/public/object.txt") {
		t.Fatal("policy did not allow non-denied matching resource")
	}
	if policy.Allows(Principal{AccessKeyID: "ak-bypass"}, ActionBypassGovernanceRetention, "arn:aws:s3:::locked-bucket/private/object.txt") {
		t.Fatal("policy allowed explicitly denied resource")
	}
}

func TestPolicyDocumentAcceptsSingleStatementObject(t *testing.T) {
	policy, err := ParsePolicyDocument(strings.NewReader(`{
		"Statement": {
			"Effect": "Allow",
			"Principal": "*",
			"Action": ["s3:GetObject", "s3:BypassGovernanceRetention"],
			"Resource": ["arn:aws:s3:::locked-bucket/*"]
		}
	}`))
	if err != nil {
		t.Fatalf("ParsePolicyDocument() error = %v", err)
	}
	if len(policy.Statements) != 1 {
		t.Fatalf("statements = %d, want 1", len(policy.Statements))
	}
	if !policy.Allows(Principal{AccessKeyID: "any"}, ActionBypassGovernanceRetention, "arn:aws:s3:::locked-bucket/key") {
		t.Fatal("policy did not allow wildcard principal")
	}
}

func TestPolicyDocumentMatchesExternalPrincipalFields(t *testing.T) {
	policy, err := ParsePolicyDocument(strings.NewReader(`{
		"Statement": [
			{
				"Sid": "AllowSubject",
				"Effect": "Allow",
				"Principal": "subject-1",
				"Action": "s3:GetObject",
				"Resource": "arn:aws:s3:::bucket/*"
			},
			{
				"Sid": "AllowGroup",
				"Effect": "Allow",
				"Principal": "storage-admins",
				"Action": "s3:PutObject",
				"Resource": "arn:aws:s3:::bucket/*"
			}
		]
	}`))
	if err != nil {
		t.Fatalf("ParsePolicyDocument() error = %v", err)
	}
	principal := Principal{
		Subject: "subject-1",
		Groups:  []string{"storage-admins"},
	}
	if decision := policy.Decide(principal, "s3:GetObject", "arn:aws:s3:::bucket/key"); !decision.Allowed || decision.StatementSid != "AllowSubject" {
		t.Fatalf("subject decision = %+v", decision)
	}
	if decision := policy.Decide(principal, "s3:PutObject", "arn:aws:s3:::bucket/key"); !decision.Allowed || decision.StatementSid != "AllowGroup" {
		t.Fatalf("group decision = %+v", decision)
	}
}
