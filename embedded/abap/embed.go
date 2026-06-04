// Package embedded provides embedded ABAP source files for ZADT_VSP deployment.
package embedded

import _ "embed"

// ZADT_VSP WebSocket Handler Components
// These files are deployed to SAP systems to enable WebSocket-based operations.

//go:embed zif_vsp_service.intf.abap
var ZifVspService string

//go:embed zcl_vsp_utils.clas.abap
var ZclVspUtils string

//go:embed zadt_cl_tadir_move.clas.abap
var ZadtClTadirMove string

//go:embed zcl_vsp_rfc_service.clas.abap
var ZclVspRfcService string

//go:embed zcl_vsp_debug_service.clas.abap
var ZclVspDebugService string

//go:embed zcl_vsp_amdp_service.clas.abap
var ZclVspAmdpService string

//go:embed zcl_vsp_git_service.clas.abap
var ZclVspGitService string

//go:embed zcl_vsp_report_service.clas.abap
var ZclVspReportService string

//go:embed zcl_vsp_apc_handler.clas.abap
var ZclVspApcHandler string

// ObjectInfo describes an embedded ABAP object.
type ObjectInfo struct {
	Type        string // INTF or CLAS
	Name        string // Object name (e.g., ZIF_VSP_SERVICE)
	Source      string // Source code
	Description string // Human-readable description
	Optional    bool   // If true, can be skipped (e.g., Git service without abapGit)
}

// GetObjects returns all ZADT_VSP objects in deployment order.
func GetObjects() []ObjectInfo {
	return []ObjectInfo{
		{
			Type:        "INTF",
			Name:        "ZIF_VSP_SERVICE",
			Source:      ZifVspService,
			Description: "Service interface for WebSocket domain handlers",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_UTILS",
			Source:      ZclVspUtils,
			Description: "Shared utilities for JSON and parameter handling",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZADT_CL_TADIR_MOVE",
			Source:      ZadtClTadirMove,
			Description: "Helper class for moving objects between packages",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_RFC_SERVICE",
			Source:      ZclVspRfcService,
			Description: "RFC domain - function module execution",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_DEBUG_SERVICE",
			Source:      ZclVspDebugService,
			Description: "Debug domain - TPDAPI integration",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_AMDP_SERVICE",
			Source:      ZclVspAmdpService,
			Description: "AMDP domain - HANA debugging (experimental)",
			Optional:    true, // AMDP debugger APIs (if_amdp_dbg_*) absent on some releases
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_GIT_SERVICE",
			Source:      ZclVspGitService,
			Description: "Git domain - abapGit export (requires abapGit)",
			Optional:    true, // Requires abapGit on SAP system
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_REPORT_SERVICE",
			Source:      ZclVspReportService,
			Description: "Report domain - background job execution with spool output",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_APC_HANDLER",
			Source:      ZclVspApcHandler,
			Description: "Main APC WebSocket handler (router)",
			Optional:    false,
		},
	}
}

// PostDeploymentInstructions returns the manual steps needed after deployment.
func PostDeploymentInstructions() string {
	return `
═══════════════════════════════════════════════════════════════════════════════
  MANUAL STEPS REQUIRED - Complete in SAP GUI
═══════════════════════════════════════════════════════════════════════════════

1. CREATE APC APPLICATION (Transaction SAPC)
   ─────────────────────────────────────────
   a. Start transaction SAPC
   b. Click "Create" button
   c. Fill in the following:
      • Application ID:    ZADT_VSP
      • Description:       VSP WebSocket Handler
      • Handler Class:     ZCL_VSP_APC_HANDLER
      • Connection State:  Stateful
   d. Save and activate

2. ACTIVATE ICF SERVICE (Transaction SICF)
   ───────────────────────────────────────
   a. Start transaction SICF
   b. Execute with default settings
   c. Navigate to: /sap/bc/apc/sap/zadt_vsp
      (The node should exist after SAPC activation)
   d. Right-click the node → "Activate Service"
   e. Confirm activation in the popup

3. TEST CONNECTION
   ────────────────
   Using wscat (npm install -g wscat):

   wscat -c "ws://HOST:PORT/sap/bc/apc/sap/zadt_vsp?sap-client=CLIENT" \
         -H "Authorization: Basic $(echo -n USER:PASS | base64)"

   Expected response:
   {"id":"welcome","success":true,"data":{"session":"...","version":"2.2.0",
    "domains":["rfc","debug","amdp","git"]}}

4. VERIFY IN VSP
   ──────────────
   # Test WebSocket connection
   vsp ws-ping

   # Test RFC domain
   vsp call-rfc RFC_SYSTEM_INFO

   # Test Git domain (if abapGit installed)
   vsp git-types

═══════════════════════════════════════════════════════════════════════════════
  For detailed documentation, see: embedded/abap/README.md
═══════════════════════════════════════════════════════════════════════════════
`
}
