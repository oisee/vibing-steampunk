// Package embedded provides embedded ABAP source files for ZADT_VSP deployment.
//
// The .abap files this package embeds are copies of src/, the abapGit
// repository and the only place they are maintained. go:embed cannot reach a
// parent directory, so they are copied here by go generate, and
// TestEmbeddedSourcesMatchSrc fails when a copy has drifted from src/. Edit the
// class in src/, then run:
//
//	go generate ./embedded/abap
package embedded

import (
	_ "embed"
	"strings"
)

//go:generate go run sync_from_src.go

// ZADT_VSP WebSocket Handler Components
// These files are deployed to SAP systems to enable WebSocket-based operations.

//go:embed zif_vsp_service.intf.abap
var ZifVspService string

//go:embed zcl_vsp_utils.clas.abap
var ZclVspUtils string

//go:embed zcl_vsp_tadir_move.clas.abap
var ZclVspTadirMove string

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

//go:embed zcx_vsp_form.clas.abap
var ZcxVspForm string

//go:embed zcl_vsp_ssf_silent.clas.abap
var ZclVspSsfSilent string

//go:embed zcl_vsp_form_service.clas.abap
var ZclVspFormService string

//go:embed zcl_vsp_transport_service.clas.abap
var ZclVspTransportService string

//go:embed zvsp_transport_buffer.prog.abap
var ZvspTransportBuffer string

//go:embed zvsp_git_import.prog.abap
var ZvspGitImport string

//go:embed zcl_vsp_apc_handler.clas.abap
var ZclVspApcHandler string

// ObjectInfo describes an embedded ABAP object.
type ObjectInfo struct {
	Type        string // INTF, CLAS or PROG
	Name        string // Object name (e.g., ZIF_VSP_SERVICE)
	Source      string // Source code
	Description string // Human-readable description
	Optional    bool   // If true, can be skipped (e.g., Git service without abapGit)
	// RequiresAbapGit: the object names abapGit's classes (or one that does),
	// so it is deployed only where abapGit is installed.
	RequiresAbapGit bool
}

// FileName is the abapGit file name of an object, the same in src/ and here.
func FileName(o ObjectInfo) string {
	ext := ".clas.abap"
	switch o.Type {
	case "INTF":
		ext = ".intf.abap"
	case "PROG":
		ext = ".prog.abap"
	}
	return strings.ToLower(o.Name) + ext
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
			Name:        "ZCL_VSP_TADIR_MOVE",
			Source:      ZclVspTadirMove,
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
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_GIT_SERVICE",
			Source:      ZclVspGitService,
			Description: "Git domain - abapGit export and zip import (requires abapGit)",
			Optional:    true, // Requires abapGit on SAP system
			// The APC handler creates it dynamically, so ZADT_VSP runs without it.
			RequiresAbapGit: true,
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
			Name:        "ZCX_VSP_FORM",
			Source:      ZcxVspForm,
			Description: "Form domain - error with a code for the client",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_SSF_SILENT",
			Source:      ZclVspSsfSilent,
			Description: "Form domain - Smart Form API without dialogs",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_FORM_SERVICE",
			Source:      ZclVspFormService,
			Description: "Form domain - SAPscript, Smart Forms and Adobe forms",
			Optional:    false,
		},
		{
			Type:        "CLAS",
			Name:        "ZCL_VSP_TRANSPORT_SERVICE",
			Source:      ZclVspTransportService,
			Description: "Transport domain - upload K/R files, add to import buffer",
			Optional:    false,
		},
		{
			Type:        "PROG",
			Name:        "ZVSP_TRANSPORT_BUFFER",
			Source:      ZvspTransportBuffer,
			Description: "VSP transport buffer step (background)",
			Optional:    false,
		},
		{
			Type:            "PROG",
			Name:            "ZVSP_GIT_IMPORT",
			Source:          ZvspGitImport,
			Description:     "VSP abapGit zip import (background)",
			Optional:        true,
			RequiresAbapGit: true,
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
   {"id":"welcome","success":true,"data":{"session":"...","version":"2.4.0",
    "domains":["rfc","debug","amdp","git","report","form"]}}

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
