package sbs

import (
	"context"
	"fmt"

	sbsservice "github.com/nosway/namrbd/gateway/service"

	"github.com/nosway/namros/internal/storage"
)

type volumeSessionClient interface {
	OpenVolume(ctx context.Context, req *sbsservice.OpenVolumeRequest) (*sbsservice.OpenVolumeResponse, error)
	CloseVolume(ctx context.Context, req *sbsservice.CloseVolumeRequest) (*sbsservice.CloseVolumeResponse, error)
}

func requireVolumeSessionCacheForIdentity(identity SessionIdentity, cache *VolumeSessionCache) error {
	if identity.Enabled() && cache == nil {
		return fmt.Errorf("%w: sbs session cache is required when session identity is enabled", storage.ErrInvalidArgument)
	}
	return nil
}

func acquireVolumeSession(
	ctx context.Context,
	client volumeSessionClient,
	cache *VolumeSessionCache,
	key VolumeSessionKey,
	openReq *sbsservice.OpenVolumeRequest,
	closeReq func(VolumeSession) *sbsservice.CloseVolumeRequest,
) (VolumeSession, func() error, error) {
	open := func(ctx context.Context) (VolumeSession, error) {
		openResp, err := client.OpenVolume(ctx, openReq)
		if err != nil {
			return VolumeSession{}, mapSBSError(err)
		}
		if openResp == nil {
			return VolumeSession{}, fmt.Errorf("%w: sbs data endpoint returned nil open volume response", storage.ErrUnavailable)
		}
		session := VolumeSession{
			VolumeID:       openResp.VolumeID,
			VolumeHandle:   openResp.VolumeHandle,
			VolumeRevision: openResp.VolumeRevision,
			ServerVersion:  openResp.ServerVersion,
		}
		if session.VolumeID == "" {
			session.VolumeID = openReq.VolumeID
		}
		if session.VolumeID != openReq.VolumeID {
			return VolumeSession{}, fmt.Errorf("%w: open volume response volume mismatch got %q want %q", storage.ErrInvalidArgument, session.VolumeID, openReq.VolumeID)
		}
		if session.VolumeHandle == "" {
			return VolumeSession{}, fmt.Errorf("%w: sbs data endpoint returned empty open volume handle", storage.ErrUnavailable)
		}
		return session, nil
	}
	closeSession := func(session VolumeSession) error {
		_, err := client.CloseVolume(context.Background(), closeReq(session))
		if err != nil {
			return mapSBSError(err)
		}
		return nil
	}
	if cache == nil {
		session, err := open(ctx)
		if err != nil {
			return VolumeSession{}, nil, err
		}
		return session, func() error {
			return closeSession(session)
		}, nil
	}
	lease, err := cache.Acquire(ctx, key, open)
	if err != nil {
		return VolumeSession{}, nil, err
	}
	return lease.Session(), func() error {
		return lease.Release(closeSession)
	}, nil
}
