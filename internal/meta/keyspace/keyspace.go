package keyspace

import (
	"encoding/hex"
	"errors"
	"strconv"
	"strings"
)

const prefix = "/namros/v1"

func MetadataSchema() string {
	return prefix + "/system/schema"
}

func Tenant(tenantID string) string {
	return prefix + "/tenants/" + EscapePathSegment(tenantID)
}

func TenantQuota(tenantID string) string {
	return prefix + "/tenants/" + EscapePathSegment(tenantID) + "/quota"
}

func TenantUsage(tenantID string) string {
	return prefix + "/tenants/" + EscapePathSegment(tenantID) + "/usage"
}

func AccessKey(accessKeyID string) string {
	return prefix + "/access-keys/" + EscapePathSegment(accessKeyID)
}

func KMSKey(keyID string) string {
	return prefix + "/kms/keys/" + EscapePathSegment(keyID)
}

func ComplianceProfileAttachment(profileID string) string {
	return prefix + "/compliance/profile-attachments/" + EscapePathSegment(profileID)
}

func BucketByName(name string) string {
	return prefix + "/buckets/by-name/" + EscapePathSegment(name)
}

func BucketByID(bucketID string) string {
	return prefix + "/buckets/by-id/" + EscapePathSegment(bucketID)
}

func BucketQuota(bucketID string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/quota"
}

func ObjectHead(bucketID, key string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/objects/" + EscapeObjectKey(key) + "/head"
}

func ObjectVersion(bucketID, key, versionSortKey string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/versions/" + EscapeObjectKey(key) + "/" + EscapePathSegment(versionSortKey)
}

func ObjectVersionByID(bucketID, versionID string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/versions-by-id/" + EscapePathSegment(versionID)
}

func ObjectManifestChunk(bucketID, versionID string, chunkNumber int) string {
	base := prefix + "/buckets/" + EscapePathSegment(bucketID) + "/object-manifests/" + EscapePathSegment(versionID) + "/chunks/"
	if chunkNumber <= 0 {
		return base
	}
	return base + leftPadInt(chunkNumber, 6)
}

func ListObject(bucketID, key string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/list/" + EscapeObjectKey(key)
}

func MultipartUpload(bucketID, uploadID string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/multipart/" + EscapePathSegment(uploadID) + "/state"
}

func MultipartCompletion(bucketID, uploadID string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/multipart/" + EscapePathSegment(uploadID) + "/completion"
}

func MultipartUploadByKey(bucketID, key, uploadID string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/multipart-by-key/" + EscapeObjectKey(key) + "/" + EscapePathSegment(uploadID)
}

func MultipartPart(bucketID, uploadID string, partNumber int) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/multipart/" + EscapePathSegment(uploadID) + "/parts/" + leftPadInt(partNumber, 5)
}

func Idempotency(scope, idempotencyKey string) string {
	return prefix + "/idempotency/" + EscapePathSegment(scope) + "/" + EscapePathSegment(idempotencyKey)
}

func GCOrphan(timeBucket, objectID string) string {
	return prefix + "/gc/orphans/" + EscapePathSegment(timeBucket) + "/" + EscapePathSegment(objectID)
}

func GCCandidate(segmentID string) string {
	base := prefix + "/gc/candidates/"
	if segmentID == "" {
		return base
	}
	return base + EscapePathSegment(segmentID)
}

func GCOperation(operationID string) string {
	return prefix + "/gc/operations/" + EscapePathSegment(operationID)
}

func DedupeOperation(operationID string) string {
	return prefix + "/dedupe/operations/" + EscapePathSegment(operationID)
}

func DedupeOperationLock(lockID string) string {
	return prefix + "/dedupe/operation-locks/" + EscapePathSegment(lockID)
}

func SharedObjectRelease(sharedObjectID, segmentID string) string {
	return SharedObjectReleasePrefix(sharedObjectID) + EscapePathSegment(segmentID)
}

func SharedObjectReleasePrefix(sharedObjectID string) string {
	base := prefix + "/gc/shared-object-releases/"
	if sharedObjectID == "" {
		return base
	}
	return base + EscapePathSegment(sharedObjectID) + "/"
}

