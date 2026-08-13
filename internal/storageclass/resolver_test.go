package storageclass

import (
	"errors"
	"testing"

	"github.com/nosway/namros/internal/storage"
)

func TestDefaultResolverResolvesReplicatedProfiles(t *testing.T) {
	resolver := DefaultResolver()

	snapshot, err := resolver.Resolve(ResolveRequest{RequestedID: "durable_r4"})
	if err != nil {
		t.Fatalf("Resolve(DURABLE_R4) error = %v", err)
	}
	if snapshot.StorageClassID != "DURABLE_R4" {
		t.Fatalf("StorageClassID = %q, want DURABLE_R4", snapshot.StorageClassID)
	}
	if snapshot.Backend != BackendLocal {
		t.Fatalf("Backend = %q, want %q", snapshot.Backend, BackendLocal)
	}
	if snapshot.Parameters[ParamRedundancyBackend] != RedundancyReplicated {
		t.Fatalf("redundancy = %q", snapshot.Parameters[ParamRedundancyBackend])
	}
	if snapshot.Parameters[ParamReplicaCount] != "4" || snapshot.Parameters[ParamReadQuorum] != "3" || snapshot.Parameters[ParamWriteQuorum] != "3" {
		t.Fatalf("replicated params = %+v", snapshot.Parameters)
	}
}

func TestDefaultResolverResolvesECProfiles(t *testing.T) {
	resolver := DefaultResolver()

	snapshot, err := resolver.Resolve(ResolveRequest{RequestedID: "EC_8_3"})
	if err != nil {
		t.Fatalf("Resolve(EC_8_3) error = %v", err)
	}
	if snapshot.Parameters[ParamRedundancyBackend] != RedundancyErasureCode {
		t.Fatalf("redundancy = %q", snapshot.Parameters[ParamRedundancyBackend])
	}
	if snapshot.Parameters[ParamDataShards] != "8" || snapshot.Parameters[ParamParityShards] != "3" {
		t.Fatalf("ec params = %+v", snapshot.Parameters)
	}
	if snapshot.Parameters[ParamMinObjectSize] != "33554432" {
		t.Fatalf("min object size = %q", snapshot.Parameters[ParamMinObjectSize])
	}
}

func TestResolverRejectsUnsupportedDisabledAndTooSmall(t *testing.T) {
	resolver, err := NewCatalog(DefaultID,
		replicated(DefaultID, 3, 2, 2),
		Definition{ID: "DISABLED", Disabled: true},
		Definition{ID: "LARGE_ONLY", MinObjectSizeBytes: 1024},
	)
	if err != nil {
		t.Fatalf("NewCatalog() error = %v", err)
	}

	if _, err := resolver.Resolve(ResolveRequest{RequestedID: "missing"}); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("Resolve(missing) error = %v, want ErrUnsupported", err)
	}
	if _, err := resolver.Resolve(ResolveRequest{RequestedID: "disabled"}); !errors.Is(err, ErrDisabled) {
		t.Fatalf("Resolve(disabled) error = %v, want ErrDisabled", err)
	}
	if _, err := resolver.Resolve(ResolveRequest{RequestedID: "large_only", HasSize: true, SizeBytes: 10}); !errors.Is(err, ErrTooSmall) {
		t.Fatalf("Resolve(large_only small) error = %v, want ErrTooSmall", err)
	}
}

func TestResolverFallbackAndBackendPreservation(t *testing.T) {
	resolver := DefaultResolver()
	fallback := storage.StorageClassSnapshot{
		StorageClassID: "STANDARD",
		Backend:        "sbs-physical",
		Parameters: map[string]string{
			"existing": "kept",
		},
	}

	got, err := resolver.Resolve(ResolveRequest{Fallback: fallback})
	if err != nil {
		t.Fatalf("Resolve(fallback) error = %v", err)
	}
	if got.Backend != "sbs-physical" || got.Parameters["existing"] != "kept" {
		t.Fatalf("fallback snapshot = %+v", got)
	}
	got.Parameters["existing"] = "mutated"
	if fallback.Parameters["existing"] != "kept" {
		t.Fatalf("fallback parameters were mutated")
	}

	requested, err := resolver.Resolve(ResolveRequest{RequestedID: "STANDARD_R3", Fallback: fallback})
	if err != nil {
		t.Fatalf("Resolve(requested with fallback backend) error = %v", err)
	}
	if requested.StorageClassID != "STANDARD_R3" || requested.Backend != "sbs-physical" {
		t.Fatalf("requested snapshot = %+v", requested)
	}
	if requested.Parameters["existing"] != "" {
		t.Fatalf("requested snapshot leaked fallback parameters: %+v", requested.Parameters)
	}
}
