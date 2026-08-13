package s3err

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestWriteXML(t *testing.T) {
	rec := httptest.NewRecorder()
	rec.Header().Set(RequestIDHeader, "req-1")
	rec.Header().Set(HostIDHeader, "host-1")
	req := httptest.NewRequest("GET", "/bucket/key", nil)

	Write(rec, req, NotImplemented("operation is not implemented"))

	if rec.Code != 501 {
		t.Fatalf("status = %d, want 501", rec.Code)
	}
	if got := rec.Header().Get("Content-Type"); got != "application/xml" {
		t.Fatalf("Content-Type = %q, want application/xml", got)
	}
	body := rec.Body.String()
	for _, want := range []string{
		`<?xml version="1.0" encoding="UTF-8"?>`,
		`<Error>`,
		`<Code>NotImplemented</Code>`,
		`<Message>operation is not implemented</Message>`,
		`<Resource>/bucket/key</Resource>`,
		`<RequestId>req-1</RequestId>`,
		`<HostId>host-1</HostId>`,
		`</Error>`,
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("body missing %q: %s", want, body)
		}
	}
}

func TestServiceUnavailable(t *testing.T) {
	err := ServiceUnavailable("storage unavailable")
	if err.Code != "ServiceUnavailable" {
		t.Fatalf("Code = %q, want ServiceUnavailable", err.Code)
	}
	if err.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("HTTPStatus = %d, want 503", err.HTTPStatus)
	}
}

func TestSlowDown(t *testing.T) {
	err := SlowDown("gateway data budget exceeded")
	if err.Code != "SlowDown" {
		t.Fatalf("Code = %q, want SlowDown", err.Code)
	}
	if err.HTTPStatus != http.StatusServiceUnavailable {
		t.Fatalf("HTTPStatus = %d, want 503", err.HTTPStatus)
	}
}