func SharedObject(sharedObjectID string) string {
	return SharedObjectPrefix() + EscapePathSegment(sharedObjectID)
}

func SharedObjectPrefix() string {
	return prefix + "/dedupe/shared-objects/"
}

func SharedObjectRef(sharedObjectID, bucketID, key, versionID string) string {
	return SharedObjectRefPrefix(sharedObjectID) + EscapePathSegment(bucketID) + "/" + EscapeObjectKey(key) + "/" + EscapePathSegment(versionID)
}

func SharedObjectRefPrefix(sharedObjectID string) string {
	base := prefix + "/dedupe/shared-object-refs/"
	if sharedObjectID == "" {
		return base
	}
	return base + EscapePathSegment(sharedObjectID) + "/"
}

func VolumePool(poolID string) string {
	base := prefix + "/storage/volume-pools/"
	if poolID == "" {
		return base
	}
	return base + EscapePathSegment(poolID)
}

func VolumeDrainOperation(operationID string) string {
	base := prefix + "/storage/volume-drain/operations/"
	if operationID == "" {
		return base
	}
	return base + EscapePathSegment(operationID)
}

func MetadataMigrationOperation(operationID string) string {
	base := prefix + "/metadata/migrations/operations/"
	if operationID == "" {
		return base
	}
	return base + EscapePathSegment(operationID)
}

func WorkerLease(workerKind, shardID string) string {
	base := prefix + "/workers/leases/"
	if workerKind == "" {
		return base
	}
	kind := base + EscapePathSegment(workerKind) + "/"
	if shardID == "" {
		return kind
	}
	return kind + EscapePathSegment(shardID)
}

func WorkerOperation(operationID string) string {
	return prefix + "/workers/operations/" + EscapePathSegment(operationID)
}

func WorkerControl(workerKind, shardID string) string {
	base := prefix + "/workers/control/"
	if workerKind == "" {
		return base
	}
	kind := base + EscapePathSegment(workerKind) + "/"
	if shardID == "" {
		return kind
	}
	return kind + EscapePathSegment(shardID)
}

func AuditEvent(eventID string) string {
	return prefix + "/audit/events/" + EscapePathSegment(eventID)
}

func AuditHead() string {
	return prefix + "/audit/head"
}

func ProtectedRefByVersion(bucketID, key, versionID, refID string) string {
	return prefix + "/buckets/" + EscapePathSegment(bucketID) + "/protected-refs/by-version/" + EscapeObjectKey(key) + "/" + EscapePathSegment(versionID) + "/" + EscapePathSegment(refID)
}

func ProtectedRefBySegment(segmentID, refID string) string {
	return prefix + "/protected-refs/by-segment/" + EscapePathSegment(segmentID) + "/" + EscapePathSegment(refID)
}

func EscapeObjectKey(key string) string {
	return escape(key)
}

func UnescapeObjectKey(encoded string) (string, error) {
	return unescape(encoded)
}

func EscapePathSegment(segment string) string {
	return escape(segment)
}

func escape(value string) string {
	var b strings.Builder
	for i := 0; i < len(value); i++ {
		c := value[i]
		if isSafe(c) {
			b.WriteByte(c)
			continue
		}
		b.WriteByte('%')
		b.WriteString(strings.ToUpper(hex.EncodeToString([]byte{c})))
	}
	return b.String()
}

func unescape(value string) (string, error) {
	out := make([]byte, 0, len(value))
	for i := 0; i < len(value); i++ {
		if value[i] != '%' {
			out = append(out, value[i])
			continue
		}
		if i+2 >= len(value) {
			return "", errors.New("truncated escape")
		}
		decoded, err := hex.DecodeString(value[i+1 : i+3])
		if err != nil {
			return "", err
		}
		out = append(out, decoded[0])
		i += 2
	}
	return string(out), nil
}

func isSafe(c byte) bool {
	return c >= 'A' && c <= 'Z' ||
		c >= 'a' && c <= 'z' ||
		c >= '0' && c <= '9' ||
		c == '-' || c == '_' || c == '.' || c == '~'
}

func leftPadInt(value, width int) string {
	text := strconv.Itoa(value)
	if len(text) >= width {
		return text
	}
	return strings.Repeat("0", width-len(text)) + text
}
