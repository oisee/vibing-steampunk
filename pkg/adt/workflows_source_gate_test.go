package adt

import (
	"context"
	"net/http"
	"strings"
	"testing"
)

// TestWriteSource_UpdateUnderAllowedPackages_DoesNotFailAtEntryGate is a
// regression test for a bug found live against a --allowed-packages server
// during the v2.58.0 reconciliation (2026-09-17): WriteSource's top-level
// gate ran the full checkMutation (including the package step) even on the
// update path, where opts.Package is empty by design — the object's URL
// isn't known yet at that point, so under an AllowedPackages policy every
// update failed immediately with "requires either ObjectURL or Package",
// before ever reaching the delegated per-type gate (which does have the
// object URL and enforces the policy correctly).
//
// This does not assert the write itself succeeds — the mock only covers the
// CSRF discovery and object-package lookup, so the flow legitimately fails
// further down (e.g. fetching class methods). It asserts specifically that
// the failure is not the premature entry-gate error.
func TestWriteSource_UpdateUnderAllowedPackages_DoesNotFailAtEntryGate(t *testing.T) {
	mock := &mockTransportClient{
		responses: map[string]*http.Response{
			"search":    newSearchResponse("/sap/bc/adt/oo/classes/zcl_test", "CLAS/OC", "ZCL_TEST", "$TMP"),
			"discovery": newTestResponse("OK"),
		},
	}
	cfg := NewConfig("https://sap.example.com:44300", "user", "pass", WithAllowedPackages("$TMP"))
	client := NewClientWithTransport(cfg, NewTransportWithClient(cfg, mock))

	_, err := client.WriteSource(context.Background(), "CLAS", "ZCL_TEST",
		"METHOD check_dates.\nENDMETHOD.",
		&WriteSourceOptions{Mode: WriteModeUpdate, Method: "CHECK_DATES"})

	if err != nil && strings.Contains(err.Error(), "requires either ObjectURL or Package") {
		t.Fatalf("WriteSource update failed at the premature entry gate instead of the delegated per-type check: %v", err)
	}
}
