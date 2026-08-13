package gateway

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/nosway/namros/internal/opsmetrics"
	"github.com/nosway/namros/internal/s3api/routing"
	"github.com/nosway/namros/internal/workerops"
)

const s3MetricsErrorCodeKey = "namros.s3_error_code"

func prometheusMetricsHandler(deps Dependencies) gin.HandlerFunc {
	metrics := deps.GatewayMetrics
	handler := promhttp.HandlerFor(metrics.Gatherer(), promhttp.HandlerOpts{})
	return func(c *gin.Context) {
		refreshWorkerBacklogMetrics(c.Request.Context(), deps)
		handler.ServeHTTP(c.Writer, c.Request)
	}
}

func refreshWorkerBacklogMetrics(ctx context.Context, deps Dependencies) {
	if deps.GatewayMetrics == nil || deps.Metadata == nil {
		return
	}
	snapshot, err := workerops.Summarize(ctx, deps.Metadata, workerops.Config{})
	if err != nil {
		return
	}
	deps.GatewayMetrics.SetWorkerBacklog(snapshot)
}

func s3MetricsMiddleware(metrics *opsmetrics.GatewayMetrics) gin.HandlerFunc {
	return func(c *gin.Context) {
		if metrics == nil || !isS3MetricsCandidate(c.Request) {
			c.Next()
			return
		}
		start := time.Now()
		api := "InvalidRequest"
		if req, err := routing.ParseRequest(c.Request); err == nil {
			api = string(req.Operation)
		}
		metrics.AddActiveS3(api, 1)
		defer metrics.AddActiveS3(api, -1)
		requestBytes := c.Request.ContentLength
		var bodyMetrics *s3RequestBodyMetricsReadCloser
		if c.Request.Body != nil && isS3RequestBodyMetricsOperation(api) {
			bodyMetrics = &s3RequestBodyMetricsReadCloser{ReadCloser: c.Request.Body}
			c.Request.Body = bodyMetrics
		}
		if isS3FirstByteMetricsOperation(api) {
			c.Writer = &s3FirstByteMetricsWriter{
				ResponseWriter: c.Writer,
				metrics:        metrics,
				api:            api,
				start:          start,
			}
		}
		c.Next()
		statusCode := c.Writer.Status()
		if statusCode == 0 {
			statusCode = http.StatusOK
		}
		if bodyMetrics != nil && bodyMetrics.bytesRead > 0 {
			metrics.ObserveS3RequestBodyRead(api, statusCode, bodyMetrics.duration)
		}
		metrics.ObserveS3(opsmetrics.S3Observation{
			API:           api,
			StatusCode:    statusCode,
			RequestBytes:  requestBytes,
			ResponseBytes: c.Writer.Size(),
			Duration:      time.Since(start),
			ErrorCode:     s3MetricsErrorCode(c),
		})
	}
}

type s3RequestBodyMetricsReadCloser struct {
	io.ReadCloser
	duration  time.Duration
	bytesRead int64
}

func (r *s3RequestBodyMetricsReadCloser) Read(p []byte) (int, error) {
	start := time.Now()
	n, err := r.ReadCloser.Read(p)
	if n > 0 {
		r.duration += time.Since(start)
		r.bytesRead += int64(n)
	}
	return n, err
}

type s3FirstByteMetricsWriter struct {
	gin.ResponseWriter
	metrics  *opsmetrics.GatewayMetrics
	api      string
	start    time.Time
	observed bool
}

func (w *s3FirstByteMetricsWriter) Write(p []byte) (int, error) {
	n, err := w.ResponseWriter.Write(p)
	if n > 0 {
		w.observe()
	}
	return n, err
}

func (w *s3FirstByteMetricsWriter) WriteString(s string) (int, error) {
	n, err := w.ResponseWriter.WriteString(s)
	if n > 0 {
		w.observe()
	}
	return n, err
}

func (w *s3FirstByteMetricsWriter) observe() {
	if w.observed {
		return
	}
	w.observed = true
	statusCode := w.Status()
	if statusCode == 0 {
		statusCode = http.StatusOK
	}
	w.metrics.ObserveS3FirstByte(w.api, statusCode, time.Since(w.start))
}

func s3MetricsErrorCode(c *gin.Context) string {
	if c == nil {
		return ""
	}
	value, ok := c.Get(s3MetricsErrorCodeKey)
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprint(typed)
	}
}

func isS3MetricsCandidate(r *http.Request) bool {
	if r == nil || r.URL == nil {
		return false
	}
	path := r.URL.Path
	for _, exact := range []string{"/healthz", "/readyz", "/metrics"} {
		if path == exact {
			return false
		}
	}
	for _, prefix := range []string{"/api/", "/debug/", "/console"} {
		if path == prefix || strings.HasPrefix(path, prefix) {
			return false
		}
	}
	return true
}

func isS3FirstByteMetricsOperation(api string) bool {
	return strings.HasPrefix(api, "Get") || strings.HasPrefix(api, "List")
}

func isS3RequestBodyMetricsOperation(api string) bool {
	switch routing.Operation(api) {
	case routing.OperationPutObject, routing.OperationUploadPart:
		return true
	default:
		return false
	}
}
