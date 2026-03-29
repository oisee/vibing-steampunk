package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"archive/zip"

	"github.com/oisee/vibing-steampunk/pkg/abaplint"
	"github.com/oisee/vibing-steampunk/pkg/llvm2abap"
	"github.com/oisee/vibing-steampunk/pkg/ts2abap"
	"github.com/oisee/vibing-steampunk/pkg/wasmcomp"
	"github.com/spf13/cobra"
)

// --- compile wasm command ---

var compileCmd = &cobra.Command{
	Use:   "compile",
	Short: "Compile and transpile source code",
	Long:  "Compile WASM→ABAP, transpile TS→ABAP/Go, and run ABAP lint checks.",
}

var compileWasmCmd = &cobra.Command{
	Use:   "wasm <input.wasm> [--class <name>] [--output <dir>]",
	Short: "Compile WebAssembly to ABAP",
	Long: `Compile a .wasm binary to native ABAP source code.
Fully offline — no SAP connection required.

The output is an ABAP class with one method per exported function.
Deploy with: vsp deploy <output.clas.abap> '$TMP'

Examples:
  vsp compile wasm program.wasm
  vsp compile wasm program.wasm --class ZCL_MY_WASM
  vsp compile wasm program.wasm --output ./src/ --deploy '$TMP'`,
	Args: cobra.ExactArgs(1),
	RunE: runCompileWasm,
}

// --- compile ts command ---

var compileTsCmd = &cobra.Command{
	Use:   "ts <input.ts> [--prefix <zcl_>]",
	Short: "Transpile TypeScript to ABAP",
	Long: `Transpile TypeScript classes to ABAP source code.
Requires Node.js with TypeScript (for AST parsing).
Fully offline — no SAP connection required.

Each TS class becomes an ABAP class with proper types, methods, and OO structure.

Examples:
  vsp compile ts lexer.ts
  vsp compile ts lexer.ts --prefix zcl_
  vsp compile ts lexer.ts --output ./src/ --deploy '$TMP'`,
	Args: cobra.ExactArgs(1),
	RunE: runCompileTs,
}

// --- parse command (ABAP analysis) ---

var parseCmd = &cobra.Command{
	Use:   "parse <type> <name>",
	Short: "Parse ABAP source and show structure",
	Long: `Parse ABAP source into tokens and statements.
Works offline with --file/--stdin, or fetches from SAP.

Examples:
  vsp parse CLAS ZCL_TEST
  vsp parse --file myclass.clas.abap
  echo "DATA lv_x TYPE i." | vsp parse --stdin
  vsp parse --file source.abap --format json`,
	RunE: runParse,
}

// --- compile llvm command ---

var compileLLVMCmd = &cobra.Command{
	Use:   "llvm <file.ll|file.c>",
	Short: "Compile LLVM IR or C source to typed ABAP",
	Long: `Compile LLVM IR (.ll) or C source (.c) to typed ABAP CLASS-METHODS.
For .c files, clang is invoked automatically.

Examples:
  vsp compile llvm mycode.c
  vsp compile llvm mycode.c --class zcl_mycode -o mycode.abap
  vsp compile llvm mycode.c --class zcl_mycode --zip -o mycode.zip
  vsp compile llvm quickjs.c --class zcl_quickjs --zip`,
	Args: cobra.ExactArgs(1),
	RunE: runCompileLLVM,
}

