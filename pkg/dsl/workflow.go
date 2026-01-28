package dsl

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// Workflow represents a YAML-defined workflow.
type Workflow struct {
	Name        string            `yaml:"name"`
	Description string            `yaml:"description,omitempty"`
	Variables   map[string]string `yaml:"variables,omitempty"`
	Steps       []WorkflowStep    `yaml:"steps"`
}

// WorkflowStep represents a single step in a workflow.
type WorkflowStep struct {
	Name       string                 `yaml:"name,omitempty"`
	Action     string                 `yaml:"action"`
	Parameters map[string]interface{} `yaml:"parameters,omitempty"`
	SaveAs     string                 `yaml:"saveAs,omitempty"`
	Condition  string                 `yaml:"condition,omitempty"`
	OnFailure  string                 `yaml:"onFailure,omitempty"` // continue, fail, skip
}

// WorkflowResult represents the result of a workflow execution.
type WorkflowResult struct {
	Name        string               `json:"name"`
	Success     bool                 `json:"success"`
	StepResults []StepResult         `json:"stepResults"`
	Variables   map[string]interface{} `json:"variables"`
	Error       string               `json:"error,omitempty"`
}

// StepResult represents the result of a single step.
type StepResult struct {
	Name       string      `json:"name"`
	Action     string      `json:"action"`
	Success    bool        `json:"success"`
	Output     interface{} `json:"output,omitempty"`
	Error      string      `json:"error,omitempty"`
	Skipped    bool        `json:"skipped,omitempty"`
	SkipReason string      `json:"skipReason,omitempty"`
}

// WorkflowEngine executes YAML-defined workflows.
type WorkflowEngine struct {
	client   *adt.Client
	handlers map[string]ActionHandler
}

// ActionHandler is a function that handles a workflow action.
type ActionHandler func(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error)

// NewWorkflowEngine creates a new workflow engine.
func NewWorkflowEngine(client *adt.Client) *WorkflowEngine {
	engine := &WorkflowEngine{
		client:   client,
		handlers: make(map[string]ActionHandler),
	}

	// Register built-in handlers
	engine.RegisterHandler("search", handleSearch)
	engine.RegisterHandler("test", handleTest)
	engine.RegisterHandler("syntax_check", handleSyntaxCheck)
	engine.RegisterHandler("transform", handleTransform)
	engine.RegisterHandler("save", handleSave)
	engine.RegisterHandler("activate", handleActivate)
	engine.RegisterHandler("print", handlePrint)
	engine.RegisterHandler("fail_if", handleFailIf)
	engine.RegisterHandler("foreach", handleForEach)

	return engine
}

// RegisterHandler registers a custom action handler.
func (e *WorkflowEngine) RegisterHandler(action string, handler ActionHandler) {
	e.handlers[action] = handler
}

// LoadWorkflow loads a workflow from a YAML file.
func (e *WorkflowEngine) LoadWorkflow(path string) (*Workflow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading workflow file: %w", err)
	}

	return e.ParseWorkflow(data)
}

// ParseWorkflow parses a workflow from YAML data.
func (e *WorkflowEngine) ParseWorkflow(data []byte) (*Workflow, error) {
	var workflow Workflow
	if err := yaml.Unmarshal(data, &workflow); err != nil {
		return nil, fmt.Errorf("parsing workflow: %w", err)
	}

	return &workflow, nil
}

