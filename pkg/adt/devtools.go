package adt

import (
	"context"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"regexp"
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
// content is the source code to check
func (c *Client) SyntaxCheck(ctx context.Context, objectURL string, content string) ([]SyntaxCheckResult, error) {
	// Build the request body
	sourceURL := objectURL + "/source/main"
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
}

// Activate activates one or more ABAP objects.
// objectURL is the ADT URL of the object (e.g., "/sap/bc/adt/programs/programs/ZTEST")
// objectName is the technical name (e.g., "ZTEST")
func (c *Client) Activate(ctx context.Context, objectURL string, objectName string) (*ActivationResult, error) {
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
	ExecutionTime int             `json:"executionTime"` // in microseconds
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

	// Strip namespace prefixes for consistent parsing
	xmlStr := string(data)
	xmlStr = strings.ReplaceAll(xmlStr, "aunit:", "")
	xmlStr = strings.ReplaceAll(xmlStr, "adtcore:", "")

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
		ExecutionTime int    `xml:"executionTime,attr"`
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
	type response struct {
		RunResult runResult `xml:"runResult"`
	}

	var resp response
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

	for _, prog := range resp.RunResult.Programs {
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