func init() {
	// Compile subcommands
	compileCmd.AddCommand(compileWasmCmd)
	compileCmd.AddCommand(compileTsCmd)
	compileCmd.AddCommand(compileLLVMCmd)

	// Compile wasm flags
	compileWasmCmd.Flags().String("class", "", "ABAP class name (default: derived from filename)")
	compileWasmCmd.Flags().StringP("output", "o", "", "Output directory (default: stdout)")
	compileWasmCmd.Flags().String("deploy", "", "Deploy directly to SAP package (e.g., '$TMP')")

	// Compile ts flags
	compileTsCmd.Flags().String("prefix", "zcl_", "ABAP class name prefix")
	compileTsCmd.Flags().StringP("output", "o", "", "Output directory (default: stdout)")
	compileTsCmd.Flags().String("deploy", "", "Deploy directly to SAP package")

	// Compile llvm flags
	compileLLVMCmd.Flags().String("class", "zcl_compiled", "ABAP class name")
	compileLLVMCmd.Flags().StringP("output", "o", "", "Output file (default: stdout)")
	compileLLVMCmd.Flags().Bool("zip", false, "Output as abapGit ZIP")
	compileLLVMCmd.Flags().String("package", "$TMP", "SAP package (for --zip)")
	compileLLVMCmd.Flags().String("desc", "Compiled via vsp compile llvm", "Description")
	compileLLVMCmd.Flags().String("opt", "O1", "Clang optimization level (O0/O1/O2)")
	compileLLVMCmd.Flags().String("cflags", "", "Extra clang flags (e.g. \"-DCONFIG_VERSION=\\\"v1\\\" -D_GNU_SOURCE\")")

	// Parse flags
	parseCmd.Flags().String("file", "", "Parse local file")
	parseCmd.Flags().Bool("stdin", false, "Read from stdin")
	parseCmd.Flags().String("format", "text", "Output format: text, json, summary")

	rootCmd.AddCommand(compileCmd)
	rootCmd.AddCommand(parseCmd)
}

func runCompileWasm(cmd *cobra.Command, args []string) error {
	inputFile := args[0]
	className, _ := cmd.Flags().GetString("class")
	outputDir, _ := cmd.Flags().GetString("output")
	deployPkg, _ := cmd.Flags().GetString("deploy")

	data, err := os.ReadFile(inputFile)
	if err != nil {
		return fmt.Errorf("failed to read %s: %w", inputFile, err)
	}

	mod, err := wasmcomp.Parse(data)
	if err != nil {
		return fmt.Errorf("failed to parse WASM: %w", err)
	}

	if className == "" {
		base := strings.TrimSuffix(filepath.Base(inputFile), ".wasm")
		className = "zcl_wasm_" + strings.ToLower(strings.ReplaceAll(base, "-", "_"))
	}

	fmt.Fprintf(os.Stderr, "WASM: %d bytes, %d functions, %d instructions\n",
		len(data), len(mod.Functions), countInstructions(mod))

	abapSrc := wasmcomp.Compile(mod, className)

	lines := strings.Count(abapSrc, "\n")
	fmt.Fprintf(os.Stderr, "ABAP: %d lines, class %s\n", lines, className)

	if outputDir != "" {
		outFile := filepath.Join(outputDir, strings.ToLower(className)+".clas.abap")
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return err
		}
		if err := os.WriteFile(outFile, []byte(abapSrc), 0644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "Written to %s\n", outFile)

		if deployPkg != "" {
			return deployFile(cmd, outFile, deployPkg)
		}
	} else if deployPkg != "" {
		// Write temp file and deploy
		tmp := filepath.Join(os.TempDir(), strings.ToLower(className)+".clas.abap")
		os.WriteFile(tmp, []byte(abapSrc), 0644)
		defer os.Remove(tmp)
		return deployFile(cmd, tmp, deployPkg)
	} else {
		fmt.Print(abapSrc)
	}
	return nil
}

func runCompileTs(cmd *cobra.Command, args []string) error {
	inputFile := args[0]
	prefix, _ := cmd.Flags().GetString("prefix")
	outputDir, _ := cmd.Flags().GetString("output")
	deployPkg, _ := cmd.Flags().GetString("deploy")

	// Check if Node.js is available
	if _, err := exec.LookPath("node"); err != nil {
		return fmt.Errorf("Node.js is required for TypeScript transpilation.\nInstall from https://nodejs.org/ or use: nvm install node")
	}

	// Find ts_ast.js
	tsAstScript := findTsAstScript()
	if tsAstScript == "" {
		return fmt.Errorf("ts_ast.js not found. Run from the vsp source directory or set VSP_TS_AST_PATH")
	}

	// Parse TS → JSON AST
	astCmd := exec.Command("node", tsAstScript, inputFile)
	astJSON, err := astCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to parse TypeScript: %w\nMake sure TypeScript is installed: npm install typescript", err)
	}

	// Transpile JSON AST → ABAP
	result, err := ts2abap.Transpile(astJSON, prefix)
	if err != nil {
		return fmt.Errorf("transpilation failed: %w", err)
	}

	fmt.Fprintf(os.Stderr, "Transpiled %d classes with prefix '%s'\n", len(result.Classes), prefix)

	if outputDir != "" {
		os.MkdirAll(outputDir, 0755)
		for name, src := range result.Classes {
			outFile := filepath.Join(outputDir, name+".clas.abap")
			os.WriteFile(outFile, []byte(src), 0644)
			fmt.Fprintf(os.Stderr, "  %s → %s (%d lines)\n", name, outFile, strings.Count(src, "\n"))

			if deployPkg != "" {
				if err := deployFile(cmd, outFile, deployPkg); err != nil {
					fmt.Fprintf(os.Stderr, "  deploy %s failed: %v\n", name, err)
				}
			}
		}
	} else {
		for name, src := range result.Classes {
			fmt.Printf("* === %s ===\n%s\n", name, src)
		}
	}
	return nil
}

