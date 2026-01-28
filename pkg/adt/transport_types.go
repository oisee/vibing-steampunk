// ABOUTME: Type definitions for SAP transport management.
// ABOUTME: Includes transport requests, tasks, objects, and related options.

package adt

// TransportSummary represents a transport request summary for list operations.
type TransportSummary struct {
	Number      string          `json:"number"`
	Owner       string          `json:"owner"`
	Description string          `json:"description"`
	Type        string          `json:"type"`   // K=Workbench, W=Customizing, S=Task
	Status      string          `json:"status"` // D=Modifiable, R=Released
	StatusText  string          `json:"statusText,omitempty"`
	Target      string          `json:"target,omitempty"`
	TargetDesc  string          `json:"targetDesc,omitempty"`
	ChangedAt   string          `json:"changedAt,omitempty"`
	Client      string          `json:"client,omitempty"`
	Tasks       []TransportTask `json:"tasks,omitempty"`
}

// TransportDetails represents detailed transport information.
type TransportDetails struct {
	TransportSummary
	Objects []TransportObject `json:"objects,omitempty"`
}

// TransportTask represents a task within a transport request.
type TransportTask struct {
	Number      string            `json:"number"`
	Parent      string            `json:"parent,omitempty"`
	Owner       string            `json:"owner"`
	Description string            `json:"description"`
	Type        string            `json:"type,omitempty"`
	Status      string            `json:"status"`
	StatusText  string            `json:"statusText,omitempty"`
	Objects     []TransportObject `json:"objects,omitempty"`
}

// TransportObject represents an object in a transport.
type TransportObject struct {
	PgmID    string `json:"pgmid"` // R3TR, LIMU, CORR
	Type     string `json:"type"`  // PROG, CLAS, DEVC, etc.
	Name     string `json:"name"`
	WBType   string `json:"wbtype,omitempty"` // PROG/P, CLAS/OC, etc.
	Info     string `json:"info,omitempty"`   // "Program", "Class", etc.
	Position int    `json:"position,omitempty"`
}

// UserTransports represents transport requests grouped by type.
type UserTransports struct {
	Workbench   []TransportSummary `json:"workbench"`
	Customizing []TransportSummary `json:"customizing"`
}

// TransportInfo represents information about an object's transport status.
type TransportInfo struct {
	PgmID        string `json:"pgmid"`
	Object       string `json:"object"`
	ObjectName   string `json:"objectName"`
	Operation    string `json:"operation"`
	DevClass     string `json:"devClass"`
	Recording    string `json:"recording"`
	LockedByUser string `json:"lockedByUser,omitempty"`
	LockedInTask string `json:"lockedInTask,omitempty"`
}

// CreateTransportOptions for creating transport requests.
type CreateTransportOptions struct {
	Description    string
	Package        string
	TransportLayer string
	Type           string // "workbench" or "customizing"
}

// ReleaseAction specifies which release action to use.
type ReleaseAction string

const (
	ReleaseActionNormal      ReleaseAction = "newreleasejobs"
	ReleaseActionIgnoreLocks ReleaseAction = "relwithignlock"
	ReleaseActionSkipATC     ReleaseAction = "relObjigchkatc"
)

// ReleaseTransportOptions for releasing transports.
type ReleaseTransportOptions struct {
	// Action specifies the release action. If empty, defaults to ReleaseActionNormal.
	Action ReleaseAction
}

// GetAction returns the effective release action.
func (o ReleaseTransportOptions) GetAction() ReleaseAction {
	if o.Action != "" {
		return o.Action
	}
	return ReleaseActionNormal
}
