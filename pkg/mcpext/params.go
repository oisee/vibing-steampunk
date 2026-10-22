package mcpext

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
)

// Helpers for handlers: reading params the way the built-in actions do, and
// building results. They spare every extension writing its own.

// String returns the first of keys that is set to a non-empty value, as a
// string; numbers are formatted. Several keys allow synonyms
// ("transport", "request").
func String(params map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := params[k].(type) {
		case string:
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		case int:
			return strconv.Itoa(v)
		case json.Number:
			return v.String()
		}
	}
	return ""
}

// Bool reads a boolean param; ok is false when it is absent or not a
// boolean. "true"/"false" strings count, as MCP clients send them.
func Bool(params map[string]any, key string) (value, ok bool) {
	switch v := params[key].(type) {
	case bool:
		return v, true
	case string:
		b, err := strconv.ParseBool(strings.TrimSpace(v))
		return b, err == nil
	}
	return false, false
}

// Int reads an integer param, or def when it is absent or not a number.
func Int(params map[string]any, key string, def int) int {
	switch v := params[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		if n, err := v.Int64(); err == nil {
			return int(n)
		}
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			return n
		}
	}
	return def
}

// Strings reads a list param: a JSON array of strings, or one string with the
// items separated by commas.
func Strings(params map[string]any, key string) []string {
	var out []string
	switch v := params[key].(type) {
	case []any:
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case []string:
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	case string:
		for _, s := range strings.Split(v, ",") {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
	}
	return out
}

// JSON is a result holding v as indented JSON.
func JSON(v any) *mcp.CallToolResult {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return Errorf("encoding the result: %v", err)
	}
	return mcp.NewToolResultText(string(out))
}

// Text is a plain-text result.
func Text(s string) *mcp.CallToolResult {
	return mcp.NewToolResultText(s)
}

// Errorf is an error result: the call failed, and the text says why. Return
// it with a nil error; a non-nil error from a handler is for failures of the
// server itself.
func Errorf(format string, args ...any) *mcp.CallToolResult {
	return mcp.NewToolResultError(fmt.Sprintf(format, args...))
}
