//go:build integration

package adt

import (
	"context"
	"os"
	"testing"
)

// TestGetATCWorklistRaw fetches raw ATC XML to inspect child elements.
// Uses any available Z* class or skips if none found.
// Run with: go test -tags=integration -v -run TestGetATCWorklistRaw ./pkg/adt/
func TestGetATCWorklistRaw(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Search for any Z* class to run ATC on
	results, err := client.SearchObject(ctx, "ZCL_*", 5)
	if err != nil {
		t.Fatalf("SearchObject failed: %v", err)
	}
	if len(results) == 0 {
		t.Skip("No Z* classes found on system")
	}

	// Find first class object
	var classURI string
	for _, obj := range results {
		if obj.Type == "CLAS/OC" {
			classURI = obj.URI
			t.Logf("Using class %s for raw ATC XML test", obj.Name)
			break
		}
	}
	if classURI == "" {
		t.Skip("No Z* classes found")
	}

	// Run ATC check to get a worklist ID
	worklistID, err := client.GetATCCheckVariant(ctx, "")
	if err != nil {
		t.Fatalf("GetATCCheckVariant failed: %v", err)
	}

	// Create ATC run
	runResult, err := client.CreateATCRun(ctx, worklistID, classURI, 10)
	if err != nil {
		t.Fatalf("CreateATCRun failed: %v", err)
	}

	// Get raw XML
	rawXML, err := client.GetATCWorklistRaw(ctx, runResult.WorklistID)
	if err != nil {
		t.Fatalf("GetATCWorklistRaw failed: %v", err)
	}

	// Write to file for inspection
	err = os.WriteFile("/tmp/atc_worklist_raw.xml", []byte(rawXML), 0644)
	if err != nil {
		t.Logf("Warning: could not write to file: %v", err)
	}

	// Print to stdout
	t.Logf("Raw ATC Worklist XML:\n%s", rawXML)
}

// TestATCTagsParsing verifies that ATC finding tags are properly parsed.
// Uses any available Z* class or skips if none found.
// Run with: go test -tags=integration -v -run TestATCTagsParsing ./pkg/adt/
func TestATCTagsParsing(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Search for any Z* class to run ATC on
	results, err := client.SearchObject(ctx, "ZCL_*", 5)
	if err != nil {
		t.Fatalf("SearchObject failed: %v", err)
	}
	if len(results) == 0 {
		t.Skip("No Z* classes found on system")
	}

	// Try each class until we get findings with tags
	var worklist *ATCWorklist
	for _, obj := range results {
		if obj.Type != "CLAS/OC" {
			continue
		}
		worklist, err = client.RunATCCheck(ctx, obj.URI, "", 20)
		if err != nil {
			t.Logf("RunATCCheck on %s failed: %v, trying next...", obj.Name, err)
			continue
		}
		if len(worklist.Objects) > 0 {
			t.Logf("Using class %s for ATC tag parsing test", obj.Name)
			break
		}
	}

	if worklist == nil || len(worklist.Objects) == 0 {
		t.Skip("No ATC findings found on any Z* class")
	}

	// Count findings with tags
	tagsFound := 0
	for _, obj := range worklist.Objects {
		for _, finding := range obj.Findings {
			if len(finding.Tags) > 0 {
				tagsFound++
				t.Logf("Finding with tags: %s", finding.MessageTitle)
				t.Logf("  Line: %d, Priority: %d", finding.Line, finding.Priority)
				t.Logf("  Processor: %s, LastChangedBy: %s", finding.Processor, finding.LastChangedBy)
				for k, v := range finding.Tags {
					t.Logf("  Tag %s = %s", k, v)
				}

				// Verify common tag patterns for "not released API" findings
				if finding.CheckTitle == "Usage of Released APIs" {
					if finding.Tags[ATCTagRefObjName] == "" {
						t.Errorf("Expected REF_OBJ_NAME tag for API check finding")
					}
				}
			}
		}
	}

	if tagsFound == 0 {
		t.Skip("No findings with tags found (tags depend on ATC check variant configuration)")
	}
	t.Logf("Total findings with tags: %d", tagsFound)
}
