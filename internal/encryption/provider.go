package encryption

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/nosway/namros/internal/meta/model"
	"github.com/nosway/namros/internal/storage"
)

const (
	EnvelopeAlgorithmAES256GCM = "AES256-GCM"
	DefaultKMSKeyID            = "namros/default"
)

var (
	ErrProviderUnavailable = errors.New("encryption provider unavailable")
	ErrKeyUnavailable      = errors.New("kms key unavailable")
	ErrDecryptDenied       = errors.New("kms key state does not allow decrypt")
)

type Provider interface {
	EncryptSegment(ctx context.Context, req EncryptSegmentRequest) (EncryptSegmentResult, error)
	DecryptSegment(ctx context.Context, req DecryptSegmentRequest) (io.ReadCloser, error)
	KeyState(ctx context.Context, keyID string) (model.KMSKeyRecord, error)
}

type EncryptSegmentRequest struct {
	Plaintext     io.Reader
	PlaintextSize uint64
	Encryption    model.ServerSideEncryption
	Context       map[string]string
}

type EncryptSegmentResult struct {
	Ciphertext io.Reader
	SizeBytes  uint64
	Envelope   storage.EncryptionEnvelope
	Encryption model.ServerSideEncryption
}

type DecryptSegmentRequest struct {
	Ciphertext io.Reader
	Envelope   storage.EncryptionEnvelope
}

type LocalProvider struct {
	keys map[string]localKey
}

type localKey struct {
	keyID      string
	keyVersion string
	key        []byte
	state      model.KMSKeyState
}

func NewLocalProvider(keys map[string][]byte) (*LocalProvider, error) {
	if len(keys) == 0 {
		key := make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
		keys = map[string][]byte{DefaultKMSKeyID: key}
	}
	out := &LocalProvider{keys: make(map[string]localKey, len(keys))}
	for keyID, key := range keys {
		keyID = strings.TrimSpace(keyID)
		if keyID == "" {
			return nil, fmt.Errorf("kms key id is required")
		}
		if len(key) != 32 {
			return nil, fmt.Errorf("kms key %q must be 32 bytes", keyID)
		}
		out.keys[keyID] = localKey{
			keyID:      keyID,
			keyVersion: "local-v1",
			key:        append([]byte(nil), key...),
			state:      model.KMSKeyActive,
		}
	}
	return out, nil
}

func (p *LocalProvider) EncryptSegment(ctx context.Context, req EncryptSegmentRequest) (EncryptSegmentResult, error) {
	if err := ctx.Err(); err != nil {
		return EncryptSegmentResult{}, err
	}
	keyID := strings.TrimSpace(req.Encryption.KeyID)
	if keyID == "" {
		keyID = DefaultKMSKeyID
	}
	kmsKey, ok := p.keys[keyID]
	if !ok || model.NormalizeKMSKeyState(kmsKey.state) != model.KMSKeyActive {
		return EncryptSegmentResult{}, ErrKeyUnavailable
	}
	plaintext, err := io.ReadAll(req.Plaintext)
	if err != nil {
		return EncryptSegmentResult{}, err
	}
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return EncryptSegmentResult{}, err
	}
	wrappedDEK, err := sealAESGCM(kmsKey.key, dek, []byte(keyID))
	if err != nil {
		return EncryptSegmentResult{}, err
	}
	ciphertext, nonce, err := sealAESGCMWithNonce(dek, plaintext, contextAAD(req.Context))
	if err != nil {
		return EncryptSegmentResult{}, err
	}
	encryption := req.Encryption
	encryption.KeyID = keyID
	encryption.KeyVersion = kmsKey.keyVersion
	return EncryptSegmentResult{
		Ciphertext: bytes.NewReader(ciphertext),
		SizeBytes:  uint64(len(ciphertext)),
		Encryption: encryption,
		Envelope: storage.EncryptionEnvelope{
			Algorithm:           EnvelopeAlgorithmAES256GCM,
			KeyID:               keyID,
			KeyVersion:          kmsKey.keyVersion,
			WrappedDEK:          base64.StdEncoding.EncodeToString(wrappedDEK),
			Nonce:               base64.StdEncoding.EncodeToString(nonce),
			PlaintextSizeBytes:  uint64(len(plaintext)),
			CiphertextSizeBytes: uint64(len(ciphertext)),
			Context:             cloneStringMap(req.Context),
		},
	}, nil
}

func (p *LocalProvider) DecryptSegment(ctx context.Context, req DecryptSegmentRequest) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	keyID := strings.TrimSpace(req.Envelope.KeyID)
	kmsKey, ok := p.keys[keyID]
	if !ok {
		return nil, ErrKeyUnavailable
	}
	if !model.KMSKeyAllowsDecrypt(kmsKey.state) {
		return nil, ErrDecryptDenied
	}
	wrappedDEK, err := base64.StdEncoding.DecodeString(req.Envelope.WrappedDEK)
	if err != nil {
		return nil, err
	}
	dek, err := openAESGCM(kmsKey.key, wrappedDEK, []byte(keyID))
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(req.Envelope.Nonce)
	if err != nil {
		return nil, err
	}
	ciphertext, err := io.ReadAll(req.Ciphertext)
	if err != nil {
		return nil, err
	}
	plaintext, err := openAESGCMWithNonce(dek, nonce, ciphertext, contextAAD(req.Envelope.Context))
	if err != nil {
		return nil, err
	}
	return io.NopCloser(bytes.NewReader(plaintext)), nil
}

func (p *LocalProvider) KeyState(_ context.Context, keyID string) (model.KMSKeyRecord, error) {
	keyID = strings.TrimSpace(keyID)
	if keyID == "" {
		keyID = DefaultKMSKeyID
	}
	kmsKey, ok := p.keys[keyID]
	if !ok {
		return model.KMSKeyRecord{}, ErrKeyUnavailable
	}
	return model.KMSKeyRecord{
		KeyID:      kmsKey.keyID,
		KeyVersion: kmsKey.keyVersion,
		State:      kmsKey.state,
	}, nil
}

func sealAESGCM(key, plaintext, aad []byte) ([]byte, error) {
	ciphertext, nonce, err := sealAESGCMWithNonce(key, plaintext, aad)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 0, len(nonce)+len(ciphertext))
	out = append(out, nonce...)
	out = append(out, ciphertext...)
	return out, nil
}

func openAESGCM(key, sealed, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	if len(sealed) < gcm.NonceSize() {
		return nil, errors.New("sealed payload is shorter than nonce")
	}
	nonce := sealed[:gcm.NonceSize()]
	ciphertext := sealed[gcm.NonceSize():]
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func sealAESGCMWithNonce(key, plaintext, aad []byte) ([]byte, []byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, err
	}
	return gcm.Seal(nil, nonce, plaintext, aad), nonce, nil
}

func openAESGCMWithNonce(key, nonce, ciphertext, aad []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	return gcm.Open(nil, nonce, ciphertext, aad)
}

func contextAAD(context map[string]string) []byte {
	if len(context) == 0 {
		return nil
	}
	keys := make([]string, 0, len(context))
	for key := range context {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, key := range keys {
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(context[key])
		b.WriteByte('\n')
	}
	return []byte(b.String())
}

func cloneStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}
