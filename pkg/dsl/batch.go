package dsl

import (
	"context"
	"fmt"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// BatchBuilder provides a fluent interface for batch operations.
type BatchBuilder struct {
	client    *adt.Client
	objects   []ObjectRef
	transform TransformFunc
	transport string
	dryRun    bool
	activate  bool

	// Callbacks
	onStart    func(obj ObjectRef)
	onComplete func(obj ObjectRef, result ObjectResult)
	onError    func(obj ObjectRef, err error)
}

// TransformFunc transforms source code.
type TransformFunc func(source string, obj ObjectRef) (string, error)

// Batch creates a new batch builder.
func Batch(client *adt.Client) *BatchBuilder {
	return &BatchBuilder{
		client:  client,
		objects: []ObjectRef{},
	}
}

// Objects sets the objects to process.
func (b *BatchBuilder) Objects(objects ...ObjectRef) *BatchBuilder {
	b.objects = append(b.objects, objects...)
	return b
}

// FromSearch uses search results as targets.
func (b *BatchBuilder) FromSearch(ctx context.Context, search *SearchBuilder) (*BatchBuilder, error) {
	objects, err := search.Execute(ctx)
	if err != nil {
		return nil, err
	}
	b.objects = objects
	return b, nil
}

// Transform sets the transformation function.
func (b *BatchBuilder) Transform(fn TransformFunc) *BatchBuilder {
	b.transform = fn
	return b
}

// PrependHeader prepends a header to all sources.
func (b *BatchBuilder) PrependHeader(header string) *BatchBuilder {
	return b.Transform(func(source string, obj ObjectRef) (string, error) {
		if strings.Contains(source, header) {
			return source, nil // Already has header
		}
		return header + "\n" + source, nil
	})
}

// ReplaceAll replaces all occurrences of a pattern.
func (b *BatchBuilder) ReplaceAll(old, new string) *BatchBuilder {
	return b.Transform(func(source string, obj ObjectRef) (string, error) {
		return strings.ReplaceAll(source, old, new), nil
	})
}

// Transport sets the transport request.
func (b *BatchBuilder) Transport(transport string) *BatchBuilder {
	b.transport = transport
	return b
}

// WithActivation enables automatic activation after update.
func (b *BatchBuilder) WithActivation() *BatchBuilder {
	b.activate = true
	return b
}

// DryRun enables dry-run mode (no actual changes).
func (b *BatchBuilder) DryRun() *BatchBuilder {
	b.dryRun = true
	return b
}

// OnStart sets a callback for when processing starts.
func (b *BatchBuilder) OnStart(fn func(obj ObjectRef)) *BatchBuilder {
	b.onStart = fn
	return b
}

// OnComplete sets a callback for when processing completes.
func (b *BatchBuilder) OnComplete(fn func(obj ObjectRef, result ObjectResult)) *BatchBuilder {
	b.onComplete = fn
	return b
}

// OnError sets a callback for errors.
func (b *BatchBuilder) OnError(fn func(obj ObjectRef, err error)) *BatchBuilder {
	b.onError = fn
	return b
}

// Execute runs the batch operation.
func (b *BatchBuilder) Execute(ctx context.Context) (*BatchResult, error) {
	if b.transform == nil {
		return nil, fmt.Errorf("no transformation specified")
	}

	result := &BatchResult{
		TotalObjects: len(b.objects),
		Results:      make([]ObjectResult, 0, len(b.objects)),
	}

	for _, obj := range b.objects {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		objResult := b.processObject(ctx, obj)
		result.Results = append(result.Results, objResult)
		result.ProcessedObjects++

		switch objResult.Action {
		case "updated":
			result.SuccessCount++
		case "skipped":
			result.SkippedCount++
		case "failed":
			result.FailureCount++
		}
	}

	return result, nil
}

// processObject processes a single object.
func (b *BatchBuilder) processObject(ctx context.Context, obj ObjectRef) ObjectResult {
	result := ObjectResult{
		Object: obj,
	}

	if b.onStart != nil {
		b.onStart(obj)
	}

	// Get current source
	source, err := b.getSource(ctx, obj)
	if err != nil {
		result.Action = "failed"
		result.Message = fmt.Sprintf("failed to get source: %v", err)
		if b.onError != nil {
			b.onError(obj, err)
		}
		return result
	}

	// Apply transformation
	newSource, err := b.transform(source, obj)
	if err != nil {
		result.Action = "failed"
		result.Message = fmt.Sprintf("transformation failed: %v", err)
		if b.onError != nil {
			b.onError(obj, err)
		}
		return result
	}

	// Check if changed
	if newSource == source {
		result.Success = true
		result.Action = "skipped"
		result.Message = "no changes needed"
		if b.onComplete != nil {
			b.onComplete(obj, result)
		}
		return result
	}

	// Dry run - don't actually save
	if b.dryRun {
		result.Success = true
		result.Action = "skipped"
		result.Message = "dry run - would update"
		if b.onComplete != nil {
			b.onComplete(obj, result)
		}
		return result
	}

	// Save changes using workflow
	err = b.saveSource(ctx, obj, newSource)
	if err != nil {
		result.Action = "failed"
		result.Message = fmt.Sprintf("failed to save: %v", err)
		if b.onError != nil {
			b.onError(obj, err)
		}
		return result
	}

	result.Success = true
	result.Action = "updated"
	result.Message = "updated successfully"

	if b.onComplete != nil {
		b.onComplete(obj, result)
	}

	return result
}

// getSource retrieves the source code for an object.
func (b *BatchBuilder) getSource(ctx context.Context, obj ObjectRef) (string, error) {
	switch obj.Type {
	case TypeClass, "CLAS/OC":
		return b.client.GetSource(ctx, "CLAS", obj.Name, nil)
	case TypeProgram, "PROG/P":
		return b.client.GetSource(ctx, "PROG", obj.Name, nil)
	default:
		return "", fmt.Errorf("unsupported object type: %s", obj.Type)
	}
}

// saveSource saves the source code for an object.
func (b *BatchBuilder) saveSource(ctx context.Context, obj ObjectRef, source string) error {
	switch obj.Type {
	case TypeClass, "CLAS/OC":
		result, err := b.client.WriteClass(ctx, obj.Name, source, b.transport)
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf(result.Message)
		}
		return nil
	case TypeProgram, "PROG/P":
		result, err := b.client.WriteProgram(ctx, obj.Name, source, b.transport)
		if err != nil {
			return err
		}
		if !result.Success {
			return fmt.Errorf(result.Message)
		}
		return nil
	default:
		return fmt.Errorf("unsupported object type: %s", obj.Type)
	}
}

