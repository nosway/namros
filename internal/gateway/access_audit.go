package gateway

import (
	"context"
	"sync"
	"time"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/meta"
)

type accessAuditRecorder interface {
	RecordAccessAudit(context.Context, meta.PutAdminAuditEventRequest) error
	Close() error
}

type syncAccessAuditRecorder struct {
	repo meta.Repository
}

func (r syncAccessAuditRecorder) RecordAccessAudit(ctx context.Context, req meta.PutAdminAuditEventRequest) error {
	if r.repo == nil {
		return nil
	}
	_, err := r.repo.PutAdminAuditEvent(ctx, req)
	return err
}

func (r syncAccessAuditRecorder) Close() error {
	return nil
}

type accessAuditConfig struct {
	BatchSize     int
	QueueSize     int
	FlushInterval time.Duration
}

type asyncAccessAuditRecorder struct {
	repo          meta.Repository
	batchSize     int
	flushInterval time.Duration
	queue         chan meta.PutAdminAuditEventRequest
	done          chan struct{}
	closed        chan struct{}
	mu            sync.RWMutex
	closing       bool
	closeOnce     sync.Once
}

func accessAuditConfigFromApp(cfg config.Config) accessAuditConfig {
	return accessAuditConfig{
		BatchSize:     cfg.AccessAuditBatchSize,
		QueueSize:     cfg.AccessAuditQueueSize,
		FlushInterval: cfg.AccessAuditFlushInterval,
	}
}

func newAsyncAccessAuditRecorder(repo meta.Repository, cfg accessAuditConfig) *asyncAccessAuditRecorder {
	batchSize := cfg.BatchSize
	if batchSize <= 0 {
		batchSize = config.DefaultAccessAuditBatchSize
	}
	queueSize := cfg.QueueSize
	if queueSize <= 0 {
		queueSize = config.DefaultAccessAuditQueueSize
	}
	r := &asyncAccessAuditRecorder{
		repo:          repo,
		batchSize:     batchSize,
		flushInterval: cfg.FlushInterval,
		queue:         make(chan meta.PutAdminAuditEventRequest, queueSize),
		done:          make(chan struct{}),
		closed:        make(chan struct{}),
	}
	go r.run()
	return r
}

func (r *asyncAccessAuditRecorder) RecordAccessAudit(ctx context.Context, req meta.PutAdminAuditEventRequest) error {
	if r == nil || r.repo == nil {
		return nil
	}
	r.mu.RLock()
	if r.closing {
		r.mu.RUnlock()
		_, err := r.repo.PutAdminAuditEvent(ctx, req)
		return err
	}
	select {
	case r.queue <- req:
		r.mu.RUnlock()
		return nil
	case <-ctx.Done():
		r.mu.RUnlock()
		return ctx.Err()
	default:
		r.mu.RUnlock()
		_, err := r.repo.PutAdminAuditEvent(ctx, req)
		return err
	}
}

func (r *asyncAccessAuditRecorder) Close() error {
	if r == nil {
		return nil
	}
	r.closeOnce.Do(func() {
		r.mu.Lock()
		r.closing = true
		close(r.done)
		r.mu.Unlock()
	})
	<-r.closed
	return nil
}

func (r *asyncAccessAuditRecorder) run() {
	defer close(r.closed)
	var timer *time.Timer
	var timerC <-chan time.Time
	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timerC = nil
	}
	startTimer := func() {
		if r.flushInterval <= 0 || timerC != nil {
			return
		}
		if timer == nil {
			timer = time.NewTimer(r.flushInterval)
		} else {
			timer.Reset(r.flushInterval)
		}
		timerC = timer.C
	}
	defer stopTimer()
	batch := make([]meta.PutAdminAuditEventRequest, 0, r.batchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		stopTimer()
		_, err := r.repo.PutAdminAuditEvents(context.Background(), meta.PutAdminAuditEventsRequest{
			Events: batch,
		})
		if err != nil {
			for _, req := range batch {
				_, _ = r.repo.PutAdminAuditEvent(context.Background(), req)
			}
		}
		batch = batch[:0]
	}
	for {
		select {
		case req := <-r.queue:
			batch = append(batch, req)
			if len(batch) == 1 {
				startTimer()
			}
			if len(batch) >= r.batchSize {
				flush()
			}
		case <-timerC:
			timerC = nil
			flush()
		case <-r.done:
			for {
				select {
				case req := <-r.queue:
					batch = append(batch, req)
					if len(batch) == 1 {
						startTimer()
					}
					if len(batch) >= r.batchSize {
						flush()
					}
				default:
					flush()
					return
				}
			}
		}
	}
}
