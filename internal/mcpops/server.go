package mcpops

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

const protocolVersion = "2024-11-05"

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

type contentBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type toolCallResult struct {
	Content []contentBlock `json:"content"`
	IsError bool           `json:"isError,omitempty"`
}

func RunStdio(ctx context.Context, cfg Config, input io.Reader, output io.Writer) error {
	cfg = cfg.Normalized()
	reader := bufio.NewReader(input)
	writer := bufio.NewWriter(output)
	for {
		payload, err := readFrame(reader)
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		response := handleRPC(ctx, cfg, payload)
		if response == nil {
			continue
		}
		if err := writeFrame(writer, response); err != nil {
			return err
		}
		if err := writer.Flush(); err != nil {
			return err
		}
	}
}

func handleRPC(ctx context.Context, cfg Config, payload []byte) *rpcResponse {
	var req rpcRequest
	if err := json.Unmarshal(payload, &req); err != nil {
		return errorResponse(nil, -32700, "parse error", err.Error())
	}
	if len(req.ID) == 0 && strings.HasPrefix(req.Method, "notifications/") {
		return nil
	}
	if req.JSONRPC != "" && req.JSONRPC != "2.0" {
		return errorResponse(req.ID, -32600, "invalid request", "jsonrpc must be 2.0")
	}
	result, err := dispatchRPC(ctx, cfg, req)
	if err != nil {
		return rpcErrorFor(req.ID, err)
	}
	if len(req.ID) == 0 {
		return nil
	}
	return &rpcResponse{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func dispatchRPC(ctx context.Context, cfg Config, req rpcRequest) (any, error) {
	switch req.Method {
	case "initialize":
		return map[string]any{
			"protocolVersion": protocolVersion,
			"capabilities": map[string]any{
				"resources": map[string]any{},
				"tools":     map[string]any{},
				"prompts":   map[string]any{},
			},
			"serverInfo": map[string]any{
				"name":    "namros-mcp",
				"version": VersionInfo()["version"],
			},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "resources/list":
		return map[string]any{"resources": Resources()}, nil
	case "resources/read":
		var params struct {
			URI string `json:"uri"`
		}
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		value, err := ReadResource(ctx, cfg, params.URI)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"contents": []map[string]any{{
				"uri":      params.URI,
				"mimeType": "application/json",
				"text":     jsonText(value),
			}},
		}, nil
	case "tools/list":
		return map[string]any{"tools": Tools()}, nil
	case "tools/call":
		var params struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := decodeParams(req.Params, &params); err != nil {
			return nil, invalidParams(err)
		}
		value, err := CallTool(ctx, cfg, params.Name, params.Arguments)
		if err != nil {
			return nil, err
		}
		return toolCallResult{Content: []contentBlock{{Type: "text", Text: jsonText(value)}}}, nil
	case "prompts/list":
		return map[string]any{"prompts": PromptDefinitions()}, nil
	default:
		return nil, methodNotFound(req.Method)
	}
}

func PromptDefinitions() []map[string]any {
	return []map[string]any{
		{"name": "namros-incident-triage", "description": "Classify NAMROS symptoms and suggest maintained runbooks."},
		{"name": "namros-compat-failure-analysis", "description": "Analyze compatibility smoke output for AWS CLI, MinIO client, rclone, or s3fs-fuse."},
		{"name": "namros-release-preflight", "description": "Summarize release gate readiness and missing smoke evidence."},
		{"name": "namros-enterprise-feature-denied", "description": "Explain Community versus Enterprise denial responses without treating them as outages."},
	}
}

func decodeParams(raw json.RawMessage, target any) error {
	if len(raw) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	return decoder.Decode(target)
}

func jsonText(value any) string {
	payload, err := json.MarshalIndent(Redact(value), "", "  ")
	if err != nil {
		return fmt.Sprintf(`{"error":%q}`, err.Error())
	}
	return string(payload)
}

func readFrame(reader *bufio.Reader) ([]byte, error) {
	contentLength := -1
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break
		}
		name, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(name), "Content-Length") {
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length %q: %w", value, err)
			}
			contentLength = parsed
		}
	}
	if contentLength < 0 {
		return nil, errors.New("missing Content-Length header")
	}
	payload := make([]byte, contentLength)
	_, err := io.ReadFull(reader, payload)
	return payload, err
}

func writeFrame(writer *bufio.Writer, response *rpcResponse) error {
	payload, err := json.Marshal(response)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(writer, "Content-Length: %d\r\n\r\n", len(payload)); err != nil {
		return err
	}
	_, err = writer.Write(payload)
	return err
}

func rpcErrorFor(id json.RawMessage, err error) *rpcResponse {
	var unknownResource UnknownResourceError
	var unknownTool UnknownToolError
	switch {
	case errors.As(err, &unknownResource), errors.As(err, &unknownTool):
		return errorResponse(id, -32602, "invalid params", err.Error())
	case isMethodNotFound(err):
		return errorResponse(id, -32601, "method not found", err.Error())
	case isInvalidParams(err):
		return errorResponse(id, -32602, "invalid params", err.Error())
	default:
		return errorResponse(id, -32603, "internal error", err.Error())
	}
}

func errorResponse(id json.RawMessage, code int, message string, data any) *rpcResponse {
	return &rpcResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error:   &rpcError{Code: code, Message: message, Data: data},
	}
}

type methodNotFoundError string

func (e methodNotFoundError) Error() string {
	return string(e)
}

func methodNotFound(method string) error {
	return methodNotFoundError(fmt.Sprintf("method %q is not supported", method))
}

func isMethodNotFound(err error) bool {
	var target methodNotFoundError
	return errors.As(err, &target)
}

type invalidParamsError struct {
	err error
}

func (e invalidParamsError) Error() string {
	return e.err.Error()
}

func invalidParams(err error) error {
	return invalidParamsError{err: err}
}

func isInvalidParams(err error) bool {
	var target invalidParamsError
	return errors.As(err, &target)
}
