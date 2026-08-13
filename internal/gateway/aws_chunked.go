package gateway

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var errInvalidAWSChunkedBody = errors.New("invalid aws-chunked body")

func requestPayload(r *http.Request) (io.Reader, uint64, error) {
	if !isAWSChunkedRequest(r) {
		return r.Body, requestContentLength(r), nil
	}
	size, err := decodedContentLength(r)
	if err != nil {
		return nil, 0, err
	}
	return newAWSChunkedReader(r.Body), size, nil
}

func requestPayloadSizeKnown(r *http.Request) bool {
	if isAWSChunkedRequest(r) {
		return strings.TrimSpace(r.Header.Get("x-amz-decoded-content-length")) != ""
	}
	return r.ContentLength >= 0
}

func isAWSChunkedRequest(r *http.Request) bool {
	for _, part := range strings.Split(r.Header.Get("Content-Encoding"), ",") {
		if strings.EqualFold(strings.TrimSpace(part), "aws-chunked") {
			return true
		}
	}
	payloadHash := r.Header.Get("x-amz-content-sha256")
	return strings.HasPrefix(payloadHash, "STREAMING-")
}

func decodedContentLength(r *http.Request) (uint64, error) {
	value := strings.TrimSpace(r.Header.Get("x-amz-decoded-content-length"))
	if value == "" {
		return 0, nil
	}
	n, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid x-amz-decoded-content-length", errInvalidAWSChunkedBody)
	}
	return n, nil
}

type awsChunkedReader struct {
	br        *bufio.Reader
	remain    uint64
	done      bool
	needCRLF  bool
	readError error
}

func newAWSChunkedReader(r io.Reader) io.Reader {
	return &awsChunkedReader{br: bufio.NewReader(r)}
}

func (r *awsChunkedReader) Read(p []byte) (int, error) {
	if r.readError != nil {
		return 0, r.readError
	}
	if len(p) == 0 {
		return 0, nil
	}
	if r.done {
		return 0, io.EOF
	}
	if r.needCRLF {
		if err := r.consumeCRLF(); err != nil {
			r.readError = err
			return 0, err
		}
		r.needCRLF = false
	}
	if r.remain == 0 {
		size, err := r.nextChunkSize()
		if err != nil {
			r.readError = err
			return 0, err
		}
		if size == 0 {
			r.done = true
			if err := r.consumeTrailers(); err != nil {
				r.readError = err
				return 0, err
			}
			return 0, io.EOF
		}
		r.remain = size
	}
	if uint64(len(p)) > r.remain {
		p = p[:r.remain]
	}
	n, err := io.ReadFull(r.br, p)
	r.remain -= uint64(n)
	if err != nil {
		r.readError = fmt.Errorf("%w: truncated chunk data", errInvalidAWSChunkedBody)
		return n, r.readError
	}
	if r.remain == 0 {
		r.needCRLF = true
	}
	return n, nil
}

func (r *awsChunkedReader) nextChunkSize() (uint64, error) {
	line, err := r.readLine()
	if err != nil {
		return 0, err
	}
	sizeText, _, _ := strings.Cut(line, ";")
	sizeText = strings.TrimSpace(sizeText)
	size, err := strconv.ParseUint(sizeText, 16, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid chunk size", errInvalidAWSChunkedBody)
	}
	return size, nil
}

func (r *awsChunkedReader) readLine() (string, error) {
	line, err := r.br.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("%w: truncated chunk line", errInvalidAWSChunkedBody)
	}
	if !strings.HasSuffix(line, "\r\n") {
		return "", fmt.Errorf("%w: chunk line missing CRLF", errInvalidAWSChunkedBody)
	}
	return strings.TrimSuffix(line, "\r\n"), nil
}

func (r *awsChunkedReader) consumeCRLF() error {
	cr, err := r.br.ReadByte()
	if err != nil {
		return fmt.Errorf("%w: truncated chunk terminator", errInvalidAWSChunkedBody)
	}
	lf, err := r.br.ReadByte()
	if err != nil {
		return fmt.Errorf("%w: truncated chunk terminator", errInvalidAWSChunkedBody)
	}
	if cr != '\r' || lf != '\n' {
		return fmt.Errorf("%w: chunk data missing CRLF", errInvalidAWSChunkedBody)
	}
	return nil
}

func (r *awsChunkedReader) consumeTrailers() error {
	for {
		line, err := r.readLine()
		if err != nil {
			return err
		}
		if line == "" {
			return nil
		}
	}
}