// Execute runs a workflow.
func (e *WorkflowEngine) Execute(ctx context.Context, workflow *Workflow, opts ...ExecuteOption) (*WorkflowResult, error) {
	execCtx := NewExecutionContext(ctx, e.client)

	// Set workflow variables first (literal values, no env expansion)
	for k, v := range workflow.Variables {
		execCtx.SetVariable(k, v)
	}

	// Apply options AFTER workflow variables (so WithVariables can override)
	for _, opt := range opts {
		opt(execCtx)
	}

	result := &WorkflowResult{
		Name:        workflow.Name,
		Success:     true,
		StepResults: make([]StepResult, 0, len(workflow.Steps)),
		Variables:   make(map[string]interface{}),
	}

	// Execute steps
	for i, step := range workflow.Steps {
		stepName := step.Name
		if stepName == "" {
			stepName = fmt.Sprintf("step_%d_%s", i+1, step.Action)
		}

		stepResult := StepResult{
			Name:   stepName,
			Action: step.Action,
		}

		// Check condition
		if step.Condition != "" {
			if !e.evaluateCondition(execCtx, step.Condition) {
				stepResult.Skipped = true
				stepResult.SkipReason = "condition not met"
				stepResult.Success = true
				result.StepResults = append(result.StepResults, stepResult)
				continue
			}
		}

		// Get handler
		handler, ok := e.handlers[step.Action]
		if !ok {
			stepResult.Success = false
			stepResult.Error = fmt.Sprintf("unknown action: %s", step.Action)
			result.StepResults = append(result.StepResults, stepResult)
			result.Success = false
			result.Error = stepResult.Error
			return result, nil
		}

		// Expand variables in parameters
		params := e.expandParams(execCtx, step.Parameters)

		// Execute handler
		output, err := handler(execCtx, params)
		if err != nil {
			stepResult.Success = false
			stepResult.Error = err.Error()

			// Handle failure mode
			switch step.OnFailure {
			case "continue":
				result.StepResults = append(result.StepResults, stepResult)
				continue
			case "skip":
				stepResult.Skipped = true
				stepResult.SkipReason = "skipped due to error"
				result.StepResults = append(result.StepResults, stepResult)
				continue
			default: // "fail" or empty
				result.StepResults = append(result.StepResults, stepResult)
				result.Success = false
				result.Error = fmt.Sprintf("step '%s' failed: %s", stepName, err)
				return result, nil
			}
		}

		stepResult.Success = true
		stepResult.Output = output

		// Save result if requested
		if step.SaveAs != "" {
			execCtx.Set(step.SaveAs, output)
			result.Variables[step.SaveAs] = output
		}

		result.StepResults = append(result.StepResults, stepResult)
	}

	return result, nil
}

// ExecuteOption configures workflow execution.
type ExecuteOption func(*ExecutionContext)

// WithDryRun enables dry-run mode.
func WithDryRun(dryRun bool) ExecuteOption {
	return func(ctx *ExecutionContext) {
		ctx.SetDryRun(dryRun)
	}
}

// WithVerbose enables verbose output.
func WithVerbose(verbose bool) ExecuteOption {
	return func(ctx *ExecutionContext) {
		ctx.SetVerbose(verbose)
	}
}

// WithVariables sets additional variables.
func WithVariables(vars map[string]string) ExecuteOption {
	return func(ctx *ExecutionContext) {
		for k, v := range vars {
			ctx.SetVariable(k, v)
		}
	}
}

// expandEnvVars expands ${VAR} and $VAR in strings.
func (e *WorkflowEngine) expandEnvVars(s string) string {
	return os.ExpandEnv(s)
}

// expandParams expands variables in parameters.
func (e *WorkflowEngine) expandParams(ctx *ExecutionContext, params map[string]interface{}) map[string]interface{} {
	if params == nil {
		return nil
	}

	result := make(map[string]interface{})
	for k, v := range params {
		result[k] = e.expandValue(ctx, v)
	}
	return result
}

// expandValue recursively expands variables in a value.
func (e *WorkflowEngine) expandValue(ctx *ExecutionContext, v interface{}) interface{} {
	switch val := v.(type) {
	case string:
		// Expand ${var} references to context variables and env vars
		// Note: Only ${VAR} syntax is supported, NOT $VAR (conflicts with SAP package names like $TMP)
		re := regexp.MustCompile(`\$\{(\w+)\}`)
		expanded := re.ReplaceAllStringFunc(val, func(match string) string {
			varName := match[2 : len(match)-1]
			if ctxVal, ok := ctx.Get(varName); ok {
				return fmt.Sprintf("%v", ctxVal)
			}
			if envVal := ctx.GetVariable(varName); envVal != "" {
				return envVal
			}
			return os.Getenv(varName)
		})
		return expanded
	case []interface{}:
		result := make([]interface{}, len(val))
		for i, item := range val {
			result[i] = e.expandValue(ctx, item)
		}
		return result
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, item := range val {
			result[k] = e.expandValue(ctx, item)
		}
		return result
	default:
		return v
	}
}

