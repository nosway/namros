package sigv4

import (
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	amzDateHeader       = "X-Amz-Date"
	contentSHA256Header = "X-Amz-Content-Sha256"
	authorizationHeader = "Authorization"
	queryAlgorithm      = "X-Amz-Algorithm"
	queryCredential     = "X-Amz-Credential"
	queryDate           = "X-Amz-Date"
	queryExpires        = "X-Amz-Expires"
	querySignedHeaders  = "X-Amz-SignedHeaders"
	querySignature      = "X-Amz-Signature"
	queryContentSHA256  = "X-Amz-Content-Sha256"
	maxPresignExpiry    = 7 * 24 * time.Hour
	amzDateLayout       = "20060102T150405Z"
)

type credentialScope struct {
	AccessKeyID string
	Date        string
	Region      string
	Service     string
	Terminator  string
}

func parseAuthorizationHeader(header string) (credentialScope, []string, string, error) {
	if header == "" {
		return credentialScope{}, nil, "", fmt.Errorf("%w: Authorization header is required", ErrAccessDenied)
	}
	algorithmText, rest, ok := strings.Cut(header, " ")
	if !ok || algorithmText != algorithm {
		return credentialScope{}, nil, "", fmt.Errorf("%w: unsupported authorization algorithm", ErrInvalidRequest)
	}
	fields := make(map[string]string)
	for _, item := range strings.Split(rest, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(item), "=")
		if !ok {
			return credentialScope{}, nil, "", fmt.Errorf("%w: malformed Authorization header", ErrInvalidRequest)
		}
		fields[key] = value
	}
	scope, err := parseCredentialScope(fields["Credential"])
	if err != nil {
		return credentialScope{}, nil, "", err
	}
	signedHeaders := strings.Split(fields["SignedHeaders"], ";")
	signature := fields["Signature"]
	if fields["SignedHeaders"] == "" || signature == "" {
		return credentialScope{}, nil, "", fmt.Errorf("%w: SignedHeaders and Signature are required", ErrInvalidRequest)
	}
	return scope, signedHeaders, signature, nil
}

func parsePresignedQuery(values url.Values) (credentialScope, []string, string, time.Duration, error) {
	if values.Get(queryAlgorithm) != algorithm {
		return credentialScope{}, nil, "", 0, fmt.Errorf("%w: unsupported presign algorithm", ErrInvalidRequest)
	}
	scope, err := parseCredentialScope(values.Get(queryCredential))
	if err != nil {
		return credentialScope{}, nil, "", 0, err
	}
	signedHeadersText := values.Get(querySignedHeaders)
	signature := values.Get(querySignature)
	if signedHeadersText == "" || signature == "" {
		return credentialScope{}, nil, "", 0, fmt.Errorf("%w: presigned URL is missing signed headers or signature", ErrInvalidRequest)
	}
	expiresSeconds, err := strconv.Atoi(values.Get(queryExpires))
	if err != nil || expiresSeconds <= 0 {
		return credentialScope{}, nil, "", 0, fmt.Errorf("%w: invalid presign expiry", ErrInvalidRequest)
	}
	expires := time.Duration(expiresSeconds) * time.Second
	if expires > maxPresignExpiry {
		return credentialScope{}, nil, "", 0, fmt.Errorf("%w: presign expiry exceeds seven days", ErrInvalidRequest)
	}
	return scope, strings.Split(signedHeadersText, ";"), signature, expires, nil
}

func parseCredentialScope(value string) (credentialScope, error) {
	parts := strings.Split(value, "/")
	if len(parts) != 5 || parts[0] == "" {
		return credentialScope{}, fmt.Errorf("%w: malformed credential scope", ErrInvalidRequest)
	}
	return credentialScope{
		AccessKeyID: parts[0],
		Date:        parts[1],
		Region:      parts[2],
		Service:     parts[3],
		Terminator:  parts[4],
	}, nil
}

func parseAMZDate(value string) (time.Time, error) {
	if value == "" {
		return time.Time{}, fmt.Errorf("%w: X-Amz-Date is required", ErrAccessDenied)
	}
	t, err := time.Parse(amzDateLayout, value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%w: invalid X-Amz-Date", ErrInvalidRequest)
	}
	return t, nil
}

func requestPayloadHash(r *http.Request) string {
	if value := r.Header.Get(contentSHA256Header); value != "" {
		return value
	}
	return unsignedPayload
}

func presignedPayloadHash(values url.Values) string {
	if value := values.Get(queryContentSHA256); value != "" {
		return value
	}
	return unsignedPayload
}

func isPresigned(values url.Values) bool {
	return values.Get(querySignature) != ""
}

func validateScope(scope credentialScope, region string) error {
	if scope.Date == "" || scope.Region != region || scope.Service != "s3" || scope.Terminator != scopeTerminator {
		return fmt.Errorf("%w: credential scope does not match this endpoint", ErrSignatureDoesNotMatch)
	}
	return nil
}

func isHexSignature(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !strings.ContainsRune("0123456789abcdefABCDEF", r) {
			return false
		}
	}
	return true
}

func wrapSignatureFormatError(signature string) error {
	if !isHexSignature(signature) {
		return fmt.Errorf("%w: malformed signature", ErrInvalidRequest)
	}
	return nil
}
