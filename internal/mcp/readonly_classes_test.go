package mcp

// The classification data of TestReadOnlyInvariant (readonly_invariant_test.go
// holds the harness). This is the file to open when a tool or an action is
// added or changed.
//
// Classifying a new tool or SAP() action:
//
//  1. Add "tool <Name>" (or "SAP <action> ...", the actionCase's Name) to
//     readOnlyClasses as clsRead, clsMutate or clsExecute. MUTATE and EXECUTE
//     must be refused under --read-only, naming read-only or the safety
//     configuration, and must send no write and dial neither ZADT_VSP nor RFC.
//     Use knownGap(kind, why) only for a write --read-only does not refuse yet.
//  2. A new case in a switch-routed SAP() router also needs an actionCase in
//     actionCases, or the test fails naming the uncovered router literal.
//  3. If a tool's arguments cannot be synthesised from their names, so that
//     the call fails validation before it reaches the gate, add them to
//     toolArgOverrides.
//  4. An object-scoped mutation also belongs in packageGated, which checks that
//     --allowed-packages "Z*" refuses it for an object in $TMP.
//
// Run: go test ./internal/mcp -run TestReadOnlyInvariant -v
// (VSP_READONLY_TRACE=1 logs every call and what it sent.)

// --- classification ---------------------------------------------------------

type surfaceKind string

const (
	kindRead    surfaceKind = "READ"
	kindMutate  surfaceKind = "MUTATE"
	kindExecute surfaceKind = "EXECUTE"
)

// surfaceClass says what a tool or action does to the system. KnownGap, when
// set, names a write or execution --read-only does not refuse yet, and why it
// is recorded instead of fixed; such an entry is reported, not failed.
type surfaceClass struct {
	Kind     surfaceKind
	KnownGap string
}

var (
	clsRead    = surfaceClass{Kind: kindRead}
	clsMutate  = surfaceClass{Kind: kindMutate}
	clsExecute = surfaceClass{Kind: kindExecute}
)

func knownGap(k surfaceKind, why string) surfaceClass { return surfaceClass{Kind: k, KnownGap: why} }