// --- Pipeline Builder ---

// PipelineBuilder provides a fluent interface for building pipelines.
type PipelineBuilder struct {
	client *adt.Client
	name   string
	stages []*StageBuilder
}

// StageBuilder builds a pipeline stage.
type StageBuilder struct {
	pipeline  *PipelineBuilder
	name      string
	dependsOn []string
	steps     []Step
}

// NewPipeline creates a new pipeline builder.
func NewPipeline(client *adt.Client, name string) *PipelineBuilder {
	return &PipelineBuilder{
		client: client,
		name:   name,
		stages: []*StageBuilder{},
	}
}

// Stage adds a new stage to the pipeline.
func (p *PipelineBuilder) Stage(name string) *StageBuilder {
	stage := &StageBuilder{
		pipeline: p,
		name:     name,
		steps:    []Step{},
	}
	p.stages = append(p.stages, stage)
	return stage
}

// Build creates the pipeline definition.
func (p *PipelineBuilder) Build() *Pipeline {
	pipeline := &Pipeline{
		Name:   p.name,
		Stages: make([]Stage, 0, len(p.stages)),
	}

	for _, sb := range p.stages {
		stage := Stage{
			Name:      sb.name,
			DependsOn: sb.dependsOn,
			Steps:     sb.steps,
		}
		pipeline.Stages = append(pipeline.Stages, stage)
	}

	return pipeline
}

// DependsOn sets stage dependencies.
func (s *StageBuilder) DependsOn(stages ...string) *StageBuilder {
	s.dependsOn = append(s.dependsOn, stages...)
	return s
}

