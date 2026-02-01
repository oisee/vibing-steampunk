package adt

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
)

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
	xmlStr := StripXMLNamespaceDeclarations(StripXMLNamespaces(string(data), "aunit:", "adtcore:"))

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
		URI           string  `xml:"uri,attr"`
		Type          string  `xml:"type,attr"`
		Name          string  `xml:"name,attr"`
		ExecutionTime float64 `xml:"executionTime,attr"`
		URIType       string  `xml:"uriType,attr"`
		NavigationURI string  `xml:"navigationUri,attr"`
		Unit          string  `xml:"unit,attr"`
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
		var converted []UnitTestAlert
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
				ua.Stack = append(ua.Stack, UnitTestStackEntry(s))
			}
			converted = append(converted, ua)
		}
		return converted
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
