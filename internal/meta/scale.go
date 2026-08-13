package meta

import (
	"encoding/json"
	"fmt"

	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

const (
	MaxMultipartParts            = 10000
	MaxObjectManifestSegmentRefs = MaxMultipartParts
	MaxObjectManifestValueBytes  = DefaultMetadataValueBudgetBytes
	ObjectManifestChunkRefTarget = 1024
	DefaultMultipartCleanupLimit = 256
	MaxMultipartCleanupLimit     = 1000
)

type ObjectVersionManifestPlan struct {
	Inline        bool
	ValueBytes    int
	StoredVersion model.ObjectVersion
	StoredValue   []byte
	Chunks        []model.ObjectManifestChunk
}

func ValidateObjectManifestScale(segmentRefs []storage.SegmentRef) error {
	if len(segmentRefs) > MaxObjectManifestSegmentRefs {
		return fmt.Errorf("%w: object manifest has %d segment refs, max %d", ErrInvalidArgument, len(segmentRefs), MaxObjectManifestSegmentRefs)
	}
	return nil
}

func ValidateObjectVersionScale(version model.ObjectVersion) error {
	_, err := PlanObjectVersionManifestStorage(version)
	return err
}

func PlanObjectVersionManifestStorage(version model.ObjectVersion) (ObjectVersionManifestPlan, error) {
	if err := ValidateObjectManifestScale(objectVersionSegmentRefs(version)); err != nil {
		return ObjectVersionManifestPlan{}, err
	}
	inline := cloneObjectVersionForManifestPlan(version)
	inline.Manifest = model.ObjectManifestDescriptor{}
	valueBytes, encoded, err := objectVersionEncodedValue(inline)
	if err != nil {
		return ObjectVersionManifestPlan{}, err
	}
	if valueBytes <= MaxObjectManifestValueBytes {
		return ObjectVersionManifestPlan{
			Inline:        true,
			ValueBytes:    valueBytes,
			StoredVersion: inline,
			StoredValue:   encoded,
		}, nil
	}
	refs := objectVersionSegmentRefs(version)
	if len(refs) == 0 {
		return ObjectVersionManifestPlan{}, manifestTooLargeError(valueBytes)
	}
	chunks, err := buildObjectManifestChunks(version, refs)
	if err != nil {
		return ObjectVersionManifestPlan{}, err
	}
	chunked := cloneObjectVersionForManifestPlan(version)
	chunked.SegmentRef = storage.SegmentRef{}
	chunked.SegmentRefs = nil
	chunked.Manifest = model.ObjectManifestDescriptor{
		Encoding:   model.ObjectManifestEncodingChunked,
		RefCount:   len(refs),
		ChunkCount: len(chunks),
	}
	valueBytes, encoded, err = objectVersionEncodedValue(chunked)
	if err != nil {
		return ObjectVersionManifestPlan{}, err
	}
	if err := ValidateObjectVersionValueSize(valueBytes); err != nil {
		return ObjectVersionManifestPlan{}, err
	}
	return ObjectVersionManifestPlan{
		Inline:        false,
		ValueBytes:    valueBytes,
		StoredVersion: chunked,
		StoredValue:   encoded,
		Chunks:        chunks,
	}, nil
}

func ObjectVersionValueBytes(version model.ObjectVersion) (int, error) {
	valueBytes, _, err := objectVersionEncodedValue(version)
	return valueBytes, err
}

func objectVersionEncodedValue(version model.ObjectVersion) (int, []byte, error) {
	encoded, err := json.Marshal(version)
	if err != nil {
		return 0, nil, fmt.Errorf("%w: object version metadata cannot be encoded: %v", ErrInvalidArgument, err)
	}
	return len(encoded), encoded, nil
}

func ValidateObjectVersionValueSize(valueBytes int) error {
	if valueBytes > MaxObjectManifestValueBytes {
		return manifestTooLargeError(valueBytes)
	}
	return nil
}

func ValidateCompletedMultipartPartCount(partCount int) error {
	if partCount < 1 || partCount > MaxMultipartParts {
		return fmt.Errorf("%w: completed multipart upload has %d parts, max %d", ErrInvalidArgument, partCount, MaxMultipartParts)
	}
	return nil
}

func ValidateMultipartPartNumberSelection(partNumbers []int) error {
	if len(partNumbers) < 1 || len(partNumbers) > MaxMultipartParts {
		return fmt.Errorf("%w: multipart part selection has %d parts, max %d", ErrInvalidArgument, len(partNumbers), MaxMultipartParts)
	}
	seen := make(map[int]struct{}, len(partNumbers))
	for _, partNumber := range partNumbers {
		if partNumber < 1 || partNumber > MaxMultipartParts {
			return fmt.Errorf("%w: multipart part number must be between 1 and %d", ErrInvalidArgument, MaxMultipartParts)
		}
		if _, ok := seen[partNumber]; ok {
			return fmt.Errorf("%w: multipart part number %d is duplicated", ErrInvalidArgument, partNumber)
		}
		seen[partNumber] = struct{}{}
	}
	return nil
}

func NormalizeMultipartCleanupLimit(limit int) int {
	if limit <= 0 {
		return DefaultMultipartCleanupLimit
	}
	if limit > MaxMultipartCleanupLimit {
		return MaxMultipartCleanupLimit
	}
	return limit
}

func objectVersionSegmentRefs(version model.ObjectVersion) []storage.SegmentRef {
	if len(version.SegmentRefs) > 0 {
		return storage.CloneSegmentRefs(version.SegmentRefs)
	}
	if version.SegmentRef.SegmentID == "" {
		return nil
	}
	return []storage.SegmentRef{storage.CloneSegmentRef(version.SegmentRef)}
}

func buildObjectManifestChunks(version model.ObjectVersion, refs []storage.SegmentRef) ([]model.ObjectManifestChunk, error) {
	chunks := make([]model.ObjectManifestChunk, 0, (len(refs)+ObjectManifestChunkRefTarget-1)/ObjectManifestChunkRefTarget)
	for start := 0; start < len(refs); {
		chunkSize := min(ObjectManifestChunkRefTarget, len(refs)-start)
		for {
			chunk := model.ObjectManifestChunk{
				BucketID:    version.BucketID,
				Key:         version.Key,
				VersionID:   version.VersionID,
				ChunkNumber: len(chunks) + 1,
				SegmentRefs: storage.CloneSegmentRefs(refs[start : start+chunkSize]),
				CreatedAt:   version.CreatedAt,
			}
			valueBytes, err := objectManifestChunkValueBytes(chunk)
			if err != nil {
				return nil, err
			}
			if valueBytes <= MaxObjectManifestValueBytes {
				chunks = append(chunks, chunk)
				start += chunkSize
				break
			}
			if chunkSize == 1 {
				return nil, manifestTooLargeError(valueBytes)
			}
			chunkSize = max(1, chunkSize/2)
		}
	}
	return chunks, nil
}

func objectManifestChunkValueBytes(chunk model.ObjectManifestChunk) (int, error) {
	encoded, err := json.Marshal(chunk)
	if err != nil {
		return 0, fmt.Errorf("%w: object manifest chunk cannot be encoded: %v", ErrInvalidArgument, err)
	}
	return len(encoded), nil
}

func manifestTooLargeError(valueBytes int) error {
	return fmt.Errorf("%w: %w: object version metadata value is %d bytes, max %d", ErrInvalidArgument, ErrObjectManifestTooLarge, valueBytes, MaxObjectManifestValueBytes)
}

func cloneObjectVersionForManifestPlan(in model.ObjectVersion) model.ObjectVersion {
	out := in
	out.StorageClass = storage.CloneStorageClassSnapshot(in.StorageClass)
	out.SegmentRef = storage.CloneSegmentRef(in.SegmentRef)
	out.SegmentRefs = storage.CloneSegmentRefs(in.SegmentRefs)
	out.UserMetadata = cloneStringMapForManifestPlan(in.UserMetadata)
	out.Tags = cloneStringMapForManifestPlan(in.Tags)
	return out
}

func cloneStringMapForManifestPlan(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
