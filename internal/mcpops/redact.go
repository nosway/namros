package mcpops

import (
	"bytes"
	"encoding/json"
	"strings"
)

const redacted = "[REDACTED]"

func Redact(value any) any {
	payload, err := json.Marshal(value)
	if err != nil {
		return value
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var decoded any
	if err := decoder.Decode(&decoded); err != nil {
		return value
	}
	return redactDecoded(decoded)
}

func RedactText(value string) string {
	var decoded any
	decoder := json.NewDecoder(strings.NewReader(value))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err == nil {
		payload, marshalErr := json.MarshalIndent(redactDecoded(decoded), "", "  ")
		if marshalErr == nil {
			return string(payload)
		}
	}
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lower := strings.ToLower(line)
		if containsSecretKey(lower) {
			lines[i] = redacted
		}
	}
	return strings.Join(lines, "\n")
}

func redactDecoded(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if containsSecretKey(strings.ToLower(key)) {
				out[key] = redacted
				continue
			}
			out[key] = redactDecoded(child)
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = redactDecoded(child)
		}
		return out
	default:
		return value
	}
}

func containsSecretKey(key string) bool {
	secretFragments := []string{
		"authorization",
		"access_key",
		"secret",
		"session_token",
		"credential",
		"signature",
		"presigned",
		"password",
		"token",
		"kms_material",
		"private_key",
	}
	for _, fragment := range secretFragments {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}
