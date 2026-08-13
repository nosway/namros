package credentials

import (
	"context"
	"errors"

	"github.com/nosway/namros/internal/auth"
)

const DefaultHashIterations = 100000

var ErrCredentialNotFound = errors.New("credential not found")

type Credential struct {
	AccessKeyID     string
	SecretAccessKey string
	SecretHash      string
	Principal       auth.Principal
	Active          bool
}

type Store interface {
	LookupAccessKey(ctx context.Context, accessKeyID string) (Credential, error)
}

type StaticStore struct {
	credentials map[string]Credential
}

func NewRootCredential(accessKeyID, secretAccessKey string) (Credential, error) {
	secretHash, err := HashSecret(secretAccessKey)
	if err != nil {
		return Credential{}, err
	}
	return Credential{
		AccessKeyID:     accessKeyID,
		SecretAccessKey: secretAccessKey,
		SecretHash:      secretHash,
		Active:          true,
		Principal: auth.Principal{
			TenantID:    "root",
			AccessKeyID: accessKeyID,
			DisplayName: "root",
			Root:        true,
		},
	}, nil
}

func NewStaticStore(creds ...Credential) (*StaticStore, error) {
	store := &StaticStore{credentials: make(map[string]Credential, len(creds))}
	for _, cred := range creds {
		if cred.AccessKeyID == "" {
			return nil, errors.New("access key id is required")
		}
		if cred.SecretAccessKey == "" {
			return nil, errors.New("secret access key is required")
		}
		if _, exists := store.credentials[cred.AccessKeyID]; exists {
			return nil, errors.New("duplicate access key id")
		}
		if cred.Principal.AccessKeyID == "" {
			cred.Principal.AccessKeyID = cred.AccessKeyID
		}
		cred.Principal = cred.Principal.Clone()
		store.credentials[cred.AccessKeyID] = cred
	}
	return store, nil
}

func (s *StaticStore) LookupAccessKey(_ context.Context, accessKeyID string) (Credential, error) {
	if s == nil {
		return Credential{}, ErrCredentialNotFound
	}
	cred, ok := s.credentials[accessKeyID]
	if !ok || !cred.Active {
		return Credential{}, ErrCredentialNotFound
	}
	cred.Principal = cred.Principal.Clone()
	return cred, nil
}
