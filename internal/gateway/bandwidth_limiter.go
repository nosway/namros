package gateway

import (
	"io"
	"time"

	"github.com/nosway/namros/internal/config"
)

type bandwidthLimiter struct {
	uploadBytesPerSecond   int64
	downloadBytesPerSecond int64
	sleep                  func(time.Duration)
}

func newBandwidthLimiter(cfg config.Config) *bandwidthLimiter {
	if cfg.GatewayUploadBandwidthBytesPerSecond <= 0 && cfg.GatewayDownloadBandwidthBytesPerSecond <= 0 {
		return nil
	}
	return &bandwidthLimiter{
		uploadBytesPerSecond:   cfg.GatewayUploadBandwidthBytesPerSecond,
		downloadBytesPerSecond: cfg.GatewayDownloadBandwidthBytesPerSecond,
		sleep:                  time.Sleep,
	}
}

func (l *bandwidthLimiter) wrapUpload(body io.ReadCloser) io.ReadCloser {
	if l == nil || body == nil || l.uploadBytesPerSecond <= 0 {
		return body
	}
	return &rateLimitedReadCloser{
		ReadCloser: body,
		shaper: rateShaper{
			bytesPerSecond: l.uploadBytesPerSecond,
			sleep:          l.sleep,
		},
	}
}

func (h s3Handler) downloadWriter(w io.Writer) io.Writer {
	if h.bandwidthLimiter == nil || h.bandwidthLimiter.downloadBytesPerSecond <= 0 {
		return w
	}
	return &rateLimitedWriter{
		writer: w,
		shaper: rateShaper{
			bytesPerSecond: h.bandwidthLimiter.downloadBytesPerSecond,
			sleep:          h.bandwidthLimiter.sleep,
		},
	}
}

type rateLimitedReadCloser struct {
	io.ReadCloser
	shaper rateShaper
}

func (r *rateLimitedReadCloser) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	r.shaper.throttle(n)
	return n, err
}

type rateLimitedWriter struct {
	writer io.Writer
	shaper rateShaper
}

func (w *rateLimitedWriter) Write(p []byte) (int, error) {
	n, err := w.writer.Write(p)
	w.shaper.throttle(n)
	return n, err
}

type rateShaper struct {
	bytesPerSecond int64
	sleep          func(time.Duration)
}

func (s rateShaper) throttle(byteCount int) {
	if byteCount <= 0 || s.bytesPerSecond <= 0 {
		return
	}
	sleep := s.sleep
	if sleep == nil {
		sleep = time.Sleep
	}
	duration := time.Duration(byteCount) * time.Second / time.Duration(s.bytesPerSecond)
	if duration > 0 {
		sleep(duration)
	}
}
