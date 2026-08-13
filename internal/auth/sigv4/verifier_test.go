package sigv4

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/nosway/namros/internal/auth/credentials"
)

const (
	testAccessKey = "AKIDEXAMPLE"
	testSecretKey = "wJalrXUtnFEMI/K7MDENG+bPxRfiCYEXAMPLEKEY"
	testRegion    = "us-east-1"
)

func TestCanonicalRequestFixture(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://example.com/bucket/a%20b?prefix=a%20b&list-type=2", nil)
	req.Header.Set(amzDateHeader, "20240102T030405Z")
	req.Header.Set(contentSHA256Header, unsignedPayload)

	got, signedHeaders, err := buildCanonicalRequest(req, canonicalOptions{
		SignedHeaders: []string{"host", "x-amz-content-sha256", "x-amz-date"},
		PayloadHash:   unsignedPayload,
	})
	if err != nil {
		t.Fatalf("buildCanonicalRequest() error = %v", err)
	}
	want := "GET\n" +
		"/bucket/a%20b\n" +
		"list-type=2&prefix=a%20b\n" +
		"host:example.com\n" +
		"x-amz-content-sha256:UNSIGNED-PAYLOAD\n" +
		"x-amz-date:20240102T030405Z\n" +
		"\n" +
		"host;x-amz-content-sha256;x-amz-date\n" +
		"UNSIGNED-PAYLOAD"
	if got != want {
		t.Fatalf("canonical request:\n%s\nwant:\n%s", got, want)
	}
	if signedHeaders != "host;x-amz-content-sha256;x-amz-date" {
		t.Fatalf("signedHeaders = %q", signedHeaders)
	}
}

func TestVerifyAuthorizationHeader(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	verifier := newTestVerifier(t, now)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/photos/dir/object", nil)
	signHeaderRequest(t, req, now, testAccessKey, testSecretKey)

	result, err := verifier.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if result.AccessKeyID != testAccessKey || !result.Principal.Root {
		t.Fatalf("result principal = %+v accessKeyID = %q", result.Principal, result.AccessKeyID)
	}
	if result.CanonicalRequest == "" || result.StringToSign == "" {
		t.Fatal("debug trace is empty")
	}
}

func TestVerifyRejectsBadSignature(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	verifier := newTestVerifier(t, now)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/photos/dir/object", nil)
	signHeaderRequest(t, req, now, testAccessKey, testSecretKey)
	req.Header.Set(authorizationHeader, req.Header.Get(authorizationHeader)[:len(req.Header.Get(authorizationHeader))-1]+"0")

	_, err := verifier.Verify(context.Background(), req)
	if !errors.Is(err, ErrSignatureDoesNotMatch) {
		t.Fatalf("Verify() error = %v, want ErrSignatureDoesNotMatch", err)
	}
}

func TestVerifyMissingCredential(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	verifier := newTestVerifier(t, now)
	req := httptest.NewRequest(http.MethodGet, "http://example.com/photos/dir/object", nil)

	_, err := verifier.Verify(context.Background(), req)
	if !errors.Is(err, ErrAccessDenied) {
		t.Fatalf("Verify() error = %v, want ErrAccessDenied", err)
	}
}

func TestVerifyPresignedURL(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	verifier := newTestVerifier(t, now)
	req := httptest.NewRequest(http.MethodPut, "http://example.com/photos/object", nil)
	signPresignedRequest(t, req, now, 60*time.Second, testAccessKey, testSecretKey)

	result, err := verifier.Verify(context.Background(), req)
	if err != nil {
		t.Fatalf("Verify() error = %v", err)
	}
	if !result.Presigned {
		t.Fatal("Presigned = false, want true")
	}
}

func TestVerifyPresignedURLRejectsExpired(t *testing.T) {
	signedAt := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	verifier := newTestVerifier(t, signedAt.Add(2*time.Minute))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/photos/object", nil)
	signPresignedRequest(t, req, signedAt, 60*time.Second, testAccessKey, testSecretKey)

	_, err := verifier.Verify(context.Background(), req)
	if !errors.Is(err, ErrExpiredPresign) {
		t.Fatalf("Verify() error = %v, want ErrExpiredPresign", err)
	}
}

