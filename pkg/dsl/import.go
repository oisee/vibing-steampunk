package dsl

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// ImportFile represents a file to import with its detected type.
type ImportFile struct {
	Path        string               `json:"path"`
	ObjectType  adt.CreatableObjectType `json:"objectType"`
	ObjectName  string               `json:"objectName"`
	IncludeType adt.ClassIncludeType `json:"includeType,omitempty"` // For class includes
	Priority    int                  `json:"priority"`              // Lower = import first
}

// ImportResult represents the result of importing a single file.
type ImportResult struct {
	File       ImportFile `json:"file"`
	Success    bool       `json:"success"`
	Created    bool       `json:"created"`
	Message    string     `json:"message"`
	ObjectURL  string     `json:"objectUrl,omitempty"`
}

// BatchImportResult represents the result of a batch import.
type BatchImportResult struct {
	TotalFiles    int            `json:"totalFiles"`
	SuccessCount  int            `json:"successCount"`
	FailureCount  int            `json:"failureCount"`
	SkippedCount  int            `json:"skippedCount"`
	Results       []ImportResult `json:"results"`
}

// ExportResult represents the result of exporting a single object.
type ExportResult struct {
	ObjectType  string `json:"objectType"`
	ObjectName  string `json:"objectName"`
	IncludeType string `json:"includeType,omitempty"`
	FilePath    string `json:"filePath"`
	Success     bool   `json:"success"`
	LineCount   int    `json:"lineCount"`
	Message     string `json:"message"`
}

// BatchExportResult represents the result of a batch export.
type BatchExportResult struct {
	TotalObjects int            `json:"totalObjects"`
	SuccessCount int            `json:"successCount"`
	FailureCount int            `json:"failureCount"`
	Results      []ExportResult `json:"results"`
}

// ImportBuilder provides a fluent interface for batch imports.
type ImportBuilder struct {
	client      *adt.Client
	files       []ImportFile
	packageName string
	transport   string
	dryRun      bool
	stopOnError bool
	verbose     bool

	// Callbacks
	onStart    func(file ImportFile)
	onComplete func(result ImportResult)
	onError    func(file ImportFile, err error)
}

// Import creates a new import builder.
func Import(client *adt.Client) *ImportBuilder {
	return &ImportBuilder{
		client: client,
		files:  []ImportFile{},
	}
}

// FromDirectory scans a directory for ABAP source files.
func (b *ImportBuilder) FromDirectory(dir string) (*ImportBuilder, error) {
	files, err := ScanDirectory(dir)
	if err != nil {
		return nil, err
	}
	b.files = append(b.files, files...)
	return b, nil
}

// FromFiles adds specific files to import.
func (b *ImportBuilder) FromFiles(paths ...string) (*ImportBuilder, error) {
	for _, path := range paths {
		file, err := ParseImportFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		b.files = append(b.files, *file)
	}
	return b, nil
}

// FromFilesOrdered adds files with explicit import order (first = imported first).
// Use this when automatic priority detection doesn't work (e.g., DDLS before classes).
func (b *ImportBuilder) FromFilesOrdered(paths ...string) (*ImportBuilder, error) {
	for i, path := range paths {
		file, err := ParseImportFile(path)
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", path, err)
		}
		file.Priority = i // Explicit order
		b.files = append(b.files, *file)
	}
	return b, nil
}

// WithPriority sets manual priority for a specific file (lower = first).
func (b *ImportBuilder) WithPriority(path string, priority int) *ImportBuilder {
	for i := range b.files {
		if b.files[i].Path == path {
			b.files[i].Priority = priority
			break
		}
	}
	return b
}

// DDLSFirst sets DDLS priority to import before classes/interfaces.
// Useful when CDS views define types used by classes.
func (b *ImportBuilder) DDLSFirst() *ImportBuilder {
	for i := range b.files {
		if b.files[i].ObjectType == adt.ObjectTypeDDLS {
			b.files[i].Priority = 5 // Before interfaces (10)
		}
	}
	return b
}

