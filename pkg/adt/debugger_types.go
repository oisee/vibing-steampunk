// ABOUTME: Type definitions for ABAP debugger functionality.
// ABOUTME: Contains breakpoint, listener, session, and variable types.

package adt

// --- Breakpoint Types ---

// BreakpointKind represents the type of breakpoint.
type BreakpointKind string

const (
	BreakpointKindLine        BreakpointKind = "line"
	BreakpointKindStatement   BreakpointKind = "statement"
	BreakpointKindException   BreakpointKind = "exception"
	BreakpointKindMessage     BreakpointKind = "message"
	BreakpointKindBadi        BreakpointKind = "badi"        // Business Add-In breakpoint
	BreakpointKindEnhancement BreakpointKind = "enhancement" // Enhancement point breakpoint
	BreakpointKindWatchpoint  BreakpointKind = "watchpoint"  // Data watchpoint (variable change)
	BreakpointKindMethod      BreakpointKind = "method"      // Method/function entry breakpoint
)

// BreakpointScope determines the lifetime of a breakpoint.
type BreakpointScope string

const (
	// BreakpointScopeExternal persists across sessions (external/static breakpoints)
	BreakpointScopeExternal BreakpointScope = "external"
	// BreakpointScopeDebugger is session-bound (only during debug session)
	BreakpointScopeDebugger BreakpointScope = "debugger"
)

// DebuggingMode determines how debugging is triggered.
type DebuggingMode string

const (
	// DebuggingModeUser debugs all processes of a specific user
	DebuggingModeUser DebuggingMode = "user"
	// DebuggingModeTerminal debugs processes from a specific terminal
	DebuggingModeTerminal DebuggingMode = "terminal"
)

// Breakpoint represents an ABAP debugger breakpoint.
type Breakpoint struct {
	ID          string         `json:"id"`
	Kind        BreakpointKind `json:"kind"`
	Enabled     bool           `json:"enabled"`
	URI         string         `json:"uri,omitempty"`         // ADT URI for line breakpoints
	Line        int            `json:"line,omitempty"`        // Line number for line breakpoints
	Condition   string         `json:"condition,omitempty"`   // Optional condition expression
	Statement   string         `json:"statement,omitempty"`   // Statement type for statement breakpoints
	Exception   string         `json:"exception,omitempty"`   // Exception class for exception breakpoints
	MessageID   string         `json:"messageId,omitempty"`   // Message ID for message breakpoints
	MessageType string         `json:"messageType,omitempty"` // Message type (E, W, I, S, A)
	MessageArea string         `json:"messageArea,omitempty"` // Message class/area (e.g., "00", "SY")

	// BAdi and Enhancement breakpoints
	BadiName        string `json:"badiName,omitempty"`        // BAdi definition name
	EnhancementSpot string `json:"enhancementSpot,omitempty"` // Enhancement spot name
	EnhancementImpl string `json:"enhancementImpl,omitempty"` // Enhancement implementation name

	// Watchpoint (data breakpoint)
	Variable       string `json:"variable,omitempty"`       // Variable name to watch
	WatchCondition string `json:"watchCondition,omitempty"` // When to trigger: "change", "read", "any"

	// Method breakpoint
	ClassName  string `json:"className,omitempty"`  // Class name for method breakpoint
	MethodName string `json:"methodName,omitempty"` // Method name for method breakpoint

	// Read-only fields returned by SAP
	ActualLine int    `json:"actualLine,omitempty"` // Actual line after adjustment
	IsActive   bool   `json:"isActive,omitempty"`   // Whether BP is currently active
	ObjectName string `json:"objectName,omitempty"` // Name of the object containing the BP
}

