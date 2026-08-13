package gateway

import (
	"io"
	"strings"
	"testing"
)

func TestAWSChunkedReaderDecodesPayload(t *testing.T) {
	body := strings.Join([]string{
		"5;chunk-signature=aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa\r\nhello\r\n",
		"9;chunk-signature=bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb\r\n from mc\n\r\n",
		"0;chunk-signature=cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc\r\n",
		"x-amz-checksum-crc32:AAAAAA==\r\n",
		"\r\n",
	}, "")

	got, err := io.ReadAll(newAWSChunkedReader(strings.NewReader(body)))
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if string(got) != "hello from mc\n" {
		t.Fatalf("decoded body = %q, want %q", string(got), "hello from mc\n")
	}
}

func TestAWSChunkedReaderRejectsMalformedBody(t *testing.T) {
	_, err := io.ReadAll(newAWSChunkedReader(strings.NewReader("e;chunk-signature=abc\r\nshort")))
	if err == nil {
		t.Fatal("ReadAll() error = nil, want malformed body error")
	}
}
