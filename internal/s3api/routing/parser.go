package routing

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

type AddressingStyle string

const (
	AddressingPathStyle          AddressingStyle = "path"
	AddressingVirtualHostedStyle AddressingStyle = "virtual-hosted"
)

type Subresource string

const (
	SubresourceLocation   Subresource = "location"
	SubresourceListType   Subresource = "list-type"
	SubresourceUploads    Subresource = "uploads"
	SubresourceUploadID   Subresource = "uploadId"
	SubresourcePartNumber Subresource = "partNumber"
	SubresourceVersionID  Subresource = "versionId"
	SubresourceTagging    Subresource = "tagging"
	SubresourceRetention  Subresource = "retention"
	SubresourceLegalHold  Subresource = "legal-hold"
	SubresourceLifecycle  Subresource = "lifecycle"
	SubresourceVersioning Subresource = "versioning"
	SubresourceVersions   Subresource = "versions"
	SubresourceCORS       Subresource = "cors"
	SubresourceObjectLock Subresource = "object-lock"
	SubresourceACL        Subresource = "acl"
	SubresourceDelete     Subresource = "delete"
	SubresourcePolicy     Subresource = "policy"
	SubresourceEncryption Subresource = "encryption"
)

type Request struct {
	Method       string
	Style        AddressingStyle
	Bucket       string
	Key          string
	HasKey       bool
	Subresources map[Subresource]string
	RawQuery     url.Values
	Operation    Operation
}

type Operation string

const (
	OperationListBuckets            Operation = "ListBuckets"
	OperationCreateBucket           Operation = "CreateBucket"
	OperationHeadBucket             Operation = "HeadBucket"
	OperationDeleteBucket           Operation = "DeleteBucket"
	OperationGetBucketLocation      Operation = "GetBucketLocation"
	OperationGetBucketVersioning    Operation = "GetBucketVersioning"
	OperationPutBucketVersioning    Operation = "PutBucketVersioning"
	OperationGetBucketCORS          Operation = "GetBucketCORS"
	OperationPutBucketCORS          Operation = "PutBucketCORS"
	OperationDeleteBucketCORS       Operation = "DeleteBucketCORS"
	OperationGetBucketLifecycle     Operation = "GetBucketLifecycle"
	OperationPutBucketLifecycle     Operation = "PutBucketLifecycle"
	OperationDeleteBucketLifecycle  Operation = "DeleteBucketLifecycle"
	OperationGetBucketObjectLock    Operation = "GetBucketObjectLockConfiguration"
	OperationPutBucketObjectLock    Operation = "PutBucketObjectLockConfiguration"
	OperationGetBucketPolicy        Operation = "GetBucketPolicy"
	OperationPutBucketPolicy        Operation = "PutBucketPolicy"
	OperationDeleteBucketPolicy     Operation = "DeleteBucketPolicy"
	OperationGetBucketEncryption    Operation = "GetBucketEncryption"
	OperationPutBucketEncryption    Operation = "PutBucketEncryption"
	OperationDeleteBucketEncryption Operation = "DeleteBucketEncryption"
	OperationGetBucketACL           Operation = "GetBucketACL"
	OperationPutBucketACL           Operation = "PutBucketACL"
	OperationListObjects            Operation = "ListObjects"
	OperationListObjectsV2          Operation = "ListObjectsV2"
	OperationListObjectVersions     Operation = "ListObjectVersions"
	OperationPutObject              Operation = "PutObject"
	OperationCopyObject             Operation = "CopyObject"
	OperationGetObject              Operation = "GetObject"
	OperationHeadObject             Operation = "HeadObject"
	OperationDeleteObject           Operation = "DeleteObject"
	OperationDeleteObjects          Operation = "DeleteObjects"
	OperationGetObjectTagging       Operation = "GetObjectTagging"
	OperationPutObjectTagging       Operation = "PutObjectTagging"
	OperationDeleteObjectTagging    Operation = "DeleteObjectTagging"
	OperationGetObjectRetention     Operation = "GetObjectRetention"
	OperationPutObjectRetention     Operation = "PutObjectRetention"
	OperationGetObjectLegalHold     Operation = "GetObjectLegalHold"
	OperationPutObjectLegalHold     Operation = "PutObjectLegalHold"
	OperationGetObjectACL           Operation = "GetObjectACL"
	OperationPutObjectACL           Operation = "PutObjectACL"
	OperationCreateMultipartUpload  Operation = "CreateMultipartUpload"
	OperationListMultipartUploads   Operation = "ListMultipartUploads"
	OperationUploadPart             Operation = "UploadPart"
	OperationListParts              Operation = "ListParts"
	OperationCompleteMultipart      Operation = "CompleteMultipartUpload"
	OperationAbortMultipart         Operation = "AbortMultipartUpload"
	OperationUnsupported            Operation = "Unsupported"
)