// RAPOrder sets RAP-specific import order: DDLS → BDEF → SRVD → Classes.
// Standard RAP development order where CDS views come first.
func (b *ImportBuilder) RAPOrder() *ImportBuilder {
	for i := range b.files {
		switch b.files[i].ObjectType {
		case adt.ObjectTypeDDLS:
			b.files[i].Priority = 10 // CDS views first
		case adt.ObjectTypeBDEF:
			b.files[i].Priority = 20 // Behavior after CDS
		case adt.ObjectTypeInterface:
			b.files[i].Priority = 30
		case adt.ObjectTypeClass:
			if b.files[i].IncludeType == "" || b.files[i].IncludeType == adt.ClassIncludeMain {
				b.files[i].Priority = 40 // Main class
			} else {
				b.files[i].Priority = 45 // Includes after main
			}
		case adt.ObjectTypeSRVD:
			b.files[i].Priority = 50 // Service definition last
		}
	}
	return b
}

// CustomOrder applies a custom priority map by object type.
// Example: CustomOrder(map[string]int{"DDLS": 1, "CLAS": 2, "INTF": 3})
func (b *ImportBuilder) CustomOrder(priorities map[string]int) *ImportBuilder {
	for i := range b.files {
		typeStr := string(b.files[i].ObjectType)
		if priority, ok := priorities[typeStr]; ok {
			b.files[i].Priority = priority
		}
	}
	return b
}

// ToPackage sets the target package for new objects.
func (b *ImportBuilder) ToPackage(pkg string) *ImportBuilder {
	b.packageName = pkg
	return b
}

// WithTransport sets the transport request.
func (b *ImportBuilder) WithTransport(transport string) *ImportBuilder {
	b.transport = transport
	return b
}

// DryRun enables dry-run mode.
func (b *ImportBuilder) DryRun() *ImportBuilder {
	b.dryRun = true
	return b
}

// StopOnError stops on first error.
func (b *ImportBuilder) StopOnError() *ImportBuilder {
	b.stopOnError = true
	return b
}

// Verbose enables verbose output.
func (b *ImportBuilder) Verbose() *ImportBuilder {
	b.verbose = true
	return b
}

// OnStart sets a callback for when import starts.
func (b *ImportBuilder) OnStart(fn func(file ImportFile)) *ImportBuilder {
	b.onStart = fn
	return b
}

// OnComplete sets a callback for when import completes.
func (b *ImportBuilder) OnComplete(fn func(result ImportResult)) *ImportBuilder {
	b.onComplete = fn
	return b
}

// OnError sets a callback for errors.
func (b *ImportBuilder) OnError(fn func(file ImportFile, err error)) *ImportBuilder {
	b.onError = fn
	return b
}

// Execute runs the batch import.
func (b *ImportBuilder) Execute(ctx context.Context) (*BatchImportResult, error) {
	result := &BatchImportResult{
		TotalFiles: len(b.files),
		Results:    make([]ImportResult, 0, len(b.files)),
	}

	// Sort files by priority (classes before includes, etc.)
	sortedFiles := make([]ImportFile, len(b.files))
	copy(sortedFiles, b.files)
	sort.Slice(sortedFiles, func(i, j int) bool {
		return sortedFiles[i].Priority < sortedFiles[j].Priority
	})

	for _, file := range sortedFiles {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if b.onStart != nil {
			b.onStart(file)
		}

		importResult := b.importFile(ctx, file)
		result.Results = append(result.Results, importResult)

		if importResult.Success {
			result.SuccessCount++
		} else {
			result.FailureCount++
			if b.onError != nil {
				b.onError(file, fmt.Errorf("%s", importResult.Message))
			}
			if b.stopOnError {
				return result, fmt.Errorf("import failed for %s: %s", file.Path, importResult.Message)
			}
		}

		if b.onComplete != nil {
			b.onComplete(importResult)
		}
	}

	return result, nil
}

// importFile imports a single file.
func (b *ImportBuilder) importFile(ctx context.Context, file ImportFile) ImportResult {
	result := ImportResult{
		File: file,
	}

	if b.dryRun {
		result.Success = true
		result.Message = "dry run - would import"
		return result
	}

	deployResult, err := b.client.DeployFromFile(ctx, file.Path, b.packageName, b.transport)
	if err != nil {
		result.Message = fmt.Sprintf("deploy error: %v", err)
		return result
	}

	result.Success = deployResult.Success
	result.Created = deployResult.Created
	result.Message = deployResult.Message
	result.ObjectURL = deployResult.ObjectURL

	return result
}

// Files returns the list of files to import.
func (b *ImportBuilder) Files() []ImportFile {
	return b.files
}

// --- Export Builder ---