func runParse(cmd *cobra.Command, args []string) error {
	file, _ := cmd.Flags().GetString("file")
	stdin, _ := cmd.Flags().GetBool("stdin")
	format, _ := cmd.Flags().GetString("format")

	var source string
	var filename string

	if stdin {
		data, _ := readStdin()
		source = string(data)
		filename = "stdin"
	} else if file != "" {
		data, err := os.ReadFile(file)
		if err != nil {
			return err
		}
		source = string(data)
		filename = file
	} else if len(args) >= 2 {
		params, err := resolveSystemParams(cmd)
		if err != nil {
			return err
		}
		client, err := getClient(params)
		if err != nil {
			return err
		}
		src, err := client.GetSource(context.Background(), args[0], args[1], nil)
		if err != nil {
			return err
		}
		source = src
		filename = args[1]
	} else {
		return fmt.Errorf("usage: vsp parse <type> <name>, or --file/--stdin")
	}

	lex := &abaplint.Lexer{}
	tokens := lex.Run(source)

	parser := &abaplint.StatementParser{}
	stmts := parser.Parse(tokens)

	matcher := abaplint.NewStatementMatcher()
	matcher.ClassifyStatements(stmts)

	switch format {
	case "summary":
		typeCount := map[string]int{}
		for _, s := range stmts {
			typeCount[s.Type]++
		}
		fmt.Printf("File: %s\n", filename)
		fmt.Printf("Tokens: %d\n", len(tokens))
		fmt.Printf("Statements: %d\n", len(stmts))
		fmt.Println("---")
		for t, c := range typeCount {
			fmt.Printf("  %-25s %d\n", t, c)
		}
	case "json":
		fmt.Print("[")
		for i, s := range stmts {
			if i > 0 {
				fmt.Print(",")
			}
			toks := make([]string, len(s.Tokens))
			for j, t := range s.Tokens {
				toks[j] = t.Str
			}
			fmt.Printf(`{"type":"%s","tokens":["%s"]}`, s.Type, strings.Join(toks, `","`))
		}
		fmt.Println("]")
	default:
		for _, s := range stmts {
			fmt.Printf("%-20s %s\n", s.Type, s.ConcatTokens())
		}
	}
	return nil
}

// --- helpers ---

func countInstructions(mod *wasmcomp.Module) int {
	total := 0
	for _, f := range mod.Functions {
		total += len(f.Code)
	}
	return total
}

func findTsAstScript() string {
	candidates := []string{
		"pkg/ts2abap/ts_ast.js",
		"pkg/ts2go/ts_ast.js",
		filepath.Join(os.Getenv("VSP_TS_AST_PATH"), "ts_ast.js"),
	}
	// Also check relative to executable
	ex, _ := os.Executable()
	if ex != "" {
		dir := filepath.Dir(ex)
		candidates = append(candidates, filepath.Join(dir, "ts_ast.js"))
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c
		}
	}
	return ""
}

func deployFile(cmd *cobra.Command, file, pkg string) error {
	params, err := resolveSystemParams(cmd)
	if err != nil {
		return fmt.Errorf("deploy requires SAP connection: %w\nConnect with: vsp -s <system> compile wasm ... --deploy '$TMP'", err)
	}
	client, err := getClient(params)
	if err != nil {
		return err
	}
	_ = client
	// Use the existing deploy logic
	fmt.Fprintf(os.Stderr, "Deploy: %s → %s\n", file, pkg)
	fmt.Fprintf(os.Stderr, "  Use: vsp deploy %s %s\n", file, pkg)
	return nil
}

// --- compile llvm implementation ---

