package ctxcomp

import (
	"context"
	"os"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func TestAnalyzerOffline(t *testing.T) {
	source := `CLASS zcl_demo DEFINITION PUBLIC.
  PUBLIC SECTION.
    DATA mo_log TYPE REF TO zif_logger.
    METHODS run IMPORTING io_helper TYPE REF TO zcl_helper.
ENDCLASS.
CLASS zcl_demo IMPLEMENTATION.
  METHOD run.
    DATA(lo) = NEW zcl_factory( ).
    zcl_util=>do_stuff( ).
    CALL FUNCTION 'Z_GET_DATA'.
    * comment: zcl_fake_comment=>nope
    WRITE 'zcl_fake_string=>nope'.
  ENDMETHOD.
ENDCLASS.`

	analyzer := NewAnalyzer(nil) // offline mode
	result := analyzer.Analyze(context.Background(), source, "ZCL_DEMO")

	t.Logf("Layers used: %v", result.Layers)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("True deps: %d, False positives: %d", result.TrueDeps, result.FalsePositives)
	t.Logf("")

	for _, dep := range result.Dependencies {
		status := "TRUE"
		if dep.InString || dep.InComment {
			status = "FALSE POSITIVE"
		}
		layers := ""
		for _, l := range dep.FoundBy {
			layers += l.String() + "+"
		}
		t.Logf("  %-25s conf=%.1f  %s  [%s]", dep.Name, dep.Confidence, status, layers)
	}

	// Verify false positives detected
	for _, dep := range result.Dependencies {
		if dep.Name == "ZCL_FAKE_STRING" || dep.Name == "ZCL_FAKE_COMMENT" {
			if !dep.InString && !dep.InComment {
				t.Errorf("%s should be marked as false positive", dep.Name)
			}
		}
	}

	// Verify real deps found
	realDeps := map[string]bool{"ZIF_LOGGER": false, "ZCL_HELPER": false, "ZCL_FACTORY": false, "ZCL_UTIL": false, "Z_GET_DATA": false}
	for _, dep := range result.Dependencies {
		if _, ok := realDeps[dep.Name]; ok {
			realDeps[dep.Name] = true
		}
	}
	for name, found := range realDeps {
		if !found {
			t.Errorf("Missing real dependency: %s", name)
		}
	}
}

func TestAnalyzerLive(t *testing.T) {
	url := os.Getenv("SAP_URL")
	user := os.Getenv("SAP_USER")
	pass := os.Getenv("SAP_PASSWORD")
	if url == "" {
		t.Skip("SAP_URL not set")
	}

	client := adt.NewClient(url, user, pass, adt.WithInsecureSkipVerify())
	ctx := context.Background()

	// Read a real class
	source, err := client.GetClassSource(ctx, "ZCL_ABAPGIT_AJSON")
	if err != nil {
		t.Fatalf("GetClassSource: %v", err)
	}

	analyzer := NewAnalyzer(nil) // offline layers only for now
	result := analyzer.Analyze(ctx, source, "ZCL_ABAPGIT_AJSON")

	t.Logf("=== ZCL_ABAPGIT_AJSON ===")
	t.Logf("Lines: %d", result.TotalLines)
	t.Logf("Layers: %v", result.Layers)
	t.Logf("Duration: %v", result.Duration)
	t.Logf("True deps: %d, False positives: %d, Total: %d", result.TrueDeps, result.FalsePositives, len(result.Dependencies))
	t.Logf("")

	for _, dep := range result.Dependencies {
		status := ""
		if dep.InString {
			status = " [FALSE POSITIVE]"
		}
		layers := ""
		for _, l := range dep.FoundBy {
			layers += l.String() + "+"
		}
		t.Logf("  %-30s conf=%.2f  %s  [%s]%s", dep.Name, dep.Confidence, dep.Kind, layers, status)
	}
}
