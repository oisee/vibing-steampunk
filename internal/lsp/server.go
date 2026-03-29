package lsp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/vinchacho/vibing-steampunk/pkg/adt"
	"github.com/vinchacho/vibing-steampunk/pkg/ctxcomp"
)

// Server implements a minimal LSP server for ABAP.
type Server struct {
	transport  *Transport
	client     *adt.Client // nil if no SAP connection
	compressor *ctxcomp.Compressor
	verbose    bool

	// Document store: URI → content
	docs   map[string]string
	docsMu sync.RWMutex

	// Debounce timers per URI
	timers   map[string]*time.Timer
	timersMu sync.Mutex

	// Shutdown state
	shutdown bool
}

// lspSourceAdapter adapts adt.Client to ctxcomp.ADTSourceFetcher.
type lspSourceAdapter struct {
	client *adt.Client
}

func (a *lspSourceAdapter) GetSource(ctx context.Context, objectType, name string, opts interface{}) (string, error) {
	return a.client.GetSource(ctx, objectType, name, nil)
}

// NewServer creates a new LSP server.
// client may be nil (no SAP connection → no online diagnostics).
func NewServer(client *adt.Client, verbose bool) *Server {
	var comp *ctxcomp.Compressor
	if client != nil {
		provider := ctxcomp.NewMultiSourceProvider("", &lspSourceAdapter{client: client})
		comp = ctxcomp.NewCompressor(provider, 20)
	}
	return &Server{
		client:     client,
		compressor: comp,
		verbose:    verbose,
		docs:       make(map[string]string),
		timers:     make(map[string]*time.Timer),
	}
}

// Serve runs the LSP server on the given reader/writer (typically stdin/stdout).
func (s *Server) Serve(r io.Reader, w io.Writer) error {
	s.transport = NewTransport(r, w)

	for {
		msg, err := s.transport.Read()
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("reading message: %w", err)
		}

		s.handleMessage(msg)

		if s.shutdown && msg.Method == "exit" {
			return nil
		}
	}
}

func (s *Server) handleMessage(msg *Message) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		// no-op
	case "shutdown":
		s.shutdown = true
		s.sendResult(msg, nil)
	case "exit":
		if !s.shutdown {
			os.Exit(1)
		}
		// normal exit handled in Serve loop
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	default:
		// Unknown method — send MethodNotFound for requests (those with an ID)
		if msg.ID != nil {
			s.sendError(msg, -32601, "method not found: "+msg.Method)
		}
	}
}

func (s *Server) handleInitialize(msg *Message) {
	result := InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync: &TextDocumentSyncOptions{
				OpenClose: true,
				Change:    SyncFull,
			},
			DefinitionProvider: s.client != nil,
		},
		ServerInfo: &ServerInfo{
			Name:    "vsp-lsp",
			Version: "0.1.0",
		},
	}
	s.sendResult(msg, result)
}

func (s *Server) handleDidOpen(msg *Message) {
	var params DidOpenTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logf("didOpen unmarshal error: %v", err)
		return
	}

	uri := params.TextDocument.URI
	s.docsMu.Lock()
	s.docs[uri] = params.TextDocument.Text
	s.docsMu.Unlock()

	s.scheduleDiagnostics(uri)
	go s.publishContext(uri)
}

func (s *Server) handleDidChange(msg *Message) {
	var params DidChangeTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.logf("didChange unmarshal error: %v", err)
		return
	}

	uri := params.TextDocument.URI
	if len(params.ContentChanges) > 0 {
		// SyncFull: last change event has the full content
		s.docsMu.Lock()
		s.docs[uri] = params.ContentChanges[len(params.ContentChanges)-1].Text
		s.docsMu.Unlock()
	}

	s.scheduleDiagnostics(uri)
}

