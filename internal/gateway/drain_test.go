package gateway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/nosway/namros/internal/s3api/routing"
)

func TestGatewayDrainWaitsForInFlightWrites(t *testing.T) {
	drain := NewGatewayDrainController()
	release, err := drain.Admit(routing.OperationPutObject)
	if err != nil {
		t.Fatalf("Admit(PutObject) error = %v", err)
	}

	status := drain.StartDrain()
	if status.State != GatewayDrainDraining || status.InFlightWrites != 1 {
		t.Fatalf("StartDrain() status = %+v, want draining with one in-flight write", status)
	}

	if _, err := drain.Admit(routing.OperationPutObject); !errors.Is(err, ErrGatewayDraining) {
		t.Fatalf("Admit(PutObject after drain) error = %v, want ErrGatewayDraining", err)
	}
	if releaseRead, err := drain.Admit(routing.OperationGetObject); err != nil || releaseRead != nil {
		t.Fatalf("Admit(GetObject) release nil = %v error = %v, want allowed untracked read", releaseRead == nil, err)
	}

	timeoutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	if err := drain.WaitDrained(timeoutCtx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("WaitDrained(with in-flight write) error = %v, want deadline exceeded", err)
	}
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- drain.WaitDrained(context.Background())
	}()
	release()
	release()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("WaitDrained() error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("WaitDrained() did not return after final write release")
	}

	status = drain.Status()
	if status.State != GatewayDrainDrained || status.InFlightWrites != 0 {
		t.Fatalf("Status() = %+v, want drained with zero in-flight writes", status)
	}
}

func TestGatewayDrainWithoutInFlightWritesDrainsImmediately(t *testing.T) {
	drain := NewGatewayDrainController()

	status := drain.StartDrain()
	if status.State != GatewayDrainDrained || status.InFlightWrites != 0 {
		t.Fatalf("StartDrain() status = %+v, want drained with zero in-flight writes", status)
	}
	if err := drain.WaitDrained(context.Background()); err != nil {
		t.Fatalf("WaitDrained() error = %v", err)
	}
}

func TestGatewayDrainWaitBeforeStartReturnsError(t *testing.T) {
	drain := NewGatewayDrainController()

	if err := drain.WaitDrained(context.Background()); !errors.Is(err, ErrGatewayDrainNotStarted) {
		t.Fatalf("WaitDrained() error = %v, want ErrGatewayDrainNotStarted", err)
	}
}

func TestGatewayDrainWriteOperationClassification(t *testing.T) {
	for _, op := range []routing.Operation{
		routing.OperationPutObject,
		routing.OperationCopyObject,
		routing.OperationDeleteObject,
		routing.OperationUploadPart,
		routing.OperationCompleteMultipart,
		routing.OperationPutBucketPolicy,
	} {
		if !isGatewayDrainWriteOperation(op) {
			t.Fatalf("%s classified as read operation, want write", op)
		}
	}
	for _, op := range []routing.Operation{
		routing.OperationListBuckets,
		routing.OperationListObjectsV2,
		routing.OperationGetObject,
		routing.OperationHeadObject,
		routing.OperationGetBucketPolicy,
		routing.OperationUnsupported,
	} {
		if isGatewayDrainWriteOperation(op) {
			t.Fatalf("%s classified as write operation, want read/non-mutating", op)
		}
	}
}
