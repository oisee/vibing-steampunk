package dsl

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// TestRunner provides fluent interface for running ABAP Unit tests.
type TestRunner struct {
	client  *adt.Client
	objects []ObjectRef
	config  TestConfig

	// Callbacks
	onStart    func(obj ObjectRef)
	onComplete func(obj ObjectRef, result TestResult)
	onError    func(obj ObjectRef, err error)
}

// Test creates a new test runner.
func Test(client *adt.Client) *TestRunner {
	return &TestRunner{
		client:  client,
		objects: []ObjectRef{},
		config:  DefaultTestConfig(),
	}
}

// Objects sets the objects to test.
func (t *TestRunner) Objects(objects ...ObjectRef) *TestRunner {
	t.objects = append(t.objects, objects...)
	return t
}

// Object adds a single object to test.
func (t *TestRunner) Object(objType, name string) *TestRunner {
	t.objects = append(t.objects, ObjectRef{Type: objType, Name: name})
	return t
}

// Class adds a class to test.
func (t *TestRunner) Class(name string) *TestRunner {
	return t.Object(TypeClass, name)
}

// Program adds a program to test.
func (t *TestRunner) Program(name string) *TestRunner {
	return t.Object(TypeProgram, name)
}

// Package adds all testable objects in a package.
func (t *TestRunner) Package(pkgName string) *TestRunner {
	// Will be resolved during execution
	t.objects = append(t.objects, ObjectRef{Type: TypePackage, Name: pkgName})
	return t
}

// FromSearch uses search results as test targets.
func (t *TestRunner) FromSearch(search *SearchBuilder) *TestRunner {
	// Objects will be resolved during execution
	return t
}

// WithConfig sets the test configuration.
func (t *TestRunner) WithConfig(config TestConfig) *TestRunner {
	t.config = config
	return t
}

// IncludeDangerous includes dangerous risk level tests.
func (t *TestRunner) IncludeDangerous() *TestRunner {
	t.config.Dangerous = true
	return t
}

// IncludeLong includes long duration tests.
func (t *TestRunner) IncludeLong() *TestRunner {
	t.config.Long = true
	return t
}

// StopOnFirstFailure stops execution on first test failure.
func (t *TestRunner) StopOnFirstFailure() *TestRunner {
	t.config.StopOnFirstFailure = true
	return t
}

// Parallel sets the number of parallel test executions.
func (t *TestRunner) Parallel(n int) *TestRunner {
	t.config.Parallel = n
	return t
}

// Timeout sets the timeout for each test run.
func (t *TestRunner) Timeout(d time.Duration) *TestRunner {
	t.config.Timeout = d
	return t
}

// OnStart sets a callback for when a test starts.
func (t *TestRunner) OnStart(fn func(obj ObjectRef)) *TestRunner {
	t.onStart = fn
	return t
}

// OnComplete sets a callback for when a test completes.
func (t *TestRunner) OnComplete(fn func(obj ObjectRef, result TestResult)) *TestRunner {
	t.onComplete = fn
	return t
}

// OnError sets a callback for when a test errors.
func (t *TestRunner) OnError(fn func(obj ObjectRef, err error)) *TestRunner {
	t.onError = fn
	return t
}

// Run executes all tests and returns a summary.
func (t *TestRunner) Run(ctx context.Context) (*TestSummary, error) {
	// Resolve package references to actual objects
	objects, err := t.resolveObjects(ctx)
	if err != nil {
		return nil, err
	}

	summary := &TestSummary{
		TotalObjects: len(objects),
		Results:      make([]TestResult, 0, len(objects)),
	}

	startTime := time.Now()

	if t.config.Parallel > 1 {
		// Parallel execution
		err = t.runParallel(ctx, objects, summary)
	} else {
		// Sequential execution
		err = t.runSequential(ctx, objects, summary)
	}

	summary.TotalTime = time.Since(startTime)

	return summary, err
}

// runSequential executes tests one at a time.
func (t *TestRunner) runSequential(ctx context.Context, objects []ObjectRef, summary *TestSummary) error {
	for _, obj := range objects {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		result := t.runSingleTest(ctx, obj)
		t.aggregateResult(summary, result)

		if t.config.StopOnFirstFailure && !result.Success {
			return fmt.Errorf("test failed for %s: %s", obj.Name, result.Error)
		}
	}
	return nil
}

// runParallel executes tests in parallel.
func (t *TestRunner) runParallel(ctx context.Context, objects []ObjectRef, summary *TestSummary) error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	semaphore := make(chan struct{}, t.config.Parallel)
	errChan := make(chan error, len(objects))

	for _, obj := range objects {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		wg.Add(1)
		semaphore <- struct{}{}

		go func(obj ObjectRef) {
			defer wg.Done()
			defer func() { <-semaphore }()

			result := t.runSingleTest(ctx, obj)

			mu.Lock()
			t.aggregateResult(summary, result)
			mu.Unlock()

			if t.config.StopOnFirstFailure && !result.Success {
				errChan <- fmt.Errorf("test failed for %s: %s", obj.Name, result.Error)
			}
		}(obj)
	}

	wg.Wait()
	close(errChan)

	// Return first error if any
	for err := range errChan {
		return err
	}

	return nil
}