// ExportBuilder provides a fluent interface for batch exports.
type ExportBuilder struct {
	client    *adt.Client
	objects   []ExportObject
	outputDir string
	verbose   bool

	// Callbacks
	onStart    func(obj ExportObject)
	onComplete func(result ExportResult)
	onError    func(obj ExportObject, err error)
}

// ExportObject represents an object to export.
type ExportObject struct {
	Type        adt.CreatableObjectType
	Name        string
	IncludeType adt.ClassIncludeType // For class includes
}

// Export creates a new export builder.
func Export(client *adt.Client) *ExportBuilder {
	return &ExportBuilder{
		client:  client,
		objects: []ExportObject{},
	}
}

// Classes adds classes to export (with all includes).
func (b *ExportBuilder) Classes(names ...string) *ExportBuilder {
	for _, name := range names {
		// Add main class
		b.objects = append(b.objects, ExportObject{
			Type: adt.ObjectTypeClass,
			Name: name,
			IncludeType: adt.ClassIncludeMain,
		})
		// Add all includes
		for _, inc := range []adt.ClassIncludeType{
			adt.ClassIncludeTestClasses,
			adt.ClassIncludeDefinitions,
			adt.ClassIncludeImplementations,
			adt.ClassIncludeMacros,
		} {
			b.objects = append(b.objects, ExportObject{
				Type:        adt.ObjectTypeClass,
				Name:        name,
				IncludeType: inc,
			})
		}
	}
	return b
}

// ClassMain adds only main class source (no includes).
func (b *ExportBuilder) ClassMain(names ...string) *ExportBuilder {
	for _, name := range names {
		b.objects = append(b.objects, ExportObject{
			Type:        adt.ObjectTypeClass,
			Name:        name,
			IncludeType: adt.ClassIncludeMain,
		})
	}
	return b
}

// Programs adds programs to export.
func (b *ExportBuilder) Programs(names ...string) *ExportBuilder {
	for _, name := range names {
		b.objects = append(b.objects, ExportObject{
			Type: adt.ObjectTypeProgram,
			Name: name,
		})
	}
	return b
}

// Interfaces adds interfaces to export.
func (b *ExportBuilder) Interfaces(names ...string) *ExportBuilder {
	for _, name := range names {
		b.objects = append(b.objects, ExportObject{
			Type: adt.ObjectTypeInterface,
			Name: name,
		})
	}
	return b
}

// DDLSources adds CDS views to export.
func (b *ExportBuilder) DDLSources(names ...string) *ExportBuilder {
	for _, name := range names {
		b.objects = append(b.objects, ExportObject{
			Type: adt.ObjectTypeDDLS,
			Name: name,
		})
	}
	return b
}

// ToDirectory sets the output directory.
func (b *ExportBuilder) ToDirectory(dir string) *ExportBuilder {
	b.outputDir = dir
	return b
}

// Verbose enables verbose output.
func (b *ExportBuilder) Verbose() *ExportBuilder {
	b.verbose = true
	return b
}

// OnStart sets a callback for when export starts.
func (b *ExportBuilder) OnStart(fn func(obj ExportObject)) *ExportBuilder {
	b.onStart = fn
	return b
}

// OnComplete sets a callback for when export completes.
func (b *ExportBuilder) OnComplete(fn func(result ExportResult)) *ExportBuilder {
	b.onComplete = fn
	return b
}

// OnError sets a callback for errors.
func (b *ExportBuilder) OnError(fn func(obj ExportObject, err error)) *ExportBuilder {
	b.onError = fn
	return b
}

// Execute runs the batch export.
func (b *ExportBuilder) Execute(ctx context.Context) (*BatchExportResult, error) {
	result := &BatchExportResult{
		TotalObjects: len(b.objects),
		Results:      make([]ExportResult, 0, len(b.objects)),
	}

	// Create output directory if needed
	if b.outputDir != "" {
		if err := os.MkdirAll(b.outputDir, 0755); err != nil {
			return nil, fmt.Errorf("creating output directory: %w", err)
		}
	}

	for _, obj := range b.objects {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		if b.onStart != nil {
			b.onStart(obj)
		}

		exportResult := b.exportObject(ctx, obj)

		// Skip non-existent includes (not an error)
		if !exportResult.Success && strings.Contains(exportResult.Message, "404") {
			continue
		}

		result.Results = append(result.Results, exportResult)

		if exportResult.Success {
			result.SuccessCount++
		} else {
			result.FailureCount++
			if b.onError != nil {
				b.onError(obj, fmt.Errorf("%s", exportResult.Message))
			}
		}

		if b.onComplete != nil {
			b.onComplete(exportResult)
		}
	}

	return result, nil
}

