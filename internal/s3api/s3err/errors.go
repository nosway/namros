package s3err

import (
	"encoding/xml"
	"net/http"

	"github.com/nosway/namros/internal/s3api/xmlresp"
)

const (
	RequestIDHeader = "x-amz-request-id"
	HostIDHeader    = "x-amz-id-2"
)

type Error struct {
	Code       string
	Message    string
	HTTPStatus int
	Resource   string
}

type Response struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	Resource  string   `xml:"Resource,omitempty"`
	RequestID string   `xml:"RequestId,omitempty"`
	HostID    string   `xml:"HostId,omitempty"`
}

func New(code, message string, status int) Error {
	return Error{
		Code:       code,
		Message:    message,
		HTTPStatus: status,
	}
}

func InvalidRequest(message string) Error {
	return New("InvalidRequest", message, http.StatusBadRequest)
}

func InvalidArgument(message string) Error {
	return New("InvalidArgument", message, http.StatusBadRequest)
}

func InvalidRange(message string) Error {
	return New("InvalidRange", message, http.StatusRequestedRangeNotSatisfiable)
}

func InvalidPart(message string) Error {
	return New("InvalidPart", message, http.StatusBadRequest)
}

func NoSuchBucket(message string) Error {
	return New("NoSuchBucket", message, http.StatusNotFound)
}

func BucketAlreadyOwnedByYou(message string) Error {
	return New("BucketAlreadyOwnedByYou", message, http.StatusConflict)
}

func BucketNotEmpty(message string) Error {
	return New("BucketNotEmpty", message, http.StatusConflict)
}

func NoSuchKey(message string) Error {
	return New("NoSuchKey", message, http.StatusNotFound)
}

func NoSuchUpload(message string) Error {
	return New("NoSuchUpload", message, http.StatusNotFound)
}

func NoSuchCORSConfiguration(message string) Error {
	return New("NoSuchCORSConfiguration", message, http.StatusNotFound)
}

func NoSuchLifecycleConfiguration(message string) Error {
	return New("NoSuchLifecycleConfiguration", message, http.StatusNotFound)
}

func NoSuchBucketPolicy(message string) Error {
	return New("NoSuchBucketPolicy", message, http.StatusNotFound)
}

func ServerSideEncryptionConfigurationNotFound(message string) Error {
	return New("ServerSideEncryptionConfigurationNotFoundError", message, http.StatusNotFound)
}

func ObjectLockConfigurationNotFound(message string) Error {
	return New("ObjectLockConfigurationNotFoundError", message, http.StatusNotFound)
}

func AccessDenied(message string) Error {
	return New("AccessDenied", message, http.StatusForbidden)
}

func SignatureDoesNotMatch(message string) Error {
	return New("SignatureDoesNotMatch", message, http.StatusForbidden)
}

func RequestTimeTooSkewed(message string) Error {
	return New("RequestTimeTooSkewed", message, http.StatusForbidden)
}

func ServiceUnavailable(message string) Error {
	return New("ServiceUnavailable", message, http.StatusServiceUnavailable)
}

func SlowDown(message string) Error {
	return New("SlowDown", message, http.StatusServiceUnavailable)
}

func PreconditionFailed(message string) Error {
	return New("PreconditionFailed", message, http.StatusPreconditionFailed)
}

func NotImplemented(message string) Error {
	return New("NotImplemented", message, http.StatusNotImplemented)
}

func MethodNotAllowed(message string) Error {
	return New("MethodNotAllowed", message, http.StatusMethodNotAllowed)
}

func Write(w http.ResponseWriter, r *http.Request, err Error) {
	status := err.HTTPStatus
	if status == 0 {
		status = http.StatusInternalServerError
	}
	resource := err.Resource
	if resource == "" && r != nil && r.URL != nil {
		resource = r.URL.RequestURI()
	}
	resp := Response{
		Code:      err.Code,
		Message:   err.Message,
		Resource:  resource,
		RequestID: w.Header().Get(RequestIDHeader),
		HostID:    w.Header().Get(HostIDHeader),
	}
	_ = xmlresp.Write(w, status, resp)
}