// evaluateCondition evaluates a simple condition.
func (e *WorkflowEngine) evaluateCondition(ctx *ExecutionContext, condition string) bool {
	// Simple conditions: "exists:varName", "empty:varName", "true", "false"
	condition = strings.TrimSpace(condition)

	if strings.HasPrefix(condition, "exists:") {
		varName := strings.TrimPrefix(condition, "exists:")
		_, ok := ctx.Get(varName)
		return ok
	}

	if strings.HasPrefix(condition, "empty:") {
		varName := strings.TrimPrefix(condition, "empty:")
		val, ok := ctx.Get(varName)
		if !ok {
			return true
		}
		switch v := val.(type) {
		case []interface{}:
			return len(v) == 0
		case []ObjectRef:
			return len(v) == 0
		case string:
			return v == ""
		default:
			return false
		}
	}

	if strings.HasPrefix(condition, "not_empty:") {
		varName := strings.TrimPrefix(condition, "not_empty:")
		val, ok := ctx.Get(varName)
		if !ok {
			return false
		}
		switch v := val.(type) {
		case []interface{}:
			return len(v) > 0
		case []ObjectRef:
			return len(v) > 0
		case string:
			return v != ""
		default:
			return true
		}
	}

	return condition == "true"
}

// --- Built-in Action Handlers ---

func handleSearch(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	query, _ := params["query"].(string)
	if query == "" {
		query = "*"
	}

	maxResults := 100
	if mr, ok := params["maxResults"].(int); ok {
		maxResults = mr
	}

	search := Search(ctx.Client()).Query(query).MaxResults(maxResults)

	if types, ok := params["types"].([]interface{}); ok {
		for _, t := range types {
			if ts, ok := t.(string); ok {
				search.Types(ts)
			}
		}
	}

	if pkg, ok := params["package"].(string); ok {
		search.InPackage(pkg)
	}

	if pkgs, ok := params["packages"].([]interface{}); ok {
		for _, p := range pkgs {
			if ps, ok := p.(string); ok {
				search.InPackages(ps)
			}
		}
	}

	return search.Execute(ctx.Context())
}

func handleTest(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	runner := Test(ctx.Client())

	// Get objects from params or context
	if objectsVar, ok := params["objects"].(string); ok {
		if val, exists := ctx.Get(objectsVar); exists {
			if objects, ok := val.([]ObjectRef); ok {
				runner.Objects(objects...)
			}
		}
	}

	if className, ok := params["class"].(string); ok {
		runner.Class(className)
	}

	if pkgName, ok := params["package"].(string); ok {
		runner.Package(pkgName)
	}

	if dangerous, ok := params["dangerous"].(bool); ok && dangerous {
		runner.IncludeDangerous()
	}

	if long, ok := params["long"].(bool); ok && long {
		runner.IncludeLong()
	}

	if stopOnFailure, ok := params["stopOnFirstFailure"].(bool); ok && stopOnFailure {
		runner.StopOnFirstFailure()
	}

	return runner.Run(ctx.Context())
}

func handleSyntaxCheck(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	// Get objects from context
	objectsVar, _ := params["objects"].(string)
	if objectsVar == "" {
		return nil, fmt.Errorf("syntax_check requires 'objects' parameter")
	}

	val, exists := ctx.Get(objectsVar)
	if !exists {
		return nil, fmt.Errorf("variable '%s' not found", objectsVar)
	}

	objects, ok := val.([]ObjectRef)
	if !ok {
		return nil, fmt.Errorf("variable '%s' is not a list of objects", objectsVar)
	}

	var results []map[string]interface{}
	for _, obj := range objects {
		// Get source
		var source string
		var err error

		switch obj.Type {
		case TypeClass, "CLAS/OC":
			source, err = ctx.Client().GetSource(ctx.Context(), "CLAS", obj.Name, nil)
			if err != nil {
				return nil, err
			}
		case TypeProgram, "PROG/P":
			source, err = ctx.Client().GetSource(ctx.Context(), "PROG", obj.Name, nil)
			if err != nil {
				return nil, err
			}
		default:
			continue
		}

		// Build object URL
		objectURL := buildObjectURL(obj)

		// Run syntax check
		checkResults, err := ctx.Client().SyntaxCheck(ctx.Context(), objectURL, source)
		if err != nil {
			return nil, err
		}

		hasErrors := false
		for _, r := range checkResults {
			if r.Severity == "E" || r.Severity == "A" || r.Severity == "X" {
				hasErrors = true
				break
			}
		}

		results = append(results, map[string]interface{}{
			"object":    obj.Name,
			"success":   !hasErrors,
			"messages":  checkResults,
		})
	}

	return results, nil
}

