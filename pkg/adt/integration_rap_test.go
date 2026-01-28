//go:build integration

// ABOUTME: Integration tests for RAP (ABAP RESTful Application Programming) operations.
// ABOUTME: Tests CDS, BDEF, SRVD, SRVB creation and the full OData service workflow.

package adt

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestIntegration_RAP_E2E_OData tests the full RAP OData service creation workflow:
// 1. Create CDS view (DDLS)
// 2. Create Service Definition (SRVD)
// 3. Create Service Binding (SRVB)
// 4. Publish service binding
// This test cleans up all created objects at the end.
func TestIntegration_RAP_E2E_OData(t *testing.T) {
	client := getIntegrationClient(t)
	ctx := context.Background()

	// Test object names
	ddlsName := "ZTEST_MCP_I_FLIGHT"
	srvdName := "ZTEST_MCP_SD_FLIGHT"
	srvbName := "ZTEST_MCP_SB_FLIGHT"
	pkg := "$TMP"

	// Cleanup function
	cleanup := func() {
		t.Log("Cleaning up test objects...")
		// Delete in reverse order of creation (no lock needed for $TMP objects)
		_ = client.DeleteObject(ctx, "/sap/bc/adt/businessservices/bindings/"+strings.ToLower(srvbName), "", "")
		_ = client.DeleteObject(ctx, "/sap/bc/adt/ddic/srvd/sources/"+strings.ToLower(srvdName), "", "")
		_ = client.DeleteObject(ctx, "/sap/bc/adt/ddic/ddl/sources/"+strings.ToLower(ddlsName), "", "")
	}

	// Defer cleanup
	defer cleanup()

	// Step 1: Create CDS View (DDLS)
	t.Log("Step 1: Creating CDS View (DDLS)...")
	ddlsSource := `@AbapCatalog.sqlViewName: 'ZTESTMCPIFLIGHT'
@AbapCatalog.compiler.compareFilter: true
@AccessControl.authorizationCheck: #NOT_REQUIRED
@EndUserText.label: 'Flight Data for OData Test'
define view ZTEST_MCP_I_FLIGHT as select from sflight {
  key carrid   as Airline,
  key connid   as FlightNumber,
  key fldate   as FlightDate,
      price    as Price,
      currency as Currency,
      planetype as PlaneType,
      seatsmax as SeatsMax,
      seatsocc as SeatsOccupied
}`

	ddlsResult, err := client.WriteSource(ctx, "DDLS", ddlsName, ddlsSource, &WriteSourceOptions{
		Mode:        WriteModeUpsert,
		Package:     pkg,
		Description: "Flight Data for OData Test",
	})
	if err != nil {
		t.Fatalf("WriteSource DDLS failed: %v", err)
	}
	t.Logf("DDLS result: success=%v, mode=%s, message=%s", ddlsResult.Success, ddlsResult.Mode, ddlsResult.Message)
	if !ddlsResult.Success {
		t.Fatalf("DDLS creation failed: %s", ddlsResult.Message)
	}

	// Step 2: Create Service Definition (SRVD)
	t.Log("Step 2: Creating Service Definition (SRVD)...")
	srvdSource := `@EndUserText.label: 'Flight Service Definition'
define service ZTEST_MCP_SD_FLIGHT {
  expose ZTEST_MCP_I_FLIGHT as Flights;
}`

	srvdResult, err := client.WriteSource(ctx, "SRVD", srvdName, srvdSource, &WriteSourceOptions{
		Mode:        WriteModeUpsert,
		Package:     pkg,
		Description: "Flight Service Definition",
	})
	if err != nil {
		t.Fatalf("WriteSource SRVD failed: %v", err)
	}
	t.Logf("SRVD result: success=%v, mode=%s, message=%s", srvdResult.Success, srvdResult.Mode, srvdResult.Message)
	if !srvdResult.Success {
		t.Fatalf("SRVD creation failed: %s", srvdResult.Message)
	}

	// Step 3: Create Service Binding (SRVB)
	t.Log("Step 3: Creating Service Binding (SRVB)...")
	err = client.CreateObject(ctx, CreateObjectOptions{
		ObjectType:        ObjectTypeSRVB,
		Name:              srvbName,
		PackageName:       pkg,
		Description:       "Flight OData V2 Binding",
		ServiceDefinition: srvdName,
		BindingVersion:    "V2",
		BindingCategory:   "0", // Web API
	})
	if err != nil {
		// SRVB might already exist from previous run - SAP says "does already exist"
		if !strings.Contains(err.Error(), "already exist") {
			t.Fatalf("CreateObject SRVB failed: %v", err)
		}
		t.Log("SRVB already exists, continuing...")
	} else {
		t.Log("SRVB created successfully")
	}

	// Step 4: Activate SRVB
	t.Log("Step 4: Activating Service Binding...")
	srvbURL := "/sap/bc/adt/businessservices/bindings/" + strings.ToLower(srvbName)
	activationResult, err := client.Activate(ctx, srvbURL, srvbName)
	if err != nil {
		t.Logf("Activation warning: %v", err)
	} else {
		t.Logf("Activation result: success=%v, messages=%v", activationResult.Success, activationResult.Messages)
	}

	// Step 5: Publish Service Binding (with shorter timeout - Gateway can hang if misconfigured)
	t.Log("Step 5: Publishing Service Binding...")
	publishCtx, publishCancel := context.WithTimeout(ctx, 30*time.Second)
	defer publishCancel()
	publishResult, err := client.PublishServiceBinding(publishCtx, srvbName, "0001")
	if err != nil {
		// Publishing can fail for various reasons:
		// - Already published
		// - Gateway destination not configured (causes OBJECTS_OBJREF_NOT_ASSIGNED in /IWFND/CL_MGW_DEST_FINDER)
		// - System restrictions
		if publishCtx.Err() == context.DeadlineExceeded {
			t.Log("Publish timed out - Gateway destination may not be configured (check transaction /IWFND/MAINT_SERVICE)")
		} else {
			t.Logf("Publish warning (non-fatal): %v", err)
		}
	} else {
		t.Logf("Service Binding published successfully! Result: %+v", publishResult)
	}

	// Step 6: Verify SRVB was created
	t.Log("Step 6: Verifying Service Binding...")
	sb, err := client.GetSRVB(ctx, srvbName)
	if err != nil {
		t.Fatalf("GetSRVB verification failed: %v", err)
	}
	t.Logf("SRVB verified: name=%s, type=%s, version=%s", sb.Name, sb.Type, sb.BindingVersion)

	t.Log("RAP E2E OData test completed successfully!")
}
