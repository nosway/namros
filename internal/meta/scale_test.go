package meta

import (
	"errors"
	"strings"
	"testing"

	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

func TestValidateObjectVersionScaleRejectsOversizedValue(t *testing.T) {
	ref := storage.SegmentRef{
		SegmentID: "segment-oversized-value",
		Placement: storage.PlacementSnapshot{
			Parameters: map[string]string{
				"padding": strings.Repeat("x", MaxObjectManifestValueBytes),
			},
		},
		SizeBytes: 1,
	}
	version := model.ObjectVersion{
		BucketID:       "bucket-1",
		Key:            "large-manifest.bin",
		VersionID:      "version-1",
		VersionSortKey: "version-1",
		SegmentRef:     ref,
		SegmentRefs:    []storage.SegmentRef{ref},
		State:          model.ObjectVersionCommitted,
	}
	err := ValidateObjectVersionScale(version)
	if !errors.Is(err, ErrObjectManifestTooLarge) || !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ValidateObjectVersionScale() error = %v, want ErrObjectManifestTooLarge and ErrInvalidArgument", err)
	}
}

func TestPlanObjectVersionManifestStorageChunksLargeManifest(t *testing.T) {
	refs := make([]storage.SegmentRef, 0, ObjectManifestChunkRefTarget+1)
	for i := 0; i < ObjectManifestChunkRefTarget+1; i++ {
		refs = append(refs, storage.SegmentRef{
			SegmentID: "segment-" + strings.Repeat("x", 128) + string(rune('a'+(i%26))),
			Placement: storage.PlacementSnapshot{
				Parameters: map[string]string{
					"padding": strings.Repeat("p", 32*1024),
				},
			},
			SizeBytes: 1,
		})
	}
	version := model.ObjectVersion{
		BucketID:       "bucket-1",
		Key:            "chunked-manifest.bin",
		VersionID:      "version-1",
		VersionSortKey: "version-1",
		SegmentRefs:    refs,
		State:          model.ObjectVersionCommitted,
	}
	plan, err := PlanObjectVersionManifestStorage(version)
	if err != nil {
		t.Fatalf("PlanObjectVersionManifestStorage() error = %v", err)
	}
	if plan.Inline {
		t.Fatalf("plan.Inline = true, want chunked")
	}
	if plan.StoredVersion.Manifest.Encoding != model.ObjectManifestEncodingChunked ||
		plan.StoredVersion.Manifest.RefCount != len(refs) ||
		plan.StoredVersion.Manifest.ChunkCount != len(plan.Chunks) {
		t.Fatalf("stored manifest descriptor = %+v chunks=%d", plan.StoredVersion.Manifest, len(plan.Chunks))
	}
	if len(plan.StoredVersion.SegmentRefs) != 0 || plan.StoredVersion.SegmentRef.SegmentID != "" {
		t.Fatalf("stored version kept inline refs: %+v", plan.StoredVersion)
	}
	for _, chunk := range plan.Chunks {
		valueBytes, err := objectManifestChunkValueBytes(chunk)
		if err != nil {
			t.Fatalf("objectManifestChunkValueBytes() error = %v", err)
		}
		if valueBytes > MaxObjectManifestValueBytes {
			t.Fatalf("chunk %d value bytes = %d, max %d", chunk.ChunkNumber, valueBytes, MaxObjectManifestValueBytes)
		}
	}
}

func TestObjectVersionValueBytesReportsEncodedSize(t *testing.T) {
	version := model.ObjectVersion{
		BucketID:       "bucket-1",
		Key:            "object.bin",
		VersionID:      "version-1",
		VersionSortKey: "version-1",
		SegmentRef: storage.SegmentRef{
			SegmentID: "segment-1",
			SizeBytes: 1,
		},
		State: model.ObjectVersionCommitted,
	}
	valueBytes, err := ObjectVersionValueBytes(version)
	if err != nil {
		t.Fatalf("ObjectVersionValueBytes() error = %v", err)
	}
	if valueBytes <= 0 {
		t.Fatalf("ObjectVersionValueBytes() = %d, want positive", valueBytes)
	}
}

func TestMultipartPartLimitValidation(t *testing.T) {
	if err := ValidateCompletedMultipartPartCount(MaxMultipartParts); err != nil {
		t.Fatalf("ValidateCompletedMultipartPartCount(max) error = %v", err)
	}
	if err := ValidateCompletedMultipartPartCount(MaxMultipartParts + 1); !errors.Is(err, ErrInvalidArgument) {
		t.Fatalf("ValidateCompletedMultipartPartCount(over max) error = %v, want ErrInvalidArgument", err)
	}
	parts := []int{1, MaxMultipartParts}
	if err := ValidateMultipartPartNumberSelection(parts); err != nil {
		t.Fatalf("ValidateMultipartPartNumberSelection(valid) error = %v", err)
	}
	for name, parts := range map[string][]int{
		"zero part number": {0},
		"over max":         {MaxMultipartParts + 1},
		"duplicate":        {1, 1},
	} {
		if err := ValidateMultipartPartNumberSelection(parts); !errors.Is(err, ErrInvalidArgument) {
			t.Fatalf("ValidateMultipartPartNumberSelection(%s) error = %v, want ErrInvalidArgument", name, err)
		}
	}
}