var ErrInvalidRequest = errors.New("invalid S3 request")

func ParseRequest(r *http.Request) (Request, error) {
	if r == nil || r.URL == nil {
		return Request{}, fmt.Errorf("%w: nil request", ErrInvalidRequest)
	}

	req := Request{
		Method:       r.Method,
		Style:        AddressingPathStyle,
		Subresources: make(map[Subresource]string),
		RawQuery:     r.URL.Query(),
	}
	if bucket, ok := virtualHostedBucket(r.Host); ok {
		req.Style = AddressingVirtualHostedStyle
		req.Bucket = bucket
		req.Key, req.HasKey = keyFromPath(r.URL.Path, false)
	} else {
		req.Bucket, req.Key, req.HasKey = bucketAndKeyFromPath(r.URL.Path)
	}
	if err := parseSubresources(req.RawQuery, req.Subresources); err != nil {
		return Request{}, err
	}
	if err := validateMultipartParams(req.Subresources); err != nil {
		return Request{}, err
	}
	req.Operation = classify(req, r)
	return req, nil
}

func bucketAndKeyFromPath(path string) (bucket, key string, hasKey bool) {
	trimmed := strings.TrimPrefix(path, "/")
	if trimmed == "" {
		return "", "", false
	}
	bucket, rest, found := strings.Cut(trimmed, "/")
	if !found {
		return bucket, "", false
	}
	if rest == "" {
		return bucket, "", false
	}
	return bucket, rest, true
}

func keyFromPath(path string, bucketInPath bool) (string, bool) {
	if bucketInPath {
		_, key, hasKey := bucketAndKeyFromPath(path)
		return key, hasKey
	}
	trimmed := strings.TrimPrefix(path, "/")
	return trimmed, trimmed != ""
}

func virtualHostedBucket(hostport string) (string, bool) {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "" || host == "localhost" || net.ParseIP(host) != nil {
		return "", false
	}
	labels := strings.Split(host, ".")
	if len(labels) < 3 || labels[0] == "s3" {
		return "", false
	}
	for i := 1; i < len(labels); i++ {
		if labels[i] == "s3" {
			return labels[0], true
		}
	}
	return "", false
}

func parseSubresources(values url.Values, dst map[Subresource]string) error {
	for name, value := range values {
		switch Subresource(name) {
		case SubresourceLocation, SubresourceListType, SubresourceUploads, SubresourceUploadID, SubresourcePartNumber, SubresourceVersionID, SubresourceTagging, SubresourceRetention, SubresourceLegalHold, SubresourceLifecycle, SubresourceVersioning, SubresourceVersions, SubresourceCORS, SubresourceObjectLock, SubresourceACL, SubresourceDelete, SubresourcePolicy, SubresourceEncryption:
			dst[Subresource(name)] = first(value)
		default:
			if isS3ControlQuery(name) {
				return fmt.Errorf("%w: unsupported subresource %q", ErrInvalidRequest, name)
			}
		}
	}
	return nil
}

func validateMultipartParams(sub map[Subresource]string) error {
	if raw, ok := sub[SubresourcePartNumber]; ok {
		partNumber, err := strconv.Atoi(raw)
		if err != nil || partNumber < 1 || partNumber > 10000 {
			return fmt.Errorf("%w: partNumber must be between 1 and 10000", ErrInvalidRequest)
		}
	}
	return nil
}

func first(values []string) string {
	if len(values) == 0 {
		return ""
	}
	return values[0]
}

func isS3ControlQuery(name string) bool {
	switch name {
	case "acl", "cors", "delete", "encryption", "legal-hold", "lifecycle", "notification", "object-lock", "policy", "retention", "tagging", "torrent", "versioning", "versions", "website":
		return true
	default:
		return false
	}
}

