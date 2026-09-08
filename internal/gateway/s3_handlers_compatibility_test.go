package gateway

import (
	"net/http/httptest"
	"testing"

	"github.com/nosway/namros/internal/s3api/routing"
)

func TestValidateCompatibilityHeadersRejectsEverySSECustomerHeaderByPresence(t *testing.T) {
	headers := []string{
		"x-amz-server-side-encryption-customer-algorithm",
		"x-amz-server-side-encryption-customer-key",
		"x-amz-server-side-encryption-customer-key-md5",
		"x-amz-copy-source-server-side-encryption-customer-algorithm",
		"x-amz-copy-source-server-side-encryption-customer-key",
		"x-amz-copy-source-server-side-encryption-customer-key-md5",
	}
	values := []struct {
		name   string
		values []string
	}{
		{name: "empty", values: []string{""}},
		{name: "whitespace", values: []string{"   "}},
		{name: "non-empty", values: []string{"AES256"}},
		{name: "empty first multi-value", values: []string{"", "AES256"}},
	}

	for _, header := range headers {
		for _, value := range values {
			t.Run(header+"/"+value.name, func(t *testing.T) {
				req := httptest.NewRequest("PUT", "/bucket/object", nil)
				req.Header[header] = value.values

				got, rejected := validateCompatibilityHeaders(req, routing.OperationPutObject)
				if !rejected {
					t.Fatal("validateCompatibilityHeaders() accepted an SSE-C header")
				}
				if got.Code != "NotImplemented" {
					t.Fatalf("error code = %q, want NotImplemented", got.Code)
				}
			})
		}
	}
}

func TestValidateCompatibilityHeadersRejectsRenameObjectHeaderByPresence(t *testing.T) {
	for _, values := range [][]string{{""}, {"/bucket/source.txt"}} {
		req := httptest.NewRequest("PUT", "/bucket/destination.txt", nil)
		req.Header["x-amz-rename-source"] = values

		got, rejected := validateCompatibilityHeaders(req, routing.OperationPutObject)
		if !rejected {
			t.Fatal("validateCompatibilityHeaders() accepted x-amz-rename-source")
		}
		if got.Code != "NotImplemented" {
			t.Fatalf("error code = %q, want NotImplemented", got.Code)
		}
	}
}
