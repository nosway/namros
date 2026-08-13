package meta

import (
	"fmt"

	"github.com/nosway/namros/internal/meta/model"
)

func ValidateBucketEncryption(encryption model.ServerSideEncryption) error {
	switch encryption.Algorithm {
	case model.ServerSideEncryptionAES256:
		if encryption.KeyID != "" || encryption.KeyVersion != "" {
			return fmt.Errorf("%w: AES256 bucket encryption cannot include a KMS key", ErrInvalidArgument)
		}
		return nil
	case model.ServerSideEncryptionAWSKMS:
		return nil
	default:
		return fmt.Errorf("%w: bucket encryption algorithm must be AES256 or aws:kms", ErrInvalidArgument)
	}
}