// Search adds a search step.
func (s *StageBuilder) Search(query string, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action:     "search",
		Parameters: map[string]interface{}{"query": query},
		SaveAs:     saveAs,
	})
	return s
}

// Test adds a test step.
func (s *StageBuilder) Test(objectsVar string, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action:     "test",
		Parameters: map[string]interface{}{"objects": objectsVar},
		SaveAs:     saveAs,
	})
	return s
}

// TestPackage adds a test step for a package.
func (s *StageBuilder) TestPackage(pkg string, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action:     "test",
		Parameters: map[string]interface{}{"package": pkg},
		SaveAs:     saveAs,
	})
	return s
}

// SyntaxCheck adds a syntax check step.
func (s *StageBuilder) SyntaxCheck(objectsVar string, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action:     "syntax_check",
		Parameters: map[string]interface{}{"objects": objectsVar},
		SaveAs:     saveAs,
	})
	return s
}

// FailIfTestsFailed adds a fail condition for test failures.
func (s *StageBuilder) FailIfTestsFailed(resultsVar string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "fail_if",
		Parameters: map[string]interface{}{
			"condition": "tests_failed:" + resultsVar,
		},
	})
	return s
}

// FailIfSyntaxErrors adds a fail condition for syntax errors.
func (s *StageBuilder) FailIfSyntaxErrors(resultsVar string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "fail_if",
		Parameters: map[string]interface{}{
			"condition": "syntax_errors:" + resultsVar,
		},
	})
	return s
}

// Activate adds an activation step.
func (s *StageBuilder) Activate(objectsVar string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action:     "activate",
		Parameters: map[string]interface{}{"objects": objectsVar},
	})
	return s
}

// Print adds a print step.
func (s *StageBuilder) Print(message string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action:     "print",
		Parameters: map[string]interface{}{"message": message},
	})
	return s
}

// Import adds an import step (import files from directory).
func (s *StageBuilder) Import(directory, packageName string, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "import",
		Parameters: map[string]interface{}{
			"directory": directory,
			"package":   packageName,
		},
		SaveAs: saveAs,
	})
	return s
}

// ImportFiles adds a step to import specific files.
func (s *StageBuilder) ImportFiles(files []string, packageName string, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "import_files",
		Parameters: map[string]interface{}{
			"files":   files,
			"package": packageName,
		},
		SaveAs: saveAs,
	})
	return s
}

// Create adds a create step for a new object.
func (s *StageBuilder) Create(objectType, name, packageName, description string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "create",
		Parameters: map[string]interface{}{
			"type":        objectType,
			"name":        name,
			"package":     packageName,
			"description": description,
		},
	})
	return s
}

// WriteSource adds a step to write source code.
func (s *StageBuilder) WriteSource(objectType, name, sourceVar string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "write_source",
		Parameters: map[string]interface{}{
			"type":   objectType,
			"name":   name,
			"source": sourceVar,
		},
	})
	return s
}

// ActivateObject adds a step to activate a specific object.
func (s *StageBuilder) ActivateObject(objectType, name string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "activate_object",
		Parameters: map[string]interface{}{
			"type": objectType,
			"name": name,
		},
	})
	return s
}

// Publish adds a step to publish a service binding.
func (s *StageBuilder) Publish(bindingName, version string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "publish",
		Parameters: map[string]interface{}{
			"binding": bindingName,
			"version": version,
		},
	})
	return s
}

// Unpublish adds a step to unpublish a service binding.
func (s *StageBuilder) Unpublish(bindingName, version string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "unpublish",
		Parameters: map[string]interface{}{
			"binding": bindingName,
			"version": version,
		},
	})
	return s
}

// Query adds a step to run an SQL query.
func (s *StageBuilder) Query(sql, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action:     "query",
		Parameters: map[string]interface{}{"sql": sql},
		SaveAs:     saveAs,
	})
	return s
}

