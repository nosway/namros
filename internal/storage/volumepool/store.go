package volumepool

import (
	"context"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"

	"github.com/nosway/namros/internal/storage"
)

type Member struct {
	VolumeID             string
	Store                storage.SegmentStore
	ReadOnly             bool
	State                string
	Weight               int
	AvailableBytes       uint64
	UsedPercent          float64
	HighWatermarkPercent float64
}

type AdmissionReason string

const (
	AdmissionReasonAccepted                   AdmissionReason = "accepted"
	AdmissionReasonReadOnly                   AdmissionReason = "read_only"
	AdmissionReasonStateNotActive             AdmissionReason = "state_not_active"
	AdmissionReasonInsufficientAvailableBytes AdmissionReason = "insufficient_available_bytes"
	AdmissionReasonHighWatermark              AdmissionReason = "high_watermark"
	AdmissionReasonNoWritableMembers          AdmissionReason = "no_writable_members"
)

type WriteAdmissionRequest struct {
	SizeBytes uint64
}

type WriteAdmissionPlan struct {
	Admitted        bool                      `json:"admitted"`
	VolumeID        string                    `json:"volume_id,omitempty"`
	Reason          AdmissionReason           `json:"reason"`
	MemberDecisions []MemberAdmissionDecision `json:"member_decisions,omitempty"`
}

type MemberAdmissionDecision struct {
	VolumeID             string          `json:"volume_id"`
	Admitted             bool            `json:"admitted"`
	Reason               AdmissionReason `json:"reason"`
	State                string          `json:"state"`
	ReadOnly             bool            `json:"read_only"`
	AvailableBytes       uint64          `json:"available_bytes,omitempty"`
	UsedPercent          float64         `json:"used_percent,omitempty"`
	HighWatermarkPercent float64         `json:"high_watermark_percent,omitempty"`
}

type MemberObservation struct {
	VolumeID       string
	AvailableBytes *uint64
	UsedPercent    *float64
}

type AdmissionError struct {
	Plan WriteAdmissionPlan
}

func (e *AdmissionError) Error() string {
	if e == nil {
		return "volume pool write admission rejected"
	}
	reason := e.Plan.Reason
	if reason == "" {
		reason = AdmissionReasonNoWritableMembers
	}
	return fmt.Sprintf("%s: volume pool write admission rejected: %s", storage.ErrUnavailable, reason)
}

func (e *AdmissionError) Unwrap() error {
	return storage.ErrUnavailable
}

type Store struct {
	mu      sync.Mutex
	members map[string]Member
	order   []string
	next    int
}

const (
	StateActive   = "active"
	StateReadOnly = "read_only"
	StateDraining = "draining"
	StateDegraded = "degraded"
	StateFull     = "full"
	StateOffline  = "offline"

	maxMemberWeight = 1024
)

func New(members []Member) (*Store, error) {
	index, order, err := buildMemberIndex(members)
	if err != nil {
		return nil, err
	}
	return &Store{
		members: index,
		order:   order,
	}, nil
}

func (s *Store) UpdateMembers(members []Member) error {
	index, order, err := buildMemberIndex(members)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.members = index
	s.order = order
	if s.next >= len(s.order) {
		s.next = 0
	}
	return nil
}