// readOnlyClasses classifies every tool ("tool X") and every SAP() action
// ("SAP <action> ..."). A name the test reaches that is not here fails with
// "classify me": a new tool or route has to be placed in one of the three
// kinds before it ships.
var readOnlyClasses = map[string]surfaceClass{
	// --- tools: reads ---
	"tool AMDPGetBreakpoints":       clsRead,
	"tool AMDPGetVariables":         clsRead,
	"tool AMDPDebuggerStop":         clsRead, // ends this server's own AMDP session
	"tool AnalyzeABAPCode":          clsRead,
	"tool AnalyzeCallGraph":         clsRead,
	"tool CheckBoundaries":          clsRead,
	"tool CodeCompletion":           clsRead,
	"tool CompareCallGraphs":        clsRead,
	"tool CompareLanguages":         clsRead,
	"tool CompareSource":            clsRead,
	"tool CompareVersions":          clsRead,
	"tool DebuggerAttach":           clsRead, // takes a stopped debuggee of this user; changes nothing in it
	"tool DebuggerDetach":           clsRead,
	"tool DebuggerGetStack":         clsRead,
	"tool DebuggerGetVariables":     clsRead,
	"tool DebuggerListen":           clsRead,
	"tool ExportToFile":             clsRead, // writes a local file, not SAP
	"tool FindDefinition":           clsRead,
	"tool FindReferences":           clsRead,
	"tool GetAPIReleaseState":       clsRead,
	"tool GetATCCustomizing":        clsRead,
	"tool GetAbapHelp":              clsRead,
	"tool GetAsyncResult":           clsRead,
	"tool GetBreakpoints":           clsRead,
	"tool GetCDSDependencies":       clsRead,
	"tool GetCDSElementInfo":        clsRead,
	"tool GetCDSImpactAnalysis":     clsRead,
	"tool GetCallGraph":             clsRead,
	"tool GetCalleesOf":             clsRead,
	"tool GetCallersOf":             clsRead,
	"tool GetCheckRunResults":       clsRead,
	"tool GetClass":                 clsRead,
	"tool GetClassComponents":       clsRead,
	"tool GetClassInclude":          clsRead,
	"tool GetClassInfo":             clsRead,
	"tool GetCodeCoverage":          clsRead, // harmless ABAP Unit only; dangerous runs are refused (#283)
	"tool GetConnectionInfo":        clsRead,
	"tool GetContext":               clsRead,
	"tool GetDataElementLabels":     clsRead,
	"tool GetDump":                  clsRead,
	"tool GetFeatures":              clsRead,
	"tool GetFunction":              clsRead,
	"tool GetFunctionGroup":         clsRead,
	"tool GetInactiveObjects":       clsRead,
	"tool GetInclude":               clsRead,
	"tool GetInstalledComponents":   clsRead,
	"tool GetInterface":             clsRead,
	"tool GetMessageClassTexts":     clsRead,
	"tool GetMessages":              clsRead,
	"tool GetObjectStructure":       clsRead,
	"tool GetObjectTextsInLanguage": clsRead,
	"tool GetPackage":               clsRead,
	"tool GetPrettyPrinterSettings": clsRead,
	"tool GetProgram":               clsRead,
	"tool GetRevisionSource":        clsRead,
	"tool GetRevisions":             clsRead,
	"tool GetSQLTraceState":         clsRead,
	"tool GetSource":                clsRead,
	"tool GetStructure":             clsRead,
	"tool GetSystemInfo":            clsRead,
	"tool GetTable":                 clsRead,
	"tool GetTableContents":         clsRead,
	"tool GetTextElements":          clsRead,
	"tool GetTextPool":              clsRead,
	"tool GetTrace":                 clsRead,
	"tool GetTransaction":           clsRead,
	"tool GetTransport":             clsRead,
	"tool GetTransportInfo":         clsRead,
	"tool GetTypeHierarchy":         clsRead,
	"tool GetTypeInfo":              clsRead,
	"tool GetUserTransports":        clsRead,
	"tool GetVariants":              clsRead,
	"tool GitExport":                clsRead,
	"tool GitTypes":                 clsRead,
	"tool GraphStats":               clsRead,
	"tool GrepObject":               clsRead,
	"tool GrepObjects":              clsRead,
	"tool GrepPackage":              clsRead,
	"tool GrepPackages":             clsRead,
	"tool ListDependencies":         clsRead,
	"tool ListDumps":                clsRead,
	"tool ListSQLTraces":            clsRead,
	"tool ListTraces":               clsRead,
	"tool ListTransports":           clsRead,
	"tool PrettyPrint":              clsRead, // formats the source it is given; stores nothing
	"tool RunATCCheck":              clsRead,
	"tool RunQuery":                 clsRead, // free SQL is a read; --block-free-sql governs it, not --read-only
	"tool RunUnitTests":             clsRead, // harmless ABAP Unit only; dangerous runs are refused (#283)
	"tool SAP":                      clsRead, // the dispatcher; its actions are classified below
	"tool SaveToFile":               clsRead, // writes a local file, not SAP
	"tool SearchObject":             clsRead,
	"tool SyntaxCheck":              clsRead,
	"tool TraceExecution":           clsRead,
	"tool UI5GetApp":                clsRead,
	"tool UI5GetFileContent":        clsRead,
	"tool UI5ListApps":              clsRead,
	"tool UnlockObject":             clsRead, // releases a lock; READ locks stay allowed under --read-only

	// --- tools: writes ---
	"tool Activate":                 clsMutate,
	"tool ActivateMultiple":         clsMutate,
	"tool ActivatePackage":          clsMutate,
	"tool AssignIAMAppToCatalog":    clsMutate,
	"tool CloneObject":              clsMutate,
	"tool CreateAndActivateProgram": clsMutate,
	"tool CreateBusinessCatalog":    clsMutate,
	"tool CreateClassWithTests":     clsMutate,
	"tool CreateIAMApp":             clsMutate,
	"tool CreateObject":             clsMutate,
	"tool CreatePackage":            clsMutate,
	"tool CreateTable":              clsMutate,
	"tool CreateTestInclude":        clsMutate,
	"tool CreateTransport":          clsMutate,
	"tool DeleteObject":             clsMutate,
	"tool DeleteTransport":          clsMutate,
	"tool DeployFromFile":           clsMutate,
	"tool DeployZip":                clsMutate,
	"tool EditSource":               clsMutate,
	"tool ImportFromFile":           clsMutate,
	"tool InstallZADTVSP":           clsMutate,
	"tool LockObject":               clsMutate, // a MODIFY lock; READ locks are allowed
	"tool MoveObject":               clsMutate,
	"tool PublishServiceBinding":    clsMutate,
	"tool RecoverFailedCreate":      clsMutate,
	"tool ReleaseTransport":         clsMutate,
	"tool RenameObject":             clsMutate,
	"tool SetPrettyPrinterSettings": clsMutate,
	"tool SetTextElements":          clsMutate,
	"tool UI5CreateApp":             clsMutate,
	"tool UI5DeleteApp":             clsMutate,
	"tool UI5DeleteFile":            clsMutate,
	"tool UI5UploadFile":            clsMutate,
	"tool UnpublishServiceBinding":  clsMutate,
	"tool UpdateClassInclude":       clsMutate,
	"tool UpdateSource":             clsMutate,
	"tool WriteClass":               clsMutate,
	"tool WriteMessageClassTexts":   clsMutate,
	"tool WriteProgram":             clsMutate,
	"tool WriteSource":              clsMutate,
	"tool SetBreakpoint":            knownGap(kindMutate, gapBreakpoints),
	"tool DeleteBreakpoint":         knownGap(kindMutate, gapBreakpoints),
	"tool AMDPSetBreakpoint":        knownGap(kindMutate, gapAMDP),

	// --- tools: execution ---
	"tool CallRFC":            clsExecute,
	"tool ExecuteABAP":        clsExecute,
	"tool RunReport":          clsExecute,
	"tool RunReportAsync":     clsExecute,
	"tool DebuggerStep":       knownGap(kindExecute, gapStepping),
	"tool AMDPDebuggerStart":  knownGap(kindExecute, gapAMDP),
	"tool AMDPDebuggerResume": knownGap(kindExecute, gapAMDP),
	"tool AMDPDebuggerStep":   knownGap(kindExecute, gapAMDP),

	// --- SAP(): entry, help ---
	"SAP info":        clsRead,
	"SAP (no action)": clsRead,
	"SAP help":        clsRead,

	// --- SAP(): clsRead, query, search, grep ---
	"SAP read PROG": clsRead, "SAP read CLAS": clsRead, "SAP read INTF": clsRead, "SAP read FUNC": clsRead,
	"SAP read FUGR": clsRead, "SAP read INCL": clsRead, "SAP read DDLS": clsRead, "SAP read BDEF": clsRead,
	"SAP read SRVD": clsRead, "SAP read SRVB": clsRead, "SAP read MSAG": clsRead, "SAP read VIEW": clsRead,
	"SAP read ENHO": clsRead, "SAP read TABL": clsRead, "SAP read DEVC": clsRead, "SAP read DEVC inventory": clsRead, "SAP read IDOC": clsRead,
	"SAP read TRAN": clsRead, "SAP read TYPE_INFO": clsRead, "SAP read STRUCT": clsRead,
	"SAP read CDS_DEPS": clsRead, "SAP read CDS_IMPACT": clsRead, "SAP read CDS_ELEMENTS": clsRead,
	"SAP read TABL_CONTENTS": clsRead, "SAP read CHECK_RUN": clsRead, "SAP read API_STATE": clsRead,
	"SAP read CLASS_INFO": clsRead, "SAP read UI5_APP": clsRead, "SAP read ENHANCEMENT_OPTIONS": clsRead,
	"SAP read CLAS_INCLUDE": clsRead, "SAP read COVERAGE": clsRead, "SAP read UI5_LIST": clsRead,
	"SAP read UI5_FILE":                   clsRead,
	"SAP read COVERAGE include_dangerous": clsExecute,
	"SAP query TABL_CONTENTS":             clsRead, "SAP query SQL": clsRead, "SAP query SQL table": clsRead,
	"SAP query (no params)":   clsRead,
	"SAP query SQL in target": clsRead, "SAP query table in target": clsRead,
	// A statement on a table read is free SQL: --block-free-sql governs it
	// (GetTableContents), not --read-only.
	"SAP query table in target with sql": clsRead, "SAP query TABL with sql": clsRead,
	"SAP query TABL_CONTENTS with sql": clsRead,
	"SAP search":                       clsRead,
	"SAP grep package":                 clsRead, "SAP grep packages": clsRead, "SAP grep object": clsRead,
	"SAP grep objects": clsRead, "SAP grep (no target)": clsRead,

	// --- SAP(): edit ---
	"SAP edit PROG": clsMutate, "SAP edit CLAS": clsMutate, "SAP edit INTF": clsMutate, "SAP edit FUNC": clsMutate,
	"SAP edit INCL": clsMutate, "SAP edit DDLS": clsMutate, "SAP edit BDEF": clsMutate, "SAP edit SRVD": clsMutate,
	"SAP edit MSAG": clsMutate, "SAP edit TABL": clsMutate,
	"SAP edit EDITSOURCE":              clsMutate,
	"SAP edit LOCK":                    clsMutate,
	"SAP edit UNLOCK":                  clsRead,
	"SAP edit UPDATE_SOURCE":           clsMutate,
	"SAP edit MOVE":                    clsMutate,
	"SAP edit COMPARE_SOURCE":          clsRead,
	"SAP edit RECOVER_FAILED_CREATE":   clsMutate,
	"SAP edit ACTIVATE":                clsMutate,
	"SAP edit ACTIVATE_MULTI":          clsMutate,
	"SAP edit ACTIVATE_PACKAGE":        clsMutate,
	"SAP edit CLAS_INCLUDE":            clsMutate,
	"SAP edit PUBLISH_SERVICE":         clsMutate,
	"SAP edit UNPUBLISH_SERVICE":       clsMutate,
	"SAP edit type=publish_service":    clsMutate,
	"SAP edit type=unpublish_service":  clsMutate,
	"SAP edit type=write_program":      clsMutate,
	"SAP edit type=write_class":        clsMutate,
	"SAP edit type=set_description":    clsMutate,
	"SAP edit type=description":        clsMutate,
	"SAP edit type=deploy_from_file":   clsMutate,
	"SAP edit type=save_to_file":       clsRead, // writes a local file, not SAP
	"SAP edit type=rename":             clsMutate,
	"SAP system type=deploy_from_file": clsMutate,
	"SAP system type=save_to_file":     clsRead,
	"SAP system type=rename":           clsMutate,
	"SAP edit UI5_UPLOAD":              clsMutate,

	// #242: the class-include write paths.
	"SAP edit CLAS include=testclasses":    clsMutate,
	"SAP edit UPDATE_SOURCE class include": clsMutate,

	// --- SAP(): create, delete ---
	"SAP create OBJECT": clsMutate, "SAP create DEVC": clsMutate, "SAP create TABL": clsMutate,
	"SAP create CLONE": clsMutate, "SAP create ENHO": clsMutate, "SAP create BADI_IMPL": clsMutate,
	"SAP create DOMA": clsMutate, "SAP create DTEL": clsMutate, "SAP create STRUCT": clsMutate,
	"SAP create APPEND": clsMutate, "SAP create CLAS_TEST_INCLUDE": clsMutate, "SAP create PROGRAM": clsMutate,
	"SAP create CLASS_WITH_TESTS": clsMutate, "SAP create UI5_APP": clsMutate,
	"SAP delete OBJECT": clsMutate, "SAP delete (no target)": clsMutate,
	"SAP delete PROG by name": clsMutate, "SAP delete FUNC by name": clsMutate,
	"SAP delete UI5_FILE": clsMutate, "SAP delete UI5_APP": clsMutate,

	// --- SAP(): test ---
	"SAP test unit":                        clsRead, // harmless ABAP Unit only (#283)
	"SAP test type=unit include_dangerous": clsExecute,
	"SAP test type=atc":                    clsRead,
	"SAP test type=atc_customizing":        clsRead,
	"SAP test ATC":                         clsRead,
	"SAP test ATC_CUSTOMIZING":             clsRead,

	// --- SAP(): analyze, switch-routed ---
	"SAP analyze type=definition":                  clsRead,
	"SAP analyze type=cds_impact":                  clsRead,
	"SAP analyze type=references":                  clsRead,
	"SAP analyze type=completion":                  clsRead,
	"SAP analyze type=pretty_print":                clsRead,
	"SAP analyze type=get_pretty_printer_settings": clsRead,
	"SAP analyze type=set_pretty_printer_settings": clsMutate,
	"SAP analyze type=type_hierarchy":              clsRead,
	"SAP analyze type=class_components":            clsRead,
	"SAP analyze type=inactive_objects":            clsRead,
	"SAP analyze type=abap_help":                   clsRead,
	"SAP analyze type=syntax_check":                clsRead,
	"SAP analyze type=execute_abap":                clsExecute,
	// Creates a temporary program in $TMP and deletes it; never activates or
	// runs. A content-only check of a program that does not exist would write
	// nothing, but checks with fixed-point arithmetic off and so cannot see
	// into ABAP SQL (see CheckABAP).
	"SAP analyze type=check_abap": clsMutate,

	// --- SAP(): analyze, table-routed (AnalyzeTypes) ---
	"SAP analyze type=analyze_call_graph":  clsRead,
	"SAP analyze type=analyze_deps":        clsRead,
	"SAP analyze type=application_log":     clsRead,
	"SAP analyze type=call_graph":          clsRead,
	"SAP analyze type=callees":             clsRead,
	"SAP analyze type=callers":             clsRead,
	"SAP analyze type=check_boundaries":    clsRead,
	"SAP analyze type=cluster_read":        clsRead,
	"SAP analyze type=co_change":           clsRead,
	"SAP analyze type=compare_call_graphs": clsRead,
	"SAP analyze type=context":             clsRead,
	"SAP analyze type=cr_boundaries":       clsRead,
	"SAP analyze type=cr_history":          clsRead,
	"SAP analyze type=documentation":       clsRead,
	"SAP analyze type=dump_impact":         clsRead,
	"SAP analyze type=effects":             clsRead,
	"SAP analyze type=explain_dump":        clsRead,
	"SAP analyze type=fm_test_data":        clsRead,
	"SAP analyze type=get_dump":            clsRead,
	"SAP analyze type=get_trace":           clsRead,
	"SAP analyze type=graph_stats":         clsRead,
	"SAP analyze type=group_dumps":         clsRead,
	"SAP analyze type=health":              clsRead,
	"SAP analyze type=img_activity":        clsRead,
	"SAP analyze type=img_search":          clsRead,
	"SAP analyze type=impact":              clsRead,
	"SAP analyze type=job_list":            clsRead,
	"SAP analyze type=job_log":             clsRead,
	"SAP analyze type=lint":                clsRead,
	"SAP analyze type=list_dumps":          clsRead,
	"SAP analyze type=list_sql_traces":     clsRead,
	"SAP analyze type=list_traces":         clsRead,
	"SAP analyze type=loads":               clsRead,
	"SAP analyze type=object_structure":    clsRead,
	"SAP analyze type=parse_abap":          clsRead,
	"SAP analyze type=similar_dumps":       clsRead,
	"SAP analyze type=spool_list":          clsRead,
	"SAP analyze type=spool_read":          clsRead,
	"SAP analyze type=sql_trace_state":     clsRead,
	"SAP analyze type=tr_boundaries":       clsRead,
	"SAP analyze type=trace_execution":     clsRead,
	"SAP analyze type=usage_examples":      clsRead,
	"SAP analyze type=variants":            clsRead,
	"SAP analyze type=where_used_config":   clsRead,
	"SAP lint":                             clsRead,

	// --- SAP(): debug ---
	"SAP debug AMDP_ADT_START":       knownGap(kindExecute, gapAMDP),
	"SAP debug AMDP_ADT_BREAKPOINT":  knownGap(kindMutate, gapAMDP),
	"SAP debug AMDP_ADT_AWAIT":       clsRead,
	"SAP debug AMDP_ADT_STOP":        clsRead,
	"SAP debug AMDP_START":           knownGap(kindExecute, gapAMDP),
	"SAP debug AMDP_RESUME":          knownGap(kindExecute, gapAMDP),
	"SAP debug AMDP_STOP":            clsRead,
	"SAP debug AMDP_STEP":            knownGap(kindExecute, gapAMDP),
	"SAP debug AMDP_GET_VARIABLES":   clsRead,
	"SAP debug AMDP_SET_BREAKPOINT":  knownGap(kindMutate, gapAMDP),
	"SAP debug AMDP_GET_BREAKPOINTS": clsRead,
	"SAP debug SET_BREAKPOINT":       knownGap(kindMutate, gapBreakpoints),
	"SAP debug GET_BREAKPOINTS":      clsRead,
	"SAP debug DELETE_BREAKPOINT":    knownGap(kindMutate, gapBreakpoints),
	"SAP debug CALL_RFC":             clsExecute,
	"SAP debug MOVE":                 clsMutate,
	"SAP debug LISTEN":               clsRead,
	"SAP debug ATTACH":               clsRead,
	"SAP debug DETACH":               clsRead,
	"SAP debug STEP":                 knownGap(kindExecute, gapStepping),
	"SAP debug GET_STACK":            clsRead,
	"SAP debug GET_VARIABLES":        clsRead,
	"SAP debug RUN_REPORT":           clsExecute,
	"SAP debug RUN_REPORT_ASYNC":     clsExecute,
	"SAP debug GET_ASYNC_RESULT":     clsRead,
	"SAP debug GET_VARIANTS":         clsRead,
	"SAP debug GET_TEXT_ELEMENTS":    clsRead,
	"SAP debug SET_TEXT_ELEMENTS":    clsMutate,

	// --- SAP(): system ---
	"SAP system INFO":                         clsRead,
	"SAP system COMPONENTS":                   clsRead,
	"SAP system CONNECTION":                   clsRead,
	"SAP system FEATURES":                     clsRead,
	"SAP system type=system_info":             clsRead,
	"SAP system type=installed_components":    clsRead,
	"SAP system type=connection_info":         clsRead,
	"SAP system type=info":                    clsRead,
	"SAP system type=components":              clsRead,
	"SAP system type=connection":              clsRead,
	"SAP system type=features":                clsRead,
	"SAP system type=git_types":               clsRead,
	"SAP system type=git_export":              clsRead,
	"SAP system type=git_import_zip":          clsMutate, // abapGit deserialize into a package, as a background job
	"SAP system type=git_import_zip base64":   clsMutate,
	"SAP system type=git_import_status":       clsRead,   // TBTCO, the job log and the stored result; changes nothing
	"SAP system type=git_delete_objects":      clsMutate, // deletes TADIR items, the repository row, an empty package
	"SAP system type=git_object_versions":     clsRead,   // REPOSRC/DD* dates and abapGit's serialisation; changes nothing
	"SAP system type=install_zadt_vsp":        clsMutate,
	"SAP system type=list_dependencies":       clsRead,
	"SAP system type=deploy_zip":              clsMutate,
	"SAP system type=list_transports":         clsRead,
	"SAP system type=get_transport":           clsRead,
	"SAP system type=create_transport":        clsMutate,
	"SAP system type=release_transport":       clsMutate,
	"SAP system type=delete_transport":        clsMutate,
	"SAP system type=get_user_transports":     clsRead,
	"SAP system type=get_transport_info":      clsRead,
	"SAP system type=execute_abap":            clsExecute,
	"SAP system type=merge_transports":        clsMutate,
	"SAP system type=move_transport_object":   clsMutate,
	"SAP system type=move_object":             clsMutate,
	"SAP system type=copy_to_toc":             clsMutate,
	"SAP system type=transport_of_copies":     clsMutate,
	"SAP system type=add_transport_object":    clsMutate,
	"SAP system type=add_to_transport":        clsMutate,
	"SAP system type=remove_transport_object": clsMutate,
	"SAP system type=remove_from_transport":   clsMutate,
	"SAP system type=upload_transport":        clsMutate, // writes DIR_TRANS files, adds to the import buffer
	"SAP system type=upload_transport base64": clsMutate,
	"SAP system type=transport_buffer":        clsRead, // reads DIR_TRANS/buffer/<SID> through EPS; no tp, no job
	"SAP system type=transport_status":        clsRead, // TBTCO, the job log and the buffer file; no tp, no job
	"SAP system type=import_status":           clsRead, // TPALOG through RFC_READ_TABLE; no tp, no job
	"SAP system type=ui5_list_apps":           clsRead,
	"SAP system type=ui5_get_app":             clsRead,
	"SAP system type=ui5_get_file":            clsRead,
	"SAP system type=ui5_upload_file":         clsMutate,
	"SAP system type=ui5_delete_file":         clsMutate,
	"SAP system type=ui5_create_app":          clsMutate,
	"SAP system type=ui5_delete_app":          clsMutate,

	// --- SAP(): rfc ---
	"SAP rfc op=info":             clsRead,
	"SAP rfc (no op, no target)":  clsRead,
	"SAP rfc op=ping":             clsRead,
	"SAP rfc op=probe":            clsRead,
	"SAP rfc op=describe":         clsRead,
	"SAP rfc (no op, target)":     clsRead,
	"SAP rfc op=call":             clsExecute,
	"SAP rfc (args imply call)":   clsExecute,
	"SAP rfc op=search":           clsRead,
	"SAP rfc op=read_table":       clsRead,
	"SAP rfc op=read-table":       clsRead,
	"SAP rfc op=table":            clsRead,
	"SAP rfc op=read_table where": clsRead,    // free SQL; --block-free-sql governs it (#283)
	"SAP rfc op=run":              clsExecute, // schedules the report as an XBP background job (#261)
	"SAP rfc op=job":              clsRead,    // TBTCO status, job log, spool list of an existing job (#261)

	// --- SAP(): i18n, revisions ---
	"SAP i18n op=texts":               clsRead,
	"SAP i18n op=data_element_labels": clsRead,
	"SAP i18n op=message_class_texts": clsRead,
	"SAP i18n op=text_pool":           clsRead,
	"SAP i18n op=texts_get":           clsRead,
	"SAP i18n op=compare_languages":   clsRead,
	"SAP i18n op=texts_set":           clsMutate,
	"SAP i18n op=write_labels":        clsMutate,
	"SAP i18n op=write_message_texts": clsMutate,
	"SAP i18n op=write_text_pool":     clsMutate,
	"SAP revisions op=list":           clsRead,
	"SAP revisions op=source":         clsRead,
	"SAP revisions op=compare":        clsRead,
	"SAP history":                     clsRead,
}