func (s *Server) handleDidClose(msg *Message) {
	var params DidCloseTextDocumentParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		return
	}

	uri := params.TextDocument.URI

	// Cancel any pending diagnostics timer
	s.timersMu.Lock()
	if t, ok := s.timers[uri]; ok {
		t.Stop()
		delete(s.timers, uri)
	}
	s.timersMu.Unlock()

	// Remove document and clear diagnostics
	s.docsMu.Lock()
	delete(s.docs, uri)
	s.docsMu.Unlock()

	s.publishDiagnostics(uri, nil)
}

func (s *Server) handleDefinition(msg *Message) {
	if s.client == nil {
		s.sendResult(msg, nil)
		return
	}

	var params TextDocumentPositionParams
	if err := json.Unmarshal(msg.Params, &params); err != nil {
		s.sendError(msg, -32602, "invalid params")
		return
	}

	uri := params.TextDocument.URI
	s.docsMu.RLock()
	content, ok := s.docs[uri]
	s.docsMu.RUnlock()
	if !ok {
		s.sendResult(msg, nil)
		return
	}

	// Resolve the ADT source URL from the file URI
	objectURL, _ := uriToObjectURL(uri)
	if objectURL == "" {
		s.sendResult(msg, nil)
		return
	}
	sourceURL := objectURL + "/source/main"

	// LSP positions are 0-based; ADT uses 1-based
	line := params.Position.Line + 1
	col := params.Position.Character + 1

	// Find the word boundaries at the cursor position
	startCol, endCol := findWordBounds(content, params.Position.Line, params.Position.Character)

	ctx := context.Background()
	loc, err := s.client.FindDefinition(ctx, sourceURL, content, line, startCol+1, endCol+1, false, "")
	if err != nil {
		s.logf("FindDefinition error: %v", err)
		_ = col // col used above for word bounds
		s.sendResult(msg, nil)
		return
	}

	if loc == nil {
		s.sendResult(msg, nil)
		return
	}

	// Convert ADT location to LSP Location
	// ADT line/col are 1-based, LSP is 0-based
	result := Location{
		URI: adtURLToFileURI(loc.URL),
		Range: Range{
			Start: Position{Line: loc.Line - 1, Character: loc.Column - 1},
			End:   Position{Line: loc.Line - 1, Character: loc.Column - 1},
		},
	}
	s.sendResult(msg, result)
}

// scheduleDiagnostics debounces diagnostic runs (300ms after last change).
func (s *Server) scheduleDiagnostics(uri string) {
	s.timersMu.Lock()
	defer s.timersMu.Unlock()

	if t, ok := s.timers[uri]; ok {
		t.Stop()
	}
	s.timers[uri] = time.AfterFunc(300*time.Millisecond, func() {
		s.runDiagnostics(uri)
	})
}

// runDiagnostics performs a syntax check and publishes diagnostics.
func (s *Server) runDiagnostics(uri string) {
	if s.client == nil {
		return
	}

	s.docsMu.RLock()
	content, ok := s.docs[uri]
	s.docsMu.RUnlock()
	if !ok {
		return
	}

	objectURL, includeURL := uriToObjectURL(uri)
	if objectURL == "" {
		return
	}

	// For class includes, use the include URL directly
	checkURL := objectURL
	if includeURL != "" {
		checkURL = includeURL
	}

	ctx := context.Background()
	results, err := s.client.SyntaxCheck(ctx, checkURL, content)
	if err != nil {
		s.logf("SyntaxCheck error for %s: %v", uri, err)
		return
	}

	diags := make([]Diagnostic, 0, len(results))
	for _, r := range results {
		sev := SeverityError
		switch strings.ToUpper(r.Severity) {
		case "W":
			sev = SeverityWarning
		case "I":
			sev = SeverityInformation
		}

		// ADT line is 1-based, offset is 0-based character offset
		line := r.Line - 1
		if line < 0 {
			line = 0
		}
		col := r.Offset

		diags = append(diags, Diagnostic{
			Range: Range{
				Start: Position{Line: line, Character: col},
				End:   Position{Line: line, Character: col},
			},
			Severity: sev,
			Source:   "vsp",
			Message:  r.Text,
		})
	}

	s.publishDiagnostics(uri, diags)
}

