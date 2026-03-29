package wasmcomp

import (
	"context"
	"os"
	"testing"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
	"github.com/tetratelabs/wazero/imports/wasi_snapshot_preview1"
	"github.com/tetratelabs/wazero/sys"
)

// TestExecute_Suite runs every test case from the extended suite through wazero
// and verifies the WASM binaries produce the expected results.
func TestExecute_Suite(t *testing.T) {
	ctx := context.Background()
	wasm := buildExtendedSuiteWasm()

	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	mod, err := rt.InstantiateWithConfig(ctx, wasm,
		wazero.NewModuleConfig().WithName("suite"))
	if err != nil {
		t.Fatalf("Instantiate failed: %v", err)
	}

	for _, tc := range extendedTestCases {
		t.Run(tc.Name, func(t *testing.T) {
			fn := mod.ExportedFunction(tc.FuncName)
			if fn == nil {
				t.Fatalf("function %q not exported", tc.FuncName)
			}

			args := make([]uint64, len(tc.Args))
			for i, a := range tc.Args {
				args[i] = api.EncodeI32(a)
			}

			results, err := fn.Call(ctx, args...)
			if err != nil {
				t.Fatalf("Call failed: %v", err)
			}

			got := api.DecodeI32(results[0])
			if got != tc.Expected {
				t.Errorf("got %d, want %d", got, tc.Expected)
			}
		})
	}
}

// TestExecute_Individual runs each single-function WASM through wazero.
func TestExecute_Individual(t *testing.T) {
	ctx := context.Background()

	type singleTest struct {
		name    string
		builder func() []byte
		fn      string
		cases   []WASMTestCase
	}

	tests := []singleTest{
		{"add", buildAddWasm, "add", []WASMTestCase{
			{"2+3", "add", []int32{2, 3}, 5},
			{"0+0", "add", []int32{0, 0}, 0},
			{"-1+1", "add", []int32{-1, 1}, 0},
			{"max", "add", []int32{2147483647, 0}, 2147483647},
		}},
		{"factorial", buildFactorialWasm, "factorial", []WASMTestCase{
			{"0!", "factorial", []int32{0}, 1},
			{"1!", "factorial", []int32{1}, 1},
			{"5!", "factorial", []int32{5}, 120},
			{"10!", "factorial", []int32{10}, 3628800},
			{"12!", "factorial", []int32{12}, 479001600},
		}},
		{"fibonacci", buildFibonacciWasm, "fibonacci", []WASMTestCase{
			{"fib(0)", "fibonacci", []int32{0}, 0},
			{"fib(1)", "fibonacci", []int32{1}, 1},
			{"fib(10)", "fibonacci", []int32{10}, 55},
			{"fib(20)", "fibonacci", []int32{20}, 6765},
		}},
		{"abs", buildAbsWasm, "abs", []WASMTestCase{
			{"abs(5)", "abs", []int32{5}, 5},
			{"abs(-5)", "abs", []int32{-5}, 5},
			{"abs(0)", "abs", []int32{0}, 0},
		}},
		{"negate", buildNegateWasm, "negate", []WASMTestCase{
			{"neg(5)", "negate", []int32{5}, -5},
			{"neg(-3)", "negate", []int32{-3}, 3},
			{"neg(0)", "negate", []int32{0}, 0},
		}},
		{"sum_to", buildSumToWasm, "sum_to", []WASMTestCase{
			{"sum(10)", "sum_to", []int32{10}, 55},
			{"sum(100)", "sum_to", []int32{100}, 5050},
			{"sum(0)", "sum_to", []int32{0}, 0},
		}},
		{"collatz", buildCollatzWasm, "collatz", []WASMTestCase{
			{"col(1)", "collatz", []int32{1}, 0},
			{"col(6)", "collatz", []int32{6}, 8},
			{"col(27)", "collatz", []int32{27}, 111},
		}},
		{"pow2", buildPow2Wasm, "pow2", []WASMTestCase{
			{"2^0", "pow2", []int32{0}, 1},
			{"2^1", "pow2", []int32{1}, 2},
			{"2^10", "pow2", []int32{10}, 1024},
			{"2^20", "pow2", []int32{20}, 1048576},
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rt := wazero.NewRuntime(ctx)
			defer rt.Close(ctx)

			wasm := tt.builder()
			mod, err := rt.InstantiateWithConfig(ctx, wasm,
				wazero.NewModuleConfig().WithName(tt.name))
			if err != nil {
				t.Fatalf("Instantiate %s: %v", tt.name, err)
			}

			fn := mod.ExportedFunction(tt.fn)
			if fn == nil {
				t.Fatalf("function %q not exported", tt.fn)
			}

			for _, tc := range tt.cases {
				args := make([]uint64, len(tc.Args))
				for i, a := range tc.Args {
					args[i] = api.EncodeI32(a)
				}
				results, err := fn.Call(ctx, args...)
				if err != nil {
					t.Errorf("%s: call failed: %v", tc.Name, err)
					continue
				}
				got := api.DecodeI32(results[0])
				if got != tc.Expected {
					t.Errorf("%s: got %d, want %d", tc.Name, got, tc.Expected)
				}
			}
		})
	}
}

// TestExecute_QuickJS instantiates the full QuickJS WASM via wazero with WASI,
// proving the binary is valid and _start can be invoked.
func TestExecute_QuickJS(t *testing.T) {
	data, err := os.ReadFile("testdata/quickjs_eval.wasm")
	if err != nil {
		t.Skipf("QuickJS WASM not found: %v", err)
	}

	ctx := context.Background()
	rt := wazero.NewRuntime(ctx)
	defer rt.Close(ctx)

	// QuickJS needs WASI
	wasi_snapshot_preview1.MustInstantiate(ctx, rt)

	cfg := wazero.NewModuleConfig().
		WithName("quickjs").
		WithStdout(os.Stdout).
		WithStderr(os.Stderr)

	mod, err := rt.InstantiateWithConfig(ctx, data, cfg)
	if err != nil {
		// QuickJS _start runs automatically on instantiate with WASI.
		// If JS code calls proc_exit(0), wazero returns an ExitError.
		if exitErr, ok := err.(*sys.ExitError); ok {
			t.Logf("QuickJS exited with code %d", exitErr.ExitCode())
			if exitErr.ExitCode() == 0 {
				t.Log("PASS: QuickJS executed successfully (exit code 0)")
				return
			}
			t.Fatalf("QuickJS exited with non-zero code: %d", exitErr.ExitCode())
		}
		t.Fatalf("Instantiate QuickJS failed: %v", err)
	}
	defer mod.Close(ctx)

	t.Log("PASS: QuickJS module instantiated successfully")

	// Check exports
	start := mod.ExportedFunction("_start")
	mainVoid := mod.ExportedFunction("__main_void")
	t.Logf("Exports: _start=%v, __main_void=%v", start != nil, mainVoid != nil)
}
