package gateway

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/nosway/namros/internal/meta"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/meta/model"
)

func TestAsyncAccessAuditRecorderBatchesEvents(t *testing.T) {
	repo := &countingAuditRepository{Repository: memory.New()}
	recorder := newAsyncAccessAuditRecorder(repo, accessAuditConfig{
		BatchSize:     3,
		QueueSize:     8,
		FlushInterval: time.Hour,
	})

	for i, action := range []model.AuditAction{
		model.AuditActionGetObject,
		model.AuditActionHeadObject,
		model.AuditActionListObjects,
		model.AuditActionGetObject,
		model.AuditActionHeadObject,
	} {
		if err := recorder.RecordAccessAudit(t.Context(), meta.PutAdminAuditEventRequest{
			Action:    action,
			BucketID:  "bucket-1",
			Key:       "object.txt",
			VersionID: "version-1",
			Details: map[string]string{
				"idx": string(rune('0' + i)),
			},
		}); err != nil {
			t.Fatalf("RecordAccessAudit(%d) error = %v", i, err)
		}
	}
	if err := recorder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	batchCalls, singleCalls := repo.calls()
	if batchCalls != 2 || singleCalls != 0 {
		t.Fatalf("audit write calls = batch:%d single:%d, want batch:2 single:0", batchCalls, singleCalls)
	}
	events, err := repo.ListAuditEvents(t.Context(), meta.ListAuditEventsRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListAuditEvents() error = %v", err)
	}
	if len(events) != 5 {
		t.Fatalf("audit event count = %d, want 5: %+v", len(events), events)
	}
	for i := 1; i < len(events); i++ {
		if events[i].PreviousHash != events[i-1].EventHash {
			t.Fatalf("audit chain[%d] previous hash = %q, want %q", i, events[i].PreviousHash, events[i-1].EventHash)
		}
	}
}

func TestAsyncAccessAuditRecorderWritesSynchronouslyAfterClose(t *testing.T) {
	repo := &countingAuditRepository{Repository: memory.New()}
	recorder := newAsyncAccessAuditRecorder(repo, accessAuditConfig{
		BatchSize:     8,
		QueueSize:     8,
		FlushInterval: time.Hour,
	})
	if err := recorder.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if err := recorder.RecordAccessAudit(t.Context(), meta.PutAdminAuditEventRequest{
		Action:   model.AuditActionGetObject,
		BucketID: "bucket-1",
		Key:      "object.txt",
	}); err != nil {
		t.Fatalf("RecordAccessAudit() after close error = %v", err)
	}
	batchCalls, singleCalls := repo.calls()
	if batchCalls != 0 || singleCalls != 1 {
		t.Fatalf("audit write calls = batch:%d single:%d, want batch:0 single:1", batchCalls, singleCalls)
	}
}

func TestAsyncAccessAuditRecorderFlushesAfterInterval(t *testing.T) {
	repo := &countingAuditRepository{Repository: memory.New()}
	recorder := newAsyncAccessAuditRecorder(repo, accessAuditConfig{
		BatchSize:     8,
		QueueSize:     8,
		FlushInterval: 10 * time.Millisecond,
	})
	defer func() {
		if err := recorder.Close(); err != nil {
			t.Fatalf("Close() error = %v", err)
		}
	}()
	if err := recorder.RecordAccessAudit(t.Context(), meta.PutAdminAuditEventRequest{
		Action:   model.AuditActionHeadObject,
		BucketID: "bucket-1",
		Key:      "object.txt",
	}); err != nil {
		t.Fatalf("RecordAccessAudit() error = %v", err)
	}
	deadline := time.After(time.Second)
	tick := time.NewTicker(5 * time.Millisecond)
	defer tick.Stop()
	for {
		batchCalls, singleCalls := repo.calls()
		if batchCalls == 1 && singleCalls == 0 {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("audit write calls = batch:%d single:%d, want batch:1 single:0", batchCalls, singleCalls)
		case <-tick.C:
		}
	}
}

type countingAuditRepository struct {
	*memory.Repository
	mu          sync.Mutex
	batchCalls  int
	singleCalls int
}

func (r *countingAuditRepository) PutAdminAuditEvent(ctx context.Context, req meta.PutAdminAuditEventRequest) (model.AuditEvent, error) {
	r.mu.Lock()
	r.singleCalls++
	r.mu.Unlock()
	return r.Repository.PutAdminAuditEvent(ctx, req)
}

func (r *countingAuditRepository) PutAdminAuditEvents(ctx context.Context, req meta.PutAdminAuditEventsRequest) ([]model.AuditEvent, error) {
	r.mu.Lock()
	r.batchCalls++
	r.mu.Unlock()
	return r.Repository.PutAdminAuditEvents(ctx, req)
}

func (r *countingAuditRepository) calls() (int, int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.batchCalls, r.singleCalls
}
