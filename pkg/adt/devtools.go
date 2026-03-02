package adt

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// --- Syntax Check ---

// SyntaxCheckResult represents a single syntax check message.
type SyntaxCheckResult struct {
	URI      string `json:"uri"`
	Line     int    `json:"line"`
	Offset   int    `json:"offset"`
	Severity string `json:"severity"` // E=Error, W=Warning, I=Info
	Text     string `json:"text"`
}

// SyntaxCheck performs syntax check on ABAP source code.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// For class includes (e.g., "/sap/bc/adt/oo/classes/ZCL_FOO/includes/testclasses"),
// pass the include URL directly - no /source/main suffix will be added.
// content is the source code to check
func (c *Client) SyntaxCheck(ctx context.Context, objectURL string, content string) ([]SyntaxCheckResult, error) {
	// Build the request body
	// For class includes, the URL is used as-is (no /source/main suffix)
	sourceURL := objectURL
	if !strings.Contains(objectURL, "/includes/") {
		sourceURL = objectURL + "/source/main"
	}
	encodedContent := base64.StdEncoding.EncodeToString([]byte(content))

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<chkrun:checkObjectList xmlns:chkrun="http://www.sap.com/adt/checkrun" xmlns:adtcore="http://www.sap.com/adt/core">
  <chkrun:checkObject adtcore:uri="%s" chkrun:version="active">
    <chkrun:artifacts>
      <chkrun:artifact chkrun:contentType="text/plain; charset=utf-8" chkrun:uri="%s">
        <chkrun:content>%s</chkrun:content>
      </chkrun:artifact>
    </chkrun:artifacts>
  </chkrun:checkObject>
</chkrun:checkObjectList>`, sourceURL, sourceURL, encodedContent)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/checkruns?reporters=abapCheckRun", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("syntax check failed: %w", err)
	}

	return parseSyntaxCheckResults(resp.Body)
}

func parseSyntaxCheckResults(data []byte) ([]SyntaxCheckResult, error) {
	// The response uses namespace prefixes like chkrun:uri, chkrun:type, etc.
	// Go's xml package doesn't handle namespaced attributes well, so we strip the prefix
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "chkrun:", "")

	type checkMessage struct {
		URI       string `xml:"uri,attr"`
		Type      string `xml:"type,attr"`
		ShortText string `xml:"shortText,attr"`
	}
	type checkMessageList struct {
		Messages []checkMessage `xml:"checkMessage"`
	}
	type checkReport struct {
		MessageList checkMessageList `xml:"checkMessageList"`
	}
	type checkRunReports struct {
		Reports []checkReport `xml:"checkReport"`
	}

	var resp checkRunReports
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing syntax check response: %w", err)
	}

	var results []SyntaxCheckResult
	lineOffsetRegex := regexp.MustCompile(`([^#]+)#start=(\d+),(\d+)`)

	for _, report := range resp.Reports {
		for _, msg := range report.MessageList.Messages {
			result := SyntaxCheckResult{
				URI:      msg.URI,
				Severity: msg.Type,
				Text:     msg.ShortText,
			}

			// Parse line and offset from URI fragment
			if matches := lineOffsetRegex.FindStringSubmatch(msg.URI); matches != nil {
				result.URI = matches[1]
				result.Line, _ = strconv.Atoi(matches[2])
				result.Offset, _ = strconv.Atoi(matches[3])
			}

			results = append(results, result)
		}
	}

	return results, nil
}

// --- Activation ---

// ActivationResult represents the result of an activation.
type ActivationResult struct {
	Success  bool                       `json:"success"`
	Messages []ActivationResultMessage  `json:"messages"`
	Inactive []InactiveObject           `json:"inactive,omitempty"`
}

// ActivationResultMessage represents a message from activation.
type ActivationResultMessage struct {
	ObjDescr       string `json:"objDescr,omitempty"`
	Type           string `json:"type"` // E=Error, W=Warning, I=Info
	Line           int    `json:"line,omitempty"`
	Href           string `json:"href,omitempty"`
	ForceSupported bool   `json:"forceSupported,omitempty"`
	ShortText      string `json:"shortText"`
}

// InactiveObject represents an inactive object.
type InactiveObject struct {
	URI       string `json:"uri"`
	Type      string `json:"type"`
	Name      string `json:"name"`
	ParentURI string `json:"parentUri,omitempty"`
	User      string `json:"user,omitempty"`
	Deleted   bool   `json:"deleted,omitempty"`
}

// InactiveObjectRecord represents an inactive object with its transport info.
type InactiveObjectRecord struct {
	Object    *InactiveObject `json:"object,omitempty"`
	Transport *InactiveObject `json:"transport,omitempty"`
}

// Activate activates one or more ABAP objects.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// objectName is the technical name (e.g., "ZTEST")
func (c *Client) Activate(ctx context.Context, objectURL string, objectName string) (*ActivationResult, error) {
	// Safety check
	if err := c.checkSafety(OpActivate, "Activate"); err != nil {
		return nil, err
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<adtcore:objectReferences xmlns:adtcore="http://www.sap.com/adt/core">
  <adtcore:objectReference adtcore:uri="%s" adtcore:name="%s"/>
</adtcore:objectReferences>`, objectURL, objectName)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/activation?method=activate&preauditRequested=true", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("activation failed: %w", err)
	}

	return parseActivationResult(resp.Body)
}

func parseActivationResult(data []byte) (*ActivationResult, error) {
	result := &ActivationResult{
		Success:  true,
		Messages: []ActivationResultMessage{},
		Inactive: []InactiveObject{},
	}

	// If response is empty, activation was successful
	if len(data) == 0 {
		return result, nil
	}

	type msg struct {
		ObjDescr       string `xml:"objDescr,attr"`
		Type           string `xml:"type,attr"`
		Line           int    `xml:"line,attr"`
		Href           string `xml:"href,attr"`
		ForceSupported bool   `xml:"forceSupported,attr"`
		ShortText      struct {
			Text string `xml:"txt"`
		} `xml:"shortText"`
	}
	type messages struct {
		Msgs []msg `xml:"msg"`
	}
	type inactiveRef struct {
		URI       string `xml:"uri,attr"`
		Type      string `xml:"type,attr"`
		Name      string `xml:"name,attr"`
		ParentURI string `xml:"parentUri,attr"`
	}
	type inactiveEntry struct {
		Object *struct {
			Ref inactiveRef `xml:"ref"`
		} `xml:"object"`
	}
	type inactiveObjects struct {
		Entries []inactiveEntry `xml:"entry"`
	}
	type response struct {
		Messages messages        `xml:"messages"`
		Inactive inactiveObjects `xml:"inactiveObjects"`
	}

	var resp response
	if err := xml.Unmarshal(data, &resp); err != nil {
		// If parsing fails, try to extract any error message
		result.Success = false
		result.Messages = append(result.Messages, ActivationResultMessage{
			Type:      "E",
			ShortText: string(data),
		})
		return result, nil
	}

	for _, m := range resp.Messages.Msgs {
		result.Messages = append(result.Messages, ActivationResultMessage{
			ObjDescr:       m.ObjDescr,
			Type:           m.Type,
			Line:           m.Line,
			Href:           m.Href,
			ForceSupported: m.ForceSupported,
			ShortText:      m.ShortText.Text,
		})
		// Check for errors
		if strings.ContainsAny(m.Type, "EAX") {
			result.Success = false
		}
	}

	for _, entry := range resp.Inactive.Entries {
		if entry.Object != nil {
			result.Success = false
			result.Inactive = append(result.Inactive, InactiveObject{
				URI:       entry.Object.Ref.URI,
				Type:      entry.Object.Ref.Type,
				Name:      entry.Object.Ref.Name,
				ParentURI: entry.Object.Ref.ParentURI,
			})
		}
	}

	return result, nil
}

// GetInactiveObjects retrieves all inactive objects for the current user.
// Returns objects that have been modified but not yet activated.
func (c *Client) GetInactiveObjects(ctx context.Context) ([]InactiveObjectRecord, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/activation/inactiveobjects", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/vnd.sap.adt.inactivectsobjects.v1+xml, application/xml;q=0.8",
	})
	if err != nil {
		return nil, fmt.Errorf("get inactive objects failed: %w", err)
	}

	return parseInactiveObjects(resp.Body)
}

func parseInactiveObjects(data []byte) ([]InactiveObjectRecord, error) {
	if len(data) == 0 {
		return []InactiveObjectRecord{}, nil
	}

	// Strip namespace prefixes
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "ioc:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")

	type ref struct {
		URI       string `xml:"uri,attr"`
		Type      string `xml:"type,attr"`
		Name      string `xml:"name,attr"`
		ParentURI string `xml:"parentUri,attr"`
	}
	type objectElement struct {
		Deleted bool `xml:"deleted,attr"`
		User    string `xml:"user,attr"`
		Ref     ref    `xml:"ref"`
	}
	type entry struct {
		Object    *objectElement `xml:"object"`
		Transport *objectElement `xml:"transport"`
	}
	type inactiveObjects struct {
		Entries []entry `xml:"entry"`
	}

	var resp inactiveObjects
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing inactive objects: %w", err)
	}

	var results []InactiveObjectRecord
	for _, e := range resp.Entries {
		record := InactiveObjectRecord{}
		if e.Object != nil {
			record.Object = &InactiveObject{
				URI:       e.Object.Ref.URI,
				Type:      e.Object.Ref.Type,
				Name:      e.Object.Ref.Name,
				ParentURI: e.Object.Ref.ParentURI,
				User:      e.Object.User,
				Deleted:   e.Object.Deleted,
			}
		}
		if e.Transport != nil {
			record.Transport = &InactiveObject{
				URI:       e.Transport.Ref.URI,
				Type:      e.Transport.Ref.Type,
				Name:      e.Transport.Ref.Name,
				ParentURI: e.Transport.Ref.ParentURI,
				User:      e.Transport.User,
				Deleted:   e.Transport.Deleted,
			}
		}
		results = append(results, record)
	}

	return results, nil
}

// --- Batch Activation ---

// ActivatePackageResult represents the result of batch activation.
type ActivatePackageResult struct {
	Activated []ActivatedObject  `json:"activated"`
	Failed    []ActivationFailed `json:"failed"`
	Skipped   []string           `json:"skipped,omitempty"`
	Summary   string             `json:"summary"`
}

// ActivatedObject represents a successfully activated object.
type ActivatedObject struct {
	Name string `json:"name"`
	Type string `json:"type"`
	URI  string `json:"uri"`
}

// ActivationFailed represents a failed activation.
type ActivationFailed struct {
	Name   string `json:"name"`
	Type   string `json:"type"`
	Reason string `json:"reason"`
}

// objectTypePriority returns activation priority for object types.
// Lower number = activate first (interfaces before classes, etc.)
func objectTypePriority(objType string) int {
	priorities := map[string]int{
		"DOMA/DD": 1, // Domains first
		"DTEL/DE": 2, // Data elements
		"TABL/DT": 3, // Tables/structures
		"TTYP/TT": 4, // Table types
		"INTF/OI": 5, // Interfaces before classes
		"CLAS/OC": 6, // Classes
		"FUGR/F":  7, // Function groups
		"PROG/P":  8, // Programs
		"DDLS/DF": 9, // CDS views
	}
	if p, ok := priorities[objType]; ok {
		return p
	}
	return 50 // Unknown types last
}

// ActivatePackage activates all inactive objects in a package.
// If packageName is empty, activates ALL inactive objects for the current user.
// Objects are sorted by dependency order before activation.
func (c *Client) ActivatePackage(ctx context.Context, packageName string, maxObjects int) (*ActivatePackageResult, error) {
	// Safety check
	if err := c.checkSafety(OpActivate, "ActivatePackage"); err != nil {
		return nil, err
	}

	// Get all inactive objects
	inactive, err := c.GetInactiveObjects(ctx)
	if err != nil {
		return nil, fmt.Errorf("getting inactive objects: %w", err)
	}

	// Filter by package if specified
	var toActivate []InactiveObjectRecord
	packageName = strings.ToUpper(packageName)
	for _, rec := range inactive {
		if rec.Object == nil || rec.Object.Name == "" || rec.Object.URI == "" {
			continue
		}
		if packageName == "" || objectBelongsToPackage(rec.Object, packageName) {
			toActivate = append(toActivate, rec)
		}
	}

	// Limit number of objects
	if maxObjects > 0 && len(toActivate) > maxObjects {
		toActivate = toActivate[:maxObjects]
	}

	// Sort by dependency order (interfaces before classes, etc.)
	sort.Slice(toActivate, func(i, j int) bool {
		if toActivate[i].Object == nil || toActivate[j].Object == nil {
			return toActivate[i].Object != nil
		}
		return objectTypePriority(toActivate[i].Object.Type) < objectTypePriority(toActivate[j].Object.Type)
	})

	result := &ActivatePackageResult{
		Activated: []ActivatedObject{},
		Failed:    []ActivationFailed{},
	}

	// Activate each object
	for _, rec := range toActivate {
		if rec.Object == nil {
			continue
		}
		obj := rec.Object
		_, err := c.Activate(ctx, obj.URI, obj.Name)
		if err != nil {
			result.Failed = append(result.Failed, ActivationFailed{
				Name:   obj.Name,
				Type:   obj.Type,
				Reason: err.Error(),
			})
		} else {
			result.Activated = append(result.Activated, ActivatedObject{
				Name: obj.Name,
				Type: obj.Type,
				URI:  obj.URI,
			})
		}
	}

	result.Summary = fmt.Sprintf("Activated %d objects, %d failed", len(result.Activated), len(result.Failed))
	return result, nil
}

// objectBelongsToPackage checks if an inactive object belongs to the given package.
// Uses ParentURI which is populated from the inactive objects XML response.
func objectBelongsToPackage(obj *InactiveObject, packageName string) bool {
	if obj == nil || obj.ParentURI == "" {
		return false
	}
	expectedURI := "/sap/bc/adt/packages/" + strings.ToLower(packageName)
	return strings.EqualFold(obj.ParentURI, expectedURI)
}

// ActivatePackageIterativeResult represents the result of iterative batch activation.
type ActivatePackageIterativeResult struct {
	Iterations     int                     `json:"iterations"`
	TotalActivated int                     `json:"totalActivated"`
	TotalFailed    int                     `json:"totalFailed"`
	StillInactive  int                     `json:"stillInactive"`
	Passes         []ActivatePackageResult `json:"passes"`
	Summary        string                  `json:"summary"`
}

// ActivatePackageIterative activates all inactive objects in a package with multiple passes.
// Dependencies between objects (e.g., interfaces before classes) often require multiple
// activation passes. This method retries until no progress is made or maxIterations is reached,
// then performs a final diagnostic check.
func (c *Client) ActivatePackageIterative(ctx context.Context, packageName string, maxIterations int) (*ActivatePackageIterativeResult, error) {
	if maxIterations <= 0 {
		maxIterations = 5
	}

	result := &ActivatePackageIterativeResult{
		Passes: []ActivatePackageResult{},
	}

	for i := 0; i < maxIterations; i++ {
		passResult, err := c.ActivatePackage(ctx, packageName, 0)
		if err != nil {
			return nil, fmt.Errorf("activation pass %d failed: %w", i+1, err)
		}

		result.Passes = append(result.Passes, *passResult)
		result.TotalActivated += len(passResult.Activated)
		result.TotalFailed += len(passResult.Failed)
		result.Iterations = i + 1

		// No inactive objects found at all — nothing left to do
		if len(passResult.Activated) == 0 && len(passResult.Failed) == 0 {
			break
		}

		// Everything activated, nothing failed — done
		if len(passResult.Failed) == 0 {
			break
		}

		// No progress this pass — further retries won't help
		if len(passResult.Activated) == 0 && len(passResult.Failed) > 0 {
			break
		}
	}

	// Final diagnostic: check if any objects in this package are still inactive
	inactive, err := c.GetInactiveObjects(ctx)
	if err == nil {
		for _, rec := range inactive {
			if rec.Object == nil || rec.Object.Name == "" || rec.Object.URI == "" {
				continue
			}
			if packageName == "" || objectBelongsToPackage(rec.Object, packageName) {
				result.StillInactive++
			}
		}
	}

	// Build summary
	if result.StillInactive == 0 {
		result.Summary = fmt.Sprintf("%d activated across %d pass(es). All objects active — deployment verified.",
			result.TotalActivated, result.Iterations)
	} else {
		result.Summary = fmt.Sprintf("%d activated across %d pass(es). WARNING: %d object(s) still inactive in %s.",
			result.TotalActivated, result.Iterations, result.StillInactive, packageName)
	}

	return result, nil
}

// --- Unit Tests ---

// UnitTestRunFlags controls which tests to run.
type UnitTestRunFlags struct {
	Harmless  bool `json:"harmless"`  // Run harmless tests (risk level)
	Dangerous bool `json:"dangerous"` // Run dangerous tests
	Critical  bool `json:"critical"`  // Run critical tests
	Short     bool `json:"short"`     // Run short duration tests
	Medium    bool `json:"medium"`    // Run medium duration tests
	Long      bool `json:"long"`      // Run long duration tests
}

// DefaultUnitTestFlags returns the default test run configuration.
func DefaultUnitTestFlags() UnitTestRunFlags {
	return UnitTestRunFlags{
		Harmless:  true,
		Dangerous: false,
		Critical:  false,
		Short:     true,
		Medium:    true,
		Long:      false,
	}
}

// UnitTestResult represents the complete result of a unit test run.
type UnitTestResult struct {
	Classes []UnitTestClass `json:"classes"`
}

// UnitTestClass represents a test class result.
type UnitTestClass struct {
	URI              string           `json:"uri"`
	Type             string           `json:"type"`
	Name             string           `json:"name"`
	URIType          string           `json:"uriType,omitempty"`
	NavigationURI    string           `json:"navigationUri,omitempty"`
	DurationCategory string           `json:"durationCategory,omitempty"`
	RiskLevel        string           `json:"riskLevel,omitempty"`
	TestMethods      []UnitTestMethod `json:"testMethods"`
	Alerts           []UnitTestAlert  `json:"alerts,omitempty"`
}

// UnitTestMethod represents a test method result.
type UnitTestMethod struct {
	URI           string          `json:"uri"`
	Type          string          `json:"type"`
	Name          string          `json:"name"`
	ExecutionTime float64         `json:"executionTime"` // in seconds
	URIType       string          `json:"uriType,omitempty"`
	NavigationURI string          `json:"navigationUri,omitempty"`
	Unit          string          `json:"unit,omitempty"`
	Alerts        []UnitTestAlert `json:"alerts,omitempty"`
}

// UnitTestAlert represents a test alert (failure, exception, warning).
type UnitTestAlert struct {
	Kind     string               `json:"kind"`     // exception, failedAssertion, warning
	Severity string               `json:"severity"` // critical, fatal, tolerable, tolerant
	Title    string               `json:"title"`
	Details  []string             `json:"details,omitempty"`
	Stack    []UnitTestStackEntry `json:"stack,omitempty"`
}

// UnitTestStackEntry represents a stack trace entry.
type UnitTestStackEntry struct {
	URI         string `json:"uri"`
	Type        string `json:"type"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// RunUnitTests runs ABAP Unit tests for an object.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/oo/classes/ZCL_TEST")
func (c *Client) RunUnitTests(ctx context.Context, objectURL string, flags *UnitTestRunFlags) (*UnitTestResult, error) {
	if flags == nil {
		defaultFlags := DefaultUnitTestFlags()
		flags = &defaultFlags
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<aunit:runConfiguration xmlns:aunit="http://www.sap.com/adt/aunit">
  <external>
    <coverage active="false"/>
  </external>
  <options>
    <uriType value="semantic"/>
    <testDeterminationStrategy sameProgram="true" assignedTests="false"/>
    <testRiskLevels harmless="%t" dangerous="%t" critical="%t"/>
    <testDurations short="%t" medium="%t" long="%t"/>
    <withNavigationUri enabled="true"/>
  </options>
  <adtcore:objectSets xmlns:adtcore="http://www.sap.com/adt/core">
    <objectSet kind="inclusive">
      <adtcore:objectReferences>
        <adtcore:objectReference adtcore:uri="%s"/>
      </adtcore:objectReferences>
    </objectSet>
  </adtcore:objectSets>
</aunit:runConfiguration>`,
		flags.Harmless, flags.Dangerous, flags.Critical,
		flags.Short, flags.Medium, flags.Long,
		objectURL)

	resp, err := c.transport.Request(ctx, "/sap/bc/adt/abapunit/testruns", &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/*",
		Accept:      "application/*",
	})
	if err != nil {
		return nil, fmt.Errorf("running unit tests: %w", err)
	}

	return parseUnitTestResult(resp.Body)
}

func parseUnitTestResult(data []byte) (*UnitTestResult, error) {
	// Handle empty response (no test classes found)
	if len(data) == 0 {
		return &UnitTestResult{Classes: []UnitTestClass{}}, nil
	}

	// Strip namespace prefixes and declarations for consistent parsing
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "aunit:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")
	xmlStr = strings.ReplaceAll(xmlStr, ` xmlns:aunit="http://www.sap.com/adt/aunit"`, "")
	xmlStr = strings.ReplaceAll(xmlStr, ` xmlns:adtcore="http://www.sap.com/adt/core"`, "")

	type stackEntry struct {
		URI         string `xml:"uri,attr"`
		Type        string `xml:"type,attr"`
		Name        string `xml:"name,attr"`
		Description string `xml:"description,attr"`
	}
	type detail struct {
		Text string `xml:"text,attr"`
	}
	type alert struct {
		Kind     string `xml:"kind,attr"`
		Severity string `xml:"severity,attr"`
		Title    string `xml:"title"`
		Details  struct {
			Items []detail `xml:"detail"`
		} `xml:"details"`
		Stack struct {
			Entries []stackEntry `xml:"stackEntry"`
		} `xml:"stack"`
	}
	type testMethod struct {
		URI           string `xml:"uri,attr"`
		Type          string `xml:"type,attr"`
		Name          string `xml:"name,attr"`
		ExecutionTime float64 `xml:"executionTime,attr"`
		URIType       string `xml:"uriType,attr"`
		NavigationURI string `xml:"navigationUri,attr"`
		Unit          string `xml:"unit,attr"`
		Alerts        struct {
			Items []alert `xml:"alert"`
		} `xml:"alerts"`
	}
	type testClass struct {
		URI              string `xml:"uri,attr"`
		Type             string `xml:"type,attr"`
		Name             string `xml:"name,attr"`
		URIType          string `xml:"uriType,attr"`
		NavigationURI    string `xml:"navigationUri,attr"`
		DurationCategory string `xml:"durationCategory,attr"`
		RiskLevel        string `xml:"riskLevel,attr"`
		TestMethods      struct {
			Items []testMethod `xml:"testMethod"`
		} `xml:"testMethods"`
		Alerts struct {
			Items []alert `xml:"alert"`
		} `xml:"alerts"`
	}
	type program struct {
		TestClasses struct {
			Items []testClass `xml:"testClass"`
		} `xml:"testClasses"`
	}
	type runResult struct {
		Programs []program `xml:"program"`
	}

	var resp runResult
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing unit test results: %w", err)
	}

	result := &UnitTestResult{
		Classes: []UnitTestClass{},
	}

	// Helper to convert alerts
	convertAlerts := func(alerts []alert) []UnitTestAlert {
		var result []UnitTestAlert
		for _, a := range alerts {
			ua := UnitTestAlert{
				Kind:     a.Kind,
				Severity: a.Severity,
				Title:    a.Title,
				Details:  []string{},
				Stack:    []UnitTestStackEntry{},
			}
			for _, d := range a.Details.Items {
				if d.Text != "" {
					ua.Details = append(ua.Details, d.Text)
				}
			}
			for _, s := range a.Stack.Entries {
				ua.Stack = append(ua.Stack, UnitTestStackEntry{
					URI:         s.URI,
					Type:        s.Type,
					Name:        s.Name,
					Description: s.Description,
				})
			}
			result = append(result, ua)
		}
		return result
	}

	for _, prog := range resp.Programs {
		for _, tc := range prog.TestClasses.Items {
			class := UnitTestClass{
				URI:              tc.URI,
				Type:             tc.Type,
				Name:             tc.Name,
				URIType:          tc.URIType,
				NavigationURI:    tc.NavigationURI,
				DurationCategory: tc.DurationCategory,
				RiskLevel:        tc.RiskLevel,
				TestMethods:      []UnitTestMethod{},
				Alerts:           convertAlerts(tc.Alerts.Items),
			}

			for _, tm := range tc.TestMethods.Items {
				method := UnitTestMethod{
					URI:           tm.URI,
					Type:          tm.Type,
					Name:          tm.Name,
					ExecutionTime: tm.ExecutionTime,
					URIType:       tm.URIType,
					NavigationURI: tm.NavigationURI,
					Unit:          tm.Unit,
					Alerts:        convertAlerts(tm.Alerts.Items),
				}
				class.TestMethods = append(class.TestMethods, method)
			}

			result.Classes = append(result.Classes, class)
		}
	}

	return result, nil
}

// --- ATC (ABAP Test Cockpit) ---

// ATCCustomizing represents the ATC system configuration.
type ATCCustomizing struct {
	Properties []ATCProperty  `json:"properties"`
	Exemptions []ATCExemption `json:"exemptions"`
}

// ATCProperty represents an ATC configuration property.
type ATCProperty struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ATCExemption represents an exemption reason.
type ATCExemption struct {
	ID                     string `json:"id"`
	Title                  string `json:"title"`
	JustificationMandatory bool   `json:"justificationMandatory"`
}

// ATCRunResult represents the result of starting an ATC run.
type ATCRunResult struct {
	WorklistID string       `json:"worklistId"`
	Timestamp  int64        `json:"timestamp"`
	Infos      []ATCRunInfo `json:"infos,omitempty"`
}

// ATCRunInfo represents an info message from ATC run.
type ATCRunInfo struct {
	Type        string `json:"type"`
	Description string `json:"description"`
}

// ATCWorklist represents the ATC findings worklist.
type ATCWorklist struct {
	ID                  string         `json:"id"`
	Timestamp           int64          `json:"timestamp"`
	UsedObjectSet       string         `json:"usedObjectSet"`
	ObjectSetIsComplete bool           `json:"objectSetIsComplete"`
	ObjectSets          []ATCObjectSet `json:"objectSets"`
	Objects             []ATCObject    `json:"objects"`
}

// ATCObjectSet represents an object set in the worklist.
type ATCObjectSet struct {
	Name  string `json:"name"`
	Title string `json:"title"`
	Kind  string `json:"kind"`
}

// ATCObject represents an object with ATC findings.
type ATCObject struct {
	URI         string       `json:"uri"`
	Type        string       `json:"type"`
	Name        string       `json:"name"`
	PackageName string       `json:"packageName"`
	Author      string       `json:"author"`
	Findings    []ATCFinding `json:"findings"`
}

// ATCFinding represents a single ATC finding.
type ATCFinding struct {
	URI               string `json:"uri"`
	Location          string `json:"location"`
	Priority          int    `json:"priority"` // 1=Error, 2=Warning, 3=Info
	CheckID           string `json:"checkId"`
	CheckTitle        string `json:"checkTitle"`
	MessageID         string `json:"messageId"`
	MessageTitle      string `json:"messageTitle"`
	ExemptionApproval string `json:"exemptionApproval,omitempty"`
	ExemptionKind     string `json:"exemptionKind,omitempty"`
	QuickfixInfo      string `json:"quickfixInfo,omitempty"`
	Line              int    `json:"line,omitempty"`
	Column            int    `json:"column,omitempty"`
}

// GetATCCustomizing retrieves the ATC system configuration.
func (c *Client) GetATCCustomizing(ctx context.Context) (*ATCCustomizing, error) {
	resp, err := c.transport.Request(ctx, "/sap/bc/adt/atc/customizing", &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/xml, application/vnd.sap.atc.customizing-v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting ATC customizing: %w", err)
	}

	return parseATCCustomizing(resp.Body)
}

func parseATCCustomizing(data []byte) (*ATCCustomizing, error) {
	xmlStr := string(data)
	// Strip namespace prefix for easier parsing
	xmlStr = strings.ReplaceAll(xmlStr, "atccust:", "")

	type property struct {
		Name  string `xml:"name,attr"`
		Value string `xml:"value,attr"`
	}
	type reason struct {
		ID                     string `xml:"id,attr"`
		Title                  string `xml:"title,attr"`
		JustificationMandatory string `xml:"justificationMandatory,attr"`
	}
	type customizing struct {
		Properties struct {
			Items []property `xml:"property"`
		} `xml:"properties"`
		Exemption struct {
			Reasons struct {
				Items []reason `xml:"reason"`
			} `xml:"reasons"`
		} `xml:"exemption"`
	}

	var resp customizing
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing ATC customizing: %w", err)
	}

	result := &ATCCustomizing{
		Properties: []ATCProperty{},
		Exemptions: []ATCExemption{},
	}

	for _, p := range resp.Properties.Items {
		result.Properties = append(result.Properties, ATCProperty{
			Name:  p.Name,
			Value: p.Value,
		})
	}

	for _, r := range resp.Exemption.Reasons.Items {
		result.Exemptions = append(result.Exemptions, ATCExemption{
			ID:                     r.ID,
			Title:                  r.Title,
			JustificationMandatory: r.JustificationMandatory == "true",
		})
	}

	return result, nil
}

// GetATCCheckVariant retrieves the worklist ID for a check variant.
// If variant is empty, uses the system default check variant.
func (c *Client) GetATCCheckVariant(ctx context.Context, variant string) (string, error) {
	if variant == "" {
		// Get the default system check variant from customizing
		cust, err := c.GetATCCustomizing(ctx)
		if err != nil {
			return "", fmt.Errorf("getting ATC customizing for variant: %w", err)
		}
		for _, p := range cust.Properties {
			if p.Name == "systemCheckVariant" {
				variant = p.Value
				break
			}
		}
		if variant == "" {
			return "", fmt.Errorf("no system check variant found in ATC customizing")
		}
	}

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/atc/worklists?checkVariant=%s", variant), &RequestOptions{
		Method: http.MethodPost,
		Accept: "text/plain",
	})
	if err != nil {
		return "", fmt.Errorf("getting ATC check variant: %w", err)
	}

	return strings.TrimSpace(string(resp.Body)), nil
}

// CreateATCRun starts an ATC check run on an object.
// worklistID is from GetATCCheckVariant, objectURL is the ADT URL of the object.
// maxResults limits the number of findings returned (default 100).
func (c *Client) CreateATCRun(ctx context.Context, worklistID string, objectURL string, maxResults int) (*ATCRunResult, error) {
	if maxResults <= 0 {
		maxResults = 100
	}

	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<atc:run maximumVerdicts="%d" xmlns:atc="http://www.sap.com/adt/atc">
	<objectSets xmlns:adtcore="http://www.sap.com/adt/core">
		<objectSet kind="inclusive">
			<adtcore:objectReferences>
				<adtcore:objectReference adtcore:uri="%s"/>
			</adtcore:objectReferences>
		</objectSet>
	</objectSets>
</atc:run>`, maxResults, objectURL)

	resp, err := c.transport.Request(ctx, fmt.Sprintf("/sap/bc/adt/atc/runs?worklistId=%s", worklistID), &RequestOptions{
		Method:      http.MethodPost,
		Body:        []byte(body),
		ContentType: "application/xml",
		Accept:      "application/xml",
	})
	if err != nil {
		return nil, fmt.Errorf("creating ATC run: %w", err)
	}

	return parseATCRunResult(resp.Body)
}

func parseATCRunResult(data []byte) (*ATCRunResult, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "atcworklist:", "")

	type info struct {
		Type        string `xml:"type,attr"`
		Description string `xml:"description,attr"`
	}
	type worklistRun struct {
		WorklistID        string `xml:"worklistId"`
		WorklistTimestamp string `xml:"worklistTimestamp"`
		Infos             struct {
			Items []info `xml:"info"`
		} `xml:"infos"`
	}

	var resp worklistRun
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing ATC run result: %w", err)
	}

	result := &ATCRunResult{
		WorklistID: resp.WorklistID,
		Infos:      []ATCRunInfo{},
	}

	for _, i := range resp.Infos.Items {
		result.Infos = append(result.Infos, ATCRunInfo{
			Type:        i.Type,
			Description: i.Description,
		})
	}

	return result, nil
}

// GetATCWorklist retrieves the ATC findings worklist.
// worklistID is from CreateATCRun.
// includeExempted controls whether to include exempted findings.
func (c *Client) GetATCWorklist(ctx context.Context, worklistID string, includeExempted bool) (*ATCWorklist, error) {
	url := fmt.Sprintf("/sap/bc/adt/atc/worklists/%s?includeExemptedFindings=%t", worklistID, includeExempted)

	resp, err := c.transport.Request(ctx, url, &RequestOptions{
		Method: http.MethodGet,
		Accept: "application/atc.worklist.v1+xml",
	})
	if err != nil {
		return nil, fmt.Errorf("getting ATC worklist: %w", err)
	}

	return parseATCWorklist(resp.Body)
}

func parseATCWorklist(data []byte) (*ATCWorklist, error) {
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "atcworklist:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "atcobject:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "atcfinding:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")

	type link struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Type string `xml:"type,attr"`
	}
	type finding struct {
		URI               string `xml:"uri,attr"`
		Location          string `xml:"location,attr"`
		Priority          int    `xml:"priority,attr"`
		CheckID           string `xml:"checkId,attr"`
		CheckTitle        string `xml:"checkTitle,attr"`
		MessageID         string `xml:"messageId,attr"`
		MessageTitle      string `xml:"messageTitle,attr"`
		ExemptionApproval string `xml:"exemptionApproval,attr"`
		ExemptionKind     string `xml:"exemptionKind,attr"`
		QuickfixInfo      string `xml:"quickfixInfo,attr"`
		Link              link   `xml:"link"`
	}
	type object struct {
		URI         string `xml:"uri,attr"`
		Type        string `xml:"type,attr"`
		Name        string `xml:"name,attr"`
		PackageName string `xml:"packageName,attr"`
		Author      string `xml:"author,attr"`
		Findings    struct {
			Items []finding `xml:"finding"`
		} `xml:"findings"`
	}
	type objectSet struct {
		Name  string `xml:"name,attr"`
		Title string `xml:"title,attr"`
		Kind  string `xml:"kind,attr"`
	}
	type worklist struct {
		ID                  string `xml:"id,attr"`
		Timestamp           string `xml:"timestamp,attr"`
		UsedObjectSet       string `xml:"usedObjectSet,attr"`
		ObjectSetIsComplete string `xml:"objectSetIsComplete,attr"`
		ObjectSets          struct {
			Items []objectSet `xml:"objectSet"`
		} `xml:"objectSets"`
		Objects struct {
			Items []object `xml:"object"`
		} `xml:"objects"`
	}

	var resp worklist
	if err := xml.Unmarshal([]byte(xmlStr), &resp); err != nil {
		return nil, fmt.Errorf("parsing ATC worklist: %w", err)
	}

	result := &ATCWorklist{
		ID:                  resp.ID,
		UsedObjectSet:       resp.UsedObjectSet,
		ObjectSetIsComplete: resp.ObjectSetIsComplete == "true",
		ObjectSets:          []ATCObjectSet{},
		Objects:             []ATCObject{},
	}

	for _, os := range resp.ObjectSets.Items {
		result.ObjectSets = append(result.ObjectSets, ATCObjectSet{
			Name:  os.Name,
			Title: os.Title,
			Kind:  os.Kind,
		})
	}

	// Parse location to extract line/column
	locationRegex := regexp.MustCompile(`#start=(\d+),(\d+)`)

	for _, o := range resp.Objects.Items {
		obj := ATCObject{
			URI:         o.URI,
			Type:        o.Type,
			Name:        o.Name,
			PackageName: o.PackageName,
			Author:      o.Author,
			Findings:    []ATCFinding{},
		}

		for _, f := range o.Findings.Items {
			finding := ATCFinding{
				URI:               f.URI,
				Location:          f.Location,
				Priority:          f.Priority,
				CheckID:           f.CheckID,
				CheckTitle:        f.CheckTitle,
				MessageID:         f.MessageID,
				MessageTitle:      f.MessageTitle,
				ExemptionApproval: f.ExemptionApproval,
				ExemptionKind:     f.ExemptionKind,
				QuickfixInfo:      f.QuickfixInfo,
			}

			// Extract line and column from location
			if matches := locationRegex.FindStringSubmatch(f.Location); matches != nil {
				finding.Line, _ = strconv.Atoi(matches[1])
				finding.Column, _ = strconv.Atoi(matches[2])
			}

			obj.Findings = append(obj.Findings, finding)
		}

		result.Objects = append(result.Objects, obj)
	}

	return result, nil
}

// RunATCCheck is a convenience method that runs ATC check on an object and returns findings.
// It combines GetATCCheckVariant, CreateATCRun, and GetATCWorklist into a single call.
// variant can be empty to use the system default.
func (c *Client) RunATCCheck(ctx context.Context, objectURL string, variant string, maxResults int) (*ATCWorklist, error) {
	// Get worklist ID for the variant
	worklistID, err := c.GetATCCheckVariant(ctx, variant)
	if err != nil {
		return nil, fmt.Errorf("getting check variant: %w", err)
	}

	// Create the ATC run
	runResult, err := c.CreateATCRun(ctx, worklistID, objectURL, maxResults)
	if err != nil {
		return nil, fmt.Errorf("creating ATC run: %w", err)
	}

	// Get the worklist with findings
	worklist, err := c.GetATCWorklist(ctx, runResult.WorklistID, false)
	if err != nil {
		return nil, fmt.Errorf("getting ATC worklist: %w", err)
	}

	return worklist, nil
}