// The known gaps, each already named in the README's read-only notes.
const (
	gapBreakpoints = "setting and deleting breakpoints is not gated by --read-only (README, known gaps): " +
		"a breakpoint changes no object or data, and refusing it would make the read-only debugger useless"
	gapStepping = "debugger stepping is not gated by --read-only (README, known gaps): it advances a program " +
		"somebody else started and stopped; whether that is execution under read-only is still open"
	gapAMDP = "starting and driving an AMDP debug session is not gated by --read-only (README, known gaps); " +
		"the session runs the procedure the user triggers, and its breakpoints are session state"
)

// --- tool arguments the names do not give away ---------------------------
// toolArgOverrides are the few tools whose parameters have a shape the name
// alone does not give away. Without them the call fails validation before it
// reaches the gate, and the probe would prove nothing.
var toolArgOverrides = map[string]map[string]any{
	"CreateObject":           {"object_type": "PROG/P"},
	"RenameObject":           {"objType": "PROG/P"},
	"ActivateMultiple":       {"objects": []any{"PROG ZDEMO_REPORT"}},
	"CreateTable":            {"fields": `[{"name":"MANDT","type":"CLNT","key":true},{"name":"ID","type":"CHAR","length":10,"key":true}]`},
	"WriteMessageClassTexts": {"texts": []any{map[string]any{"number": "001", "text": "Demo"}}},
	"DeployZip":              {"source": "abapgit-standalone"},
	"CompareCallGraphs":      {"trace_data": "{}"},
	"TraceExecution":         {"run_tests": true},
	"UnlockObject":           {"lock_handle": "FAKELOCKHANDLE"},
	"RecoverFailedCreate":    {"object_type": "PROG/P"},
}