// exportObject exports a single object.
func (b *ExportBuilder) exportObject(ctx context.Context, obj ExportObject) ExportResult {
	result := ExportResult{
		ObjectType:  string(obj.Type),
		ObjectName:  obj.Name,
		IncludeType: string(obj.IncludeType),
	}

	var saveResult *adt.SaveToFileResult
	var err error

	// Handle class includes specially
	if obj.Type == adt.ObjectTypeClass && obj.IncludeType != "" && obj.IncludeType != adt.ClassIncludeMain {
		saveResult, err = b.client.SaveClassIncludeToFile(ctx, obj.Name, obj.IncludeType, b.outputDir)
	} else {
		saveResult, err = b.client.SaveToFile(ctx, obj.Type, obj.Name, "", b.outputDir)
	}

	if err != nil {
		result.Message = fmt.Sprintf("export error: %v", err)
		return result
	}

	result.Success = saveResult.Success
	result.FilePath = saveResult.FilePath
	result.LineCount = saveResult.LineCount
	result.Message = saveResult.Message

	return result
}

// --- Helper Functions ---

// ScanDirectory scans a directory for ABAP source files.
func ScanDirectory(dir string) ([]ImportFile, error) {
	var files []ImportFile

	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}

		// Check for supported extensions
		if !isABAPSourceFile(path) {
			return nil
		}

		file, err := ParseImportFile(path)
		if err != nil {
			// Skip files we can't parse
			return nil
		}

		files = append(files, *file)
		return nil
	})

	if err != nil {
		return nil, err
	}

	return files, nil
}

// ParseImportFile parses a file path and returns import metadata.
func ParseImportFile(path string) (*ImportFile, error) {
	info, err := adt.ParseABAPFile(path)
	if err != nil {
		return nil, err
	}

	file := &ImportFile{
		Path:        path,
		ObjectType:  info.ObjectType,
		ObjectName:  info.ObjectName,
		IncludeType: info.ClassIncludeType,
		Priority:    getPriority(info.ObjectType, info.ClassIncludeType),
	}

	return file, nil
}

// isABAPSourceFile checks if a file is an ABAP source file.
func isABAPSourceFile(path string) bool {
	lower := strings.ToLower(path)
	extensions := []string{
		".abap",
		".asddls",
		".asbdef",
		".srvdsrv",
	}
	for _, ext := range extensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// getPriority returns import priority (lower = first).
// Order: Interfaces → Classes (main) → Programs → Class includes → DDLS → BDEF → SRVD
func getPriority(objType adt.CreatableObjectType, includeType adt.ClassIncludeType) int {
	switch objType {
	case adt.ObjectTypeInterface:
		return 10
	case adt.ObjectTypeClass:
		if includeType == "" || includeType == adt.ClassIncludeMain {
			return 20 // Main class first
		}
		return 25 // Includes after main class
	case adt.ObjectTypeProgram:
		return 30
	case adt.ObjectTypeFunctionGroup:
		return 40
	case adt.ObjectTypeDDLS:
		return 50
	case adt.ObjectTypeBDEF:
		return 60
	case adt.ObjectTypeSRVD:
		return 70
	default:
		return 100
	}
}

// --- Convenience Functions ---

// ImportDirectory imports all ABAP files from a directory.
func ImportDirectory(ctx context.Context, client *adt.Client, dir, packageName, transport string) (*BatchImportResult, error) {
	builder, err := Import(client).FromDirectory(dir)
	if err != nil {
		return nil, err
	}
	return builder.
		ToPackage(packageName).
		WithTransport(transport).
		Execute(ctx)
}

// ExportClass exports a class with all its includes.
func ExportClass(ctx context.Context, client *adt.Client, className, outputDir string) (*BatchExportResult, error) {
	return Export(client).
		Classes(className).
		ToDirectory(outputDir).
		Execute(ctx)
}

// ExportClasses exports multiple classes with all their includes.
func ExportClasses(ctx context.Context, client *adt.Client, classNames []string, outputDir string) (*BatchExportResult, error) {
	return Export(client).
		Classes(classNames...).
		ToDirectory(outputDir).
		Execute(ctx)
}
