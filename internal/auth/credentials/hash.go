package credentials

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/pbkdf2"
)

const (
	hashAlgorithm = "namros-pbkdf2-sha256"
	hashVersion   = "v=1"
	hashKeyLength = 32
)

var (
	ErrInvalidSecretHash = errors.New("invalid secret hash")
	ErrSecretMismatch    = errors.New("secret does not match hash")
)

func HashSecret(secret string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	return HashSecretWithSalt(secret, salt, DefaultHashIterations)
}

func HashSecretWithSalt(secret string, salt []byte, iterations int) (string, error) {
	if secret == "" {
		return "", errors.New("secret is required")
	}
	if len(salt) == 0 {
		return "", errors.New("salt is required")
	}
	if iterations <= 0 {
		return "", errors.New("iterations must be positive")
	}
	digest := pbkdf2.Key([]byte(secret), salt, iterations, hashKeyLength, sha256.New)
	return strings.Join([]string{
		hashAlgorithm,
		hashVersion,
		"i=" + strconv.Itoa(iterations),
		"s=" + base64.RawStdEncoding.EncodeToString(salt),
		"h=" + base64.RawStdEncoding.EncodeToString(digest),
	}, "$"), nil
}

func VerifySecretHash(secret, encoded string) error {
	params, err := parseSecretHash(encoded)
	if err != nil {
		return err
	}
	digest := pbkdf2.Key([]byte(secret), params.salt, params.iterations, len(params.digest), sha256.New)
	if subtle.ConstantTimeCompare(digest, params.digest) != 1 {
		return ErrSecretMismatch
	}
	return nil
}

type secretHashParams struct {
	iterations int
	salt       []byte
	digest     []byte
}

func parseSecretHash(encoded string) (secretHashParams, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 5 || parts[0] != hashAlgorithm || parts[1] != hashVersion {
		return secretHashParams{}, ErrInvalidSecretHash
	}
	iterationsText, ok := strings.CutPrefix(parts[2], "i=")
	if !ok {
		return secretHashParams{}, ErrInvalidSecretHash
	}
	iterations, err := strconv.Atoi(iterationsText)
	if err != nil || iterations <= 0 {
		return secretHashParams{}, ErrInvalidSecretHash
	}
	saltText, ok := strings.CutPrefix(parts[3], "s=")
	if !ok {
		return secretHashParams{}, ErrInvalidSecretHash
	}
	salt, err := base64.RawStdEncoding.DecodeString(saltText)
	if err != nil || len(salt) == 0 {
		return secretHashParams{}, ErrInvalidSecretHash
	}
	digestText, ok := strings.CutPrefix(parts[4], "h=")
	if !ok {
		return secretHashParams{}, ErrInvalidSecretHash
	}
	digest, err := base64.RawStdEncoding.DecodeString(digestText)
	if err != nil || len(digest) == 0 {
		return secretHashParams{}, ErrInvalidSecretHash
	}
	if len(digest) < 16 {
		return secretHashParams{}, fmt.Errorf("%w: digest is too short", ErrInvalidSecretHash)
	}
	return secretHashParams{iterations: iterations, salt: salt, digest: digest}, nil
}