// runSingleTest runs tests for a single object.
func (t *TestRunner) runSingleTest(ctx context.Context, obj ObjectRef) TestResult {
	result := TestResult{
		Object: obj,
	}

	if t.onStart != nil {
		t.onStart(obj)
	}

	startTime := time.Now()

	// Build object URL
	objectURL := t.buildObjectURL(obj)
	if objectURL == "" {
		result.Error = "unable to determine object URL"
		if t.onError != nil {
			t.onError(obj, fmt.Errorf("%s", result.Error))
		}
		return result
	}

	// Create test flags from config
	flags := &adt.UnitTestRunFlags{
		Harmless:  t.config.Harmless,
		Dangerous: t.config.Dangerous,
		Critical:  t.config.Critical,
		Short:     t.config.Short,
		Medium:    t.config.Medium,
		Long:      t.config.Long,
	}

	// Run the tests
	testResult, err := t.client.RunUnitTests(ctx, objectURL, flags)
	if err != nil {
		result.Error = err.Error()
		if t.onError != nil {
			t.onError(obj, err)
		}
		return result
	}

	result.ExecutionTime = time.Since(startTime)

	// Parse results
	result.Success = true
	for _, class := range testResult.Classes {
		classResult := TestClassResult{
			Name:    class.Name,
			Methods: make([]TestMethodResult, 0, len(class.TestMethods)),
		}

		for _, method := range class.TestMethods {
			methodResult := TestMethodResult{
				Name:          method.Name,
				Success:       len(method.Alerts) == 0,
				ExecutionTime: time.Duration(method.ExecutionTime) * time.Microsecond,
			}

			result.TotalTests++
			if methodResult.Success {
				result.PassedTests++
			} else {
				result.FailedTests++
				result.Success = false
				// Collect failure message
				if len(method.Alerts) > 0 {
					methodResult.Message = method.Alerts[0].Title
				}
			}

			classResult.Methods = append(classResult.Methods, methodResult)
		}

		result.Classes = append(result.Classes, classResult)
	}

	if t.onComplete != nil {
		t.onComplete(obj, result)
	}

	return result
}

// resolveObjects resolves package references to actual testable objects.
func (t *TestRunner) resolveObjects(ctx context.Context) ([]ObjectRef, error) {
	var resolved []ObjectRef

	for _, obj := range t.objects {
		if obj.Type == TypePackage {
			// Search for testable objects in package
			search := Search(t.client).
				Query(obj.Name + "/*").
				Types(TypeClass, TypeProgram).
				MaxResults(500)

			objects, err := search.Execute(ctx)
			if err != nil {
				return nil, fmt.Errorf("resolving package %s: %w", obj.Name, err)
			}
			resolved = append(resolved, objects...)
		} else {
			resolved = append(resolved, obj)
		}
	}

	return resolved, nil
}

// buildObjectURL constructs the ADT URL for an object.
func (t *TestRunner) buildObjectURL(obj ObjectRef) string {
	if obj.URL != "" {
		return obj.URL
	}

	name := obj.Name
	switch obj.Type {
	case TypeClass, "CLAS/OC":
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
	case TypeProgram, "PROG/P":
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", name)
	case TypeInterface, "INTF/OI":
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
	case TypeFunction:
		return fmt.Sprintf("/sap/bc/adt/functions/groups/%s/fmodules/%s", obj.Package, name)
	default:
		return ""
	}
}

// aggregateResult adds a test result to the summary.
func (t *TestRunner) aggregateResult(summary *TestSummary, result TestResult) {
	summary.Results = append(summary.Results, result)
	summary.TestedObjects++

	if result.Success {
		summary.PassedObjects++
	} else {
		summary.FailedObjects++
	}

	summary.TotalTests += result.TotalTests
	summary.PassedTests += result.PassedTests
	summary.FailedTests += result.FailedTests
	summary.SkippedTests += result.SkippedTests
}

// --- Convenience Functions ---

// RunTests is a shorthand for running tests on specified objects.
func RunTests(ctx context.Context, client *adt.Client, objects ...ObjectRef) (*TestSummary, error) {
	return Test(client).Objects(objects...).Run(ctx)
}

// RunTestsForClass runs tests for a single class.
func RunTestsForClass(ctx context.Context, client *adt.Client, className string) (*TestSummary, error) {
	return Test(client).Class(className).Run(ctx)
}

// RunTestsForPackage runs tests for all objects in a package.
func RunTestsForPackage(ctx context.Context, client *adt.Client, packageName string) (*TestSummary, error) {
	return Test(client).Package(packageName).Run(ctx)
}
