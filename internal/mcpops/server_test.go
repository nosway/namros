package mcpops

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestRunStdioProtocolSmoke(t *testing.T) {
	requests := []map[string]any{
		{"jsonrpc": "2.0", "id": 1, "method": "initialize"},
		{"jsonrpc": "2.0", "id": 2, "method": "resources/list"},
		{"jsonrpc": "2.0", "id": 3, "method": "tools/list"},
		{
			"jsonrpc": "2.0",
			"id":      4,
			"method":  "tools/call",
			"params": map[string]any{
				"name":      "namros.chaos.report.latest",
				"arguments": map[string]any{},
			},
		},
	}
	input := bytes.NewBuffer(nil)
	for _, request := range requests {
		input.Write(mustMCPFrame(t, request))
	}
	output := bytes.NewBuffer(nil)
	if err := RunStdio(context.Background(), DefaultConfig(), input, output); err != nil {
		t.Fatalf("RunStdio() error = %v", err)
	}
	reader := bufio.NewReader(output)
	responses := make([]rpcResponse, 0, len(requests))
	for range requests {
		payload, err := readFrame(reader)
		if err != nil {
			t.Fatalf("read response frame: %v", err)
		}
		var response rpcResponse
		if err := json.Unmarshal(payload, &response); err != nil {
			t.Fatalf("response JSON decode: %v", err)
		}
		if response.JSONRPC != "2.0" {
			t.Fatalf("jsonrpc = %q, want 2.0", response.JSONRPC)
		}
		if response.Error != nil {
			t.Fatalf("response error = %+v", response.Error)
		}
		responses = append(responses, response)
	}
	if !strings.Contains(string(mustJSON(t, responses[1].Result)), "namros://product/edition") {
		t.Fatalf("resources/list result = %s", mustJSON(t, responses[1].Result))
	}
	if !strings.Contains(string(mustJSON(t, responses[2].Result)), "namros.health.check") {
		t.Fatalf("tools/list result = %s", mustJSON(t, responses[2].Result))
	}
	if !enterpriseOverlayTest() && !strings.Contains(string(mustJSON(t, responses[3].Result)), "enterprise_required") {
		t.Fatalf("tools/call result = %s", mustJSON(t, responses[3].Result))
	}
}

func mustMCPFrame(t *testing.T, value any) []byte {
	t.Helper()
	payload := mustJSON(t, value)
	return []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload))
}

func mustJSON(t *testing.T, value any) []byte {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	return payload
}
