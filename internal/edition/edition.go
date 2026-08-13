package edition

import (
	"fmt"
	"sort"
	"strings"
)

const (
	Community  = "community"
	Enterprise = "enterprise"
)

const (
	FeatureCoreS3API             = "core_s3_api"
	FeatureMultipart             = "multipart"
	FeatureVersioning            = "versioning"
	FeatureTaggingCORS           = "tagging_cors"
	FeatureBasicEncryption       = "basic_encryption"
	FeatureWORMObjectLock        = "worm_object_lock"
	FeatureDedupe                = "dedupe"
	FeatureErasureCoding         = "erasure_coding"
	FeatureSSEKMS                = "sse_kms"
	FeatureComplianceEvidence    = "compliance_evidence"
	FeatureTiKVMetadataCluster   = "tikv_metadata_cluster"
	FeatureActiveActiveGateway   = "active_active_gateway"
	FeatureSBSReplicatedObject   = "sbs_replicated_object"
	FeatureAdvancedOps           = "advanced_ops"
	FeatureDiscoveryExportBundle = "discovery_export_bundle"
	FeatureExternalIAMFederation = "external_iam_federation"
)

func Current() string {
	return current
}

type Feature struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	MinimumEdition string `json:"minimum_edition"`
	Enabled        bool   `json:"enabled"`
	Summary        string `json:"summary"`
}

var catalog = []Feature{
	{ID: FeatureCoreS3API, Name: "Core S3-compatible API", MinimumEdition: Community, Summary: "Bucket/object CRUD, list/head/range GET, copy, delete, and common S3 compatibility behavior."},
	{ID: FeatureMultipart, Name: "Multipart upload", MinimumEdition: Community, Summary: "Initiate/upload/complete/abort/list multipart workflows."},
	{ID: FeatureVersioning, Name: "Bucket versioning", MinimumEdition: Community, Summary: "S3 versioning metadata, delete markers, and version listing."},
	{ID: FeatureTaggingCORS, Name: "Tagging/CORS/presigned compatibility", MinimumEdition: Community, Summary: "Object tagging, user metadata, CORS preflight/config, and presigned URL compatibility."},
	{ID: FeatureBasicEncryption, Name: "Basic encryption baseline", MinimumEdition: Community, Summary: "TLS and basic at-rest encryption posture suitable for community deployments; compliance-grade KMS remains enterprise."},
	{ID: FeatureWORMObjectLock, Name: "WORM/Object Lock", MinimumEdition: Enterprise, Summary: "Object Lock, retention, legal hold, governance bypass, protected refs, and WORM audit evidence."},
	{ID: FeatureDedupe, Name: "Dedupe", MinimumEdition: Enterprise, Summary: "Post-process/ingest-assisted verified dedupe, shared object accounting, and dedupe operations."},
	{ID: FeatureErasureCoding, Name: "Erasure coding", MinimumEdition: Enterprise, Summary: "SBS EC-backed storage classes and EC multipart write/read paths."},
	{ID: FeatureSSEKMS, Name: "SSE-KMS and key lifecycle", MinimumEdition: Enterprise, Summary: "KMS key id/version/state metadata, rotation/revocation evidence, and key-state admission."},
	{ID: FeatureComplianceEvidence, Name: "Compliance evidence", MinimumEdition: Enterprise, Summary: "Evidence packages, retention/access/key/time-source evidence, policy simulation, and compliance profile attachments."},
	{ID: FeatureTiKVMetadataCluster, Name: "Distributed metadata cluster", MinimumEdition: Community, Summary: "TiKV authoritative metadata backend for stateless multi-gateway operation."},
	{ID: FeatureActiveActiveGateway, Name: "Active-active gateway", MinimumEdition: Community, Summary: "Multiple stateless gateways, etcd registry/health, failover, and shared metadata/storage operation."},
	{ID: FeatureSBSReplicatedObject, Name: "SBS replicated object storage", MinimumEdition: Community, Summary: "SBS physical replicated object segment storage without EC storage-class routing."},
	{ID: FeatureAdvancedOps, Name: "Advanced operations", MinimumEdition: Enterprise, Summary: "Metadata backup/restore apply, readiness reports, operations metrics aggregation, and repair/scrub reports."},
	{ID: FeatureDiscoveryExportBundle, Name: "Discovery export bundle", MinimumEdition: Enterprise, Summary: "External legal discovery bundle, chain-of-custody payload manifest, SIEM/webhook export."},
	{ID: FeatureExternalIAMFederation, Name: "External IAM federation", MinimumEdition: Enterprise, Summary: "External IdP/IAM claim mapping, STS-style sessions, bucket/prefix policy simulation, and IAM decision evidence."},
}

func Normalize(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return Community
	}
	return value
}

func Validate(value string) error {
	switch Normalize(value) {
	case Community, Enterprise:
		return nil
	default:
		return fmt.Errorf("unsupported edition %q", value)
	}
}

func Catalog() []Feature {
	out := make([]Feature, len(catalog))
	copy(out, catalog)
	return out
}

func FeaturesFor(value string) []Feature {
	normalized := Normalize(value)
	out := Catalog()
	for i := range out {
		out[i].Enabled = Allows(normalized, out[i].ID)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].ID < out[j].ID
	})
	return out
}

func Allows(value, featureID string) bool {
	normalized := Normalize(value)
	for _, feature := range catalog {
		if feature.ID != featureID {
			continue
		}
		switch feature.MinimumEdition {
		case Community:
			return normalized == Community || normalized == Enterprise
		case Enterprise:
			return normalized == Enterprise
		default:
			return false
		}
	}
	return false
}

func Require(value, featureID string) error {
	if err := Validate(value); err != nil {
		return err
	}
	if Allows(value, featureID) {
		return nil
	}
	return fmt.Errorf("feature %q is supported in NAMROS Enterprise Edition", featureID)
}