// BreakpointRequest contains parameters for creating breakpoints.
type BreakpointRequest struct {
	Scope           BreakpointScope `json:"scope"`
	DebuggingMode   DebuggingMode   `json:"debuggingMode"`
	TerminalID      string          `json:"terminalId,omitempty"`
	User            string          `json:"user,omitempty"`
	IdeID           string          `json:"ideId,omitempty"`           // IDE identifier (default: "vsp")
	ClientID        string          `json:"clientId,omitempty"`        // Client ID for breakpoints
	SystemDebugging bool            `json:"systemDebugging,omitempty"` // Enable system debugging
	Deactivated     bool            `json:"deactivated,omitempty"`     // Create breakpoints in deactivated state
	SyncScopeURI    string          `json:"syncScopeUri,omitempty"`    // Partial sync scope URI
	Breakpoints     []Breakpoint    `json:"breakpoints"`
}

// BreakpointResponse contains the result of breakpoint operations.
type BreakpointResponse struct {
	Breakpoints []Breakpoint `json:"breakpoints"`
}

// --- Debug Listener Types ---

// DebuggeeKind represents the type of debuggee.
type DebuggeeKind string

const (
	DebuggeeKindDebuggee         DebuggeeKind = "debuggee"
	DebuggeeKindPostMortem       DebuggeeKind = "postmortem"
	DebuggeeKindPostMortemDialog DebuggeeKind = "postmortem_dialog"
)

// Debuggee represents a process that has hit a breakpoint and is waiting for debugging.
type Debuggee struct {
	ID           string       `json:"debuggeeId"`
	Kind         DebuggeeKind `json:"kind"`
	Client       int          `json:"client"`
	TerminalID   string       `json:"terminalId"`
	IdeID        string       `json:"ideId"`
	User         string       `json:"debuggeeUser"`
	Program      string       `json:"program"`
	Include      string       `json:"include"`
	Line         int          `json:"line"`
	RFCDest      string       `json:"rfcDest,omitempty"`
	AppServer    string       `json:"appServer,omitempty"`
	SystemID     string       `json:"systemId,omitempty"`
	SystemNumber int          `json:"systemNumber,omitempty"`
	Timestamp    int64        `json:"timestamp,omitempty"`
	IsAttachable bool         `json:"isAttachable"`
	IsSameServer bool         `json:"isSameServer"`
	InstanceName string       `json:"instanceName,omitempty"`
	// For post-mortem debugging (short dumps)
	DumpID     string `json:"dumpId,omitempty"`
	DumpDate   string `json:"dumpDate,omitempty"`
	DumpTime   string `json:"dumpTime,omitempty"`
	DumpHost   string `json:"dumpHost,omitempty"`
	DumpUser   string `json:"dumpUser,omitempty"`
	DumpClient string `json:"dumpClient,omitempty"`
	DumpURI    string `json:"dumpUri,omitempty"`
}

// ListenerConflict represents a conflict with another debug listener.
type ListenerConflict struct {
	ConflictText string `json:"conflictText"`
	IdeUser      string `json:"ideUser"`
}

// ListenResult represents the result of a debug listen operation.
type ListenResult struct {
	Debuggee *Debuggee         `json:"debuggee,omitempty"`
	Conflict *ListenerConflict `json:"conflict,omitempty"`
	TimedOut bool              `json:"timedOut"`
}

// ListenOptions configures the debug listener.
type ListenOptions struct {
	DebuggingMode    DebuggingMode `json:"debuggingMode"`
	User             string        `json:"user,omitempty"`       // Required for user mode
	TerminalID       string        `json:"terminalId,omitempty"` // Auto-generated if empty
	IdeID            string        `json:"ideId,omitempty"`      // Default: "vsp"
	TimeoutSeconds   int           `json:"timeout,omitempty"`    // Default: 240
	CheckConflict    bool          `json:"checkConflict"`
	NotifyOnConflict bool          `json:"notifyOnConflict"`
}

// --- Debug Session Types ---

// DebugStepType represents the type of debug step operation.
type DebugStepType string

