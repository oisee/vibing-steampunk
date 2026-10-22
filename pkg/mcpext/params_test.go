package mcpext

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestParams(t *testing.T) {
	p := map[string]any{
		"transport": "  ", "request": "A4HK900001", "count": float64(3), "num": json.Number("7"),
		"flag": true, "flagstr": "false", "list": []any{"A", " ", "B"}, "csv": "x, y,,z", "n": "12",
	}
	if got := String(p, "transport", "request"); got != "A4HK900001" {
		t.Errorf("String synonyms = %q", got)
	}
	if got := String(p, "count"); got != "3" {
		t.Errorf("String of a number = %q", got)
	}
	if v, ok := Bool(p, "flag"); !v || !ok {
		t.Error("Bool(true)")
	}
	if v, ok := Bool(p, "flagstr"); v || !ok {
		t.Error(`Bool("false")`)
	}
	if _, ok := Bool(p, "missing"); ok {
		t.Error("Bool of a missing param")
	}
	if Int(p, "count", 0) != 3 || Int(p, "num", 0) != 7 || Int(p, "n", 0) != 12 || Int(p, "missing", 5) != 5 {
		t.Error("Int")
	}
	if got := Strings(p, "list"); !reflect.DeepEqual(got, []string{"A", "B"}) {
		t.Errorf("Strings(array) = %v", got)
	}
	if got := Strings(p, "csv"); !reflect.DeepEqual(got, []string{"x", "y", "z"}) {
		t.Errorf("Strings(csv) = %v", got)
	}
	if res := Errorf("no %s", "luck"); !res.IsError {
		t.Error("Errorf is no error result")
	}
	if res := JSON(map[string]int{"a": 1}); res.IsError {
		t.Error("JSON is an error result")
	}
}
