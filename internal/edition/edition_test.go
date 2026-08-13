package edition

import (
	"os"
	"testing"
)

func TestCurrentIsCommunityInPublicSourceTree(t *testing.T) {
	if os.Getenv("NAMROS_ENTERPRISE_OVERLAY_TEST") == "1" && Current() == Enterprise {
		t.Skip("public Community source identity check is not applicable to Enterprise overlay test runs")
	}
	if got := Current(); got != Community {
		t.Fatalf("Current() = %q, want %q", got, Community)
	}
}

func TestFeaturesForCommunityAndEnterprise(t *testing.T) {
	community := FeaturesFor(Community)
	enterprise := FeaturesFor(Enterprise)
	if len(community) == 0 || len(enterprise) != len(community) {
		t.Fatalf("feature catalog sizes community=%d enterprise=%d", len(community), len(enterprise))
	}
	if !Allows(Community, FeatureCoreS3API) || !Allows(Community, FeatureBasicEncryption) || !Allows(Community, FeatureTiKVMetadataCluster) || !Allows(Community, FeatureActiveActiveGateway) || !Allows(Community, FeatureSBSReplicatedObject) {
		t.Fatalf("community baseline feature missing")
	}
	if Allows(Community, FeatureWORMObjectLock) || Allows(Community, FeatureDedupe) || Allows(Community, FeatureErasureCoding) || Allows(Community, FeatureSSEKMS) {
		t.Fatalf("community allowed enterprise feature")
	}
	if !Allows(Enterprise, FeatureWORMObjectLock) || !Allows(Enterprise, FeatureDedupe) || !Allows(Enterprise, FeatureErasureCoding) || !Allows(Enterprise, FeatureSSEKMS) {
		t.Fatalf("enterprise feature not allowed")
	}
}

func TestValidateAndRequire(t *testing.T) {
	if err := Validate(""); err != nil {
		t.Fatalf("Validate(empty) error = %v", err)
	}
	if err := Validate("invalid"); err == nil {
		t.Fatal("Validate(invalid) error = nil, want error")
	}
	if err := Require("invalid", FeatureDedupe); err == nil {
		t.Fatal("Require(invalid,dedupe) error = nil, want unsupported edition")
	}
	if err := Require(Community, FeatureDedupe); err == nil {
		t.Fatal("Require(community,dedupe) error = nil, want enterprise requirement")
	}
	if err := Require(Enterprise, FeatureDedupe); err != nil {
		t.Fatalf("Require(enterprise,dedupe) error = %v", err)
	}
}