func (s *Server) publishDiagnostics(uri string, diags []Diagnostic) {
	if diags == nil {
		diags = []Diagnostic{}
	}
	params := PublishDiagnosticsParams{
		URI:         uri,
		Diagnostics: diags,
	}
	data, _ := json.Marshal(params)
	s.transport.Write(&Message{
		Method: "textDocument/publishDiagnostics",
		Params: data,
	})
}

// publishContext runs context compression and sends a vsp/context notification.
// Best-effort: failures are logged but don't affect other LSP operations.
func (s *Server) publishContext(uri string) {
	if s.compressor == nil {
		return
	}

	s.docsMu.RLock()
	content, ok := s.docs[uri]
	s.docsMu.RUnlock()
	if !ok || content == "" {
		return
	}

	// Determine object type and name from URI
	objectURL, _ := uriToObjectURL(uri)
	if objectURL == "" {
		return
	}

	// Infer object type/name from the URL pattern
	objectType, objectName := urlToTypeAndName(objectURL)
	if objectType == "" {
		return
	}

	ctx := context.Background()
	result, err := s.compressor.Compress(ctx, content, objectName, objectType)
	if err != nil {
		s.logf("context compression error for %s: %v", uri, err)
		return
	}

	if result.Prologue == "" {
		return
	}

	// Send custom notification
	type contextParams struct {
		URI      string `json:"uri"`
		Prologue string `json:"prologue"`
		Stats    struct {
			DepsFound    int `json:"depsFound"`
			DepsResolved int `json:"depsResolved"`
			DepsFailed   int `json:"depsFailed"`
			TotalLines   int `json:"totalLines"`
		} `json:"stats"`
	}

	params := contextParams{URI: uri, Prologue: result.Prologue}
	params.Stats.DepsFound = result.Stats.DepsFound
	params.Stats.DepsResolved = result.Stats.DepsResolved
	params.Stats.DepsFailed = result.Stats.DepsFailed
	params.Stats.TotalLines = result.Stats.TotalLines

	data, _ := json.Marshal(params)
	s.transport.Write(&Message{
		Method: "vsp/context",
		Params: data,
	})

	s.logf("published context for %s: %d deps resolved, %d lines", uri, result.Stats.DepsResolved, result.Stats.TotalLines)
}

// urlToTypeAndName extracts ABAP object type and name from an ADT URL path.
func urlToTypeAndName(objectURL string) (string, string) {
	parts := strings.Split(objectURL, "/")
	if len(parts) < 2 {
		return "", ""
	}

	// /sap/bc/adt/oo/classes/ZCL_FOO → CLAS, ZCL_FOO
	// /sap/bc/adt/oo/interfaces/ZIF_FOO → INTF, ZIF_FOO
	// /sap/bc/adt/programs/programs/ZPROG → PROG, ZPROG
	// /sap/bc/adt/functions/groups/ZFUGR → FUGR, ZFUGR
	name := parts[len(parts)-1]

	for i := len(parts) - 2; i >= 0; i-- {
		switch parts[i] {
		case "classes":
			return "CLAS", strings.ToUpper(name)
		case "interfaces":
			return "INTF", strings.ToUpper(name)
		case "programs":
			return "PROG", strings.ToUpper(name)
		case "groups":
			return "FUGR", strings.ToUpper(name)
		}
	}

	return "", ""
}

// --- Helpers ---

func (s *Server) sendResult(msg *Message, result interface{}) {
	data, _ := json.Marshal(result)
	raw := json.RawMessage(data)
	s.transport.Write(&Message{
		ID:     msg.ID,
		Result: raw,
	})
}

func (s *Server) sendError(msg *Message, code int, message string) {
	s.transport.Write(&Message{
		ID:    msg.ID,
		Error: &ResponseError{Code: code, Message: message},
	})
}