const (
	DebugStepInto       DebugStepType = "stepInto"
	DebugStepOver       DebugStepType = "stepOver"
	DebugStepReturn     DebugStepType = "stepReturn"
	DebugStepContinue   DebugStepType = "stepContinue"
	DebugStepRunToLine  DebugStepType = "stepRunToLine"
	DebugStepJumpToLine DebugStepType = "stepJumpToLine"
	DebugTerminate      DebugStepType = "terminateDebuggee"
)

// DebugSettings contains debugger session settings.
type DebugSettings struct {
	SystemDebugging       bool `json:"systemDebugging"`
	CreateExceptionObject bool `json:"createExceptionObject"`
	BackgroundRFC         bool `json:"backgroundRFC"`
	SharedObjectDebugging bool `json:"sharedObjectDebugging"`
	ShowDataAging         bool `json:"showDataAging"`
	UpdateDebugging       bool `json:"updateDebugging"`
}

// DebugAction represents an available debugger action.
type DebugAction struct {
	Name     string `json:"name"`
	Style    string `json:"style"`
	Group    string `json:"group"`
	Title    string `json:"title"`
	Link     string `json:"link"`
	Value    string `json:"value"`
	Disabled bool   `json:"disabled"`
}

// DebugReachedBreakpoint represents a breakpoint that was hit.
type DebugReachedBreakpoint struct {
	ID                               string `json:"id"`
	Kind                             string `json:"kind"`
	UnresolvableCondition            string `json:"unresolvableCondition,omitempty"`
	UnresolvableConditionErrorOffset string `json:"unresolvableConditionErrorOffset,omitempty"`
}

// DebugState contains the current debug session state.
type DebugState struct {
	IsRFC                      bool          `json:"isRfc"`
	IsSameSystem               bool          `json:"isSameSystem"`
	ServerName                 string        `json:"serverName"`
	DebugSessionID             string        `json:"debugSessionId"`
	ProcessID                  int           `json:"processId"`
	IsPostMortem               bool          `json:"isPostMortem"`
	IsUserAuthorizedForChanges bool          `json:"isUserAuthorizedForChanges"`
	DebuggeeSessionID          string        `json:"debuggeeSessionId"`
	AbapTraceState             string        `json:"abapTraceState"`
	CanAdvancedTableFeatures   bool          `json:"canAdvancedTableFeatures"`
	IsNonExclusive             bool          `json:"isNonExclusive"`
	IsNonExclusiveToggled      bool          `json:"isNonExclusiveToggled"`
	GuiEditorGuid              string        `json:"guiEditorGuid"`
	SessionTitle               string        `json:"sessionTitle"`
	IsSteppingPossible         bool          `json:"isSteppingPossible"`
	IsTerminationPossible      bool          `json:"isTerminationPossible"`
	Actions                    []DebugAction `json:"actions,omitempty"`
}

// DebugAttachResult contains the result of attaching to a debuggee.
type DebugAttachResult struct {
	DebugState
	ReachedBreakpoints []DebugReachedBreakpoint `json:"reachedBreakpoints,omitempty"`
}

// DebugStepResult contains the result of a step operation.
type DebugStepResult struct {
	DebugState
	IsDebuggeeChanged  bool                     `json:"isDebuggeeChanged"`
	Settings           DebugSettings            `json:"settings"`
	ReachedBreakpoints []DebugReachedBreakpoint `json:"reachedBreakpoints,omitempty"`
}

// DebugStackEntry represents a single entry in the call stack.
type DebugStackEntry struct {
	StackPosition int    `json:"stackPosition"`
	StackType     string `json:"stackType"` // ABAP, DYNP, ENHANCEMENT
	StackURI      string `json:"stackUri"`
	ProgramName   string `json:"programName"`
	IncludeName   string `json:"includeName"`
	Line          int    `json:"line"`
	EventType     string `json:"eventType"`
	EventName     string `json:"eventName"`
	SourceType    string `json:"sourceType"` // ABAP, DYNP, ST
	SystemProgram bool   `json:"systemProgram"`
	IsVit         bool   `json:"isVit"`
	URI           string `json:"uri"`
}

