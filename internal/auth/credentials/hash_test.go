package credentials

import (
	"errors"
	"strings"
	"testing"
)

func TestSecretHashRoundTrip(t *testing.T) {
	hash, err := HashSecretWithSalt("test-secret", []byte("0123456789abcdef"), 1000)
	if err != nil {
		t.Fatalf("HashSecretWithSalt() error = %v", err)
	}
	if !strings.HasPrefix(hash, "namros-pbkdf2-sha256$v=1$i=1000$s=") {
		t.Fatalf("hash format = %q", hash)
	}
	if err := VerifySecretHash("test-secret", hash); err != nil {
		t.Fatalf("VerifySecretHash() error = %v", err)
	}
	if err := VerifySecretHash("wrong-secret", hash); !errors.Is(err, ErrSecretMismatch) {
		t.Fatalf("VerifySecretHash(wrong) error = %v, want ErrSecretMismatch", err)
	}
}

func TestStaticStoreLookup(t *testing.T) {
	cred, err := NewRootCredential("root-access", "root-secret")
	if err != nil {
		t.Fatalf("NewRootCredential() error = %v", err)
	}
	store, err := NewStaticStore(cred)
	if err != nil {
		t.Fatalf("NewStaticStore() error = %v", err)
	}
	got, err := store.LookupAccessKey(t.Context(), "root-access")
	if err != nil {
		t.Fatalf("LookupAccessKey() error = %v", err)
	}
	if !got.Principal.Root || got.Principal.AccessKeyID != "root-access" {
		t.Fatalf("principal = %+v", got.Principal)
	}
	if _, err := store.LookupAccessKey(t.Context(), "missing"); !errors.Is(err, ErrCredentialNotFound) {
		t.Fatalf("LookupAccessKey(missing) error = %v, want ErrCredentialNotFound", err)
	}
}