func (s *Store) UpdateMemberObservations(observations []MemberObservation) int {
	if len(observations) == 0 {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	updated := 0
	for _, observation := range observations {
		member, ok := s.members[strings.TrimSpace(observation.VolumeID)]
		if !ok {
			continue
		}
		if observation.AvailableBytes != nil {
			member.AvailableBytes = *observation.AvailableBytes
		}
		if observation.UsedPercent != nil && *observation.UsedPercent >= 0 && *observation.UsedPercent <= 100 {
			member.UsedPercent = *observation.UsedPercent
		}
		s.members[member.VolumeID] = member
		updated++
	}
	return updated
}

func (s *Store) PutSegment(ctx context.Context, req storage.PutSegmentRequest) (storage.SegmentRef, error) {
	member, _, err := s.nextWritableMember(req.SizeBytes)
	if err != nil {
		return storage.SegmentRef{}, err
	}
	ref, err := member.Store.PutSegment(ctx, req)
	if err != nil {
		return storage.SegmentRef{}, err
	}
	s.debitAvailable(member.VolumeID, req.SizeBytes)
	return stampVolumeID(ref, member.VolumeID), nil
}

func (s *Store) GetSegment(ctx context.Context, ref storage.SegmentRef, off, length uint64) (io.ReadCloser, error) {
	member, err := s.memberForRef(ref)
	if err != nil {
		return nil, err
	}
	return member.Store.GetSegment(ctx, ref, off, length)
}

func (s *Store) DeleteSegment(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	member, err := s.memberForRef(ref)
	if err != nil {
		return err
	}
	return member.Store.DeleteSegment(ctx, ref, reason)
}

func (s *Store) ValidateSegment(ctx context.Context, ref storage.SegmentRef) error {
	member, err := s.memberForRef(ref)
	if err != nil {
		return err
	}
	validator, ok := member.Store.(storage.SegmentValidator)
	if !ok {
		return nil
	}
	return validator.ValidateSegment(ctx, ref)
}

func (s *Store) MarkOrphan(ctx context.Context, ref storage.SegmentRef, reason storage.DeleteReason) error {
	member, err := s.memberForRef(ref)
	if err != nil {
		return err
	}
	tracker, ok := member.Store.(storage.OrphanTracker)
	if !ok {
		return fmt.Errorf("%w: volume pool member %q does not support orphan tracking", storage.ErrInvalidArgument, member.VolumeID)
	}
	return tracker.MarkOrphan(ctx, ref, reason)
}

func (s *Store) ListGCCandidates(ctx context.Context, limit int) ([]storage.GCCandidate, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	members := make([]Member, 0, len(s.order))
	for _, volumeID := range s.order {
		members = append(members, s.members[volumeID])
	}
	s.mu.Unlock()

	var out []storage.GCCandidate
	for _, member := range members {
		tracker, ok := member.Store.(storage.OrphanTracker)
		if !ok {
			continue
		}
		candidates, err := tracker.ListGCCandidates(ctx, 0)
		if err != nil {
			return nil, err
		}
		for _, candidate := range candidates {
			candidate.Ref = stampVolumeID(candidate.Ref, member.VolumeID)
			out = append(out, candidate)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (s *Store) PlanWrite(req WriteAdmissionRequest) WriteAdmissionPlan {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.planWriteLocked(req.SizeBytes, false)
}

func (s *Store) nextWritableMember(sizeBytes uint64) (Member, WriteAdmissionPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	plan := s.planWriteLocked(sizeBytes, true)
	if !plan.Admitted {
		return Member{}, plan, &AdmissionError{Plan: plan}
	}
	return s.members[plan.VolumeID], plan, nil
}

func (s *Store) planWriteLocked(sizeBytes uint64, advance bool) WriteAdmissionPlan {
	plan := WriteAdmissionPlan{Reason: AdmissionReasonNoWritableMembers}
	seen := make(map[string]struct{}, len(s.members))
	for i := 0; i < len(s.order); i++ {
		idx := (s.next + i) % len(s.order)
		member := s.members[s.order[idx]]
		decision := member.writeAdmissionDecision(sizeBytes)
		if _, ok := seen[member.VolumeID]; !ok {
			plan.MemberDecisions = append(plan.MemberDecisions, decision)
			seen[member.VolumeID] = struct{}{}
		}
		if decision.Admitted {
			if advance {
				s.next = (idx + 1) % len(s.order)
			}
			plan.Admitted = true
			plan.VolumeID = member.VolumeID
			plan.Reason = AdmissionReasonAccepted
			return plan
		}
	}
	return plan
}

func (s *Store) debitAvailable(volumeID string, sizeBytes uint64) {
	if sizeBytes == 0 {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	member, ok := s.members[volumeID]
	if !ok || member.AvailableBytes == 0 {
		return
	}
	if sizeBytes >= member.AvailableBytes {
		member.AvailableBytes = 0
	} else {
		member.AvailableBytes -= sizeBytes
	}
	s.members[volumeID] = member
}

func (s *Store) memberForRef(ref storage.SegmentRef) (Member, error) {
	volumeID := volumeIDFromRef(ref)
	s.mu.Lock()
	defer s.mu.Unlock()
	if volumeID == "" && len(s.order) == 1 {
		return s.members[s.order[0]], nil
	}
	member, ok := s.members[volumeID]
	if !ok {
		return Member{}, fmt.Errorf("%w: volume pool member %q not found", storage.ErrNotFound, volumeID)
	}
	return member, nil
}

func volumeIDFromRef(ref storage.SegmentRef) string {
	if ref.Placement.Parameters != nil && ref.Placement.Parameters["volume_id"] != "" {
		return ref.Placement.Parameters["volume_id"]
	}
	for _, chunk := range ref.Placement.Chunks {
		if chunk.VolumeID != "" {
			return chunk.VolumeID
		}
	}
	return ""
}

func normalizeMember(member Member) (Member, error) {
	state, err := NormalizeState(member.State, member.ReadOnly)
	if err != nil {
		return Member{}, err
	}
	member.State = state
	member.ReadOnly = member.ReadOnly || state == StateReadOnly
	if member.Weight == 0 {
		member.Weight = 1
	}
	if member.Weight < 0 || member.Weight > maxMemberWeight {
		return Member{}, fmt.Errorf("%w: volume pool member %q weight must be between 0 and %d", storage.ErrInvalidArgument, member.VolumeID, maxMemberWeight)
	}
	if member.UsedPercent < 0 || member.UsedPercent > 100 {
		return Member{}, fmt.Errorf("%w: volume pool member %q used_percent must be between 0 and 100", storage.ErrInvalidArgument, member.VolumeID)
	}
	if member.HighWatermarkPercent < 0 || member.HighWatermarkPercent > 100 {
		return Member{}, fmt.Errorf("%w: volume pool member %q high_watermark_percent must be between 0 and 100", storage.ErrInvalidArgument, member.VolumeID)
	}
	return member, nil
}

func buildMemberIndex(members []Member) (map[string]Member, []string, error) {
	if len(members) == 0 {
		return nil, nil, fmt.Errorf("%w: volume pool requires at least one member", storage.ErrInvalidArgument)
	}
	index := make(map[string]Member, len(members))
	order := make([]string, 0, len(members))
	for _, member := range members {
		if member.VolumeID == "" {
			return nil, nil, fmt.Errorf("%w: volume pool member volume id is required", storage.ErrInvalidArgument)
		}
		if member.Store == nil {
			return nil, nil, fmt.Errorf("%w: volume pool member store is required", storage.ErrInvalidArgument)
		}
		if _, exists := index[member.VolumeID]; exists {
			return nil, nil, fmt.Errorf("%w: duplicate volume pool member %q", storage.ErrInvalidArgument, member.VolumeID)
		}
		normalized, err := normalizeMember(member)
		if err != nil {
			return nil, nil, err
		}
		member = normalized
		index[member.VolumeID] = member
		for i := 0; i < member.Weight; i++ {
			order = append(order, member.VolumeID)
		}
	}
	return index, order, nil
}

func NormalizeState(state string, readOnly bool) (string, error) {
	state = strings.ToLower(strings.TrimSpace(strings.ReplaceAll(state, "-", "_")))
	if state == "" {
		if readOnly {
			return StateReadOnly, nil
		}
		return StateActive, nil
	}
	switch state {
	case StateActive, StateReadOnly, StateDraining, StateDegraded, StateFull, StateOffline:
		return state, nil
	default:
		return "", fmt.Errorf("%w: unsupported volume pool member state %q", storage.ErrInvalidArgument, state)
	}
}

func (m Member) writeAdmissionDecision(sizeBytes uint64) MemberAdmissionDecision {
	decision := MemberAdmissionDecision{
		VolumeID:             m.VolumeID,
		Admitted:             true,
		Reason:               AdmissionReasonAccepted,
		State:                m.State,
		ReadOnly:             m.ReadOnly,
		AvailableBytes:       m.AvailableBytes,
		UsedPercent:          m.UsedPercent,
		HighWatermarkPercent: m.HighWatermarkPercent,
	}
	if m.ReadOnly {
		decision.Admitted = false
		decision.Reason = AdmissionReasonReadOnly
		return decision
	}
	if m.State != StateActive {
		decision.Admitted = false
		decision.Reason = AdmissionReasonStateNotActive
		return decision
	}
	if m.AvailableBytes > 0 && sizeBytes > m.AvailableBytes {
		decision.Admitted = false
		decision.Reason = AdmissionReasonInsufficientAvailableBytes
		return decision
	}
	if m.HighWatermarkPercent > 0 && m.UsedPercent >= m.HighWatermarkPercent {
		decision.Admitted = false
		decision.Reason = AdmissionReasonHighWatermark
		return decision
	}
	return decision
}

func stampVolumeID(ref storage.SegmentRef, volumeID string) storage.SegmentRef {
	out := storage.CloneSegmentRef(ref)
	if out.Placement.Parameters == nil {
		out.Placement.Parameters = make(map[string]string, 1)
	}
	out.Placement.Parameters["volume_id"] = volumeID
	if len(out.Placement.Chunks) == 0 {
		out.Placement.Chunks = []storage.PlacementChunk{{VolumeID: volumeID, SizeBytes: out.SizeBytes, LengthBytes: out.SizeBytes}}
	} else {
		for i := range out.Placement.Chunks {
			out.Placement.Chunks[i].VolumeID = volumeID
		}
	}
	return out
}

var _ storage.SegmentStore = (*Store)(nil)
var _ storage.SegmentValidator = (*Store)(nil)
var _ storage.OrphanTracker = (*Store)(nil)
