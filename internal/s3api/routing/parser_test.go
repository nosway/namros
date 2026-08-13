package routing

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseRequestPathStyle(t *testing.T) {
	tests := []struct {
		name      string
		method    string
		target    string
		bucket    string
		key       string
		hasKey    bool
		operation Operation
	}{
		{
			name:      "list buckets",
			method:    http.MethodGet,
			target:    "/",
			operation: OperationListBuckets,
		},
		{
			name:      "bucket location",
			method:    http.MethodGet,
			target:    "/photos?location",
			bucket:    "photos",
			operation: OperationGetBucketLocation,
		},
		{
			name:      "bucket root with trailing slash",
			method:    http.MethodPut,
			target:    "/photos/",
			bucket:    "photos",
			operation: OperationCreateBucket,
		},
		{
			name:      "get bucket versioning",
			method:    http.MethodGet,
			target:    "/photos?versioning",
			bucket:    "photos",
			operation: OperationGetBucketVersioning,
		},
		{
			name:      "put bucket versioning",
			method:    http.MethodPut,
			target:    "/photos?versioning",
			bucket:    "photos",
			operation: OperationPutBucketVersioning,
		},
		{
			name:      "get bucket cors",
			method:    http.MethodGet,
			target:    "/photos?cors",
			bucket:    "photos",
			operation: OperationGetBucketCORS,
		},
		{
			name:      "put bucket cors",
			method:    http.MethodPut,
			target:    "/photos?cors",
			bucket:    "photos",
			operation: OperationPutBucketCORS,
		},
		{
			name:      "delete bucket cors",
			method:    http.MethodDelete,
			target:    "/photos?cors",
			bucket:    "photos",
			operation: OperationDeleteBucketCORS,
		},
		{
			name:      "get bucket lifecycle",
			method:    http.MethodGet,
			target:    "/photos?lifecycle",
			bucket:    "photos",
			operation: OperationGetBucketLifecycle,
		},
		{
			name:      "put bucket lifecycle",
			method:    http.MethodPut,
			target:    "/photos?lifecycle",
			bucket:    "photos",
			operation: OperationPutBucketLifecycle,
		},
		{
			name:      "delete bucket lifecycle",
			method:    http.MethodDelete,
			target:    "/photos?lifecycle",
			bucket:    "photos",
			operation: OperationDeleteBucketLifecycle,
		},
		{
			name:      "get bucket object lock",
			method:    http.MethodGet,
			target:    "/photos?object-lock",
			bucket:    "photos",
			operation: OperationGetBucketObjectLock,
		},
		{
			name:      "put bucket object lock",
			method:    http.MethodPut,
			target:    "/photos?object-lock",
			bucket:    "photos",
			operation: OperationPutBucketObjectLock,
		},
		{
			name:      "get bucket policy",
			method:    http.MethodGet,
			target:    "/photos?policy",
			bucket:    "photos",
			operation: OperationGetBucketPolicy,
		},
		{
			name:      "put bucket policy",
			method:    http.MethodPut,
			target:    "/photos?policy",
			bucket:    "photos",
			operation: OperationPutBucketPolicy,
		},
		{
			name:      "delete bucket policy",
			method:    http.MethodDelete,
			target:    "/photos?policy",
			bucket:    "photos",
			operation: OperationDeleteBucketPolicy,
		},
		{
			name:      "get bucket encryption",
			method:    http.MethodGet,
			target:    "/photos?encryption",
			bucket:    "photos",
			operation: OperationGetBucketEncryption,
		},
		{
			name:      "put bucket encryption",
			method:    http.MethodPut,
			target:    "/photos?encryption",
			bucket:    "photos",
			operation: OperationPutBucketEncryption,
		},
		{
			name:      "delete bucket encryption",
			method:    http.MethodDelete,
			target:    "/photos?encryption",
			bucket:    "photos",
			operation: OperationDeleteBucketEncryption,
		},
		{
			name:      "get bucket acl",
			method:    http.MethodGet,
			target:    "/photos?acl",
			bucket:    "photos",
			operation: OperationGetBucketACL,
		},
		{
			name:      "put bucket acl",
			method:    http.MethodPut,
			target:    "/photos?acl",
			bucket:    "photos",
			operation: OperationPutBucketACL,
		},
		{
			name:      "delete objects",
			method:    http.MethodPost,
			target:    "/photos?delete",
			bucket:    "photos",
			operation: OperationDeleteObjects,
		},
		{
			name:      "list object versions",
			method:    http.MethodGet,
			target:    "/photos?versions&prefix=raw/",
			bucket:    "photos",
			operation: OperationListObjectVersions,
		},
		{
			name:      "list objects v2",
			method:    http.MethodGet,
			target:    "/photos?list-type=2&prefix=raw/",
			bucket:    "photos",
			operation: OperationListObjectsV2,
		},
		{
			name:      "list objects v1",
			method:    http.MethodGet,
			target:    "/photos?prefix=raw/",
			bucket:    "photos",
			operation: OperationListObjects,
		},
		{
			name:      "list multipart uploads",
			method:    http.MethodGet,
			target:    "/photos?uploads&prefix=raw/",
			bucket:    "photos",
			operation: OperationListMultipartUploads,
		},
		{
			name:      "directory marker key preserved",
			method:    http.MethodPut,
			target:    "/photos/dir/",
			bucket:    "photos",
			key:       "dir/",
			hasKey:    true,
			operation: OperationPutObject,
		},
		{
			name:      "empty path segment key preserved",
			method:    http.MethodGet,
			target:    "/photos/a//b",
			bucket:    "photos",
			key:       "a//b",
			hasKey:    true,
			operation: OperationGetObject,
		},
		{
			name:      "get object version",
			method:    http.MethodGet,
			target:    "/photos/a.txt?versionId=v1",
			bucket:    "photos",
			key:       "a.txt",
			hasKey:    true,
			operation: OperationGetObject,
		},
		{
			name:      "upload part",
			method:    http.MethodPut,
			target:    "/photos/big.bin?partNumber=7&uploadId=u1",
			bucket:    "photos",
			key:       "big.bin",
			hasKey:    true,
			operation: OperationUploadPart,
		},
		{
			name:      "get object tagging",
			method:    http.MethodGet,
			target:    "/photos/object.txt?tagging",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationGetObjectTagging,
		},
		{
			name:      "put object tagging",
			method:    http.MethodPut,
			target:    "/photos/object.txt?tagging",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationPutObjectTagging,
		},
		{
			name:      "delete object tagging",
			method:    http.MethodDelete,
			target:    "/photos/object.txt?tagging",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationDeleteObjectTagging,
		},
		{
			name:      "get object retention",
			method:    http.MethodGet,
			target:    "/photos/object.txt?retention&versionId=v1",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationGetObjectRetention,
		},
		{
			name:      "put object retention",
			method:    http.MethodPut,
			target:    "/photos/object.txt?retention",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationPutObjectRetention,
		},
		{
			name:      "get object legal hold",
			method:    http.MethodGet,
			target:    "/photos/object.txt?legal-hold",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationGetObjectLegalHold,
		},
		{
			name:      "put object legal hold",
			method:    http.MethodPut,
			target:    "/photos/object.txt?legal-hold",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationPutObjectLegalHold,
		},
		{
			name:      "get object acl",
			method:    http.MethodGet,
			target:    "/photos/object.txt?acl",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationGetObjectACL,
		},
		{
			name:      "put object acl",
			method:    http.MethodPut,
			target:    "/photos/object.txt?acl",
			bucket:    "photos",
			key:       "object.txt",
			hasKey:    true,
			operation: OperationPutObjectACL,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.target, nil)
			got, err := ParseRequest(req)
			if err != nil {
				t.Fatalf("ParseRequest() error = %v", err)
			}
			if got.Style != AddressingPathStyle {
				t.Fatalf("style = %q, want %q", got.Style, AddressingPathStyle)
			}
			if got.Bucket != tt.bucket || got.Key != tt.key || got.HasKey != tt.hasKey || got.Operation != tt.operation {
				t.Fatalf("parsed = bucket %q key %q hasKey %v op %q, want bucket %q key %q hasKey %v op %q",
					got.Bucket, got.Key, got.HasKey, got.Operation, tt.bucket, tt.key, tt.hasKey, tt.operation)
			}
		})
	}
}