func classify(req Request, httpReq *http.Request) Operation {
	if req.Bucket == "" {
		if req.Method == http.MethodGet && !req.HasKey {
			return OperationListBuckets
		}
		return OperationUnsupported
	}
	if _, ok := req.Subresources[SubresourceLocation]; ok && !req.HasKey && req.Method == http.MethodGet {
		return OperationGetBucketLocation
	}
	if _, ok := req.Subresources[SubresourceVersioning]; ok && !req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetBucketVersioning
		case http.MethodPut:
			return OperationPutBucketVersioning
		}
	}
	if _, ok := req.Subresources[SubresourceCORS]; ok && !req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetBucketCORS
		case http.MethodPut:
			return OperationPutBucketCORS
		case http.MethodDelete:
			return OperationDeleteBucketCORS
		}
	}
	if _, ok := req.Subresources[SubresourceLifecycle]; ok && !req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetBucketLifecycle
		case http.MethodPut:
			return OperationPutBucketLifecycle
		case http.MethodDelete:
			return OperationDeleteBucketLifecycle
		}
	}
	if _, ok := req.Subresources[SubresourceObjectLock]; ok && !req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetBucketObjectLock
		case http.MethodPut:
			return OperationPutBucketObjectLock
		}
	}
	if _, ok := req.Subresources[SubresourcePolicy]; ok && !req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetBucketPolicy
		case http.MethodPut:
			return OperationPutBucketPolicy
		case http.MethodDelete:
			return OperationDeleteBucketPolicy
		}
	}
	if _, ok := req.Subresources[SubresourceEncryption]; ok && !req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetBucketEncryption
		case http.MethodPut:
			return OperationPutBucketEncryption
		case http.MethodDelete:
			return OperationDeleteBucketEncryption
		}
	}
	if _, ok := req.Subresources[SubresourceACL]; ok && !req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetBucketACL
		case http.MethodPut:
			return OperationPutBucketACL
		}
	}
	if _, ok := req.Subresources[SubresourceDelete]; ok && !req.HasKey && req.Method == http.MethodPost {
		return OperationDeleteObjects
	}
	if _, ok := req.Subresources[SubresourceVersions]; ok && !req.HasKey && req.Method == http.MethodGet {
		return OperationListObjectVersions
	}
	if _, ok := req.Subresources[SubresourceListType]; ok && !req.HasKey && req.Method == http.MethodGet {
		return OperationListObjectsV2
	}
	if _, ok := req.Subresources[SubresourceUploads]; ok && !req.HasKey && req.Method == http.MethodGet {
		return OperationListMultipartUploads
	}
	if _, ok := req.Subresources[SubresourceUploads]; ok && req.HasKey && req.Method == http.MethodPost {
		return OperationCreateMultipartUpload
	}
	if _, hasUploadID := req.Subresources[SubresourceUploadID]; hasUploadID && req.HasKey {
		_, hasPartNumber := req.Subresources[SubresourcePartNumber]
		switch {
		case hasPartNumber && req.Method == http.MethodPut:
			return OperationUploadPart
		case req.Method == http.MethodGet:
			return OperationListParts
		case req.Method == http.MethodPost:
			return OperationCompleteMultipart
		case req.Method == http.MethodDelete:
			return OperationAbortMultipart
		}
	}
	if _, ok := req.Subresources[SubresourceTagging]; ok && req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetObjectTagging
		case http.MethodPut:
			return OperationPutObjectTagging
		case http.MethodDelete:
			return OperationDeleteObjectTagging
		}
	}
	if _, ok := req.Subresources[SubresourceRetention]; ok && req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetObjectRetention
		case http.MethodPut:
			return OperationPutObjectRetention
		}
	}
	if _, ok := req.Subresources[SubresourceLegalHold]; ok && req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetObjectLegalHold
		case http.MethodPut:
			return OperationPutObjectLegalHold
		}
	}
	if _, ok := req.Subresources[SubresourceACL]; ok && req.HasKey {
		switch req.Method {
		case http.MethodGet:
			return OperationGetObjectACL
		case http.MethodPut:
			return OperationPutObjectACL
		}
	}
	if req.HasKey {
		switch req.Method {
		case http.MethodPut:
			if httpReq != nil && httpReq.Header.Get("x-amz-copy-source") != "" {
				return OperationCopyObject
			}
			return OperationPutObject
		case http.MethodGet:
			return OperationGetObject
		case http.MethodHead:
			return OperationHeadObject
		case http.MethodDelete:
			return OperationDeleteObject
		default:
			return OperationUnsupported
		}
	}
	switch req.Method {
	case http.MethodPut:
		return OperationCreateBucket
	case http.MethodHead:
		return OperationHeadBucket
	case http.MethodDelete:
		return OperationDeleteBucket
	case http.MethodGet:
		return OperationListObjects
	default:
		return OperationUnsupported
	}
}
