package volumepool

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"sort"
	"testing"
	"time"

	"github.com/nosway/namros/internal/storage"
)

func TestStoreRoutesWritesRoundRobinAndReadsByVolume(t *testing.T) {
	v1 := newFakeStore("v1")
	v2 := newFakeStore("v2")
	store, err := New([]Member{
		{VolumeID: "18a00001", Store: v1},
		{VolumeID: "18a00002", Store: v2},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref1 := putPayload(t, store, "alpha")
	ref2 := putPayload(t, store, "bravo")
	if got, want := volumeIDFromRef(ref1), "18a00001"; got != want {
		t.Fatalf("ref1 volume = %q, want %q", got, want)
	}
	if got, want := volumeIDFromRef(ref2), "18a00002"; got != want {
		t.Fatalf("ref2 volume = %q, want %q", got, want)
	}
	if v1.puts != 1 || v2.puts != 1 {
		t.Fatalf("puts = v1:%d v2:%d, want 1/1", v1.puts, v2.puts)
	}

	got := readPayload(t, store, ref2, 1, 3)
	if string(got) != "rav" {
		t.Fatalf("routed range read = %q, want rav", got)
	}
	if v1.reads != 0 || v2.reads != 1 {
		t.Fatalf("reads = v1:%d v2:%d, want 0/1", v1.reads, v2.reads)
	}

	if err := store.DeleteSegment(t.Context(), ref1, storage.DeleteReasonManualGC); err != nil {
		t.Fatalf("DeleteSegment() error = %v", err)
	}
	if v1.deletes != 1 || v2.deletes != 0 {
		t.Fatalf("deletes = v1:%d v2:%d, want 1/0", v1.deletes, v2.deletes)
	}
}

func TestStoreSkipsReadOnlyMembersAndListsStampedOrphans(t *testing.T) {
	readonly := newFakeStore("readonly")
	writable := newFakeStore("writable")
	store, err := New([]Member{
		{VolumeID: "18a00001", Store: readonly, ReadOnly: true},
		{VolumeID: "18a00002", Store: writable},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref := putPayload(t, store, "payload")
	if got, want := volumeIDFromRef(ref), "18a00002"; got != want {
		t.Fatalf("written volume = %q, want %q", got, want)
	}
	if readonly.puts != 0 || writable.puts != 1 {
		t.Fatalf("puts = readonly:%d writable:%d, want 0/1", readonly.puts, writable.puts)
	}

	readonlyRef := fakeRef("old", "legacy")
	readonlyRef.Placement.Parameters = nil
	readonly.queue = append(readonly.queue, storage.GCCandidate{
		Ref:       readonlyRef,
		Reason:    storage.DeleteReasonPublishFailed,
		CreatedAt: time.Date(2026, 7, 17, 0, 0, 1, 0, time.UTC),
	})
	if err := store.MarkOrphan(t.Context(), ref, storage.DeleteReasonMultipartAborted); err != nil {
		t.Fatalf("MarkOrphan() error = %v", err)
	}

	candidates, err := store.ListGCCandidates(t.Context(), 10)
	if err != nil {
		t.Fatalf("ListGCCandidates() error = %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate len = %d, want 2", len(candidates))
	}
	if got, want := volumeIDFromRef(candidates[0].Ref), "18a00001"; got != want {
		t.Fatalf("candidate[0] volume = %q, want %q", got, want)
	}
	if got, want := volumeIDFromRef(candidates[1].Ref), "18a00002"; got != want {
		t.Fatalf("candidate[1] volume = %q, want %q", got, want)
	}
	candidates[0].Ref.Placement.Parameters["mutated"] = "yes"
	if _, ok := readonly.queue[0].Ref.Placement.Parameters["mutated"]; ok {
		t.Fatal("ListGCCandidates returned mutable candidate refs")
	}
}

func TestStoreAdmissionSkipsNonActiveAndWatermarkedMembers(t *testing.T) {
	degraded := newFakeStore("degraded")
	full := newFakeStore("full")
	draining := newFakeStore("draining")
	watermarked := newFakeStore("watermarked")
	tooSmall := newFakeStore("too-small")
	writable := newFakeStore("writable")
	store, err := New([]Member{
		{VolumeID: "18a00001", Store: degraded, State: StateDegraded},
		{VolumeID: "18a00002", Store: full, State: StateFull},
		{VolumeID: "18a00003", Store: draining, State: StateDraining},
		{VolumeID: "18a00004", Store: watermarked, UsedPercent: 95, HighWatermarkPercent: 90},
		{VolumeID: "18a00005", Store: tooSmall, AvailableBytes: 3},
		{VolumeID: "18a00006", Store: writable, AvailableBytes: 128},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref := putPayload(t, store, "payload")
	if got, want := volumeIDFromRef(ref), "18a00006"; got != want {
		t.Fatalf("written volume = %q, want %q", got, want)
	}
	if degraded.puts != 0 || full.puts != 0 || draining.puts != 0 || watermarked.puts != 0 || tooSmall.puts != 0 || writable.puts != 1 {
		t.Fatalf("puts = degraded:%d full:%d draining:%d watermarked:%d tooSmall:%d writable:%d, want only writable",
			degraded.puts, full.puts, draining.puts, watermarked.puts, tooSmall.puts, writable.puts)
	}
}

func TestStoreWriteAdmissionPlanExplainsSelectionAndRejections(t *testing.T) {
	store, err := New([]Member{
		{VolumeID: "18a00001", Store: newFakeStore("readonly"), State: StateReadOnly},
		{VolumeID: "18a00002", Store: newFakeStore("draining"), State: StateDraining},
		{VolumeID: "18a00003", Store: newFakeStore("too-small"), AvailableBytes: 3},
		{VolumeID: "18a00004", Store: newFakeStore("watermarked"), UsedPercent: 95, HighWatermarkPercent: 90},
		{VolumeID: "18a00005", Store: newFakeStore("writable"), AvailableBytes: 128},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	plan := store.PlanWrite(WriteAdmissionRequest{SizeBytes: 7})
	if !plan.Admitted || plan.VolumeID != "18a00005" || plan.Reason != AdmissionReasonAccepted {
		t.Fatalf("PlanWrite() = %+v, want accepted 18a00005", plan)
	}
	wantReasons := map[string]AdmissionReason{
		"18a00001": AdmissionReasonReadOnly,
		"18a00002": AdmissionReasonStateNotActive,
		"18a00003": AdmissionReasonInsufficientAvailableBytes,
		"18a00004": AdmissionReasonHighWatermark,
		"18a00005": AdmissionReasonAccepted,
	}
	if got := admissionDecisionReasons(plan); !equalAdmissionReasons(got, wantReasons) {
		t.Fatalf("decision reasons = %v, want %v", got, wantReasons)
	}

	ref := putPayload(t, store, "payload")
	if got := volumeIDFromRef(ref); got != "18a00005" {
		t.Fatalf("PutSegment() selected %q, want same volume as plan", got)
	}
}

func TestStoreWriteAdmissionErrorIncludesRejectedPlan(t *testing.T) {
	store, err := New([]Member{
		{VolumeID: "18a00001", Store: newFakeStore("draining"), State: StateDraining},
		{VolumeID: "18a00002", Store: newFakeStore("watermarked"), UsedPercent: 99, HighWatermarkPercent: 90},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	_, err = store.PutSegment(t.Context(), storage.PutSegmentRequest{Reader: bytes.NewReader([]byte("blocked")), SizeBytes: 7})
	if !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("PutSegment(no writable) error = %v, want ErrUnavailable", err)
	}
	var admissionErr *AdmissionError
	if !errors.As(err, &admissionErr) {
		t.Fatalf("PutSegment(no writable) error type = %T, want AdmissionError", err)
	}
	if admissionErr.Plan.Admitted || admissionErr.Plan.Reason != AdmissionReasonNoWritableMembers {
		t.Fatalf("admission plan = %+v, want rejected no_writable_members", admissionErr.Plan)
	}
	wantReasons := map[string]AdmissionReason{
		"18a00001": AdmissionReasonStateNotActive,
		"18a00002": AdmissionReasonHighWatermark,
	}
	if got := admissionDecisionReasons(admissionErr.Plan); !equalAdmissionReasons(got, wantReasons) {
		t.Fatalf("decision reasons = %v, want %v", got, wantReasons)
	}
}

func TestStoreAdmissionHonorsWeightAndAvailableBudget(t *testing.T) {
	v1 := newFakeStore("v1")
	v2 := newFakeStore("v2")
	store, err := New([]Member{
		{VolumeID: "18a00001", Store: v1, Weight: 2, AvailableBytes: 6},
		{VolumeID: "18a00002", Store: v2, Weight: 1, AvailableBytes: 128},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	ref1 := putPayload(t, store, "one")
	ref2 := putPayload(t, store, "two")
	ref3 := putPayload(t, store, "three")
	if got, want := []string{volumeIDFromRef(ref1), volumeIDFromRef(ref2), volumeIDFromRef(ref3)}, []string{"18a00001", "18a00001", "18a00002"}; got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("write volumes = %v, want %v", got, want)
	}
	if v1.puts != 2 || v2.puts != 1 {
		t.Fatalf("puts = v1:%d v2:%d, want 2/1", v1.puts, v2.puts)
	}
}

func TestStoreUpdateMembersRefreshesWriteAdmission(t *testing.T) {
	v1 := newFakeStore("v1")
	v2 := newFakeStore("v2")
	store, err := New([]Member{{VolumeID: "18a00001", Store: v1}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	oldRef := putPayload(t, store, "old")
	if got := volumeIDFromRef(oldRef); got != "18a00001" {
		t.Fatalf("old ref volume = %q", got)
	}

	if err := store.UpdateMembers([]Member{
		{VolumeID: "18a00001", Store: v1, State: StateDraining},
		{VolumeID: "18a00002", Store: v2, State: StateActive},
	}); err != nil {
		t.Fatalf("UpdateMembers() error = %v", err)
	}
	newRef := putPayload(t, store, "new")
	if got := volumeIDFromRef(newRef); got != "18a00002" {
		t.Fatalf("new ref volume = %q, want refreshed writable member", got)
	}
	if got := readPayload(t, store, oldRef, 0, 0); string(got) != "old" {
		t.Fatalf("old ref read = %q", got)
	}
	if got := readPayload(t, store, newRef, 0, 0); string(got) != "new" {
		t.Fatalf("new ref read = %q", got)
	}
	if err := store.UpdateMembers([]Member{{VolumeID: "broken"}}); !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("UpdateMembers(invalid) error = %v, want ErrInvalidArgument", err)
	}
	refAfterFailedUpdate := putPayload(t, store, "still-new")
	if got := volumeIDFromRef(refAfterFailedUpdate); got != "18a00002" {
		t.Fatalf("ref after failed update volume = %q, want previous snapshot", got)
	}
}

func TestStoreUpdateMemberObservationsRefreshesCapacityHintsOnly(t *testing.T) {
	writable := newFakeStore("writable")
	draining := newFakeStore("draining")
	store, err := New([]Member{
		{VolumeID: "18a00001", Store: writable, State: StateActive, AvailableBytes: 3, UsedPercent: 10},
		{VolumeID: "18a00002", Store: draining, State: StateDraining, AvailableBytes: 1, UsedPercent: 10},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if plan := store.PlanWrite(WriteAdmissionRequest{SizeBytes: 7}); plan.Admitted || plan.MemberDecisions[0].Reason != AdmissionReasonInsufficientAvailableBytes {
		t.Fatalf("initial PlanWrite() = %+v, want insufficient capacity", plan)
	}

	available := uint64(128)
	used := 45.0
	updated := store.UpdateMemberObservations([]MemberObservation{
		{VolumeID: "18a00001", AvailableBytes: &available, UsedPercent: &used},
		{VolumeID: "18a00002", AvailableBytes: &available, UsedPercent: &used},
		{VolumeID: "missing", AvailableBytes: &available},
	})
	if updated != 2 {
		t.Fatalf("UpdateMemberObservations() = %d, want 2", updated)
	}
	plan := store.PlanWrite(WriteAdmissionRequest{SizeBytes: 7})
	if !plan.Admitted || plan.VolumeID != "18a00001" {
		t.Fatalf("updated PlanWrite() = %+v, want active volume 18a00001", plan)
	}
	ref := putPayload(t, store, "payload")
	if got := volumeIDFromRef(ref); got != "18a00001" {
		t.Fatalf("PutSegment() volume = %q, want active member 18a00001", got)
	}
	plan = store.PlanWrite(WriteAdmissionRequest{SizeBytes: 7})
	reasons := admissionDecisionReasons(plan)
	if reasons["18a00002"] != AdmissionReasonStateNotActive {
		t.Fatalf("draining member reason = %q, want metadata state preserved", reasons["18a00002"])
	}
}

func TestStoreAllowsSingleMemberRefsWithoutVolume(t *testing.T) {
	member := newFakeStore("single")
	store, err := New([]Member{{VolumeID: "18a00001", Store: member}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	ref := fakeRef("single-1", "single")
	ref.Placement.Parameters = nil
	ref.Placement.Chunks = nil
	member.objects[ref.SegmentID] = []byte("single volume payload")

	got := readPayload(t, store, ref, 7, 6)
	if string(got) != "volume" {
		t.Fatalf("single-member read = %q, want volume", got)
	}
}

func TestStoreRejectsInvalidMembersAndMissingVolumes(t *testing.T) {
	if _, err := New(nil); !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("New(nil) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := New([]Member{{VolumeID: "v1"}}); !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("New(nil store) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := New([]Member{
		{VolumeID: "v1", Store: newFakeStore("v1-a")},
		{VolumeID: "v1", Store: newFakeStore("v1-b")},
	}); !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("New(duplicate) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := New([]Member{{VolumeID: "v1", Store: newFakeStore("v1"), State: "mystery"}}); !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("New(invalid state) error = %v, want ErrInvalidArgument", err)
	}
	if _, err := New([]Member{{VolumeID: "v1", Store: newFakeStore("v1"), Weight: 2048}}); !errors.Is(err, storage.ErrInvalidArgument) {
		t.Fatalf("New(invalid weight) error = %v, want ErrInvalidArgument", err)
	}
	readonlyStore, err := New([]Member{{VolumeID: "v1", Store: newFakeStore("v1"), State: StateDraining}})
	if err != nil {
		t.Fatalf("New(readonly state) error = %v", err)
	}
	if _, err := readonlyStore.PutSegment(t.Context(), storage.PutSegmentRequest{Reader: bytes.NewReader([]byte("blocked")), SizeBytes: 7}); !errors.Is(err, storage.ErrUnavailable) {
		t.Fatalf("PutSegment(no writable) error = %v, want ErrUnavailable", err)
	}

	store, err := New([]Member{
		{VolumeID: "v1", Store: newFakeStore("v1")},
		{VolumeID: "v2", Store: newFakeStore("v2")},
	})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if _, err := store.GetSegment(t.Context(), fakeRef("missing", "v3"), 0, 0); !errors.Is(err, storage.ErrNotFound) {
		t.Fatalf("GetSegment(missing volume) error = %v, want ErrNotFound", err)
	}
}

func admissionDecisionReasons(plan WriteAdmissionPlan) map[string]AdmissionReason {
	out := make(map[string]AdmissionReason, len(plan.MemberDecisions))
	for _, decision := range plan.MemberDecisions {
		out[decision.VolumeID] = decision.Reason
	}
	return out
}

func equalAdmissionReasons(a, b map[string]AdmissionReason) bool {
	if len(a) != len(b) {
		return false
	}
	for key, want := range b {
		if a[key] != want {
			return false
		}
	}
	return true
}

func putPayload(t *testing.T, store storage.SegmentStore, payload string) storage.SegmentRef {
	t.Helper()
	ref, err := store.PutSegment(t.Context(), storage.PutSegmentRequest{
		Reader:    bytes.NewReader([]byte(payload)),
		SizeBytes: uint64(len(payload)),
	})
	if err != nil {
		t.Fatalf("PutSegment() error = %v", err)
	}
	return ref
}

func readPayload(t *testing.T, store storage.SegmentStore, ref storage.SegmentRef, off, length uint64) []byte {
	t.Helper()
	reader, err := store.GetSegment(t.Context(), ref, off, length)
	if err != nil {
		t.Fatalf("GetSegment() error = %v", err)
	}
	defer reader.Close()
	payload, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	return payload
}

type fakeStore struct {
	name     string
	puts     int
	reads    int
	deletes  int
	objects  map[string][]byte
	queue    []storage.GCCandidate
	validate int
}

func newFakeStore(name string) *fakeStore {
	return &fakeStore{name: name, objects: make(map[string][]byte)}
}

func (s *fakeStore) PutSegment(ctx context.Context, req storage.PutSegmentRequest) (storage.SegmentRef, error) {
	if err := ctx.Err(); err != nil {
		return storage.SegmentRef{}, err
	}
	payload, err := io.ReadAll(req.Reader)
	if err != nil {
		return storage.SegmentRef{}, err
	}
	if req.SizeBytes != 0 && req.SizeBytes != uint64(len(payload)) {
		return storage.SegmentRef{}, fmt.Errorf("%w: size mismatch", storage.ErrInvalidArgument)
	}
	s.puts++
	segmentID := fmt.Sprintf("%s-%d", s.name, s.puts)
	s.objects[segmentID] = append([]byte(nil), payload...)
	return fakeRef(segmentID, s.name, payload), nil
}

func (s *fakeStore) GetSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	payload, ok := s.objects[ref.SegmentID]
	if !ok {
		return nil, storage.ErrNotFound
	}
	s.reads++
	if off > uint64(len(payload)) {
		return nil, storage.ErrInvalidRange
	}
	end := uint64(len(payload))
	if length > 0 && off+length < end {
		end = off + length
	}
	return io.NopCloser(bytes.NewReader(payload[off:end])), nil
}

func (s *fakeStore) DeleteSegment(ctx context.Context, ref storage.SegmentRef, _ storage.DeleteReason) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := s.objects[ref.SegmentID]; !ok {
		return storage.ErrNotFound
	}
	s.deletes++
	delete(s.objects, ref.SegmentID)
	return nil
}

func (s *fakeStore) ValidateSegment(ctx context.Context, ref storage.SegmentRef) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, ok := s.objects[ref.SegmentID]; !ok {
		return storage.ErrNotFound
	}
	s.validate++
	return nil
}

func (s *fakeStore) MarkOrphan(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.queue = append(s.queue, storage.GCCandidate{
		Ref:       storage.CloneSegmentRef(ref),
		Reason:    reason,
		CreatedAt: time.Date(2026, 7, 17, 0, 0, len(s.queue)+2, 0, time.UTC),
	})
	return nil
}

func (s *fakeStore) ListGCCandidates(ctx context.Context, limit int) ([]storage.GCCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	out := append([]storage.GCCandidate(nil), s.queue...)
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func fakeRef(segmentID, backend string, payload ...[]byte) storage.SegmentRef {
	data := []byte("payload")
	if len(payload) > 0 {
		data = payload[0]
	}
	return storage.SegmentRef{
		SegmentID: segmentID,
		Placement: storage.PlacementSnapshot{
			Backend: backend,
			Parameters: map[string]string{
				"volume_id": backend,
			},
			Chunks: []storage.PlacementChunk{
				{VolumeID: backend, SizeBytes: uint64(len(data)), LengthBytes: uint64(len(data))},
			},
		},
		SizeBytes: uint64(len(data)),
		Digest: storage.Digest{
			Algorithm: "sha256",
			Hex:       sha256Hex(data),
		},
		CreatedAt: time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC),
	}
}

func sha256Hex(payload []byte) string {
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

var _ storage.SegmentStore = (*fakeStore)(nil)
var _ storage.SegmentValidator = (*fakeStore)(nil)
var _ storage.OrphanTracker = (*fakeStore)(nil)
