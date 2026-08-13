package meta

import "errors"

var (
	ErrNotFound               = errors.New("not found")
	ErrAlreadyExists          = errors.New("already exists")
	ErrBucketNotEmpty         = errors.New("bucket not empty")
	ErrCASConflict            = errors.New("cas conflict")
	ErrInvalidArgument        = errors.New("invalid argument")
	ErrObjectManifestTooLarge = errors.New("object manifest too large")
	ErrObjectLocked           = errors.New("object is protected by object lock")
	ErrKMSKeyUnavailable      = errors.New("kms key unavailable")
	ErrQuotaExceeded          = errors.New("quota exceeded")
	ErrUnavailable            = errors.New("metadata unavailable")
	ErrWorkerPaused           = errors.New("worker paused")
	ErrWorkerCanceled         = errors.New("worker canceled")
)
