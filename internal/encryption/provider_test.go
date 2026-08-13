package encryption

import (
	"bytes"
	"io"
	"testing"

	"github.com/nosway/namros/internal/meta/model"
)

func TestLocalProviderEncryptDecryptSegment(t *testing.T) {
	key := bytes.Repeat([]byte{7}, 32)
	provider, err := NewLocalProvider(map[string][]byte{"kms-key": key})
	if err != nil {
		t.Fatalf("NewLocalProvider() error = %v", err)
	}
	result, err := provider.EncryptSegment(t.Context(), EncryptSegmentRequest{
		Plaintext:     bytes.NewReader([]byte("hello encrypted world")),
		PlaintextSize: 21,
		Encryption: model.ServerSideEncryption{
			Algorithm: model.ServerSideEncryptionAWSKMS,
			KeyID:     "kms-key",
		},
		Context: map[string]string{"bucket": "b", "key": "k"},
	})
	if err != nil {
		t.Fatalf("EncryptSegment() error = %v", err)
	}
	if result.Envelope.WrappedDEK == "" || result.Envelope.Nonce == "" || result.SizeBytes == 0 {
		t.Fatalf("envelope/result not populated: %+v", result)
	}
	plaintext, err := provider.DecryptSegment(t.Context(), DecryptSegmentRequest{
		Ciphertext: result.Ciphertext,
		Envelope:   result.Envelope,
	})
	if err != nil {
		t.Fatalf("DecryptSegment() error = %v", err)
	}
	defer plaintext.Close()
	got, err := io.ReadAll(plaintext)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "hello encrypted world" {
		t.Fatalf("plaintext = %q", got)
	}
}