// --- SAP() actions routed by switch statements ---------------------------
// actionCases are the SAP() actions routed by switch statements, which cannot
// be enumerated at run time. TestReadOnlyInvariant parses the route functions
// and fails on any routed literal no case here covers, so a new case in a
// router cannot go unclassified either.
func actionCases() []actionCase {
	obj := synthObjectURL
	srcEdit := func(typ, name string) actionCase {
		return actionCase{Name: "SAP edit " + typ, Action: "edit", Target: typ + " " + name,
			Exact: true, Params: kv("source", fakeSource, "package", "$TMP", "transport", "TR-EXAMPLE", "description", "Demo")}
	}
	srcRead := func(typ, name string) actionCase {
		return actionCase{Name: "SAP read " + typ, Action: "read", Target: typ + " " + name, Exact: true, Params: kv("parent", "ZDEMO_FG")}
	}
	sys := func(typ, like string, extra ...any) actionCase {
		return actionCase{Name: "SAP system type=" + typ, Action: "system", Like: like, Params: mergeArgs(kv("type", typ), kv(extra...))}
	}
	dbg := func(target, like string, extra ...any) actionCase {
		return actionCase{Name: "SAP debug " + target, Action: "debug", Target: target, Like: like, Params: kv(extra...)}
	}
	an := func(typ, like string, extra ...any) actionCase {
		return actionCase{Name: "SAP analyze type=" + typ, Action: "analyze", Like: like, Params: mergeArgs(kv("type", typ), kv(extra...))}
	}
	rfc := func(name, target string, extra ...any) actionCase {
		return actionCase{Name: "SAP rfc " + name, Action: "rfc", Target: target, Exact: true, Params: kv(extra...)}
	}
	cases := []actionCase{
		{Name: "SAP info", Action: "info", Exact: true},
		{Name: "SAP (no action)", Action: "", Exact: true},
		{Name: "SAP help", Action: "help", Exact: true},

		// read
		srcRead("PROG", "ZDEMO_REPORT"), srcRead("CLAS", "ZCL_DEMO"), srcRead("INTF", "ZIF_DEMO"),
		srcRead("FUNC", "Z_DEMO_FM"), srcRead("FUGR", "ZDEMO_FG"), srcRead("INCL", "ZDEMO_INCL"),
		srcRead("DDLS", "ZDEMO_DDLS"), srcRead("BDEF", "ZDEMO_BDEF"), srcRead("SRVD", "ZDEMO_SRV"),
		srcRead("SRVB", "ZDEMO_SRVB"), srcRead("MSAG", "ZDEMO_MSG"), srcRead("VIEW", "ZDEMO_VIEW"),
		srcRead("ENHO", "ZDEMO_ENHO"), srcRead("TABL", "ZDEMO_TAB"), srcRead("DEVC", "$TMP"),
		srcRead("IDOC", "0000000000000001"), srcRead("TRAN", "ZDEMO_TCODE"), srcRead("TYPE_INFO", "ZDEMO_TYPE"),
		srcRead("STRUCT", "ZDEMO_TAB"), srcRead("CDS_DEPS", "ZDEMO_DDLS"), srcRead("CDS_IMPACT", "ZDEMO_DDLS"),
		srcRead("CDS_ELEMENTS", "ZDEMO_DDLS"), srcRead("TABL_CONTENTS", "T000"), srcRead("CHECK_RUN", "ZDEMO_RUN"),
		srcRead("API_STATE", obj), srcRead("CLASS_INFO", "ZCL_DEMO"), srcRead("UI5_APP", "ZDEMO_APP"),
		{Name: "SAP read DEVC inventory", Action: "read", Target: "DEVC $TMP", Exact: true, Params: kv("inventory", true)},
		{Name: "SAP read ENHANCEMENT_OPTIONS", Action: "read", Target: "ENHANCEMENT_OPTIONS ZDEMO_REPORT", Params: kv("object_type", "PROG")},
		{Name: "SAP read CLAS_INCLUDE", Action: "read", Target: "CLAS_INCLUDE ZCL_DEMO", Exact: true, Params: kv("include_type", "testclasses")},
		{Name: "SAP read COVERAGE", Action: "read", Target: "COVERAGE " + obj, Exact: true},
		{Name: "SAP read COVERAGE include_dangerous", Action: "read", Target: "COVERAGE " + obj, Exact: true, Params: kv("include_dangerous", true)},
		{Name: "SAP read UI5_LIST", Action: "read", Target: "UI5_LIST", Like: "UI5ListApps"},
		{Name: "SAP read UI5_FILE", Action: "read", Target: "UI5_FILE", Like: "UI5GetFileContent"},

		// query
		{Name: "SAP query TABL_CONTENTS", Action: "query", Target: "TABL_CONTENTS T000", Exact: true, Params: kv("max_rows", 1.0)},
		{Name: "SAP query SQL", Action: "query", Target: "SQL", Exact: true, Params: kv("sql", "SELECT * FROM T000")},
		{Name: "SAP query SQL table", Action: "query", Target: "SQL T000", Exact: true},
		{Name: "SAP query (no params)", Action: "query", Exact: true},
		{Name: "SAP query SQL in target", Action: "query", Target: "SELECT * FROM T000", Exact: true},
		{Name: "SAP query table in target", Action: "query", Target: "T000", Exact: true},
		{Name: "SAP query table in target with sql", Action: "query", Target: "T000", Exact: true, Params: kv("sql", "SELECT * FROM T000")},
		{Name: "SAP query TABL with sql", Action: "query", Target: "TABL T000", Exact: true, Params: kv("sql", "SELECT * FROM T000")},
		{Name: "SAP query TABL_CONTENTS with sql", Action: "query", Target: "TABL_CONTENTS T000", Exact: true, Params: kv("sql", "SELECT * FROM T000")},

		// search and grep
		{Name: "SAP search", Action: "search", Target: "ZDEMO*", Exact: true},
		{Name: "SAP grep package", Action: "grep", Exact: true, Params: kv("pattern", "WRITE", "package", "$TMP")},
		{Name: "SAP grep packages", Action: "grep", Exact: true, Params: kv("pattern", "WRITE", "packages", []any{"$TMP"})},
		{Name: "SAP grep object", Action: "grep", Exact: true, Params: kv("pattern", "WRITE", "object_url", obj)},
		{Name: "SAP grep objects", Action: "grep", Exact: true, Params: kv("pattern", "WRITE", "object_urls", []any{obj})},
		{Name: "SAP grep (no target)", Action: "grep", Exact: true, Params: kv("pattern", "WRITE")},

		// edit
		srcEdit("PROG", "ZDEMO_REPORT"), srcEdit("CLAS", "ZCL_DEMO"), srcEdit("INTF", "ZIF_DEMO"),
		srcEdit("FUNC", "Z_DEMO_FM"), srcEdit("INCL", "ZDEMO_INCL"), srcEdit("DDLS", "ZDEMO_DDLS"),
		srcEdit("BDEF", "ZDEMO_BDEF"), srcEdit("SRVD", "ZDEMO_SRV"), srcEdit("MSAG", "ZDEMO_MSG"),
		srcEdit("TABL", "ZDEMO_TAB"),
		{Name: "SAP edit EDITSOURCE", Action: "edit", Target: "EDITSOURCE", Like: "EditSource"},
		{Name: "SAP edit LOCK", Action: "edit", Target: "LOCK", Like: "LockObject"},
		{Name: "SAP edit UNLOCK", Action: "edit", Target: "UNLOCK", Like: "UnlockObject"},
		{Name: "SAP edit UPDATE_SOURCE", Action: "edit", Target: "UPDATE_SOURCE", Like: "UpdateSource"},
		// #242: a class include is written at its own URL, through its own
		// branch; both must still be refused read-only and package-gated.
		{Name: "SAP edit CLAS include=testclasses", Action: "edit", Target: "CLAS ZCL_DEMO", Exact: true,
			Params: kv("source", fakeSource, "include", "testclasses", "transport", "TR-EXAMPLE")},
		{Name: "SAP edit UPDATE_SOURCE class include", Action: "edit", Target: "UPDATE_SOURCE", Exact: true,
			Params: kv("object_url", synthClassURL+"/includes/testclasses", "source", fakeSource, "transport", "TR-EXAMPLE")},
		{Name: "SAP edit MOVE", Action: "edit", Target: "MOVE", Like: "MoveObject"},
		{Name: "SAP edit COMPARE_SOURCE", Action: "edit", Target: "COMPARE_SOURCE", Like: "CompareSource"},
		{Name: "SAP edit RECOVER_FAILED_CREATE", Action: "edit", Target: "RECOVER_FAILED_CREATE", Like: "RecoverFailedCreate"},
		{Name: "SAP edit ACTIVATE", Action: "edit", Target: "ACTIVATE", Like: "Activate"},
		{Name: "SAP edit ACTIVATE_MULTI", Action: "edit", Target: "ACTIVATE_MULTI", Like: "ActivateMultiple"},
		{Name: "SAP edit ACTIVATE_PACKAGE", Action: "edit", Target: "ACTIVATE_PACKAGE", Like: "ActivatePackage"},
		{Name: "SAP edit CLAS_INCLUDE", Action: "edit", Target: "CLAS_INCLUDE", Like: "UpdateClassInclude"},
		{Name: "SAP edit PUBLISH_SERVICE", Action: "edit", Target: "PUBLISH_SERVICE", Like: "PublishServiceBinding"},
		{Name: "SAP edit UNPUBLISH_SERVICE", Action: "edit", Target: "UNPUBLISH_SERVICE", Like: "UnpublishServiceBinding"},
		{Name: "SAP edit type=publish_service", Action: "edit", Like: "PublishServiceBinding", Params: kv("type", "publish_service")},
		{Name: "SAP edit type=unpublish_service", Action: "edit", Like: "UnpublishServiceBinding", Params: kv("type", "unpublish_service")},
		{Name: "SAP edit type=write_program", Action: "edit", Like: "WriteProgram", Params: kv("type", "write_program")},
		{Name: "SAP edit type=write_class", Action: "edit", Like: "WriteClass", Params: kv("type", "write_class")},
		// Exact: with a source parameter, routeSourceAction would claim the
		// call as a WriteSource before routeWorkflowAction saw it.
		{Name: "SAP edit type=set_description", Action: "edit", Target: "PROG ZDEMO_REPORT", Exact: true, Params: kv("type", "set_description", "description", "Demo")},
		{Name: "SAP edit type=description", Action: "edit", Target: "PROG ZDEMO_REPORT", Exact: true, Params: kv("type", "description", "description", "Demo")},
		{Name: "SAP edit type=deploy_from_file", Action: "edit", Like: "DeployFromFile", Params: kv("type", "deploy_from_file")},
		{Name: "SAP edit type=save_to_file", Action: "edit", Like: "SaveToFile", Params: kv("type", "save_to_file")},
		{Name: "SAP edit type=rename", Action: "edit", Like: "RenameObject", Params: kv("type", "rename")},
		{Name: "SAP system type=deploy_from_file", Action: "system", Like: "DeployFromFile", Params: kv("type", "deploy_from_file")},
		{Name: "SAP system type=save_to_file", Action: "system", Like: "SaveToFile", Params: kv("type", "save_to_file")},
		{Name: "SAP system type=rename", Action: "system", Like: "RenameObject", Params: kv("type", "rename")},
		{Name: "SAP edit UI5_UPLOAD", Action: "edit", Target: "UI5_UPLOAD", Like: "UI5UploadFile"},

		// create
		{Name: "SAP create OBJECT", Action: "create", Target: "OBJECT", Like: "CreateObject"},
		{Name: "SAP create DEVC", Action: "create", Target: "DEVC", Like: "CreatePackage"},
		{Name: "SAP create TABL", Action: "create", Target: "TABL", Like: "CreateTable"},
		{Name: "SAP create CLONE", Action: "create", Target: "CLONE", Like: "CloneObject"},
		{Name: "SAP create ENHO", Action: "create", Target: "ENHO", Exact: true, Params: kv("name", "ZDEMO_ENHO", "spot", "ZDEMO_SPOT",
			"option", "ZDEMO_OPTION", "program", "ZDEMO_REPORT", "source", fakeSource, "package", "$TMP", "description", "Demo")},
		{Name: "SAP create BADI_IMPL", Action: "create", Target: "BADI_IMPL", Exact: true, Params: kv("name", "ZDEMO_BADI_IMPL", "spot", "ZDEMO_SPOT",
			"badi", "ZDEMO_BADI", "implementation", "ZDEMO_IMPL", "class", "ZCL_DEMO", "package", "$TMP", "description", "Demo")},
		{Name: "SAP create DOMA", Action: "create", Target: "DOMA ZDEMO_DOMA", Exact: true, Params: kv("description", "Demo", "package", "$TMP", "data_type", "CHAR", "length", 10.0)},
		{Name: "SAP create DTEL", Action: "create", Target: "DTEL ZDEMO_DTEL", Exact: true, Params: kv("description", "Demo", "package", "$TMP", "domain", "ZDEMO_DOMA")},
		{Name: "SAP create STRUCT", Action: "create", Target: "STRUCT ZDEMO_STRUCT", Exact: true, Params: kv("description", "Demo", "package", "$TMP", "source", "@EndUserText.label : 'Demo'\ndefine structure zdemo_struct { field : abap.char(10); }")},
		{Name: "SAP create APPEND", Action: "create", Target: "APPEND ZDEMO_APPEND", Exact: true, Params: kv("description", "Demo", "package", "$TMP", "source", "@EndUserText.label : 'Demo'\nextend type zdemo_tab with zdemo_append { zzfield : abap.char(10); }")},
		{Name: "SAP create CLAS_TEST_INCLUDE", Action: "create", Target: "CLAS_TEST_INCLUDE", Like: "CreateTestInclude"},
		{Name: "SAP create PROGRAM", Action: "create", Target: "PROGRAM", Like: "CreateAndActivateProgram"},
		{Name: "SAP create CLASS_WITH_TESTS", Action: "create", Target: "CLASS_WITH_TESTS", Like: "CreateClassWithTests"},
		{Name: "SAP create UI5_APP", Action: "create", Target: "UI5_APP", Like: "UI5CreateApp"},

		// delete
		{Name: "SAP delete OBJECT", Action: "delete", Target: "OBJECT", Like: "DeleteObject"},
		{Name: "SAP delete (no target)", Action: "delete", Like: "DeleteObject"},
		{Name: "SAP delete PROG by name", Action: "delete", Target: "PROG ZDEMO_REPORT", Exact: true},
		{Name: "SAP delete FUNC by name", Action: "delete", Target: "FUNC Z_DEMO_FM", Exact: true},
		{Name: "SAP delete UI5_FILE", Action: "delete", Target: "UI5_FILE", Like: "UI5DeleteFile"},
		{Name: "SAP delete UI5_APP", Action: "delete", Target: "UI5_APP", Like: "UI5DeleteApp"},

		// test
		{Name: "SAP test unit", Action: "test", Exact: true, Params: kv("object_url", obj)},
		{Name: "SAP test type=unit include_dangerous", Action: "test", Exact: true, Params: kv("type", "unit", "object_url", obj, "include_dangerous", true)},
		{Name: "SAP test type=atc", Action: "test", Like: "RunATCCheck", Params: kv("type", "atc")},
		{Name: "SAP test type=atc_customizing", Action: "test", Like: "GetATCCustomizing", Params: kv("type", "atc_customizing")},
		{Name: "SAP test ATC", Action: "test", Target: "ATC", Exact: true, Params: kv("object_uri", obj)},
		{Name: "SAP test ATC_CUSTOMIZING", Action: "test", Target: "ATC_CUSTOMIZING", Exact: true},

		// analyze types routed by switch
		an("definition", "FindDefinition"), an("references", "FindReferences"), an("completion", "CodeCompletion"),
		an("pretty_print", "PrettyPrint"), an("get_pretty_printer_settings", "GetPrettyPrinterSettings"),
		an("set_pretty_printer_settings", "SetPrettyPrinterSettings"), an("type_hierarchy", "GetTypeHierarchy"),
		an("class_components", "GetClassComponents"), an("inactive_objects", "GetInactiveObjects"),
		an("abap_help", "GetAbapHelp"), an("syntax_check", "SyntaxCheck"), an("execute_abap", "ExecuteABAP"),
		{Name: "SAP analyze type=check_abap", Action: "analyze", Exact: true, Params: kv("type", "check_abap", "code", "DATA lv TYPE i.")},

		// debug
		dbg("AMDP_ADT_START", "AMDPDebuggerStart"), dbg("AMDP_ADT_BREAKPOINT", "AMDPSetBreakpoint"),
		dbg("AMDP_ADT_AWAIT", "AMDPDebuggerResume"), dbg("AMDP_ADT_STOP", "AMDPDebuggerStop"),
		dbg("AMDP_START", "AMDPDebuggerStart"), dbg("AMDP_RESUME", "AMDPDebuggerResume"),
		dbg("AMDP_STOP", "AMDPDebuggerStop"), dbg("AMDP_STEP", "AMDPDebuggerStep"),
		dbg("AMDP_GET_VARIABLES", "AMDPGetVariables"), dbg("AMDP_SET_BREAKPOINT", "AMDPSetBreakpoint"),
		dbg("AMDP_GET_BREAKPOINTS", "AMDPGetBreakpoints"),
		dbg("SET_BREAKPOINT", "SetBreakpoint"), dbg("GET_BREAKPOINTS", "GetBreakpoints"),
		dbg("DELETE_BREAKPOINT", "DeleteBreakpoint"), dbg("CALL_RFC", "CallRFC"), dbg("MOVE", "MoveObject"),
		dbg("LISTEN", "DebuggerListen"), dbg("ATTACH", "DebuggerAttach"), dbg("DETACH", "DebuggerDetach"),
		dbg("STEP", "DebuggerStep"), dbg("GET_STACK", "DebuggerGetStack"), dbg("GET_VARIABLES", "DebuggerGetVariables"),
		dbg("RUN_REPORT", "RunReport"), dbg("RUN_REPORT_ASYNC", "RunReportAsync"), dbg("GET_ASYNC_RESULT", "GetAsyncResult"),
		dbg("GET_VARIANTS", "GetVariants"), dbg("GET_TEXT_ELEMENTS", "GetTextElements"), dbg("SET_TEXT_ELEMENTS", "SetTextElements"),

		// system
		{Name: "SAP system INFO", Action: "system", Target: "INFO", Exact: true},
		{Name: "SAP system COMPONENTS", Action: "system", Target: "COMPONENTS", Exact: true},
		{Name: "SAP system CONNECTION", Action: "system", Target: "CONNECTION", Exact: true},
		{Name: "SAP system FEATURES", Action: "system", Target: "FEATURES", Exact: true},
		{Name: "SAP system type=system_info", Action: "system", Exact: true, Params: kv("type", "system_info")},
		{Name: "SAP system type=installed_components", Action: "system", Exact: true, Params: kv("type", "installed_components")},
		{Name: "SAP system type=connection_info", Action: "system", Exact: true, Params: kv("type", "connection_info")},
		{Name: "SAP system type=info", Action: "system", Exact: true, Params: kv("type", "info")},
		{Name: "SAP system type=components", Action: "system", Exact: true, Params: kv("type", "components")},
		{Name: "SAP system type=connection", Action: "system", Exact: true, Params: kv("type", "connection")},
		{Name: "SAP system type=features", Action: "system", Exact: true, Params: kv("type", "features")},
		sys("git_types", "GitTypes"), sys("git_export", "GitExport"),
		{Name: "SAP system type=git_import_zip", Action: "system", Exact: true, Params: kv("type", "git_import_zip", "file_path", "demo.zip", "package", "$TMP")},
		{Name: "SAP system type=git_import_zip base64", Action: "system", Exact: true, Params: kv("type", "git_import_zip",
			"zip_base64", "UEsFBgAAAAAAAAAAAAAAAAAAAAAAAA==", "package", "$TMP", "overwrite", true)},
		{Name: "SAP system type=git_import_status", Action: "system", Exact: true, Params: kv("type", "git_import_status", "job", "12345678")},
		{Name: "SAP system type=git_delete_objects", Action: "system", Exact: true, Params: kv("type", "git_delete_objects", "package", "$TMP",
			"objects", []any{"PROG ZDEMO_REPORT"})},
		{Name: "SAP system type=git_object_versions", Action: "system", Exact: true, Params: kv("type", "git_object_versions", "package", "$TMP",
			"objects", []any{"PROG ZDEMO_REPORT"}, "sha256", true)},
		sys("install_zadt_vsp", "InstallZADTVSP"), sys("list_dependencies", "ListDependencies"),
		sys("deploy_zip", "DeployZip"),
		sys("list_transports", "ListTransports"), sys("get_transport", "GetTransport"),
		sys("create_transport", "CreateTransport"), sys("release_transport", "ReleaseTransport"),
		sys("delete_transport", "DeleteTransport"), sys("get_user_transports", "GetUserTransports"),
		sys("get_transport_info", "GetTransportInfo"), sys("execute_abap", "ExecuteABAP"),
		{Name: "SAP system type=merge_transports", Action: "system", Exact: true, Params: kv("type", "merge_transports", "source", "TR-EXAMPLE", "target", "TR-EXAMPLE2")},
		{Name: "SAP system type=move_transport_object", Action: "system", Exact: true, Params: kv("type", "move_transport_object", "object", "R3TR PROG ZDEMO_REPORT", "from", "TR-EXAMPLE", "to", "TR-EXAMPLE2")},
		{Name: "SAP system type=move_object", Action: "system", Exact: true, Params: kv("type", "move_object", "object", "R3TR PROG ZDEMO_REPORT", "from", "TR-EXAMPLE", "to", "TR-EXAMPLE2")},
		{Name: "SAP system type=copy_to_toc", Action: "system", Exact: true, Params: kv("type", "copy_to_toc", "transport", "TR-EXAMPLE", "target", "QAS")},
		{Name: "SAP system type=transport_of_copies", Action: "system", Exact: true, Params: kv("type", "transport_of_copies", "transport", "TR-EXAMPLE", "target", "QAS")},
		{Name: "SAP system type=add_transport_object", Action: "system", Exact: true, Params: kv("type", "add_transport_object", "object", "R3TR PROG ZDEMO_REPORT", "transport", "TR-EXAMPLE")},
		{Name: "SAP system type=add_to_transport", Action: "system", Exact: true, Params: kv("type", "add_to_transport", "object", "R3TR PROG ZDEMO_REPORT", "transport", "TR-EXAMPLE")},
		{Name: "SAP system type=remove_transport_object", Action: "system", Exact: true, Params: kv("type", "remove_transport_object", "object", "R3TR PROG ZDEMO_REPORT", "transport", "TR-EXAMPLE")},
		{Name: "SAP system type=remove_from_transport", Action: "system", Exact: true, Params: kv("type", "remove_from_transport", "object", "R3TR PROG ZDEMO_REPORT", "transport", "TR-EXAMPLE")},
		{Name: "SAP system type=upload_transport", Action: "system", Exact: true, Params: kv("type", "upload_transport", "cofile_path", "K900001.XYZ", "datafile_path", "R900001.XYZ")},
		{Name: "SAP system type=upload_transport base64", Action: "system", Exact: true, Params: kv("type", "upload_transport",
			"cofile_name", "K900001.XYZ", "cofile_base64", "VEVTVFVTRVIgSyBRQVMgMwo=", "datafile_name", "R900001.XYZ", "datafile_base64", "AAE=")},
		{Name: "SAP system type=transport_buffer", Action: "system", Exact: true, Params: kv("type", "transport_buffer")},
		{Name: "SAP system type=transport_status", Action: "system", Exact: true, Params: kv("type", "transport_status", "transport", "TR-EXAMPLE", "job", "12345678")},
		{Name: "SAP system type=import_status", Action: "system", Exact: true, Params: kv("type", "import_status", "transport", "TR-EXAMPLE")},
		sys("ui5_list_apps", "UI5ListApps"), sys("ui5_get_app", "UI5GetApp"), sys("ui5_get_file", "UI5GetFileContent"),
		sys("ui5_upload_file", "UI5UploadFile"), sys("ui5_delete_file", "UI5DeleteFile"),
		sys("ui5_create_app", "UI5CreateApp"), sys("ui5_delete_app", "UI5DeleteApp"),

		// rfc
		rfc("op=info", "", "op", "info"), rfc("(no op, no target)", ""),
		rfc("op=ping", "", "op", "ping"), rfc("op=probe", "", "op", "probe"),
		rfc("op=describe", "STFC_CONNECTION", "op", "describe"), rfc("(no op, target)", "STFC_CONNECTION"),
		rfc("op=call", "Z_DEMO_FM", "op", "call", "args", map[string]any{"N": 21.0}),
		rfc("(args imply call)", "Z_DEMO_FM", "args", map[string]any{"N": 21.0}),
		rfc("op=search", "ZDEMO*", "op", "search"),
		rfc("op=read_table", "T000", "op", "read_table", "top", 1.0),
		rfc("op=read-table", "T000", "op", "read-table", "top", 1.0),
		rfc("op=table", "T000", "op", "table", "top", 1.0),
		rfc("op=read_table where", "T000", "op", "read_table", "top", 1.0, "where", "MANDT = '001'"),
		rfc("op=run", "ZDEMO_REPORT", "op", "run", "wait", 0.0),
		rfc("op=job", "VSP_ZDEMO_REPORT", "op", "job", "job_count", "12345678"),

		{Name: "SAP history", Action: "history", Target: "PROG ZDEMO_REPORT", Exact: true},
	}
	return cases
}

