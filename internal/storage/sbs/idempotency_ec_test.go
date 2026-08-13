//go:build namros_ec

package sbs

import "testing"

func TestECRequestContextIncludesWriterSession(t *testing.T) {
	a := &ECStore{volumeID: "18a00001", attachmentID: "att-a", generation: 1}
	b := &ECStore{volumeID: "18a00001", attachmentID: "att-b", generation: 1}
	c := &ECStore{volumeID: "18a00001", attachmentID: "att-a", generation: 2}

	keyA := a.requestContext("write-ec", "object-1-0-0").IdempotencyKey
	if keyA == b.requestContext("write-ec", "object-1-0-0").IdempotencyKey {
		t.Fatal("ec idempotency key ignored attachment id")
	}
	if keyA == c.requestContext("write-ec", "object-1-0-0").IdempotencyKey {
		t.Fatal("ec idempotency key ignored generation")
	}
}