// DebugStackInfo contains the call stack information.
type DebugStackInfo struct {
	IsRFC                 bool              `json:"isRfc"`
	IsSameSystem          bool              `json:"isSameSystem"`
	ServerName            string            `json:"serverName"`
	DebugCursorStackIndex int               `json:"debugCursorStackIndex,omitempty"`
	Stack                 []DebugStackEntry `json:"stack"`
}

// DebugMetaType represents the metatype of a variable.
type DebugMetaType string

const (
	DebugMetaTypeSimple     DebugMetaType = "simple"
	DebugMetaTypeString     DebugMetaType = "string"
	DebugMetaTypeStructure  DebugMetaType = "structure"
	DebugMetaTypeTable      DebugMetaType = "table"
	DebugMetaTypeDataRef    DebugMetaType = "dataref"
	DebugMetaTypeObjectRef  DebugMetaType = "objectref"
	DebugMetaTypeClass      DebugMetaType = "class"
	DebugMetaTypeObject     DebugMetaType = "object"
	DebugMetaTypeBoxRef     DebugMetaType = "boxref"
	DebugMetaTypeBoxedComp  DebugMetaType = "boxedcomp"
	DebugMetaTypeAnonymComp DebugMetaType = "anonymcomp"
	DebugMetaTypeUnknown    DebugMetaType = "unknown"
)

// DebugVariable represents a variable in the debugger.
type DebugVariable struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	DeclaredTypeName  string        `json:"declaredTypeName"`
	ActualTypeName    string        `json:"actualTypeName"`
	Kind              string        `json:"kind"`
	InstantiationKind string        `json:"instantiationKind"`
	AccessKind        string        `json:"accessKind"`
	MetaType          DebugMetaType `json:"metaType"`
	ParameterKind     string        `json:"parameterKind"`
	Value             string        `json:"value"`
	HexValue          string        `json:"hexValue,omitempty"`
	ReadOnly          bool          `json:"readOnly"`
	TechnicalType     string        `json:"technicalType"`
	Length            int           `json:"length"`
	TableBody         string        `json:"tableBody,omitempty"`
	TableLines        int           `json:"tableLines,omitempty"`
	IsValueIncomplete bool          `json:"isValueIncomplete"`
	IsException       bool          `json:"isException"`
	InheritanceLevel  int           `json:"inheritanceLevel,omitempty"`
	InheritanceClass  string        `json:"inheritanceClass,omitempty"`
}

// IsComplexType returns true if the variable has a complex type that can be expanded.
func (v *DebugVariable) IsComplexType() bool {
	switch v.MetaType {
	case DebugMetaTypeStructure, DebugMetaTypeTable, DebugMetaTypeDataRef,
		DebugMetaTypeObjectRef, DebugMetaTypeClass, DebugMetaTypeObject, DebugMetaTypeBoxRef:
		return true
	default:
		return false
	}
}

// DebugVariableHierarchy represents a parent-child relationship between variables.
type DebugVariableHierarchy struct {
	ParentID  string `json:"parentId"`
	ChildID   string `json:"childId"`
	ChildName string `json:"childName"`
}

// DebugChildVariablesInfo contains child variables and their hierarchy.
type DebugChildVariablesInfo struct {
	Hierarchies []DebugVariableHierarchy `json:"hierarchies"`
	Variables   []DebugVariable          `json:"variables"`
}

// --- Batch Types ---

// DebugBatchOperation represents a single operation in a batch request.
type DebugBatchOperation struct {
	Method      string // HTTP method (POST, GET)
	Path        string // Path with query params (e.g., "/sap/bc/adt/debugger?method=stepOver")
	ContentType string // Content-Type header (optional)
	Accept      string // Accept header
	Body        string // Request body (optional)
}

// DebugBatchResponse represents a single response from a batch request.
type DebugBatchResponse struct {
	StatusCode  int
	ContentType string
	Body        []byte
}