// Export adds a step to export objects to files.
func (s *StageBuilder) Export(objectsVar, outputDir string, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "export",
		Parameters: map[string]interface{}{
			"objects":   objectsVar,
			"outputDir": outputDir,
		},
		SaveAs: saveAs,
	})
	return s
}

// ExportClasses adds a step to export specific classes.
func (s *StageBuilder) ExportClasses(classNames []string, outputDir string, saveAs string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "export_classes",
		Parameters: map[string]interface{}{
			"classes":   classNames,
			"outputDir": outputDir,
		},
		SaveAs: saveAs,
	})
	return s
}

// SetVariable adds a step to set a variable.
func (s *StageBuilder) SetVariable(name, value string) *StageBuilder {
	s.steps = append(s.steps, Step{
		Action: "set_var",
		Parameters: map[string]interface{}{
			"name":  name,
			"value": value,
		},
	})
	return s
}

// Then returns to the pipeline builder to add more stages.
func (s *StageBuilder) Then() *PipelineBuilder {
	return s.pipeline
}

// --- Convenience Pipelines ---

// TestPipeline creates a standard test pipeline.
func TestPipeline(client *adt.Client, packagePattern string) *Pipeline {
	return NewPipeline(client, "test").
		Stage("discover").
			Search(packagePattern, "objects").
			Then().
		Stage("validate").
			DependsOn("discover").
			SyntaxCheck("objects", "syntaxResults").
			FailIfSyntaxErrors("syntaxResults").
			Then().
		Stage("test").
			DependsOn("validate").
			Test("objects", "testResults").
			FailIfTestsFailed("testResults").
			Then().
		Build()
}

// CIPipeline creates a CI/CD pipeline.
func CIPipeline(client *adt.Client, packagePattern string) *Pipeline {
	return NewPipeline(client, "ci").
		Stage("discover").
			Search(packagePattern, "objects").
			Print("Found objects to test").
			Then().
		Stage("syntax").
			DependsOn("discover").
			SyntaxCheck("objects", "syntaxResults").
			FailIfSyntaxErrors("syntaxResults").
			Print("Syntax check passed").
			Then().
		Stage("test").
			DependsOn("syntax").
			Test("objects", "testResults").
			FailIfTestsFailed("testResults").
			Print("All tests passed").
			Then().
		Build()
}

// DeployPipeline creates a deployment pipeline (import → activate → test).
func DeployPipeline(client *adt.Client, sourceDir, packageName string) *Pipeline {
	return NewPipeline(client, "deploy").
		Stage("import").
			Import(sourceDir, packageName, "importResults").
			Print("Files imported").
			Then().
		Stage("activate").
			DependsOn("import").
			Search(packageName+"*", "objects").
			Activate("objects").
			Print("Objects activated").
			Then().
		Stage("test").
			DependsOn("activate").
			Test("objects", "testResults").
			Print("Tests completed").
			Then().
		Build()
}

// RAPPipeline creates a full RAP service deployment pipeline.
// Stages: import → activate → publish → verify
func RAPPipeline(client *adt.Client, sourceDir, packageName, serviceBinding string) *Pipeline {
	return NewPipeline(client, "rap-deploy").
		Stage("import").
			Import(sourceDir, packageName, "importResults").
			Print("RAP artifacts imported").
			Then().
		Stage("activate").
			DependsOn("import").
			Search(packageName+"*", "objects").
			Activate("objects").
			Print("All objects activated").
			Then().
		Stage("publish").
			DependsOn("activate").
			Publish(serviceBinding, "0001").
			Print("Service binding published").
			Then().
		Stage("verify").
			DependsOn("publish").
			Query("SELECT * FROM "+serviceBinding+" WHERE 1=0", "queryResult").
			Print("Service verified").
			Then().
		Build()
}

// ExportPipeline creates an export pipeline for backup/git.
func ExportPipeline(client *adt.Client, packagePattern, outputDir string) *Pipeline {
	return NewPipeline(client, "export").
		Stage("discover").
			Search(packagePattern, "objects").
			Print("Found objects to export").
			Then().
		Stage("export").
			DependsOn("discover").
			Export("objects", outputDir, "exportResults").
			Print("Objects exported").
			Then().
		Build()
}
