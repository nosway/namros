package meta

import (
	"fmt"
	"strings"
	"time"

	"github.com/nosway/namros/internal/meta/model"
)

const maxVolumePoolMemberWeight = 1024

func BuildVolumePool(existing model.VolumePool, req PutVolumePoolRequest, now time.Time) (model.VolumePool, error) {
	poolID := strings.TrimSpace(req.PoolID)
	if poolID == "" {
		return model.VolumePool{}, fmt.Errorf("%w: volume pool id is required", ErrInvalidArgument)
	}
	if existing.PoolID != "" && existing.PoolID != poolID {
		return model.VolumePool{}, fmt.Errorf("%w: volume pool id mismatch", ErrInvalidArgument)
	}
	generation := req.Generation
	if generation == 0 {
		generation = existing.Generation + 1
		if generation == 0 {
			generation = 1
		}
	} else if existing.PoolID != "" && generation <= existing.Generation {
		return model.VolumePool{}, fmt.Errorf("%w: volume pool %q generation %d must be greater than current generation %d", ErrCASConflict, poolID, generation, existing.Generation)
	}

	members, err := NormalizeVolumePoolMembers(req.Members)
	if err != nil {
		return model.VolumePool{}, err
	}
	createdAt := existing.CreatedAt
	if createdAt.IsZero() {
		createdAt = now.UTC()
	}
	return model.VolumePool{
		PoolID:          poolID,
		Generation:      generation,
		DurabilityClass: strings.TrimSpace(req.DurabilityClass),
		StorageClassIDs: cleanVolumePoolStringList(req.StorageClassIDs),
		Members:         members,
		CreatedAt:       createdAt,
		UpdatedAt:       now.UTC(),
	}, nil
}

func NormalizeVolumePoolMembers(in []model.VolumePoolMember) ([]model.VolumePoolMember, error) {
	if len(in) == 0 {
		return nil, fmt.Errorf("%w: volume pool requires at least one member", ErrInvalidArgument)
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]model.VolumePoolMember, 0, len(in))
	for _, member := range in {
		member.VolumeID = strings.TrimSpace(member.VolumeID)
		if member.VolumeID == "" {
			return nil, fmt.Errorf("%w: volume pool member volume id is required", ErrInvalidArgument)
		}
		if _, ok := seen[member.VolumeID]; ok {
			return nil, fmt.Errorf("%w: duplicate volume pool member %q", ErrInvalidArgument, member.VolumeID)
		}
		seen[member.VolumeID] = struct{}{}
		state, err := NormalizeVolumePoolState(member.State, member.ReadOnly)
		if err != nil {
			return nil, err
		}
		member.State = state
		member.ReadOnly = member.ReadOnly || state == model.VolumePoolStateReadOnly
		if member.Weight == 0 {
			member.Weight = 1
		}
		if member.Weight < 0 || member.Weight > maxVolumePoolMemberWeight {
			return nil, fmt.Errorf("%w: volume pool member %q weight must be between 0 and %d", ErrInvalidArgument, member.VolumeID, maxVolumePoolMemberWeight)
		}
		if member.UsedPercent < 0 || member.UsedPercent > 100 {
			return nil, fmt.Errorf("%w: volume pool member %q used_percent must be between 0 and 100", ErrInvalidArgument, member.VolumeID)
		}
		if member.HighWatermarkPercent < 0 || member.HighWatermarkPercent > 100 {
			return nil, fmt.Errorf("%w: volume pool member %q high_watermark_percent must be between 0 and 100", ErrInvalidArgument, member.VolumeID)
		}
		if !member.LastObservedAt.IsZero() {
			member.LastObservedAt = member.LastObservedAt.UTC()
		}
		out = append(out, member)
	}
	return out, nil
}

func NormalizeVolumePoolState(state model.VolumePoolState, readOnly bool) (model.VolumePoolState, error) {
	normalized := model.VolumePoolState(strings.ToLower(strings.TrimSpace(strings.ReplaceAll(string(state), "-", "_"))))
	if normalized == "" {
		if readOnly {
			return model.VolumePoolStateReadOnly, nil
		}
		return model.VolumePoolStateActive, nil
	}
	switch normalized {
	case model.VolumePoolStateActive, model.VolumePoolStateReadOnly, model.VolumePoolStateDraining, model.VolumePoolStateDegraded, model.VolumePoolStateFull, model.VolumePoolStateOffline:
		return normalized, nil
	default:
		return "", fmt.Errorf("%w: unsupported volume pool member state %q", ErrInvalidArgument, normalized)
	}
}

func CloneVolumePool(in model.VolumePool) model.VolumePool {
	out := in
	out.StorageClassIDs = append([]string(nil), in.StorageClassIDs...)
	out.Members = append([]model.VolumePoolMember(nil), in.Members...)
	return out
}

func CloneVolumePools(in []model.VolumePool) []model.VolumePool {
	if len(in) == 0 {
		return nil
	}
	out := make([]model.VolumePool, len(in))
	for i := range in {
		out[i] = CloneVolumePool(in[i])
	}
	return out
}

func cleanVolumePoolStringList(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, value := range in {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
