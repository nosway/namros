package sigv4

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nosway/namros/internal/auth"
	"github.com/nosway/namros/internal/auth/credentials"
)

var (
	ErrAccessDenied          = errors.New("access denied")
	ErrSignatureDoesNotMatch = errors.New("signature does not match")
	ErrInvalidRequest        = errors.New("invalid request")
	ErrRequestTimeTooSkewed  = errors.New("request time too skewed")
	ErrExpiredPresign        = errors.New("presigned URL has expired")
)

type Config struct {
	Region      string
	Service     string
	Credentials credentials.Store
	Now         func() time.Time
	MaxSkew     time.Duration
}

type Verifier struct {
	region      string
	service     string
	credentials credentials.Store
	now         func() time.Time
	maxSkew     time.Duration
}

type Result struct {
	Principal        auth.Principal
	AccessKeyID      string
	Presigned        bool
	RequestTime      time.Time
	CanonicalRequest string
	StringToSign     string
	SignedHeaders    string
}

func NewVerifier(cfg Config) *Verifier {
	service := cfg.Service
	if service == "" {
		service = "s3"
	}
	now := cfg.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}
	maxSkew := cfg.MaxSkew
	if maxSkew == 0 {
		maxSkew = 15 * time.Minute
	}
	return &Verifier{
		region:      cfg.Region,
		service:     service,
		credentials: cfg.Credentials,
		now:         now,
		maxSkew:     maxSkew,
	}
}

func (v *Verifier) Verify(ctx context.Context, r *http.Request) (Result, error) {
	if r == nil || r.URL == nil {
		return Result{}, fmt.Errorf("%w: request is required", ErrInvalidRequest)
	}
	if v == nil || v.credentials == nil {
		return Result{}, fmt.Errorf("%w: credential resolver is not configured", ErrAccessDenied)
	}
	if isPresigned(r.URL.Query()) {
		return v.verifyPresigned(ctx, r)
	}
	return v.verifyHeader(ctx, r)
}

func (v *Verifier) verifyHeader(ctx context.Context, r *http.Request) (Result, error) {
	scope, signedHeaders, signature, err := parseAuthorizationHeader(r.Header.Get(authorizationHeader))
	if err != nil {
		return Result{}, err
	}
	if err := validateScope(scope, v.region); err != nil {
		return Result{}, err
	}
	if err := wrapSignatureFormatError(signature); err != nil {
		return Result{}, err
	}
	requestTime, err := parseAMZDate(r.Header.Get(amzDateHeader))
	if err != nil {
		return Result{}, err
	}
	if scope.Date != requestTime.Format("20060102") {
		return Result{}, fmt.Errorf("%w: credential date does not match X-Amz-Date", ErrSignatureDoesNotMatch)
	}
	if err := v.validateClockSkew(requestTime); err != nil {
		return Result{}, err
	}
	return v.verifySignature(ctx, r, signatureInput{
		scope:            scope,
		signedHeaders:    signedHeaders,
		signature:        signature,
		requestTime:      requestTime,
		payloadHash:      requestPayloadHash(r),
		excludeSignature: false,
		presigned:        false,
	})
}

func (v *Verifier) verifyPresigned(ctx context.Context, r *http.Request) (Result, error) {
	values := r.URL.Query()
	scope, signedHeaders, signature, expires, err := parsePresignedQuery(values)
	if err != nil {
		return Result{}, err
	}
	if err := validateScope(scope, v.region); err != nil {
		return Result{}, err
	}
	if err := wrapSignatureFormatError(signature); err != nil {
		return Result{}, err
	}
	requestTime, err := parseAMZDate(values.Get(queryDate))
	if err != nil {
		return Result{}, err
	}
	if scope.Date != requestTime.Format("20060102") {
		return Result{}, fmt.Errorf("%w: credential date does not match X-Amz-Date", ErrSignatureDoesNotMatch)
	}
	now := v.now()
	if now.Before(requestTime.Add(-v.maxSkew)) {
		return Result{}, fmt.Errorf("%w: request time is in the future", ErrRequestTimeTooSkewed)
	}
	if now.After(requestTime.Add(expires)) {
		return Result{}, fmt.Errorf("%w: presigned URL expired", ErrExpiredPresign)
	}
	return v.verifySignature(ctx, r, signatureInput{
		scope:            scope,
		signedHeaders:    signedHeaders,
		signature:        signature,
		requestTime:      requestTime,
		payloadHash:      presignedPayloadHash(values),
		excludeSignature: true,
		presigned:        true,
	})
}

type signatureInput struct {
	scope            credentialScope
	signedHeaders    []string
	signature        string
	requestTime      time.Time
	payloadHash      string
	excludeSignature bool
	presigned        bool
}

func (v *Verifier) verifySignature(ctx context.Context, r *http.Request, input signatureInput) (Result, error) {
	cred, err := v.credentials.LookupAccessKey(ctx, input.scope.AccessKeyID)
	if err != nil {
		if errors.Is(err, credentials.ErrCredentialNotFound) {
			return Result{}, fmt.Errorf("%w: credential not found", ErrAccessDenied)
		}
		return Result{}, err
	}
	canonicalRequest, signedHeaderText, err := buildCanonicalRequest(r, canonicalOptions{
		SignedHeaders:         input.signedHeaders,
		PayloadHash:           input.payloadHash,
		ExcludeQuerySignature: input.excludeSignature,
	})
	if err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalidRequest, err)
	}
	stringToSign := buildStringToSign(input.requestTime, input.scope, canonicalRequest)
	expected := hex.EncodeToString(hmacSHA256(deriveSigningKey(cred.SecretAccessKey, input.scope.Date, input.scope.Region, input.scope.Service), stringToSign))
	if !hmac.Equal([]byte(expected), []byte(input.signature)) {
		return Result{}, fmt.Errorf("%w: computed signature did not match", ErrSignatureDoesNotMatch)
	}
	return Result{
		Principal:        cred.Principal,
		AccessKeyID:      cred.AccessKeyID,
		Presigned:        input.presigned,
		RequestTime:      input.requestTime,
		CanonicalRequest: canonicalRequest,
		StringToSign:     stringToSign,
		SignedHeaders:    signedHeaderText,
	}, nil
}

func (v *Verifier) validateClockSkew(requestTime time.Time) error {
	now := v.now()
	if requestTime.Before(now.Add(-v.maxSkew)) || requestTime.After(now.Add(v.maxSkew)) {
		return fmt.Errorf("%w: X-Amz-Date is outside allowed skew", ErrRequestTimeTooSkewed)
	}
	return nil
}

func buildStringToSign(requestTime time.Time, scope credentialScope, canonicalRequest string) string {
	return algorithm + "\n" +
		requestTime.Format(amzDateLayout) + "\n" +
		scope.Date + "/" + scope.Region + "/" + scope.Service + "/" + scope.Terminator + "\n" +
		sha256Hex(canonicalRequest)
}

func deriveSigningKey(secret, date, region, service string) []byte {
	kDate := hmacSHA256([]byte("AWS4"+secret), date)
	kRegion := hmacSHA256(kDate, region)
	kService := hmacSHA256(kRegion, service)
	return hmacSHA256(kService, scopeTerminator)
}

func hmacSHA256(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}
