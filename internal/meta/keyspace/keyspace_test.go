package keyspace

import "testing"

func TestObjectKeyEscapingRoundTrip(t *testing.T) {
	keys := []string{
		"dir/",
		"a//b",
		"space key",
		string([]byte{'i', 'n', 'v', 0xff, 'a', 'l', 'i', 'd'}),
	}
	for _, key := range keys {
		encoded := EscapeObjectKey(key)
		decoded, err := UnescapeObjectKey(encoded)
		if err != nil {
			t.Fatalf("UnescapeObjectKey(%q) error = %v", encoded, err)
		}
		if decoded != key {
			t.Fatalf("round trip = %q, want %q", decoded, key)
		}
		if key == "dir/" && encoded != "dir%2F" {
			t.Fatalf("dir slash encoded = %q, want dir%%2F", encoded)
		}
	}
}

func TestKeyBuilders(t *testing.T) {
	if got := ObjectHead("bucket/1", "dir/object"); got != "/namros/v1/buckets/bucket%2F1/objects/dir%2Fobject/head" {
		t.Fatalf("ObjectHead = %q", got)
	}
	if got := MultipartPart("bucket", "upload", 7); got != "/namros/v1/buckets/bucket/multipart/upload/parts/00007" {
		t.Fatalf("MultipartPart = %q", got)
	}
	if got := MultipartCompletion("bucket", "upload"); got != "/namros/v1/buckets/bucket/multipart/upload/completion" {
		t.Fatalf("MultipartCompletion = %q", got)
	}
	if got := MultipartUploadByKey("bucket", "dir/object", "upload/1"); got != "/namros/v1/buckets/bucket/multipart-by-key/dir%2Fobject/upload%2F1" {
		t.Fatalf("MultipartUploadByKey = %q", got)
	}
}