// --- the package gate -------------------------------------------------------
// packageGated are the object-scoped mutations a server started with
// --allowed-packages "Z*" (and without --read-only) must refuse for an object
// in $TMP, before any write. An empty value means the package gate must hold;
// a non-empty one is a known gap and its reason.
var packageGated = map[string]string{
	"tool CloneObject":              "",
	"tool CreateAndActivateProgram": "",
	"tool CreateClassWithTests":     "",
	"tool CreateObject":             "",
	"tool CreateTable":              "",
	"tool CreateTestInclude":        "",
	"tool DeleteObject":             "",
	"tool DeployFromFile":           "",
	"tool EditSource":               "",
	"tool ExecuteABAP":              "", // its temporary program goes to $TMP
	"tool ImportFromFile":           "",
	"tool MoveObject":               "", // into $TMP: refused for the target package
	"tool MoveObject out of $TMP":   "", // into ZDEMO_PKG: refused for the package the object is in now
	"tool RecoverFailedCreate":      "",
	"tool RenameObject":             "",
	"tool UI5CreateApp":             "",
	"tool UI5UploadFile":            "", // fails closed: UI5 app→package resolution is not implemented
	"tool UpdateClassInclude":       "",
	"tool UpdateSource":             "",
	"tool WriteClass":               "",
	"tool WriteMessageClassTexts":   "",
	"tool WriteProgram":             "",
	"tool WriteSource":              "",
	"SAP edit PROG":                 "",
	"SAP edit type=set_description": "",
	"SAP create DOMA":               "",
	"SAP create DTEL":               "",
	"SAP create STRUCT":             "",
	"SAP create BADI_IMPL":          "",
	"SAP i18n op=texts_set":         "",

	// #242: the class-include write paths.
	"SAP edit CLAS include=testclasses":    "",
	"SAP edit UPDATE_SOURCE class include": "",

	"tool Activate":                gapActivationPackage,
	"tool ActivateMultiple":        gapActivationPackage,
	"tool CreateIAMApp":            gapActivationPackage + "; an IAM object that already exists goes straight to activation",
	"tool LockObject":              "a MODIFY lock is checked as an operation but not against --allowed-packages; the write it precedes is",
	"tool PublishServiceBinding":   "publishing a service binding is checked as an update (#283) but not against --allowed-packages: the binding's package needs a lookup",
	"tool UnpublishServiceBinding": "unpublishing a service binding is checked as an update (#283) but not against --allowed-packages: the binding's package needs a lookup",
	"tool SetTextElements":         "writes a program's texts through ZADT_VSP; refused under --read-only (#283), not checked against --allowed-packages: the program's package needs a lookup",
}

const gapActivationPackage = "activation is checked as an operation (A) but not against --allowed-packages: resolving each " +
	"object's package would add a lookup to every write workflow's activation, and fail closed for objects the quick search cannot place"
