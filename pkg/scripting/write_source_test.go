package scripting

import (
	"context"
	"errors"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	lua "github.com/yuin/gopher-lua"
)

func TestLuaWriteSourceLegacyCallPreservesNilOptions(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	engine.writeSource = func(_ context.Context, objectType, name, source string, opts *adt.WriteSourceOptions) (*adt.WriteSourceResult, error) {
		if objectType != "PROG" || name != "Z_SYNTHETIC" || source != "REPORT z_synthetic." {
			t.Fatalf("unexpected arguments: %q %q %q", objectType, name, source)
		}
		if opts != nil {
			t.Fatalf("legacy three-argument call supplied options: %#v", opts)
		}
		return &adt.WriteSourceResult{Success: true}, nil
	}

	if err := engine.Execute(`ok, message = writeSource("PROG", "Z_SYNTHETIC", "REPORT z_synthetic.")`); err != nil {
		t.Fatal(err)
	}
	if engine.L.GetGlobal("ok") != lua.LTrue {
		t.Fatalf("ok = %v, want true", engine.L.GetGlobal("ok"))
	}
	if engine.L.GetGlobal("message") != lua.LNil {
		t.Fatalf("message = %v, want nil", engine.L.GetGlobal("message"))
	}
}

//nolint:misspell // CLAS is the SAP ADT object type used by the Lua API.
func TestLuaWriteSourceForwardsOptions(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()

	engine.writeSource = func(_ context.Context, _, _, _ string, opts *adt.WriteSourceOptions) (*adt.WriteSourceResult, error) {
		if opts == nil {
			t.Fatal("options were not forwarded")
		}
		want := adt.WriteSourceOptions{
			Mode:        adt.WriteModeUpdate,
			Description: "Synthetic description",
			Package:     "$TMP",
			TestSource:  "test source",
			Transport:   "REQ-TEST-001",
			Method:      "RUN",
		}
		if *opts != want {
			t.Fatalf("options = %#v, want %#v", *opts, want)
		}
		return &adt.WriteSourceResult{Success: true}, nil
	}

	script := `ok, message = writeSource("CLAS", "ZCL_SYNTHETIC", "source", {
		mode = "UPDATE",
		description = "Synthetic description",
		package = "$TMP",
		test_source = "test source",
		transport = "REQ-TEST-001",
		method = "RUN"
	})`
	if err := engine.Execute(script); err != nil {
		t.Fatal(err)
	}
	if engine.L.GetGlobal("ok") != lua.LTrue {
		t.Fatalf("ok = %v, want true", engine.L.GetGlobal("ok"))
	}
}

func TestLuaWriteSourceReturnsDiagnosticForUnsuccessfulResult(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()
	engine.writeSource = func(context.Context, string, string, string, *adt.WriteSourceOptions) (*adt.WriteSourceResult, error) {
		return &adt.WriteSourceResult{Success: false, Message: "synthetic activation failure"}, nil
	}

	if err := engine.Execute(`ok, message = writeSource("PROG", "Z_SYNTHETIC", "source", {})`); err != nil {
		t.Fatal(err)
	}
	if engine.L.GetGlobal("ok") != lua.LFalse {
		t.Fatalf("ok = %v, want false", engine.L.GetGlobal("ok"))
	}
	if got := lua.LVAsString(engine.L.GetGlobal("message")); got != "synthetic activation failure" {
		t.Fatalf("message = %q", got)
	}
}

func TestLuaWriteSourceReturnsGoError(t *testing.T) {
	engine := NewLuaEngine(nil)
	defer engine.Close()
	engine.writeSource = func(context.Context, string, string, string, *adt.WriteSourceOptions) (*adt.WriteSourceResult, error) {
		return nil, errors.New("synthetic request error")
	}

	if err := engine.Execute(`ok, message = writeSource("PROG", "Z_SYNTHETIC", "source")`); err != nil {
		t.Fatal(err)
	}
	if engine.L.GetGlobal("ok") != lua.LFalse {
		t.Fatalf("ok = %v, want false", engine.L.GetGlobal("ok"))
	}
	if got := lua.LVAsString(engine.L.GetGlobal("message")); got != "synthetic request error" {
		t.Fatalf("message = %q", got)
	}
}
