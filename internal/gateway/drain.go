package gateway

import (
	"context"
	"errors"
	"sync"

	"github.com/nosway/namros/internal/s3api/routing"
)

type GatewayDrainState string

const (
	GatewayDrainActive   GatewayDrainState = "active"
	GatewayDrainDraining GatewayDrainState = "draining"
	GatewayDrainDrained  GatewayDrainState = "drained"
)

var (
	ErrGatewayDraining        = errors.New("gateway is draining")
	ErrGatewayDrainNotStarted = errors.New("gateway drain has not started")
)

type GatewayDrainStatus struct {
	State          GatewayDrainState `json:"state"`
	InFlightWrites int               `json:"in_flight_writes"`
}

type GatewayDrainController struct {
	mu             sync.Mutex
	state          GatewayDrainState
	inFlightWrites int
	drained        chan struct{}
}

func NewGatewayDrainController() *GatewayDrainController {
	return &GatewayDrainController{
		state:   GatewayDrainActive,
		drained: make(chan struct{}),
	}
}

func (d *GatewayDrainController) Admit(op routing.Operation) (func(), error) {
	if d == nil || !isGatewayDrainWriteOperation(op) {
		return nil, nil
	}

	d.mu.Lock()
	if d.state != GatewayDrainActive {
		d.mu.Unlock()
		return nil, ErrGatewayDraining
	}
	d.inFlightWrites++
	d.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(d.finishWrite)
	}, nil
}

func (d *GatewayDrainController) StartDrain() GatewayDrainStatus {
	if d == nil {
		return GatewayDrainStatus{State: GatewayDrainDrained}
	}

	d.mu.Lock()
	defer d.mu.Unlock()

	if d.state == GatewayDrainActive {
		d.state = GatewayDrainDraining
	}
	if d.state == GatewayDrainDraining && d.inFlightWrites == 0 {
		d.markDrainedLocked()
	}
	return d.statusLocked()
}

func (d *GatewayDrainController) WaitDrained(ctx context.Context) error {
	if d == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	d.mu.Lock()
	if d.state == GatewayDrainActive {
		d.mu.Unlock()
		return ErrGatewayDrainNotStarted
	}
	if d.state == GatewayDrainDrained {
		d.mu.Unlock()
		return nil
	}
	drained := d.drained
	d.mu.Unlock()

	select {
	case <-drained:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *GatewayDrainController) Status() GatewayDrainStatus {
	if d == nil {
		return GatewayDrainStatus{State: GatewayDrainActive}
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	return d.statusLocked()
}

func (d *GatewayDrainController) finishWrite() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.inFlightWrites > 0 {
		d.inFlightWrites--
	}
	if d.state == GatewayDrainDraining && d.inFlightWrites == 0 {
		d.markDrainedLocked()
	}
}

func (d *GatewayDrainController) markDrainedLocked() {
	if d.state == GatewayDrainDrained {
		return
	}
	d.state = GatewayDrainDrained
	close(d.drained)
}

func (d *GatewayDrainController) statusLocked() GatewayDrainStatus {
	return GatewayDrainStatus{
		State:          d.state,
		InFlightWrites: d.inFlightWrites,
	}
}

func isGatewayDrainWriteOperation(op routing.Operation) bool {
	switch op {
	case routing.OperationCreateBucket,
		routing.OperationDeleteBucket,
		routing.OperationPutBucketVersioning,
		routing.OperationPutBucketCORS,
		routing.OperationDeleteBucketCORS,
		routing.OperationPutBucketLifecycle,
		routing.OperationDeleteBucketLifecycle,
		routing.OperationPutBucketObjectLock,
		routing.OperationPutBucketPolicy,
		routing.OperationDeleteBucketPolicy,
		routing.OperationPutBucketEncryption,
		routing.OperationDeleteBucketEncryption,
		routing.OperationPutBucketACL,
		routing.OperationPutObject,
		routing.OperationCopyObject,
		routing.OperationDeleteObject,
		routing.OperationDeleteObjects,
		routing.OperationPutObjectTagging,
		routing.OperationDeleteObjectTagging,
		routing.OperationPutObjectRetention,
		routing.OperationPutObjectLegalHold,
		routing.OperationPutObjectACL,
		routing.OperationCreateMultipartUpload,
		routing.OperationUploadPart,
		routing.OperationCompleteMultipart,
		routing.OperationAbortMultipart:
		return true
	default:
		return false
	}
}
