package gateway

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"
)

func TestBandwidthLimiterWrapUploadPreservesBytes(t *testing.T) {
	var slept time.Duration
	limiter := &bandwidthLimiter{
		uploadBytesPerSecond: 4,
		sleep: func(d time.Duration) {
			slept += d
		},
	}
	body := limiter.wrapUpload(io.NopCloser(strings.NewReader("abcdefgh")))
	got, err := io.ReadAll(body)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "abcdefgh" {
		t.Fatalf("upload bytes = %q, want abcdefgh", string(got))
	}
	if slept != 2*time.Second {
		t.Fatalf("slept = %s, want 2s", slept)
	}
}

func TestBandwidthLimiterDownloadWriterPreservesBytes(t *testing.T) {
	var slept time.Duration
	handler := s3Handler{bandwidthLimiter: &bandwidthLimiter{
		downloadBytesPerSecond: 5,
		sleep: func(d time.Duration) {
			slept += d
		},
	}}
	var out bytes.Buffer
	n, err := handler.downloadWriter(&out).Write([]byte("abcdefghij"))
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if n != 10 || out.String() != "abcdefghij" {
		t.Fatalf("Write() = %d bytes %q, want 10 abcdefghij", n, out.String())
	}
	if slept != 2*time.Second {
		t.Fatalf("slept = %s, want 2s", slept)
	}
}