func TestVerifyRejectsClockSkew(t *testing.T) {
	now := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	verifier := newTestVerifier(t, now.Add(30*time.Minute))
	req := httptest.NewRequest(http.MethodGet, "http://example.com/photos/object", nil)
	signHeaderRequest(t, req, now, testAccessKey, testSecretKey)

	_, err := verifier.Verify(context.Background(), req)
	if !errors.Is(err, ErrRequestTimeTooSkewed) {
		t.Fatalf("Verify() error = %v, want ErrRequestTimeTooSkewed", err)
	}
}

func newTestVerifier(t *testing.T, now time.Time) *Verifier {
	t.Helper()
	cred, err := credentials.NewRootCredential(testAccessKey, testSecretKey)
	if err != nil {
		t.Fatalf("NewRootCredential() error = %v", err)
	}
	store, err := credentials.NewStaticStore(cred)
	if err != nil {
		t.Fatalf("NewStaticStore() error = %v", err)
	}
	return NewVerifier(Config{
		Region:      testRegion,
		Credentials: store,
		Now:         func() time.Time { return now },
	})
}

func signHeaderRequest(t *testing.T, req *http.Request, now time.Time, accessKeyID, secretKey string) {
	t.Helper()
	req.Header.Set(amzDateHeader, now.Format(amzDateLayout))
	req.Header.Set(contentSHA256Header, unsignedPayload)
	signedHeaders := []string{"host", "x-amz-content-sha256", "x-amz-date"}
	canonicalRequest, signedHeaderText, err := buildCanonicalRequest(req, canonicalOptions{
		SignedHeaders: signedHeaders,
		PayloadHash:   unsignedPayload,
	})
	if err != nil {
		t.Fatalf("buildCanonicalRequest() error = %v", err)
	}
	scope := credentialScope{
		AccessKeyID: accessKeyID,
		Date:        now.Format("20060102"),
		Region:      testRegion,
		Service:     "s3",
		Terminator:  scopeTerminator,
	}
	stringToSign := buildStringToSign(now, scope, canonicalRequest)
	signature := hex.EncodeToString(testHMAC(deriveSigningKey(secretKey, scope.Date, scope.Region, scope.Service), stringToSign))
	req.Header.Set(authorizationHeader, algorithm+" Credential="+accessKeyID+"/"+scope.Date+"/"+scope.Region+"/"+scope.Service+"/"+scope.Terminator+", SignedHeaders="+signedHeaderText+", Signature="+signature)
}

func signPresignedRequest(t *testing.T, req *http.Request, now time.Time, expires time.Duration, accessKeyID, secretKey string) {
	t.Helper()
	scope := credentialScope{
		AccessKeyID: accessKeyID,
		Date:        now.Format("20060102"),
		Region:      testRegion,
		Service:     "s3",
		Terminator:  scopeTerminator,
	}
	values := req.URL.Query()
	values.Set(queryAlgorithm, algorithm)
	values.Set(queryCredential, accessKeyID+"/"+scope.Date+"/"+scope.Region+"/"+scope.Service+"/"+scope.Terminator)
	values.Set(queryDate, now.Format(amzDateLayout))
	values.Set(queryExpires, strconvSeconds(expires))
	values.Set(querySignedHeaders, "host")
	req.URL.RawQuery = values.Encode()

	canonicalRequest, _, err := buildCanonicalRequest(req, canonicalOptions{
		SignedHeaders:         []string{"host"},
		PayloadHash:           unsignedPayload,
		ExcludeQuerySignature: true,
	})
	if err != nil {
		t.Fatalf("buildCanonicalRequest() error = %v", err)
	}
	stringToSign := buildStringToSign(now, scope, canonicalRequest)
	signature := hex.EncodeToString(testHMAC(deriveSigningKey(secretKey, scope.Date, scope.Region, scope.Service), stringToSign))
	values = req.URL.Query()
	values.Set(querySignature, signature)
	req.URL.RawQuery = values.Encode()
}

func strconvSeconds(d time.Duration) string {
	return strconv.Itoa(int(d / time.Second))
}

func testHMAC(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func TestAWSPercentEncode(t *testing.T) {
	values := url.Values{}
	values.Add("prefix", "a b")
	values.Add("x", "1/2")
	u := &url.URL{RawQuery: values.Encode()}
	got := canonicalQueryString(u, false)
	if got != "prefix=a%20b&x=1%2F2" {
		t.Fatalf("canonicalQueryString = %q", got)
	}
}