func TestParseRequestCopyObject(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/photos/copy.txt", nil)
	req.Header.Set("x-amz-copy-source", "/photos/source.txt")
	got, err := ParseRequest(req)
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if got.Operation != OperationCopyObject {
		t.Fatalf("operation = %q, want %q", got.Operation, OperationCopyObject)
	}
}

func TestParseRequestVirtualHostedStyle(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://photos.s3.localhost.local:9000/dir/object", nil)
	got, err := ParseRequest(req)
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if got.Style != AddressingVirtualHostedStyle {
		t.Fatalf("style = %q, want %q", got.Style, AddressingVirtualHostedStyle)
	}
	if got.Bucket != "photos" || got.Key != "dir/object" || got.Operation != OperationGetObject {
		t.Fatalf("parsed = bucket %q key %q op %q", got.Bucket, got.Key, got.Operation)
	}
}

func TestParseRequestRejectsUnsupportedSubresource(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/photos?website", nil)
	if _, err := ParseRequest(req); err == nil {
		t.Fatal("ParseRequest() error = nil, want error")
	}
}

func TestParseRequestRejectsInvalidPartNumber(t *testing.T) {
	req := httptest.NewRequest(http.MethodPut, "/photos/big.bin?partNumber=0&uploadId=u1", nil)
	if _, err := ParseRequest(req); err == nil {
		t.Fatal("ParseRequest() error = nil, want error")
	}
}
