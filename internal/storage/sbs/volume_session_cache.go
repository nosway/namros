package sbs

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/nosway/namros/internal/storage"
)

type VolumeSession struct {
	VolumeID       string
	VolumeHandle   string
	VolumeRevision uint64
	ServerVersion  string
}

type VolumeSessionCache struct {
	mu      sync.Mutex
	entries map[VolumeSessionKey]*volumeSessionEntry
}

type volumeSessionEntry struct {
	session VolumeSession
	refs    int
}

type VolumeSessionLease struct {
	cache   *VolumeSessionCache
	key     VolumeSessionKey
	session VolumeSession

	once sync.Once
	err  error
}

func NewVolumeSessionCache() *VolumeSessionCache {
	return &VolumeSessionCache{entries: make(map[VolumeSessionKey]*volumeSessionEntry)}
}

func (c *VolumeSessionCache) Acquire(ctx context.Context, key VolumeSessionKey, open func(context.Context) (VolumeSession, error)) (*VolumeSessionLease, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if c == nil {
		return nil, fmt.Errorf("%w: sbs volume session cache is nil", storage.ErrInvalidArgument)
	}
	key = normalizeVolumeSessionKey(key)
	if err := key.Validate(); err != nil {
		return nil, err
	}
	if open == nil {
		return nil, fmt.Errorf("%w: sbs volume session open function is required", storage.ErrInvalidArgument)
	}

	c.mu.Lock()
	if entry, ok := c.entries[key]; ok {
		entry.refs++
		lease := &VolumeSessionLease{cache: c, key: key, session: entry.session}
		c.mu.Unlock()
		return lease, nil
	}

	session, err := open(ctx)
	if err != nil {
		c.mu.Unlock()
		return nil, err
	}
	session.VolumeID = strings.TrimSpace(session.VolumeID)
	session.VolumeHandle = strings.TrimSpace(session.VolumeHandle)
	if session.VolumeID == "" {
		session.VolumeID = key.VolumeID
	}
	if session.VolumeID != key.VolumeID {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: open volume response volume mismatch got %q want %q", storage.ErrInvalidArgument, session.VolumeID, key.VolumeID)
	}
	if session.VolumeHandle == "" {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: sbs data endpoint returned empty open volume handle", storage.ErrUnavailable)
	}
	c.entries[key] = &volumeSessionEntry{session: session, refs: 1}
	lease := &VolumeSessionLease{cache: c, key: key, session: session}
	c.mu.Unlock()
	return lease, nil
}

func (l *VolumeSessionLease) Session() VolumeSession {
	if l == nil {
		return VolumeSession{}
	}
	return l.session
}

func (l *VolumeSessionLease) Release(close func(VolumeSession) error) error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		l.err = l.cache.release(l.key, close)
	})
	return l.err
}

func (c *VolumeSessionCache) release(key VolumeSessionKey, close func(VolumeSession) error) error {
	if c == nil {
		return nil
	}
	key = normalizeVolumeSessionKey(key)
	c.mu.Lock()
	entry, ok := c.entries[key]
	if !ok {
		c.mu.Unlock()
		return nil
	}
	if entry.refs > 1 {
		entry.refs--
		c.mu.Unlock()
		return nil
	}
	delete(c.entries, key)
	session := entry.session
	c.mu.Unlock()
	if close == nil {
		return nil
	}
	return close(session)
}

func (c *VolumeSessionCache) Len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.entries)
}

func (key VolumeSessionKey) Validate() error {
	key = normalizeVolumeSessionKey(key)
	if key.DataEndpoint == "" {
		return fmt.Errorf("%w: sbs session data endpoint is required", storage.ErrInvalidArgument)
	}
	if key.VolumeID == "" {
		return fmt.Errorf("%w: sbs session volume id is required", storage.ErrInvalidArgument)
	}
	if key.AccessMode == "" {
		return fmt.Errorf("%w: sbs session access mode is required", storage.ErrInvalidArgument)
	}
	if key.GatewayID == "" {
		return fmt.Errorf("%w: sbs session gateway id is required", storage.ErrInvalidArgument)
	}
	if key.AttachmentID == "" {
		return fmt.Errorf("%w: sbs session attachment id is required", storage.ErrInvalidArgument)
	}
	if key.Generation == 0 {
		return fmt.Errorf("%w: sbs session generation is required", storage.ErrInvalidArgument)
	}
	return nil
}

func normalizeVolumeSessionKey(key VolumeSessionKey) VolumeSessionKey {
	key.DataEndpoint = strings.TrimSpace(key.DataEndpoint)
	key.VolumeID = strings.TrimSpace(key.VolumeID)
	key.AccessMode = strings.TrimSpace(key.AccessMode)
	key.GatewayID = strings.TrimSpace(key.GatewayID)
	key.AttachmentID = strings.TrimSpace(key.AttachmentID)
	key.PoolID = strings.TrimSpace(key.PoolID)
	key.WriterGroupID = strings.TrimSpace(key.WriterGroupID)
	key.GatewayInstanceID = strings.TrimSpace(key.GatewayInstanceID)
	key.SessionID = strings.TrimSpace(key.SessionID)
	return key
}