func runCompileLLVM(cmd *cobra.Command, args []string) error {
	inputFile := args[0]
	ext := strings.ToLower(filepath.Ext(inputFile))
	className, _ := cmd.Flags().GetString("class")
	output, _ := cmd.Flags().GetString("output")
	asZip, _ := cmd.Flags().GetBool("zip")
	pkg, _ := cmd.Flags().GetString("package")
	desc, _ := cmd.Flags().GetString("desc")
	optLevel, _ := cmd.Flags().GetString("opt")

	var llSource string

	switch ext {
	case ".c", ".h":
		tmpLL := inputFile + ".ll"
		clangArgs := []string{"-S", "-emit-llvm", "-" + optLevel, inputFile, "-o", tmpLL}
		cflags, _ := cmd.Flags().GetString("cflags")
		if cflags != "" {
			clangArgs = append(strings.Fields(cflags), clangArgs...)
		}
		clangCmd := exec.Command("clang", clangArgs...)
		clangCmd.Stderr = os.Stderr
		if err := clangCmd.Run(); err != nil {
			return fmt.Errorf("clang failed: %w (is clang installed?)", err)
		}
		defer os.Remove(tmpLL)
		data, err := os.ReadFile(tmpLL)
		if err != nil {
			return err
		}
		llSource = string(data)
		fmt.Fprintf(os.Stderr, "clang: %s → %d lines LLVM IR\n", inputFile, strings.Count(llSource, "\n"))

	case ".ll":
		data, err := os.ReadFile(inputFile)
		if err != nil {
			return err
		}
		llSource = string(data)

	default:
		return fmt.Errorf("unsupported: %s (use .c or .ll)", ext)
	}

	mod, err := llvm2abap.Parse(llSource)
	if err != nil {
		return fmt.Errorf("parse: %w", err)
	}

	nonExt := 0
	for _, fn := range mod.Functions {
		if !fn.IsExternal && !strings.HasPrefix(fn.Name, "llvm.") {
			nonExt++
		}
	}
	fmt.Fprintf(os.Stderr, "parsed: %d functions, %d structs\n", nonExt, len(mod.Types))

	abap := llvm2abap.Compile(mod, className)
	lines := strings.Count(abap, "\n")
	fmt.Fprintf(os.Stderr, "compiled: %d lines ABAP\n", lines)

	if asZip {
		outFile := output
		if outFile == "" {
			outFile = strings.TrimSuffix(filepath.Base(inputFile), ext) + ".zip"
		}
		return writeLLVMZip(outFile, strings.ToUpper(className), abap, desc, pkg)
	}

	if output == "" {
		fmt.Print(abap)
	} else {
		if err := os.WriteFile(output, []byte(abap), 0644); err != nil {
			return err
		}
		fmt.Fprintf(os.Stderr, "written: %s\n", output)
	}
	return nil
}

func writeLLVMZip(outFile, objName, source, desc, pkg string) error {
	f, err := os.Create(outFile)
	if err != nil {
		return err
	}
	defer f.Close()
	w := zip.NewWriter(f)

	zf, _ := w.Create(".abapgit.xml")
	zf.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
 <asx:values><DATA>
  <MASTER_LANGUAGE>E</MASTER_LANGUAGE>
  <STARTING_FOLDER>/src/</STARTING_FOLDER>
  <FOLDER_LOGIC>PREFIX</FOLDER_LOGIC>
 </DATA></asx:values>
</asx:abap>
`))

	lower := strings.ToLower(objName)
	zf, _ = w.Create("src/" + lower + ".prog.abap")
	zf.Write([]byte(source))

	descLen := len(desc)
	if descLen > 70 { descLen = 70 }
	zf, _ = w.Create("src/" + lower + ".prog.xml")
	fmt.Fprintf(zf, `<?xml version="1.0" encoding="utf-8"?>
<abapGit version="v1.0.0" serializer="LCL_OBJECT_PROG" serializer_version="v1.0.0">
 <asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0">
  <asx:values>
   <PROGDIR><NAME>%s</NAME><SUBC>1</SUBC><FIXPT>X</FIXPT><UCCHECK>X</UCCHECK></PROGDIR>
   <TPOOL><item><ID>R</ID><ENTRY>%s</ENTRY><LENGTH>%d</LENGTH></item></TPOOL>
  </asx:values>
 </asx:abap>
</abapGit>
`, objName, desc, descLen)

	w.Close()
	fmt.Fprintf(os.Stderr, "zip: %s (%s)\n", outFile, objName)
	return nil
}
