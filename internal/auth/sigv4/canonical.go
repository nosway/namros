package sigv4

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	algorithm          = "AWS4-HMAC-SHA256"
	scopeTerminator    = "aws4_request"
	unsignedPayload    = "UNSIGNED-PAYLOAD"
	emptyPayloadSHA256 = "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
)

type canonicalOptions struct {
	SignedHeaders         []string
	PayloadHash           string
	ExcludeQuerySignature bool
}

func buildCanonicalRequest(r *http.Request, opts canonicalOptions) (string, string, error) {
	if r == nil || r.URL == nil {
		return "", "", errors.New("request URL is required")
	}
	signedHeaders := normalizeSignedHeaders(opts.SignedHeaders)
	if len(signedHeaders) == 0 {
		return "", "", errors.New("signed headers are required")
	}
	if !sort.StringsAreSorted(signedHeaders) {
		return "", "", errors.New("signed headers must be sorted")
	}
	canonicalHeaders, err := buildCanonicalHeaders(r, signedHeaders)
	if err != nil {
		return "", "", err
	}
	payloadHash := opts.PayloadHash
	if payloadHash == "" {
		payloadHash = emptyPayloadSHA256
	}
	signedHeaderText := strings.Join(signedHeaders, ";")
	canonical := strings.Join([]string{
		r.Method,
		canonicalURI(r.URL),
		canonicalQueryString(r.URL, opts.ExcludeQuerySignature),
		canonicalHeaders,
		signedHeaderText,
		payloadHash,
	}, "\n")
	return canonical, signedHeaderText, nil
}

func buildCanonicalHeaders(r *http.Request, signedHeaders []string) (string, error) {
	var b strings.Builder
	for _, name := range signedHeaders {
		value, ok := canonicalHeaderValue(r, name)
		if !ok {
			return "", fmt.Errorf("signed header %q is missing", name)
		}
		b.WriteString(name)
		b.WriteByte(':')
		b.WriteString(value)
		b.WriteByte('\n')
	}
	return b.String(), nil
}

func canonicalHeaderValue(r *http.Request, name string) (string, bool) {
	if name == "host" {
		if r.Host == "" {
			return "", false
		}
		return normalizeHeaderValue(r.Host), true
	}
	values, ok := r.Header[http.CanonicalHeaderKey(name)]
	if !ok || len(values) == 0 {
		return "", false
	}
	normalized := make([]string, 0, len(values))
	for _, value := range values {
		normalized = append(normalized, normalizeHeaderValue(value))
	}
	return strings.Join(normalized, ","), true
}

func normalizeSignedHeaders(headers []string) []string {
	normalized := make([]string, 0, len(headers))
	seen := make(map[string]struct{}, len(headers))
	for _, header := range headers {
		header = strings.ToLower(strings.TrimSpace(header))
		if header == "" {
			continue
		}
		if _, ok := seen[header]; ok {
			continue
		}
		seen[header] = struct{}{}
		normalized = append(normalized, header)
	}
	return normalized
}

func normalizeHeaderValue(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func canonicalURI(u *url.URL) string {
	path := u.EscapedPath()
	if path == "" {
		return "/"
	}
	return path
}

func canonicalQueryString(u *url.URL, excludeSignature bool) string {
	values := u.Query()
	if excludeSignature {
		values.Del("X-Amz-Signature")
	}
	type pair struct {
		key   string
		value string
	}
	var pairs []pair
	for key, vals := range values {
		if len(vals) == 0 {
			pairs = append(pairs, pair{key: key})
			continue
		}
		for _, value := range vals {
			pairs = append(pairs, pair{key: key, value: value})
		}
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].key == pairs[j].key {
			return pairs[i].value < pairs[j].value
		}
		return pairs[i].key < pairs[j].key
	})
	encoded := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		encoded = append(encoded, awsPercentEncode(pair.key)+"="+awsPercentEncode(pair.value))
	}
	return strings.Join(encoded, "&")
}

func awsPercentEncode(value string) string {
	var b strings.Builder
	for len(value) > 0 {
		r, size := utf8.DecodeRuneInString(value)
		if r == utf8.RuneError && size == 1 {
			b.WriteString(fmt.Sprintf("%%%02X", value[0]))
			value = value[1:]
			continue
		}
		if isUnreserved(r) {
			b.WriteRune(r)
		} else {
			for _, c := range []byte(string(r)) {
				b.WriteString(fmt.Sprintf("%%%02X", c))
			}
		}
		value = value[size:]
	}
	return b.String()
}

func isUnreserved(r rune) bool {
	return r >= 'A' && r <= 'Z' ||
		r >= 'a' && r <= 'z' ||
		r >= '0' && r <= '9' ||
		r == '-' || r == '_' || r == '.' || r == '~'
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
