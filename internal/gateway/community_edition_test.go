package gateway

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/nosway/namros/internal/config"
	"github.com/nosway/namros/internal/meta/memory"
	"github.com/nosway/namros/internal/storage/local"
)

func TestCommunityEditionRejectsEnterpriseS3Features(t *testing.T) {
	skipEnterpriseOverlayCommunityAssertion(t)
	cfg := config.Default()
	segmentStore, err := local.New(t.TempDir())
	if err != nil {
		t.Fatalf("local.New() error = %v", err)
	}
	handler := NewHandlerWithDeps(cfg, Dependencies{
		Metadata: memory.New(),
		Storage:  segmentStore,
		Orphans:  segmentStore,
	})

	createLockedBucket := communityPerformSigned(t, handler, cfg, http.MethodPut, "/community-lock", nil, map[string]string{
		"X-Amz-Bucket-Object-Lock-Enabled": "true",
	})
	communityAssertEnterpriseFeatureBlocked(t, createLockedBucket)

	createBucket := communityPerformSigned(t, handler, cfg, http.MethodPut, "/community-basic", nil, nil)
	if createBucket.Code != http.StatusOK {
		t.Fatalf("CreateBucket status = %d body = %s", createBucket.Code, createBucket.Body.String())
	}

	putObjectLockHeaders := communityPerformSigned(t, handler, cfg, http.MethodPut, "/community-basic/object.txt", strings.NewReader("locked"), map[string]string{
		"X-Amz-Object-Lock-Mode":              "GOVERNANCE",
		"X-Amz-Object-Lock-Retain-Until-Date": "2030-01-02T03:04:05Z",
	})
	communityAssertEnterpriseFeatureBlocked(t, putObjectLockHeaders)

	putSSEKMS := communityPerformSigned(t, handler, cfg, http.MethodPut, "/community-basic/kms.txt", strings.NewReader("encrypted"), map[string]string{
		"X-Amz-Server-Side-Encryption":                "aws:kms",
		"X-Amz-Server-Side-Encryption-Aws-Kms-Key-Id": "kms-key-community",
	})
	communityAssertEnterpriseFeatureBlocked(t, putSSEKMS)

	putEC := communityPerformSigned(t, handler, cfg, http.MethodPut, "/community-basic/ec.txt", strings.NewReader("ec"), map[string]string{
		"X-Amz-Storage-Class": "EC_4_2",
	})
	communityAssertEnterpriseFeatureBlocked(t, putEC)
}

func communityPerformSigned(t *testing.T, handler http.Handler, cfg config.Config, method, target string, body io.Reader, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	if body == nil {
		body = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, target, body)
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	communitySignRequest(t, req, cfg)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	return rec
}

func communitySignRequest(t *testing.T, req *http.Request, cfg config.Config) {
	t.Helper()
	now := time.Now().UTC()
	amzDate := now.Format("20060102T150405Z")
	date := now.Format("20060102")
	req.Header.Set("X-Amz-Date", amzDate)
	req.Header.Set("X-Amz-Content-Sha256", "UNSIGNED-PAYLOAD")
	signedHeaders := "host;x-amz-content-sha256;x-amz-date"
	canonicalRequest := strings.Join([]string{
		req.Method,
		req.URL.EscapedPath(),
		req.URL.Query().Encode(),
		"host:" + req.Host + "\n" +
			"x-amz-content-sha256:UNSIGNED-PAYLOAD\n" +
			"x-amz-date:" + amzDate + "\n",
		signedHeaders,
		"UNSIGNED-PAYLOAD",
	}, "\n")
	scope := date + "/" + cfg.Region + "/s3/aws4_request"
	stringToSign := strings.Join([]string{
		"AWS4-HMAC-SHA256",
		amzDate,
		scope,
		communitySHA256Hex(canonicalRequest),
	}, "\n")
	signingKey := communitySigningKey(cfg.RootSecretAccessKey, date, cfg.Region, "s3")
	signature := hex.EncodeToString(communityHMAC(signingKey, stringToSign))
	req.Header.Set("Authorization", "AWS4-HMAC-SHA256 Credential="+cfg.RootAccessKeyID+"/"+scope+", SignedHeaders="+signedHeaders+", Signature="+signature)
}

func communitySigningKey(secret, date, region, service string) []byte {
	kDate := communityHMAC([]byte("AWS4"+secret), date)
	kRegion := communityHMAC(kDate, region)
	kService := communityHMAC(kRegion, service)
	return communityHMAC(kService, "aws4_request")
}

func communityHMAC(key []byte, value string) []byte {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte(value))
	return mac.Sum(nil)
}

func communitySHA256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func communityS3ErrorCode(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Code string `xml:"Code"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("S3 error XML decode: %v; body = %s", err, rec.Body.String())
	}
	return body.Code
}

func communityS3ErrorMessage(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Message string `xml:"Message"`
	}
	if err := xml.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("S3 error XML decode: %v; body = %s", err, rec.Body.String())
	}
	return body.Message
}

func communityAssertEnterpriseFeatureBlocked(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("enterprise feature status = %d body = %s, want 501", rec.Code, rec.Body.String())
	}
	if code := communityS3ErrorCode(t, rec); code != "NotImplemented" {
		t.Fatalf("enterprise feature error code = %q, want NotImplemented", code)
	}
	if message := communityS3ErrorMessage(t, rec); !strings.Contains(message, "NAMROS Enterprise Edition") {
		t.Fatalf("enterprise feature message = %q, want Enterprise Edition", message)
	}
}