func (s *Server) logf(format string, args ...interface{}) {
	if s.verbose {
		fmt.Fprintf(os.Stderr, "[LSP] "+format+"\n", args...)
	}
}

// uriToObjectURL converts a file:// URI to an ADT object URL for SyntaxCheck.
// Returns (objectURL, includeURL). includeURL is non-empty for class includes.
func uriToObjectURL(fileURI string) (string, string) {
	// Parse file:// URI to get the path
	parsed, err := url.Parse(fileURI)
	if err != nil {
		return "", ""
	}
	filePath := parsed.Path

	baseName := filepath.Base(filePath)
	lowerBase := strings.ToLower(baseName)

	// Detect object type and name from abapGit-style filename conventions
	switch {
	case strings.HasSuffix(lowerBase, ".clas.testclasses.abap"):
		name := extractName(baseName, ".clas.testclasses.abap")
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(name))
		includeURL := objectURL + "/includes/testclasses"
		return objectURL, includeURL

	case strings.HasSuffix(lowerBase, ".clas.locals_def.abap"):
		name := extractName(baseName, ".clas.locals_def.abap")
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(name))
		includeURL := objectURL + "/includes/definitions"
		return objectURL, includeURL

	case strings.HasSuffix(lowerBase, ".clas.locals_imp.abap"):
		name := extractName(baseName, ".clas.locals_imp.abap")
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(name))
		includeURL := objectURL + "/includes/implementations"
		return objectURL, includeURL

	case strings.HasSuffix(lowerBase, ".clas.macros.abap"):
		name := extractName(baseName, ".clas.macros.abap")
		objectURL := fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(name))
		includeURL := objectURL + "/includes/macros"
		return objectURL, includeURL

	case strings.HasSuffix(lowerBase, ".clas.abap"):
		name := extractName(baseName, ".clas.abap")
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", url.PathEscape(name)), ""

	case strings.HasSuffix(lowerBase, ".prog.abap"):
		name := extractName(baseName, ".prog.abap")
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", url.PathEscape(name)), ""

	case strings.HasSuffix(lowerBase, ".intf.abap"):
		name := extractName(baseName, ".intf.abap")
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", url.PathEscape(name)), ""

	case strings.HasSuffix(lowerBase, ".fugr.abap"):
		name := extractName(baseName, ".fugr.abap")
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s", url.PathEscape(name)), ""

	case strings.HasSuffix(lowerBase, ".ddls.asddls"):
		name := strings.ToLower(extractName(baseName, ".ddls.asddls"))
		return fmt.Sprintf("/sap/bc/adt/ddic/ddl/sources/%s", url.PathEscape(name)), ""
	}

	return "", ""
}

// extractName removes the suffix from a filename and converts to uppercase,
// handling the abapGit # → / namespace convention.
func extractName(baseName, suffix string) string {
	name := baseName[:len(baseName)-len(suffix)]
	name = strings.ToUpper(name)
	name = strings.ReplaceAll(name, "#", "/")
	return name
}

// adtURLToFileURI is a best-effort conversion of an ADT URL back to a file URI.
// For now, returns the ADT URL as-is since we don't have workspace mapping.
func adtURLToFileURI(adtURL string) string {
	// TODO: map ADT URLs back to workspace file URIs
	return adtURL
}

// findWordBounds finds the start and end column (0-based) of the word at the given position.
func findWordBounds(content string, line, col int) (int, int) {
	lines := strings.Split(content, "\n")
	if line < 0 || line >= len(lines) {
		return col, col
	}
	ln := lines[line]
	if col < 0 || col >= len(ln) {
		return col, col
	}

	start := col
	for start > 0 && isWordChar(ln[start-1]) {
		start--
	}
	end := col
	for end < len(ln) && isWordChar(ln[end]) {
		end++
	}
	return start, end
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_' || b == '/'
}