func handleTransform(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	if ctx.IsDryRun() {
		return map[string]interface{}{"dryRun": true, "action": "transform"}, nil
	}

	// Transform is handled by batch operations
	return nil, fmt.Errorf("transform action not yet implemented")
}

func handleSave(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	if ctx.IsDryRun() {
		return map[string]interface{}{"dryRun": true, "action": "save"}, nil
	}

	return nil, fmt.Errorf("save action not yet implemented")
}

func handleActivate(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	if ctx.IsDryRun() {
		return map[string]interface{}{"dryRun": true, "action": "activate"}, nil
	}

	// Get objects from context
	objectsVar, _ := params["objects"].(string)
	if objectsVar == "" {
		return nil, fmt.Errorf("activate requires 'objects' parameter")
	}

	val, exists := ctx.Get(objectsVar)
	if !exists {
		return nil, fmt.Errorf("variable '%s' not found", objectsVar)
	}

	objects, ok := val.([]ObjectRef)
	if !ok {
		return nil, fmt.Errorf("variable '%s' is not a list of objects", objectsVar)
	}

	var results []map[string]interface{}
	for _, obj := range objects {
		objectURL := buildObjectURL(obj)
		result, err := ctx.Client().Activate(ctx.Context(), objectURL, obj.Name)
		if err != nil {
			return nil, err
		}
		results = append(results, map[string]interface{}{
			"object":  obj.Name,
			"success": result.Success,
			"messages": result.Messages,
		})
	}

	return results, nil
}

func handlePrint(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	message, _ := params["message"].(string)
	fmt.Println(message)
	return nil, nil
}

func handleFailIf(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	condition, _ := params["condition"].(string)
	message, _ := params["message"].(string)

	// Check common conditions
	if strings.HasPrefix(condition, "tests_failed:") {
		varName := strings.TrimPrefix(condition, "tests_failed:")
		val, exists := ctx.Get(varName)
		if exists {
			if summary, ok := val.(*TestSummary); ok {
				if summary.FailedTests > 0 {
					if message == "" {
						message = fmt.Sprintf("%d tests failed", summary.FailedTests)
					}
					return nil, fmt.Errorf(message)
				}
			}
		}
	}

	if strings.HasPrefix(condition, "syntax_errors:") {
		varName := strings.TrimPrefix(condition, "syntax_errors:")
		val, exists := ctx.Get(varName)
		if exists {
			if results, ok := val.([]map[string]interface{}); ok {
				for _, r := range results {
					if success, ok := r["success"].(bool); ok && !success {
						if message == "" {
							message = fmt.Sprintf("syntax errors found in %v", r["object"])
						}
						return nil, fmt.Errorf(message)
					}
				}
			}
		}
	}

	return nil, nil
}

func handleForEach(ctx *ExecutionContext, params map[string]interface{}) (interface{}, error) {
	// ForEach iterates over a collection
	collectionVar, _ := params["collection"].(string)
	if collectionVar == "" {
		return nil, fmt.Errorf("foreach requires 'collection' parameter")
	}

	val, exists := ctx.Get(collectionVar)
	if !exists {
		return nil, fmt.Errorf("variable '%s' not found", collectionVar)
	}

	// The actual iteration would be handled by workflow expansion
	// This is a placeholder for the concept
	return val, nil
}

// buildObjectURL constructs the ADT URL for an object.
func buildObjectURL(obj ObjectRef) string {
	if obj.URL != "" {
		return obj.URL
	}

	name := strings.ToUpper(obj.Name)
	switch obj.Type {
	case TypeClass, "CLAS/OC":
		return fmt.Sprintf("/sap/bc/adt/oo/classes/%s", name)
	case TypeProgram, "PROG/P":
		return fmt.Sprintf("/sap/bc/adt/programs/programs/%s", name)
	case TypeInterface, "INTF/OI":
		return fmt.Sprintf("/sap/bc/adt/oo/interfaces/%s", name)
	default:
		return ""
	}
}
