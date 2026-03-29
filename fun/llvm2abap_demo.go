// Demo: LLVM IR → ABAP compilation
// Usage: go run fun/llvm2abap_demo.go <file.ll>
//    or: go run fun/llvm2abap_demo.go (uses built-in example)
package main

import (
	"fmt"
	"os"
	"os/exec"
	"github.com/oisee/vibing-steampunk/pkg/llvm2abap"
)

const exampleC = `
int add(int a, int b) { return a + b; }
int factorial(int n) {
    int r = 1;
    for (int i = 2; i <= n; i++) r *= i;
    return r;
}
int fibonacci(int n) {
    if (n <= 1) return n;
    int a = 0, b = 1;
    for (int i = 2; i <= n; i++) {
        int t = a + b; a = b; b = t;
    }
    return b;
}
double lerp(double a, double b, double t) { return a + (b - a) * t; }
`

func main() {
	var llFile string

	if len(os.Args) > 1 {
		llFile = os.Args[1]
	} else {
		// Compile built-in example
		fmt.Println("=== No .ll file provided, compiling built-in example ===")
		os.WriteFile("/tmp/llvm2abap_example.c", []byte(exampleC), 0644)
		cmd := exec.Command("clang", "-S", "-emit-llvm", "-O1",
			"/tmp/llvm2abap_example.c", "-o", "/tmp/llvm2abap_example.ll")
		if out, err := cmd.CombinedOutput(); err != nil {
			fmt.Printf("clang error: %v\n%s\n", err, out)
			os.Exit(1)
		}
		llFile = "/tmp/llvm2abap_example.ll"
		fmt.Printf("C source:\n%s\n", exampleC)
	}

	src, err := os.ReadFile(llFile)
	if err != nil {
		fmt.Printf("Error reading %s: %v\n", llFile, err)
		os.Exit(1)
	}

	mod, err := llvm2abap.Parse(string(src))
	if err != nil {
		fmt.Printf("Parse error: %v\n", err)
		os.Exit(1)
	}

	nonExt := 0
	for _, fn := range mod.Functions {
		if !fn.IsExternal {
			nonExt++
		}
	}
	fmt.Printf("Parsed: %d functions, %d structs\n\n", nonExt, len(mod.Types))

	abap := llvm2abap.Compile(mod, "zcl_compiled")
	fmt.Println(abap)
}
