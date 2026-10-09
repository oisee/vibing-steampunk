package embedded

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// ZCL_VSP_GIT_SERVICE imports an abapGit zip into a package. These checks read
// the source vsp install deploys and fail on any statement that would let it
// do more than that: reach another system, start an arbitrary program, run
// in a task of its own, wait in the APC session, write a database table
// other than its own INDX(ZV) area, or decide past the caller's overwrite
// and package choices.

// gitServiceFunctions are the only function modules the service may call.
var gitServiceFunctions = map[string]bool{
	// The import runs as background job ZVSP_GIT_IMPORT: abapGit commits
	// with WAIT, which an APC session may not (APC_ILLEGAL_STATEMENT, probed
	// live), and an import may run for minutes.
	"JOB_OPEN":   true,
	"JOB_SUBMIT": true,
	"JOB_CLOSE":  true,
	// The job step's parameters: a protected variant per job (an APC session
	// may not SUBMIT), deleted once its job has ended.
	"RS_CREATE_VARIANT":    true,
	"RS_VARIANT_DELETE":    true,
	"GET_JOB_RUNTIME_INFO": true,
	// import_status reads the job's log (read-only).
	"BP_JOBLOG_READ": true,
	// A job that will not run (JOB_CLOSE failed, or not released) is
	// deleted again, with its variant and zip; only ZVSP_GIT_IMPORT jobs.
	"BP_JOB_DELETE": true,
	// The export's base64 (older code).
	"SSFC_BASE64_DECODE": true,
	"SSFC_BASE64_ENCODE": true,
}

func gitServiceSource(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "src", "zcl_vsp_git_service.clas.abap"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// dbWriteRe finds every statement that may write a table: Open SQL's
// INSERT, UPDATE, MODIFY and DELETE in any form (with or without FROM, any
// target, chained), and the internal-table statements that share their
// keywords. Each one must be in gitAllowedWrites.
var (
	dbWriteRe   = regexp.MustCompile(`^(INSERT|UPDATE|MODIFY|DELETE)\b`)
	toDatabase  = regexp.MustCompile(`\bDATABASE\s+(\S+)`)
	gitDynCall  = regexp.MustCompile(`CALL METHOD \S*(->|=>)\(`)
	gitDynFunc  = regexp.MustCompile(`(?i)^CALL FUNCTION \(`)
	assignRe    = regexp.MustCompile(`^(LV_REPORT|LV_JOBNAME)\s*=\s*(.+)$`)
	jobNameDecl = regexp.MustCompile(`CONSTANTS C_JOB_NAME TYPE TBTCJOB-JOBNAME VALUE '([A-Z0-9_]+)'`)
)

// gitAllowedWrites are the only INSERT/UPDATE/MODIFY/DELETE statements the
// service has: its own INDX(ZV) rows, and one internal table.
var gitAllowedWrites = map[string]bool{
	"DELETE FROM DATABASE INDX(ZV) ID LV_KEY":                                              true,
	"DELETE FROM INDX WHERE RELID = 'ZV' AND SRTFD LIKE 'VSPGITR%' AND AEDAT < @LV_CUTOFF": true,
	"DELETE ADJACENT DUPLICATES FROM RT_PACKAGES":                                          true,
}

// normStmt upper-cases a statement and folds its whitespace, as
// abapStatements does.
func normStmt(s string) string { return strings.ToUpper(strings.Join(strings.Fields(s), " ")) }

// seqAt is the index of the first run of consecutive statements that match
// want one for one: equal after normStmt, or, for an entry ending in "*",
// starting with what precedes the "*". -1 if there is none.
func seqAt(stmts []string, want ...string) int {
	match := func(st, w string) bool {
		if p, ok := strings.CutSuffix(w, "*"); ok {
			return strings.HasPrefix(normStmt(st), normStmt(p))
		}
		return normStmt(st) == normStmt(w)
	}
	for i := 0; i+len(want) <= len(stmts); i++ {
		ok := true
		for j, w := range want {
			if !match(stmts[i+j], w) {
				ok = false
				break
			}
		}
		if ok {
			return i
		}
	}
	return -1
}

// countExact counts the statements equal to want (normStmt).
func countExact(stmts []string, want string) int {
	n := 0
	for _, st := range stmts {
		if normStmt(st) == normStmt(want) {
			n++
		}
	}
	return n
}

// checkGitService returns every rule src breaks.
func checkGitService(src string) []string {
	var bad []string
	stmts := abapStatements(src)
	upSrc := strings.ToUpper(strings.Join(stmts, "\n"))

	if m := jobNameDecl.FindStringSubmatch(upSrc); m == nil || m[1] != "ZVSP_GIT_IMPORT" {
		bad = append(bad, "c_job_name must be the constant 'ZVSP_GIT_IMPORT'")
	}

	for _, st := range stmts {
		up := strings.ToUpper(st)
		if gitDynFunc.MatchString(st) {
			bad = append(bad, "dynamic CALL FUNCTION: "+st)
			continue
		}
		if m := callFunctionRe.FindStringSubmatch(st); m != nil {
			name := m[1]
			if !strings.HasPrefix(name, "'") || !strings.HasSuffix(name, "'") {
				bad = append(bad, "dynamic CALL FUNCTION "+name+": the function called must be a literal")
				continue
			}
			name = strings.ToUpper(strings.Trim(name, "'"))
			for _, kw := range []string{" DESTINATION ", " STARTING NEW TASK", " IN BACKGROUND TASK", " IN BACKGROUND UNIT", " IN UPDATE TASK"} {
				if strings.Contains(up+" ", kw) {
					bad = append(bad, "CALL FUNCTION '"+name+"' with"+kw+": every call must run here, synchronously")
				}
			}
			if !gitServiceFunctions[name] {
				bad = append(bad, "CALL FUNCTION '"+name+"' is not one the git service may call")
			}
			switch name {
			case "JOB_SUBMIT":
				if m := bindingRe("REPORT").FindStringSubmatch(up); m == nil || m[1] != "LV_REPORT" {
					bad = append(bad, "JOB_SUBMIT: report must be lv_report (c_job_name)")
				}
				if m := bindingRe("VARIANT").FindStringSubmatch(up); m == nil || m[1] != "LV_VARIANT" {
					bad = append(bad, "JOB_SUBMIT: variant must be lv_variant, the job's own")
				}
				ms := bindingRe("AUTHCKNAM").FindAllStringSubmatch(up, -1)
				if len(ms) == 0 {
					bad = append(bad, "JOB_SUBMIT: authcknam must be sy-uname")
				}
				for _, m := range ms {
					if m[1] != "SY-UNAME" {
						bad = append(bad, "JOB_SUBMIT: authcknam must be sy-uname")
					}
				}
				for _, p := range []string{"COMMANDNAME", "EXTPGM_NAME", "EXTPGM_PARAM", "OPERATINGSYSTEM", "TARGETSYSTEM"} {
					if regexp.MustCompile(`\b` + p + `\s*=`).MatchString(up) {
						bad = append(bad, "JOB_SUBMIT: "+p+" must not be passed")
					}
				}
			case "BP_JOB_DELETE":
				if m := bindingRe("JOBNAME").FindStringSubmatch(up); m == nil || m[1] != "LV_JOBNAME" {
					bad = append(bad, "BP_JOB_DELETE: only jobs of lv_jobname (c_job_name)")
				}
			case "RS_CREATE_VARIANT", "RS_VARIANT_DELETE":
				key := map[string]string{"RS_CREATE_VARIANT": "CURR_REPORT", "RS_VARIANT_DELETE": "REPORT"}[name]
				if m := bindingRe(key).FindStringSubmatch(up); m == nil || m[1] != "LV_REPORT" {
					bad = append(bad, name+": only variants of lv_report (c_job_name)")
				}
			}
			continue
		}
		// No other program is started, no screen, no transaction.
		for _, kw := range []string{"SUBMIT ", "CALL TRANSACTION", "LEAVE TO TRANSACTION", "CALL SCREEN", "CALL SELECTION-SCREEN",
			"GENERATE SUBROUTINE POOL", "INSERT REPORT", "EXEC SQL", "OPEN DATASET", "DELETE DATASET", "CALL 'SYSTEM'"} {
			if strings.HasPrefix(up, kw) || strings.Contains(up, " "+kw) {
				bad = append(bad, "statement not allowed in the git service: "+st)
			}
		}
		// Nothing waits in the APC session.
		if strings.Contains(up, "COMMIT WORK AND WAIT") || strings.HasPrefix(up, "WAIT ") || strings.Contains(up, "SET UPDATE TASK LOCAL") {
			bad = append(bad, "the service never waits (an APC session may not): "+st)
		}
		if strings.HasPrefix(up, "CREATE OBJECT") && strings.Contains(up, "TYPE (") {
			bad = append(bad, "dynamic CREATE OBJECT: "+st)
		}
		// One dynamic method call: abapGit's new_offline, whose name
		// parameter changed between releases.
		if gitDynCall.MatchString(up) && up != "CALL METHOD LI_SRV->('NEW_OFFLINE') PARAMETER-TABLE LT_PARAMS" {
			bad = append(bad, "dynamic method call other than new_offline: "+st)
		}
		// Database writes: its own INDX(ZV) area only.
		if dbWriteRe.MatchString(up) && !gitAllowedWrites[up] {
			bad = append(bad, "database write outside INDX(ZV) (or a table statement not on the list): "+st)
		}
		if strings.HasPrefix(up, "EXPORT") && strings.Contains(up, "DATABASE") {
			ms := toDatabase.FindAllStringSubmatch(up, -1)
			if len(ms) == 0 {
				bad = append(bad, "EXPORT TO DATABASE outside INDX(ZV): "+st)
			}
			for _, m := range ms {
				if m[1] != "INDX(ZV)" {
					bad = append(bad, "EXPORT TO DATABASE outside INDX(ZV): "+st)
				}
			}
		}
		// A variant is only ever the job's own, VSP<job number>.
		if strings.HasPrefix(up, "LV_VARIANT =") && up != "LV_VARIANT = |VSP{ LV_JOBCOUNT }|" {
			bad = append(bad, "lv_variant may only be VSP<job number>: "+st)
		}
		if m := assignRe.FindStringSubmatch(up); m != nil && m[2] != "C_JOB_NAME" {
			bad = append(bad, strings.ToLower(m[1])+" may only be c_job_name: "+st)
		}
	}
	bad = append(bad, checkGitPolicy(stmts)...)
	return bad
}

// Statement runs the policy is pinned to. Every condition is matched whole
// (whitespace folded), with the refusal code it sets and its RETURN: a
// condition weakened ("IF 1 = 2 AND ..."), a clause dropped, or a refusal
// removed no longer matches. Messages are matched by their start only.
var (
	// evaluate_checks: conflicts and unmet requirements refuse before any
	// decision is made.
	gitEvalRefusals = [][]string{
		{"IF CS_CHECKS-REQUIREMENTS-MET = 'N'", "EV_CODE = `REQUIREMENTS_NOT_MET`", "EV_MESSAGE = *", "RETURN", "ENDIF"},
		{"IF CS_CHECKS-DEPENDENCIES-MET = 'N'", "EV_CODE = `DEPENDENCIES_NOT_MET`", "EV_MESSAGE = *", "RETURN", "ENDIF"},
		{"IF CS_CHECKS-WARNING_PACKAGE IS NOT INITIAL", "LOOP AT CS_CHECKS-WARNING_PACKAGE INTO DATA(LS_PKG)", "APPEND *", "ENDLOOP",
			"EV_CODE = `PACKAGE_CONFLICT`", "EV_MESSAGE = *", "RETURN", "ENDIF"},
		{"IF CS_CHECKS-DATA_LOSS IS NOT INITIAL", "LOOP AT CS_CHECKS-DATA_LOSS INTO DATA(LS_LOSS)", "APPEND *", "ENDLOOP",
			"EV_CODE = `DATA_LOSS`", "EV_MESSAGE = *", "RETURN", "ENDIF"},
		{"IF CS_CHECKS-CUSTOMIZING-REQUIRED = ABAP_TRUE", "EV_CODE = `CUSTOMIZING_NOT_SUPPORTED`", "EV_MESSAGE = *", "RETURN", "ENDIF"},
	}
	// The one exemption from overwrite: the target package's own entry,
	// when the package existed without a repository -- kept ('N'), never
	// written.
	gitEvalPackageKept = []string{
		"IF IV_NEW_REPO = ABAP_TRUE AND <LS_OVER>-OBJ_TYPE = 'DEVC' AND <LS_OVER>-OBJ_NAME = IS_PARAMS-PACKAGE AND ( <LS_OVER>-ACTION = LC_UPDATE OR <LS_OVER>-ACTION = LC_OVERWRITE )",
		"LS_DECISION-ACTION = `PACKAGE_KEPT`",
		"<LS_OVER>-DECISION = 'N'",
		"LS_DECISION-DECISION = <LS_OVER>-DECISION",
		"APPEND LS_DECISION TO ET_DECISIONS",
		"CONTINUE",
		"ENDIF",
	}
	gitEvalAfterLoop = []string{
		"IF LT_REFUSED IS NOT INITIAL", "EV_CODE = `OVERWRITE_REFUSED`", "EV_MESSAGE = *", "RETURN", "ENDIF",
		"IF CS_CHECKS-TRANSPORT-REQUIRED = ABAP_TRUE", "IF IS_PARAMS-TRANSPORT IS INITIAL", "EV_CODE = `TRANSPORT_REQUIRED`", "EV_MESSAGE = *", "RETURN", "ENDIF",
		"CS_CHECKS-TRANSPORT-TRANSPORT = IS_PARAMS-TRANSPORT", "ELSE", "CLEAR CS_CHECKS-TRANSPORT-TRANSPORT", "ENDIF",
	}

	// check_packages, whole: every file below the starting folder maps to a
	// package the caller listed, no package is created on the way.
	gitCheckPackages = []string{
		"DATA(LT_LISTED) = SPLIT_PACKAGES( IS_PARAMS-PACKAGES )",
		"DATA(LO_DOT) = II_REPO->GET_DOT_ABAPGIT( )",
		"DATA(LV_START) = LO_DOT->GET_STARTING_FOLDER( )",
		"DATA(LV_START_LEN) = STRLEN( LV_START )",
		"DATA(LO_LOGIC) = ZCL_ABAPGIT_FOLDER_LOGIC=>GET_INSTANCE( )",
		"LOOP AT IT_FILES INTO DATA(LS_FILE)",
		"IF STRLEN( LS_FILE-PATH ) < LV_START_LEN", "CONTINUE", "ENDIF",
		"IF SUBSTRING( VAL = LS_FILE-PATH LEN = LV_START_LEN ) <> LV_START", "CONTINUE", "ENDIF",
		"DATA(LV_PACKAGE) = LO_LOGIC->PATH_TO_PACKAGE( IV_TOP = IS_PARAMS-PACKAGE IO_DOT = LO_DOT IV_PATH = LS_FILE-PATH IV_CREATE_IF_NOT_EXISTS = ABAP_FALSE )",
		"IF LV_PACKAGE IS NOT INITIAL AND NOT LINE_EXISTS( LT_LISTED[ TABLE_LINE = LV_PACKAGE ] )",
		"RV_MESSAGE = *", "RETURN", "ENDIF",
		"ENDLOOP",
	}

	// zip_limits, whole: the directory's entries and declared size, and one
	// .abapgit.xml, before anything is decompressed.
	gitZipLimits = []string{
		"DATA: LO_ZIP TYPE REF TO CL_ABAP_ZIP, LV_TOTAL TYPE INT8, LV_DOTS TYPE I",
		"CLEAR: EV_CODE, EV_MESSAGE",
		"CREATE OBJECT LO_ZIP",
		"LO_ZIP->LOAD( EXPORTING ZIP = IV_ZIP EXCEPTIONS ZIP_PARSE_ERROR = 1 OTHERS = 2 )",
		"IF SY-SUBRC <> 0", "EV_CODE = `INVALID_ZIP`", "EV_MESSAGE = *", "RETURN", "ENDIF",
		"IF LINES( LO_ZIP->FILES ) > C_MAX_ENTRIES", "EV_CODE = `TOO_LARGE`", "EV_MESSAGE = *", "RETURN", "ENDIF",
		"LOOP AT LO_ZIP->FILES INTO DATA(LS_FILE)",
		"IF LS_FILE-SIZE < 0", "EV_CODE = `TOO_LARGE`", "EV_MESSAGE = *", "RETURN", "ENDIF",
		"LV_TOTAL = LV_TOTAL + LS_FILE-SIZE",
		"IF LS_FILE-NAME = '.ABAPGIT.XML'", "LV_DOTS = LV_DOTS + 1", "ENDIF",
		"ENDLOOP",
		"IF LV_TOTAL > C_MAX_UNZIPPED", "EV_CODE = `TOO_LARGE`", "EV_MESSAGE = *", "RETURN", "ENDIF",
		"IF LV_DOTS <> 1", "EV_CODE = `INVALID_ZIP`", "EV_MESSAGE = *", "RETURN", "ENDIF",
	}

	// do_import, in this order, each before abapGit deserializes.
	gitImportLimits = []string{
		"ZIP_LIMITS( EXPORTING IV_ZIP = IV_ZIP IMPORTING EV_CODE = LV_CODE EV_MESSAGE = LV_MESSAGE )",
		"IF LV_CODE IS NOT INITIAL", "RS_RESULT-OUTCOME = `REFUSED`", "RS_RESULT-CODE = LV_CODE", "RS_RESULT-MESSAGE = *", "RETURN", "ENDIF",
	}
	gitImportLoad       = []string{"DATA(LT_FILES) = ZCL_ABAPGIT_ZIP=>LOAD( IV_ZIP )"}
	gitImportDotMissing = []string{"IF LV_HAS_DOT = ABAP_FALSE", "RS_RESULT-OUTCOME = `REFUSED`", "RS_RESULT-CODE = `INVALID_ZIP`", "RS_RESULT-MESSAGE = *", "RETURN", "ENDIF"}
	gitImportPkgMissing = []string{
		"IF IS_PARAMS-PACKAGE(1) <> '$' AND ZCL_ABAPGIT_FACTORY=>GET_SAP_PACKAGE( IS_PARAMS-PACKAGE )->EXISTS( ) = ABAP_FALSE",
		"RS_RESULT-OUTCOME = `REFUSED`", "RS_RESULT-CODE = `PACKAGE_MISSING`", "RS_RESULT-MESSAGE = *", "RETURN", "ENDIF",
	}
	gitImportRepo = []string{
		"ZCL_ABAPGIT_REPO_SRV=>GET_INSTANCE( )->GET_REPO_FROM_PACKAGE( EXPORTING IV_PACKAGE = IS_PARAMS-PACKAGE IMPORTING EI_REPO = LI_REPO EV_REASON = LV_REASON )",
		"IF LI_REPO IS BOUND",
		"RS_RESULT-REPO_KEY = LI_REPO->GET_KEY( )",
		"RS_RESULT-REPO_NAME = LI_REPO->GET_NAME( )",
		"IF LI_REPO->GET_PACKAGE( ) <> IS_PARAMS-PACKAGE", "RS_RESULT-OUTCOME = `REFUSED`", "RS_RESULT-CODE = `REPO_OTHER_PACKAGE`", "RS_RESULT-MESSAGE = *", "RETURN", "ENDIF",
		"IF IS_PARAMS-OVERWRITE = ABAP_FALSE", "RS_RESULT-OUTCOME = `REFUSED`", "RS_RESULT-CODE = `REPO_EXISTS`", "RS_RESULT-MESSAGE = *", "RETURN", "ENDIF",
		"IF LI_REPO->IS_OFFLINE( ) = ABAP_FALSE", "RS_RESULT-OUTCOME = `REFUSED`", "RS_RESULT-CODE = `REPO_ONLINE`", "RS_RESULT-MESSAGE = *", "RETURN", "ENDIF",
		"ELSE",
		"LI_REPO = NEW_OFFLINE_REPO( IV_NAME = IS_PARAMS-REPO_NAME IV_PACKAGE = IS_PARAMS-PACKAGE )",
		"LV_CREATED = ABAP_TRUE",
	}
	gitImportChecks = []string{
		"LV_MESSAGE = CHECK_PACKAGES( II_REPO = LI_REPO IT_FILES = LT_FILES IS_PARAMS = IS_PARAMS )",
		"IF LV_MESSAGE IS NOT INITIAL", "LV_CODE = `PACKAGE_NOT_LISTED`", "ELSE",
		"EVALUATE_CHECKS( EXPORTING IS_PARAMS = IS_PARAMS IV_NEW_REPO = LV_CREATED IMPORTING EV_CODE = LV_CODE EV_MESSAGE = LV_MESSAGE ET_DECISIONS = RS_RESULT-DECISIONS CHANGING CS_CHECKS = LS_CHECKS )",
		"ENDIF",
		"IF LV_CODE IS NOT INITIAL", "RS_RESULT-OUTCOME = `REFUSED`", "RS_RESULT-CODE = LV_CODE", "RS_RESULT-MESSAGE = *",
		"IF LV_CREATED = ABAP_TRUE", "ZCL_ABAPGIT_REPO_SRV=>GET_INSTANCE( )->DELETE( LI_REPO )", "COMMIT WORK", "RS_RESULT-REPO_CREATED = ABAP_FALSE",
		"RS_RESULT-MESSAGE = *", "CLEAR RS_RESULT-REPO_KEY", "ENDIF",
		"RETURN", "ENDIF",
	}
	gitImportDeserialize = []string{"LI_REPO->DESERIALIZE( IS_CHECKS = LS_CHECKS II_LOG = LI_LOG )"}

	// import_begin: the target is among the packages the caller checked,
	// and each of them is a package name.
	gitBeginPackages = []string{
		"DATA(LT_PACKAGES) = SPLIT_PACKAGES( LV_PACKAGES )",
		"IF NOT LINE_EXISTS( LT_PACKAGES[ TABLE_LINE = CONV DEVCLASS( LV_PACKAGE ) ] )",
		"RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'INVALID_PARAM' IV_MESSAGE = |PACKAGES MUST LIST *", "RETURN", "ENDIF",
		"LOOP AT LT_PACKAGES INTO DATA(LV_LISTED)",
		"IF VALID_PACKAGE( CONV #( LV_LISTED ) ) = ABAP_FALSE", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'INVALID_PARAM' *", "RETURN", "ENDIF",
		"ENDLOOP",
	}
	// import_begin: a second begin never discards an upload in progress.
	gitBeginOne = []string{
		"DATA: LV_UUID TYPE SYSUUID_C32, LS_PARAMS TYPE TY_IMPORT_PARAMS",
		"IF MS_UPLOAD-ID IS NOT INITIAL", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'UPLOAD_IN_PROGRESS' *", "RETURN", "ENDIF",
		"DATA(LV_PARAMS) = IS_MESSAGE-PARAMS",
	}
	gitBeginParams = []string{
		"LS_PARAMS = VALUE #( PACKAGE = LV_PACKAGE REPO_NAME = LV_REPO_NAME OVERWRITE = XSDBOOL( LV_OVERWRITE = 'TRUE' ) TRANSPORT = LV_TRANSPORT PACKAGES = LV_PACKAGES )",
	}

	// import_commit: the size and the SHA-256 declared at begin, then the
	// zip's limits, before the job is started.
	gitCommitChecks = []string{
		"DATA(LS_UP) = MS_UPLOAD", "CLEAR MS_UPLOAD",
		"IF XSTRLEN( LS_UP-DATA ) <> LS_UP-SIZE", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'INCOMPLETE' *", "RETURN", "ENDIF",
		"IF SHA256( LS_UP-DATA ) <> LS_UP-SHA", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'CHECKSUM_MISMATCH' *", "RETURN", "ENDIF",
		"ZIP_LIMITS( EXPORTING IV_ZIP = LS_UP-DATA IMPORTING EV_CODE = LV_CODE EV_MESSAGE = LV_MESSAGE )",
		"IF LV_CODE IS NOT INITIAL", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = LV_CODE *", "RETURN", "ENDIF",
		"START_JOB( EXPORTING IV_ZIP = LS_UP-DATA IS_PARAMS = LS_UP-PARAMS IV_PUSH_ID = IV_SESSION_ID IMPORTING EV_JOBCOUNT = LV_JOBCOUNT EV_ERROR = LV_ERROR )",
	}

	// run_job: nothing outside its own job; its own protected, unchanged
	// variant; the ticket taken once; zip and parameters as scheduled.
	gitRunJob = []string{
		"IF SY-SUBRC <> 0 OR LV_JOBNAME <> C_JOB_NAME", "RETURN", "ENDIF",
		"LS_RESULT-PACKAGE = TO_UPPER( CONDENSE( CONV STRING( IV_PACKAGE ) ) )",
		"DATA(LV_OWN_VARIANT) = CONV RSVAR-VARIANT( |VSP{ LV_JOBCOUNT }| )",
		"SELECT SINGLE PROTECTED, ENAME, AENAME FROM VARID INTO @DATA(LS_VARID) WHERE REPORT = @C_JOB_NAME AND VARIANT = @LV_OWN_VARIANT",
		"DATA(LV_VARIANT_FOUND) = XSDBOOL( SY-SUBRC = 0 )",
		"SELECT SINGLE VTEXT FROM VARIT INTO @DATA(LV_VTEXT) WHERE REPORT = @C_JOB_NAME AND VARIANT = @LV_OWN_VARIANT",
		"IF SY-SUBRC <> 0 OR LV_VTEXT <> 'VSP GIT IMPORT'", "LV_VARIANT_FOUND = ABAP_FALSE", "ENDIF",
		"IF SY-SLSET <> LV_OWN_VARIANT OR LV_VARIANT_FOUND = ABAP_FALSE OR LS_VARID-PROTECTED <> 'X' OR LS_VARID-ENAME <> SY-UNAME OR ( LS_VARID-AENAME IS NOT INITIAL AND LS_VARID-AENAME <> SY-UNAME )",
		"LS_RESULT-OUTCOME = `REFUSED`", "LS_RESULT-CODE = `VARIANT_MISMATCH`", "LS_RESULT-MESSAGE = *",
		"STORE_RESULT( IV_JOBCOUNT = LV_JOBCOUNT IS_RESULT = LS_RESULT )", "JOB_LOG( LS_RESULT )",
		"PUBLISH( IS_RESULT = LS_RESULT IV_JOBCOUNT = LV_JOBCOUNT IV_PUSH_ID = IV_PUSH_ID )", "RETURN", "ENDIF",
		"LV_KEY = |VSPGITZ{ LV_JOBCOUNT }|",
		"IMPORT ZIP = LV_ZIP PARAMS = LS_PARAMS FROM DATABASE INDX(ZV) ID LV_KEY",
		"DATA(LV_FOUND) = XSDBOOL( SY-SUBRC = 0 )",
		"DELETE FROM DATABASE INDX(ZV) ID LV_KEY", "COMMIT WORK",
		"IF LV_FOUND = ABAP_FALSE", "LS_RESULT-OUTCOME = `REFUSED`", "LS_RESULT-CODE = `TICKET_MISSING`", "LS_RESULT-MESSAGE = *",
		"ELSEIF SHA_B64( SHA256( LV_ZIP ) ) <> CONDENSE( CONV STRING( IV_ZIP_SHA ) ) OR META_SHA( LS_PARAMS ) <> CONDENSE( CONV STRING( IV_META_SHA ) ) OR LS_PARAMS-PACKAGE <> LS_RESULT-PACKAGE",
		"LS_RESULT-OUTCOME = `REFUSED`", "LS_RESULT-CODE = `TICKET_MISMATCH`", "LS_RESULT-MESSAGE = *",
		"ELSE", "LS_RESULT = DO_IMPORT( IV_ZIP = LV_ZIP IS_PARAMS = LS_PARAMS )", "ENDIF",
	}

	// import_status: only the caller's own job, by its job row, and by the
	// user its result records (the only proof once SM37's row is gone).
	gitStatusOwner = []string{
		"LV_JOBNAME = C_JOB_NAME", "LV_JOBCOUNT = LV_JOB",
		"SELECT SINGLE STATUS, SDLUNAME FROM TBTCO INTO @DATA(LS_JOB) WHERE JOBNAME = @LV_JOBNAME AND JOBCOUNT = @LV_JOBCOUNT",
		"DATA(LV_FOUND) = XSDBOOL( SY-SUBRC = 0 )",
		"IF LV_FOUND = ABAP_TRUE AND LS_JOB-SDLUNAME <> SY-UNAME", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'NOT_YOUR_JOB' *", "RETURN", "ENDIF",
		"LV_KEY = |VSPGITR{ LV_JOB }|",
		"SELECT SINGLE USERA FROM INDX INTO @DATA(LV_RESULT_OWNER) WHERE RELID = 'ZV' AND SRTFD = @LV_KEY AND SRTF2 = 0",
		"IF SY-SUBRC = 0 AND LV_RESULT_OWNER <> SY-UNAME", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'NOT_YOUR_JOB' *", "RETURN", "ENDIF",
		"IMPORT RESULT = LV_JSON FROM DATABASE INDX(ZV) TO LS_INDX ID LV_KEY",
		"DATA(LV_HAS_RESULT) = XSDBOOL( SY-SUBRC = 0 AND LV_JSON IS NOT INITIAL )",
		"IF LV_HAS_RESULT = ABAP_TRUE AND LS_INDX-USERA <> SY-UNAME", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'NOT_YOUR_JOB' *", "RETURN", "ENDIF",
	}
	gitStoreOwner = []string{"LV_KEY = |VSPGITR{ IV_JOBCOUNT }|", "LS_INDX-AEDAT = SY-DATUM", "LS_INDX-USERA = SY-UNAME"}

	// delete_repo: the repository of exactly this package (and key), only an
	// offline one, only of an empty package, with S_DEVELOP; then its row.
	gitDeleteRepo = []string{
		"LV_DEVCLASS = LV_PACKAGE", "LV_OBJ_NAME = LV_PACKAGE",
		"AUTHORITY-CHECK OBJECT 'S_DEVELOP' ID 'DEVCLASS' FIELD LV_DEVCLASS ID 'OBJTYPE' FIELD 'DEVC' ID 'OBJNAME' FIELD LV_OBJ_NAME ID 'P_GROUP' DUMMY ID 'ACTVT' FIELD '06'",
		"IF SY-SUBRC <> 0", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'NOT_AUTHORIZED' *", "RETURN", "ENDIF",
		"TRY",
		"DATA(LT_REPOS) = ZCL_ABAPGIT_PERSIST_FACTORY=>GET_REPO( )->LIST( )",
		"READ TABLE LT_REPOS INTO DATA(LS_REPO) WITH KEY PACKAGE = LV_PACKAGE",
		"IF SY-SUBRC <> 0", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'REPO_NOT_FOUND' *", "RETURN", "ENDIF",
		"IF LV_REPO_KEY IS NOT INITIAL AND LV_REPO_KEY <> LS_REPO-KEY", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'REPO_KEY_MISMATCH' *", "RETURN", "ENDIF",
		"LI_REPO = ZCL_ABAPGIT_REPO_SRV=>GET_INSTANCE( )->GET( LS_REPO-KEY )",
		"IF LV_EXPECT_NAME IS NOT INITIAL AND LI_REPO->GET_NAME( ) <> LV_EXPECT_NAME", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'REPO_NAME_MISMATCH' *", "RETURN", "ENDIF",
		"IF LI_REPO->IS_OFFLINE( ) = ABAP_FALSE", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'REPO_ONLINE' *", "RETURN", "ENDIF",
		"SELECT SINGLE OBJ_NAME FROM TADIR INTO @DATA(LV_LEFT) WHERE DEVCLASS = @LV_DEVCLASS AND DELFLAG = @SPACE AND NOT ( PGMID = 'R3TR' AND OBJECT = 'DEVC' AND OBJ_NAME = @LV_OBJ_NAME )",
		"IF SY-SUBRC = 0", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'PACKAGE_NOT_EMPTY' *", "RETURN", "ENDIF",
		"SELECT SINGLE DEVCLASS FROM TDEVC INTO @DATA(LV_CHILD) WHERE PARENTCL = @LV_DEVCLASS",
		"IF SY-SUBRC = 0", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'PACKAGE_NOT_EMPTY' *", "RETURN", "ENDIF",
		"DATA(LV_NAME) = LI_REPO->GET_NAME( )",
		"ZCL_ABAPGIT_REPO_SRV=>GET_INSTANCE( )->DELETE( LI_REPO )",
	}
	// package_objects reports the repository of every row it finds, its
	// state unknown when abapGit cannot open it, and fails when the list
	// cannot be read: Go must never miss a registration.
	gitPackageObjectsRepo = []string{
		"TRY",
		"DATA(LT_REPOS) = ZCL_ABAPGIT_PERSIST_FACTORY=>GET_REPO( )->LIST( )",
		"CATCH ZCX_ABAPGIT_EXCEPTION INTO DATA(LX_LIST)",
		"RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'REPO_LIST_FAILED' *", "RETURN",
		"ENDTRY",
		"READ TABLE LT_REPOS INTO DATA(LS_REPO) WITH KEY PACKAGE = LV_PACKAGE",
		"IF SY-SUBRC = 0",
		"LV_REPO_STATE = `UNKNOWN`",
		"LV_REPO_NAME = LS_REPO-KEY",
		"TRY",
		"DATA(LI_REPO) = ZCL_ABAPGIT_REPO_SRV=>GET_INSTANCE( )->GET( LS_REPO-KEY )",
		"LV_REPO_NAME = LI_REPO->GET_NAME( )",
		"LV_REPO_STATE = COND #( WHEN LI_REPO->IS_OFFLINE( ) = ABAP_TRUE THEN `OFFLINE` ELSE `ONLINE` )",
		"CATCH ZCX_ABAPGIT_EXCEPTION ##NO_HANDLER",
		"ENDTRY",
		"LV_REPO = ZCL_VSP_UTILS=>JSON_OBJ( ZCL_VSP_UTILS=>JSON_JOIN( VALUE #( ( ZCL_VSP_UTILS=>JSON_STR( IV_KEY = 'KEY' IV_VALUE = CONV #( LS_REPO-KEY ) ) ) " +
			"( ZCL_VSP_UTILS=>JSON_STR( IV_KEY = 'NAME' IV_VALUE = LV_REPO_NAME ) ) ( ZCL_VSP_UTILS=>JSON_BOOL( IV_KEY = 'OFFLINE' IV_VALUE = XSDBOOL( LV_REPO_STATE = `OFFLINE` ) ) ) " +
			"( ZCL_VSP_UTILS=>JSON_STR( IV_KEY = 'REPO_STATE' IV_VALUE = LV_REPO_STATE ) ) ) ) )",
		"ENDIF",
		"RS_RESPONSE = ZCL_VSP_UTILS=>BUILD_SUCCESS( *",
	}

	// housekeeping, whole: variants and zips of ended (or vanished) jobs,
	// results older than a week, and this user's never-released jobs a day on.
	gitHousekeeping = []string{
		"DATA: LV_JOBNAME TYPE TBTCJOB-JOBNAME, LV_JOBCOUNT TYPE TBTCJOB-JOBCOUNT, LV_KEY TYPE INDX-SRTFD, LV_REPORT TYPE SYREPID",
		"LV_JOBNAME = C_JOB_NAME",
		"LV_REPORT = C_JOB_NAME",
		"SELECT VARIANT FROM VARID INTO TABLE @DATA(LT_OLD) WHERE REPORT = @C_JOB_NAME AND VARIANT LIKE 'VSP%'",
		"LOOP AT LT_OLD INTO DATA(LS_OLD)",
		"LV_JOBCOUNT = LS_OLD-VARIANT+3",
		"SELECT SINGLE STATUS FROM TBTCO INTO @DATA(LV_STATUS) WHERE JOBNAME = @LV_JOBNAME AND JOBCOUNT = @LV_JOBCOUNT",
		"IF SY-SUBRC <> 0 OR LV_STATUS = 'F' OR LV_STATUS = 'A'",
		"CALL FUNCTION 'RS_VARIANT_DELETE' EXPORTING REPORT = LV_REPORT VARIANT = LS_OLD-VARIANT FLAG_CONFIRMSCREEN = 'X' SUPPRESS_MESSAGE = 'X' SUPPRESS_INPUT_DIALOG = 'X' EXCEPTIONS OTHERS = 1",
		"LV_KEY = |VSPGITZ{ LV_JOBCOUNT }|",
		"DELETE FROM DATABASE INDX(ZV) ID LV_KEY",
		"ENDIF",
		"ENDLOOP",
		"DATA(LV_CUTOFF) = CONV D( SY-DATUM - 7 )",
		"DELETE FROM INDX WHERE RELID = 'ZV' AND SRTFD LIKE 'VSPGITR%' AND AEDAT < @LV_CUTOFF",
		"DATA(LV_STALE) = CONV D( SY-DATUM - 1 )",
		"SELECT JOBCOUNT FROM TBTCO INTO TABLE @DATA(LT_STALE) WHERE JOBNAME = @LV_JOBNAME AND STATUS = 'P' AND SDLUNAME = @SY-UNAME AND SDLDATE < @LV_STALE",
		"LOOP AT LT_STALE INTO DATA(LS_STALE)",
		"DROP_JOB( LS_STALE-JOBCOUNT )",
		"ENDLOOP",
	}

	// drop_job, whole: only a ZVSP_GIT_IMPORT job of this user, its own
	// variant and its own zip.
	gitDropJob = []string{
		"DATA: LV_JOBNAME TYPE TBTCJOB-JOBNAME, LV_JOBCOUNT TYPE TBTCJOB-JOBCOUNT, LV_VARIANT TYPE RSVAR-VARIANT, LV_KEY TYPE INDX-SRTFD, LV_REPORT TYPE SYREPID",
		"LV_JOBNAME = C_JOB_NAME",
		"LV_REPORT = C_JOB_NAME",
		"LV_JOBCOUNT = IV_JOBCOUNT",
		"SELECT SINGLE SDLUNAME FROM TBTCO INTO @DATA(LV_OWNER) WHERE JOBNAME = @LV_JOBNAME AND JOBCOUNT = @LV_JOBCOUNT",
		"IF SY-SUBRC = 0 AND LV_OWNER <> SY-UNAME", "RETURN", "ENDIF",
		"CALL FUNCTION 'BP_JOB_DELETE' EXPORTING JOBCOUNT = LV_JOBCOUNT JOBNAME = LV_JOBNAME FORCEDMODE = 'X' EXCEPTIONS OTHERS = 1",
		"LV_VARIANT = |VSP{ LV_JOBCOUNT }|",
		"CALL FUNCTION 'RS_VARIANT_DELETE' EXPORTING REPORT = LV_REPORT VARIANT = LV_VARIANT FLAG_CONFIRMSCREEN = 'X' SUPPRESS_MESSAGE = 'X' SUPPRESS_INPUT_DIALOG = 'X' EXCEPTIONS OTHERS = 1",
		"LV_KEY = |VSPGITZ{ LV_JOBCOUNT }|",
		"DELETE FROM DATABASE INDX(ZV) ID LV_KEY",
		"COMMIT WORK",
	}

	// delete_repo's expected name is the caller's "name", nothing else.
	gitDeleteRepoExpectName = "DATA(LV_EXPECT_NAME) = ZCL_VSP_UTILS=>EXTRACT_PARAM( IV_PARAMS = IS_MESSAGE-PARAMS IV_NAME = 'NAME' )"

	// object_versions: a package name, S_DEVELOP display on it, 1 to
	// c_max_versions well-formed "TYPE NAME" items -- each refusing -- and
	// a version only for an object whose TADIR package is this package.
	gitObjectVersions = []string{
		"IF VALID_PACKAGE( LV_PACKAGE ) = ABAP_FALSE", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'INVALID_PARAM' *", "RETURN", "ENDIF",
		"LV_DEVCLASS = LV_PACKAGE",
		"AUTHORITY-CHECK OBJECT 'S_DEVELOP' ID 'DEVCLASS' FIELD LV_DEVCLASS ID 'OBJTYPE' DUMMY ID 'OBJNAME' DUMMY ID 'P_GROUP' DUMMY ID 'ACTVT' FIELD '03'",
		"IF SY-SUBRC <> 0", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'NOT_AUTHORIZED' *", "RETURN", "ENDIF",
		"SPLIT LV_OBJECTS AT ',' INTO TABLE DATA(LT_PARTS)",
		"IF LINES( LT_PARTS ) = 0 OR LINES( LT_PARTS ) > C_MAX_VERSIONS", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'INVALID_PARAM' *", "RETURN", "ENDIF",
		"LOOP AT LT_PARTS INTO DATA(LV_PART)",
		"SPLIT CONDENSE( LV_PART ) AT SPACE INTO DATA(LV_T) DATA(LV_N) DATA(LV_REST)",
		"IF LV_T IS INITIAL OR LV_N IS INITIAL OR LV_REST IS NOT INITIAL OR STRLEN( LV_T ) > 4 OR STRLEN( LV_N ) > 40",
		"RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'INVALID_PARAM' *", "RETURN", "ENDIF",
		"LV_TYPE = LV_T", "LV_NAME = LV_N",
		"CLEAR: LV_STAMP, LV_STAMP_ERR, LV_SHA, LV_SHA_ERR, LV_FILES, LV_INACTIVE",
		"CLEAR LS_TADIR",
		"SELECT SINGLE DEVCLASS, MASTERLANG FROM TADIR WHERE PGMID = 'R3TR' AND OBJECT = @LV_TYPE AND OBJ_NAME = @LV_NAME AND DELFLAG = @SPACE INTO CORRESPONDING FIELDS OF @LS_TADIR",
		"DATA(LV_IN) = XSDBOOL( SY-SUBRC = 0 AND LS_TADIR-DEVCLASS = LV_DEVCLASS )",
		"IF LV_IN = ABAP_TRUE",
		"OBJECT_STAMP( EXPORTING IV_TYPE = LV_TYPE IV_NAME = LV_NAME IMPORTING EV_STAMP = LV_STAMP EV_ERROR = LV_STAMP_ERR )",
		"LV_INACTIVE = OBJECT_INACTIVE( IV_TYPE = LV_TYPE IV_NAME = LV_NAME )",
		"IF LV_WANT_SHA = ABAP_TRUE",
		"OBJECT_SHA256( *",
		"ENDIF",
		"ENDIF",
	}

	// object_stamp, whole: the documented v2 stamp -- every dated version
	// row, active and inactive, of exactly the object's own rows (CS
	// excluded), their number, and the digest of its dateless tables;
	// another type has none, and says so.
	gitObjectStamp = []string{
		"TYPES: BEGIN OF TY_ROW, D TYPE D, T TYPE T, END OF TY_ROW",
		"DATA: LT_ROWS TYPE STANDARD TABLE OF TY_ROW WITH DEFAULT KEY, LT_DIGEST TYPE STRING_TABLE, LV_XML TYPE XSTRING, LV_TABLES TYPE STRING, LV_POOL TYPE STRING, LV_LIKE TYPE STRING, LV_TEXT TYPE PROGNAME, LV_DIGEST TYPE STRING, LV_NEWEST TYPE STRING",
		"CLEAR: EV_STAMP, EV_ERROR",
		"CASE IV_TYPE",
		"WHEN 'CLAS' OR 'INTF'",
		"LV_TABLES = `REPOSRC.REPOTEXT.SEOCLASSDF.SEOCLASSTX.SEOCOMPOTX`",
		"LV_POOL = |{ IV_NAME WIDTH = 30 PAD = '=' }|",
		"LV_LIKE = |{ LV_POOL }%|",
		"SELECT PROGNAME, UDAT, UTIME FROM REPOSRC WHERE PROGNAME LIKE @LV_LIKE INTO TABLE @DATA(LT_POOL)",
		"LOOP AT LT_POOL INTO DATA(LS_POOL)",
		"IF STRLEN( LV_POOL ) = 30 AND LS_POOL-PROGNAME(30) = LV_POOL AND LS_POOL-PROGNAME+30 <> 'CS'",
		"APPEND VALUE #( D = LS_POOL-UDAT T = LS_POOL-UTIME ) TO LT_ROWS",
		"ENDIF",
		"ENDLOOP",
		"LV_TEXT = COND #( WHEN IV_TYPE = 'CLAS' THEN |{ LV_POOL }CP| ELSE |{ LV_POOL }IP| )",
		"SELECT UDAT AS D, UTIME AS T FROM REPOTEXT WHERE PROGNAME = @LV_TEXT APPENDING CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT * FROM SEOCLASSDF WHERE CLSNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_SEODF)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_SEODF RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"SELECT * FROM SEOCLASSTX WHERE CLSNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_SEOTX)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_SEOTX RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"SELECT * FROM SEOCOMPOTX WHERE CLSNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_SEOCTX)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_SEOCTX RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"WHEN 'PROG'",
		"LV_TABLES = `REPOSRC.REPOTEXT.D020S`",
		"SELECT UDAT AS D, UTIME AS T FROM REPOSRC WHERE PROGNAME = @IV_NAME INTO CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT UDAT AS D, UTIME AS T FROM REPOTEXT WHERE PROGNAME = @IV_NAME APPENDING CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT DGEN AS D, TGEN AS T FROM D020S WHERE PROG = @IV_NAME APPENDING CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"WHEN 'TABL'",
		"LV_TABLES = `DD02L.DD09L.DD12L.DD02T.DD35L.TDDAT`",
		"SELECT AS4DATE AS D, AS4TIME AS T FROM DD02L WHERE TABNAME = @IV_NAME INTO CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT AS4DATE AS D, AS4TIME AS T FROM DD09L WHERE TABNAME = @IV_NAME APPENDING CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT AS4DATE AS D, AS4TIME AS T FROM DD12L WHERE SQLTAB = @IV_NAME APPENDING CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT * FROM DD02T WHERE TABNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_DD02T)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_DD02T RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"SELECT * FROM DD35L WHERE TABNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_DD35L)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_DD35L RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"SELECT * FROM TDDAT WHERE TABNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_TDDAT)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_TDDAT RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"WHEN 'DTEL'",
		"LV_TABLES = `DD04L.DD04T`",
		"SELECT AS4DATE AS D, AS4TIME AS T FROM DD04L WHERE ROLLNAME = @IV_NAME INTO CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT * FROM DD04T WHERE ROLLNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_DD04T)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_DD04T RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"WHEN 'DOMA'",
		"LV_TABLES = `DD01L.DD01T.DD07L.DD07T`",
		"SELECT AS4DATE AS D, AS4TIME AS T FROM DD01L WHERE DOMNAME = @IV_NAME INTO CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT * FROM DD01T WHERE DOMNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_DD01T)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_DD01T RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"SELECT * FROM DD07L WHERE DOMNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_DD07L)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_DD07L RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"SELECT * FROM DD07T WHERE DOMNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_DD07T)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_DD07T RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"WHEN 'TTYP'",
		"LV_TABLES = `DD40L.DD40T`",
		"SELECT AS4DATE AS D, AS4TIME AS T FROM DD40L WHERE TYPENAME = @IV_NAME INTO CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT * FROM DD40T WHERE TYPENAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_DD40T)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_DD40T RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"WHEN 'DDLS'",
		"LV_TABLES = `DDDDLSRC.DDDDLSRCT`",
		"SELECT AS4DATE AS D, AS4TIME AS T FROM DDDDLSRC WHERE DDLNAME = @IV_NAME INTO CORRESPONDING FIELDS OF TABLE @LT_ROWS",
		"SELECT * FROM DDDDLSRCT WHERE DDLNAME = @IV_NAME ORDER BY PRIMARY KEY INTO TABLE @DATA(LT_DDLST)",
		"CALL TRANSFORMATION ID SOURCE ROWS = LT_DDLST RESULT XML LV_XML",
		"APPEND SHA256( LV_XML ) TO LT_DIGEST",
		"WHEN OTHERS",
		"EV_ERROR = |NO STAMP FOR TYPE { IV_TYPE }: ONLY CLAS, INTF, PROG, TABL, DTEL, DOMA, TTYP AND DDLS HAVE ONE|",
		"RETURN",
		"ENDCASE",
		"IF LT_ROWS IS INITIAL",
		"RETURN",
		"ENDIF",
		"LOOP AT LT_ROWS INTO DATA(LS_ROW)",
		"IF |{ LS_ROW-D }{ LS_ROW-T }| > LV_NEWEST",
		"LV_NEWEST = |{ LS_ROW-D }{ LS_ROW-T }|",
		"ENDIF",
		"ENDLOOP",
		"LV_DIGEST = TO_LOWER( SHA256( CL_ABAP_CODEPAGE=>CONVERT_TO( CONCAT_LINES_OF( TABLE = LT_DIGEST SEP = `,` ) ) ) )",
		"IF STRLEN( LV_DIGEST ) <> 64",
		"EV_ERROR = `THE STAMP'S DIGEST COULD NOT BE COMPUTED`",
		"RETURN",
		"ENDIF",
		"EV_STAMP = |V2:{ LV_TABLES }:{ LV_NEWEST }:{ LINES( LT_ROWS ) }:{ LV_DIGEST(16) }|",
	}

	// object_inactive, whole: the inactive worklist, for the object and
	// its parts.
	gitObjectInactive = []string{
		"DATA: LV_LIKE TYPE STRING, LV_POOL TYPE STRING",
		"RV_INACTIVE = ABAP_FALSE",
		"LV_POOL = |{ IV_NAME WIDTH = 30 PAD = '=' }|",
		"LV_LIKE = |{ IV_NAME }%|",
		"SELECT OBJECT, OBJ_NAME FROM DWINACTIV WHERE OBJ_NAME LIKE @LV_LIKE INTO TABLE @DATA(LT_INACTIVE)",
		"LOOP AT LT_INACTIVE INTO DATA(LS_INACTIVE)",
		"IF LS_INACTIVE-OBJ_NAME = IV_NAME OR LS_INACTIVE-OBJ_NAME(30) = IV_NAME OR ( ( IV_TYPE = 'CLAS' OR IV_TYPE = 'INTF' ) AND LS_INACTIVE-OBJ_NAME(30) = LV_POOL )",
		"RV_INACTIVE = ABAP_TRUE",
		"RETURN",
		"ENDIF",
		"ENDLOOP",
	}

	// object_sha256, whole: abapGit's serialisation in the main language
	// only; a failure, no file, or a hash that is not 64 digits is an
	// error and no hash.
	gitObjectSHA256 = []string{
		"DATA: LT_LINES TYPE STRING_TABLE, LS_FILES TYPE ZIF_ABAPGIT_OBJECTS=>TY_SERIALIZATION",
		"CLEAR: EV_SHA256, EV_FILES, EV_ERROR",
		"TRY",
		"LS_FILES = ZCL_ABAPGIT_OBJECTS=>SERIALIZE( IS_ITEM = VALUE #( OBJ_TYPE = IV_TYPE OBJ_NAME = IV_NAME DEVCLASS = IV_DEVCLASS ) IO_I18N_PARAMS = ZCL_ABAPGIT_I18N_PARAMS=>NEW( IV_MAIN_LANGUAGE = IV_LANGUAGE IV_MAIN_LANGUAGE_ONLY = ABAP_TRUE ) )",
		"CATCH CX_ROOT INTO DATA(LX_ERROR)",
		"EV_ERROR = |ABAPGIT COULD NOT SERIALISE IT: { LX_ERROR->GET_TEXT( ) }|",
		"RETURN",
		"ENDTRY",
		"LOOP AT LS_FILES-FILES INTO DATA(LS_FILE)",
		"APPEND |{ LS_FILE-FILENAME }={ TO_LOWER( SHA256( LS_FILE-DATA ) ) }| TO LT_LINES",
		"ENDLOOP",
		"IF LT_LINES IS INITIAL",
		"EV_ERROR = `ABAPGIT SERIALISED NO FILE OF IT`",
		"RETURN",
		"ENDIF",
		"SORT LT_LINES",
		"DATA(LV_TEXT) = CONCAT_LINES_OF( TABLE = LT_LINES SEP = CL_ABAP_CHAR_UTILITIES=>NEWLINE )",
		"EV_SHA256 = TO_LOWER( SHA256( CL_ABAP_CODEPAGE=>CONVERT_TO( LV_TEXT ) ) )",
		"IF STRLEN( EV_SHA256 ) <> 64",
		"CLEAR EV_SHA256",
		"EV_ERROR = `THE SHA-256 COULD NOT BE COMPUTED`",
		"RETURN",
		"ENDIF",
		"EV_FILES = LINES( LT_LINES )",
	}

	gitPackageObjectsAuth = []string{
		"LV_DEVCLASS = LV_PACKAGE",
		"AUTHORITY-CHECK OBJECT 'S_DEVELOP' ID 'DEVCLASS' FIELD LV_DEVCLASS ID 'OBJTYPE' DUMMY ID 'OBJNAME' DUMMY ID 'P_GROUP' DUMMY ID 'ACTVT' FIELD '03'",
		"IF SY-SUBRC <> 0", "RS_RESPONSE = ERR( IV_ID = IS_MESSAGE-ID IV_CODE = 'NOT_AUTHORIZED' *", "RETURN", "ENDIF",
		"SELECT SINGLE DEVCLASS FROM TDEVC INTO @DATA(LV_EXISTS) WHERE DEVCLASS = @LV_PACKAGE",
	}
)

// inOrder says every index is found and each comes after the one before.
func inOrder(idx ...int) bool {
	for i, x := range idx {
		if x < 0 || (i > 0 && x <= idx[i-1]) {
			return false
		}
	}
	return true
}

// checkGitPolicy pins the import's decisions to the caller's choices.
func checkGitPolicy(stmts []string) []string {
	var bad []string
	need := func(method string, seq []string, what string) int {
		i := seqAt(methodStatements(stmts, method), seq...)
		if i < 0 {
			bad = append(bad, strings.ToLower(method)+": "+what)
		}
		return i
	}

	ev := methodStatements(stmts, "EVALUATE_CHECKS")
	if len(ev) == 0 {
		return []string{"evaluate_checks is missing"}
	}
	// Each WHEN branch of the action CASE: what it sets.
	branch := map[string][]string{}
	cur := ""
	for _, st := range ev {
		up := strings.ToUpper(st)
		switch {
		case strings.HasPrefix(up, "WHEN "):
			cur = up
		case up == "ENDCASE":
			cur = ""
		case cur != "":
			branch[cur] = append(branch[cur], up)
		}
	}
	joined := func(k string) string { return strings.Join(branch[k], "\n") }
	if b := joined("WHEN LC_DELETE"); !strings.Contains(b, "<LS_OVER>-DECISION = 'N'") || strings.Contains(b, "<LS_OVER>-DECISION = 'Y'") {
		bad = append(bad, "a local object the zip does not have must always be kept (decision 'N')")
	}
	if b := joined("WHEN LC_UPDATE OR LC_OVERWRITE OR LC_DELETE_ADD"); !regexp.MustCompile(`(^|\n)IF IS_PARAMS-OVERWRITE = ABAP_TRUE\n<LS_OVER>-DECISION = 'Y'\nELSE\n<LS_OVER>-DECISION = 'N'\nAPPEND `).MatchString(b) {
		bad = append(bad, "an existing object may be changed only with overwrite = true")
	}
	for w, code := range map[string]string{"WHEN LC_NO_SUPPORT": "UNSUPPORTED_OBJECT", "WHEN LC_PACKMOVE": "PACKAGE_CONFLICT",
		"WHEN LC_DATA_LOSS": "DATA_LOSS", "WHEN OTHERS": "UNKNOWN_ACTION"} {
		if b := branch[w]; seqAt(b, "EV_CODE = `"+code+"`", "EV_MESSAGE = *", "RETURN") < 0 || strings.Contains(strings.Join(b, "\n"), "DECISION = 'Y'") {
			bad = append(bad, w+" must refuse the import ("+code+")")
		}
	}
	// Conflicts and unmet requirements refuse before any decision; the
	// overwrite refusals and the transport after the loop.
	loop := seqAt(ev, "LOOP AT CS_CHECKS-OVERWRITE ASSIGNING FIELD-SYMBOL(<LS_OVER>)")
	for _, seq := range gitEvalRefusals {
		if i := seqAt(ev, seq...); i < 0 || loop < 0 || i > loop {
			bad = append(bad, "evaluate_checks must refuse, before any decision, on: "+seq[0]+" ("+seq[len(seq)-4]+")")
		}
	}
	if i := seqAt(ev, gitEvalPackageKept...); i < 0 || i < loop || i > seqAt(ev, "CASE <LS_OVER>-ACTION") {
		bad = append(bad, "evaluate_checks: only the target package's own entry, of a package without a repository, is kept without overwrite -- and kept ('N')")
	}
	if i := seqAt(ev, gitEvalAfterLoop...); i < 0 || i < loop {
		bad = append(bad, "evaluate_checks must refuse on objects it may not overwrite, and take only the caller's transport")
	}
	// The transport is the caller's, never one found on the system.
	for _, st := range ev {
		u := strings.ToUpper(st)
		if strings.HasPrefix(u, "CS_CHECKS-TRANSPORT-TRANSPORT = ") && u != "CS_CHECKS-TRANSPORT-TRANSPORT = IS_PARAMS-TRANSPORT" {
			bad = append(bad, "the transport may only be the caller's: "+st)
		}
	}

	// check_packages and zip_limits, whole.
	if cp := methodStatements(stmts, "CHECK_PACKAGES"); len(cp) != len(gitCheckPackages) || seqAt(cp, gitCheckPackages...) != 0 {
		bad = append(bad, "check_packages must map every file below the starting folder to a listed package, creating none (as pinned)")
	}
	if zl := methodStatements(stmts, "ZIP_LIMITS"); len(zl) != len(gitZipLimits) || seqAt(zl, gitZipLimits...) != 0 {
		bad = append(bad, "zip_limits must refuse a zip over c_max_entries or c_max_unzipped, or without exactly one .abapgit.xml (as pinned)")
	}
	up := strings.ToUpper(strings.Join(stmts, "\n"))
	for _, c := range []string{"CONSTANTS C_MAX_ENTRIES TYPE I VALUE 50000", "CONSTANTS C_MAX_UNZIPPED TYPE INT8 VALUE 209715200"} {
		if !strings.Contains(up, "\n"+c+"\n") {
			bad = append(bad, "the zip limits must be "+c)
		}
	}

	// do_import: the limits, the zip, the package, the repository, the
	// package check and the policy -- each refusing -- then deserialize.
	imp := methodStatements(stmts, "DO_IMPORT")
	limits := need("DO_IMPORT", gitImportLimits, "the zip's limits must refuse before abapGit loads it")
	load := need("DO_IMPORT", gitImportLoad, "abapGit loads the zip once")
	dot := need("DO_IMPORT", gitImportDotMissing, "a zip without .abapgit.xml must be refused (INVALID_ZIP)")
	pkg := need("DO_IMPORT", gitImportPkgMissing, "a transportable package that does not exist must be refused (PACKAGE_MISSING)")
	repo := need("DO_IMPORT", gitImportRepo, "an existing repository must be refused unless it is this package's own, offline one and overwrite is set (REPO_OTHER_PACKAGE, REPO_EXISTS, REPO_ONLINE)")
	checks := need("DO_IMPORT", gitImportChecks, "the package check and the policy must refuse, removing a repository made for the import")
	deser := need("DO_IMPORT", gitImportDeserialize, "deserialize with the checks evaluated")
	if !inOrder(limits, load, dot, pkg, repo, checks, deser) {
		bad = append(bad, "do_import must check the limits, the zip, the package, the repository, the packages and the policy, in that order, before deserialize")
	}
	if n := strings.Count(strings.ToUpper(strings.Join(imp, "\n")), "->DESERIALIZE("); n != 1 {
		bad = append(bad, "do_import must deserialize exactly once")
	}
	if n := strings.Count(up, "ZCL_ABAPGIT_ZIP=>LOAD("); n != 1 {
		bad = append(bad, "abapGit loads a zip only in do_import, after zip_limits")
	}

	need("IMPORT_BEGIN", gitBeginPackages, "the target must be among the packages listed, each a package name")
	need("IMPORT_BEGIN", gitBeginParams, "overwrite only on \"true\"")
	if i := seqAt(methodStatements(stmts, "IMPORT_BEGIN"), gitBeginOne...); i != 0 || countExact(methodStatements(stmts, "IMPORT_BEGIN"), "CLEAR MS_UPLOAD") != 0 {
		bad = append(bad, "import_begin must refuse while an upload is in progress, and never clear it (UPLOAD_IN_PROGRESS)")
	}
	need("IMPORT_COMMIT", gitCommitChecks, "the size, the SHA-256 and the zip's limits must be checked before the job starts")

	need("RUN_JOB", gitRunJob, "run_job must do nothing outside its own job, check its own unchanged variant, then the zip's and the parameters' SHA-256, before it imports")
	if n := countExact(methodStatements(stmts, "RUN_JOB"), "LS_RESULT = DO_IMPORT( IV_ZIP = LV_ZIP IS_PARAMS = LS_PARAMS )"); n != 1 || strings.Count(up, "DO_IMPORT(") != 1 {
		bad = append(bad, "do_import is called once, from run_job")
	}

	need("HANDLE_IMPORT_STATUS", gitStatusOwner, "import_status must answer only the caller's own job (NOT_YOUR_JOB, by the job row and by the result's user)")
	need("STORE_RESULT", gitStoreOwner, "the result must record its user")

	// start_job: a job that will not run is deleted again, with its
	// variant and zip, on every failure after JOB_OPEN.
	if n := countExact(methodStatements(stmts, "START_JOB"), "DROP_JOB( LV_JOBCOUNT )"); n != 4 {
		bad = append(bad, "start_job must drop the job after each of RS_CREATE_VARIANT, JOB_SUBMIT and JOB_CLOSE failing, and when it is not released")
	}

	// delete_repo deletes the repository of exactly the package named, only
	// an offline one of an empty package.
	del := methodStatements(stmts, "HANDLE_DELETE_REPO")
	need("HANDLE_DELETE_REPO", gitDeleteRepo, "delete_repo must delete only the offline repository registered for exactly that package (and key), of an empty package, with S_DEVELOP")
	if n := strings.Count(strings.ToUpper(strings.Join(del, "\n")), "->DELETE("); n != 1 {
		bad = append(bad, "delete_repo must delete one repository, once")
	}
	if strings.Contains(strings.ToUpper(strings.Join(del, "\n")), "PURGE(") {
		bad = append(bad, "delete_repo must not purge (delete objects)")
	}
	if countExact(del, gitDeleteRepoExpectName) != 1 {
		bad = append(bad, "delete_repo must take the expected repository name from its \"name\" parameter")
	}
	need("HANDLE_PACKAGE_OBJECTS", gitPackageObjectsAuth, "package_objects must check S_DEVELOP display on the package")

	// object_versions: read-only, refusing what it cannot read, and a
	// version only for an object of this package; its stamp and hash as
	// documented.
	need("HANDLE_OBJECT_VERSIONS", gitObjectVersions, "object_versions must refuse a bad package, a package without S_DEVELOP display, too many or malformed objects, and read a version only for an object of this package")
	// Every answer says whether the object is inactive: Go treats a missing
	// "inactive" as unknown and refuses a sha256 match on it.
	if ov := methodStatements(stmts, "HANDLE_OBJECT_VERSIONS"); !strings.Contains(normStmt(strings.Join(ov, " ")), "( ZCL_VSP_UTILS=>JSON_BOOL( IV_KEY = 'INACTIVE' IV_VALUE = LV_INACTIVE ) )") {
		bad = append(bad, "object_versions must answer inactive for every object")
	}
	if !strings.Contains(up, "\nCONSTANTS C_MAX_VERSIONS TYPE I VALUE 500\n") {
		bad = append(bad, "object_versions' limit must be CONSTANTS c_max_versions TYPE i VALUE 500")
	}
	if !strings.Contains(up, "\nWHEN 'OBJECT_VERSIONS'\nRS_RESPONSE = HANDLE_OBJECT_VERSIONS( IS_MESSAGE )\n") {
		bad = append(bad, "action object_versions must be handle_object_versions")
	}
	for _, w := range []struct {
		method string
		want   []string
	}{{"OBJECT_STAMP", gitObjectStamp}, {"OBJECT_SHA256", gitObjectSHA256}, {"OBJECT_INACTIVE", gitObjectInactive}} {
		if m := methodStatements(stmts, w.method); len(m) != len(w.want) || seqAt(m, w.want...) != 0 {
			bad = append(bad, strings.ToLower(w.method)+" must compute the documented version, and nothing else (as pinned)")
		}
	}
	for _, m := range []string{"HANDLE_OBJECT_VERSIONS", "OBJECT_STAMP", "OBJECT_SHA256", "OBJECT_INACTIVE"} {
		for _, st := range methodStatements(stmts, m) {
			u := normStmt(st)
			for _, kw := range []string{"COMMIT", "ROLLBACK", "CALL FUNCTION", "->DESERIALIZE(", "->DELETE(", "ENQUEUE", "DEQUEUE", "CALL METHOD"} {
				if strings.HasPrefix(u, kw) || strings.Contains(u, kw) {
					bad = append(bad, strings.ToLower(m)+" must be read-only: "+st)
				}
			}
		}
	}
	po := methodStatements(stmts, "HANDLE_PACKAGE_OBJECTS")
	if i := seqAt(po, gitPackageObjectsRepo...); i < 0 || !strings.Contains(normStmt(po[len(po)-1]), `( COND #( WHEN LV_REPO IS NOT INITIAL THEN |"REPO":{ LV_REPO }| ) )`) {
		bad = append(bad, "package_objects must report every registered repository (state unknown when abapGit cannot open it) and fail when the list cannot be read")
	}
	for _, w := range []struct {
		method string
		want   []string
	}{{"HOUSEKEEPING", gitHousekeeping}, {"DROP_JOB", gitDropJob}} {
		if m := methodStatements(stmts, w.method); len(m) != len(w.want) || seqAt(m, w.want...) != 0 {
			bad = append(bad, strings.ToLower(w.method)+" must remove only ZVSP_GIT_IMPORT jobs of this user, their own variants and VSPGIT keys (as pinned)")
		}
	}

	// INDX(ZV) is shared (VSPFIX* rows live there too): every key this
	// service reads, writes or deletes is a VSPGITZ/VSPGITR key.
	for _, st := range stmts {
		u := normStmt(st)
		if !lvKeyRe.MatchString(u) {
			continue
		}
		switch {
		case strings.HasPrefix(u, "DATA: ") && regexp.MustCompile(`\bLV_KEY TYPE INDX-SRTFD(,|$)`).MatchString(u):
		case u == "SELECT SINGLE USERA FROM INDX INTO @DATA(LV_RESULT_OWNER) WHERE RELID = 'ZV' AND SRTFD = @LV_KEY AND SRTF2 = 0":
		case lvKeyAssign.MatchString(u):
		case lvKeyUse.MatchString(u):
		default:
			bad = append(bad, "lv_key may only be a VSPGITZ/VSPGITR key of INDX(ZV): "+st)
		}
	}
	for _, st := range stmts {
		u := normStmt(st)
		if strings.HasPrefix(u, "EXPORT") && strings.Contains(u, "DATABASE") && !lvKeyUse.MatchString(u) {
			bad = append(bad, "EXPORT TO DATABASE only under lv_key: "+st)
		}
	}
	return bad
}

var (
	lvKeyRe     = regexp.MustCompile(`\bLV_KEY\b`)
	lvKeyAssign = regexp.MustCompile(`^LV_KEY = \|VSPGIT[ZR]\{ [A-Z_]+ \}\|$`)
	lvKeyUse    = regexp.MustCompile(`^(EXPORT [A-Z_]+ = [A-Z_]+( [A-Z_]+ = [A-Z_]+)* TO|IMPORT [A-Z_]+ = [A-Z_]+( [A-Z_]+ = [A-Z_]+)* FROM|DELETE FROM) DATABASE INDX\(ZV\)( FROM LS_INDX| TO LS_INDX)? ID LV_KEY$`)
)

func TestGitServiceOnlyImports(t *testing.T) {
	if bad := checkGitService(gitServiceSource(t)); len(bad) > 0 {
		t.Errorf("ZCL_VSP_GIT_SERVICE breaks its rules:\n  %s", strings.Join(bad, "\n  "))
	}
}

// TestGitServiceGuardBites mutates the real source the ways a change could
// widen what the service does, and requires the guard to catch each.
func TestGitServiceGuardBites(t *testing.T) {
	src := gitServiceSource(t)
	if len(checkGitService(src)) != 0 {
		t.Fatal("the unmutated source must pass first")
	}
	mutations := map[string]struct{ old, new string }{
		"job FM over RFC": {"CALL FUNCTION 'JOB_OPEN'\n      EXPORTING", "CALL FUNCTION 'JOB_OPEN' DESTINATION 'NONE'\n      EXPORTING"},
		"job in a new task": {"CALL FUNCTION 'BP_JOBLOG_READ'\n        EXPORTING",
			"CALL FUNCTION 'BP_JOBLOG_READ' STARTING NEW TASK 'T'\n        EXPORTING"},
		"another function":    {"CALL FUNCTION 'GET_JOB_RUNTIME_INFO'", "CALL FUNCTION 'RFC_ABAP_INSTALL_AND_RUN'"},
		"dynamic function":    {"CALL FUNCTION 'GET_JOB_RUNTIME_INFO'", "CALL FUNCTION ('GET_JOB_RUNTIME_INFO')"},
		"another job report":  {"    housekeeping( ).\n    lv_report = c_job_name.", "    housekeeping( ).\n    lv_report = iv_report."},
		"job as another user": {"        authcknam         = sy-uname", "        authcknam         = 'DDIC'"},
		"job runs a command": {"        authcknam         = sy-uname",
			"        authcknam         = sy-uname\n        extpgm_name       = lv_cmd"},
		"report literal changed": {"VALUE 'ZVSP_GIT_IMPORT'", "VALUE 'RSPARAM'"},
		"submit":                 {"    housekeeping( ).\n", "    housekeeping( ).\n    SUBMIT (lv_prog) AND RETURN.\n"},
		"call transaction":       {"    housekeeping( ).\n", "    housekeeping( ).\n    CALL TRANSACTION 'SE38'.\n"},
		"commit and wait in APC": {"    \" The ticket is on the database before the job can start.\n    COMMIT WORK.",
			"    \" The ticket is on the database before the job can start.\n    COMMIT WORK AND WAIT."},
		"write another table":  {"    COMMIT WORK.\n  ENDMETHOD.", "    COMMIT WORK.\n    DELETE FROM tadir WHERE devclass = @is_params-package.\n  ENDMETHOD."},
		"export elsewhere":     {"EXPORT result = lv_json TO DATABASE indx(zv)", "EXPORT result = lv_json TO DATABASE indx(zz)"},
		"another dynamic call": {"CALL METHOD li_srv->('NEW_OFFLINE') PARAMETER-TABLE lt_params.", "CALL METHOD li_srv->('PURGE') PARAMETER-TABLE lt_params."},
		"delete local objects": {"          ls_decision-action = `delete`.\n          <ls_over>-decision = 'N'.",
			"          ls_decision-action = `delete`.\n          <ls_over>-decision = 'Y'."},
		"overwrite regardless": {"          IF is_params-overwrite = abap_true.\n            <ls_over>-decision = 'Y'.",
			"          IF is_params-overwrite = abap_true OR 1 = 1.\n            <ls_over>-decision = 'Y'."},
		"package conflict tolerated": {"      ev_code = `PACKAGE_CONFLICT`.\n      ev_message = |Objects of the zip",
			"      ev_message = |Objects of the zip"},
		"packmove allowed": {"        WHEN lc_packmove.\n          ev_code = `PACKAGE_CONFLICT`.",
			"        WHEN lc_packmove.\n          <ls_over>-decision = 'Y'.\n          ls_decision-action = `packmove`."},
		"transport found on the system": {"      cs_checks-transport-transport = is_params-transport.",
			"      cs_checks-transport-transport = zcl_abapgit_factory=>get_default_transport( )->get( ).\n      cs_checks-transport-transport = is_params-transport."},
		"no package check": {"        lv_message = check_packages( ii_repo = li_repo it_files = lt_files is_params = is_params ).",
			"        CLEAR lv_message."},
		"import without SHA check": {"    ELSEIF sha_b64( sha256( lv_zip ) ) <> condense( CONV string( iv_zip_sha ) )",
			"    ELSEIF 1 = 2 AND sha_b64( sha256( lv_zip ) ) <> condense( CONV string( iv_zip_sha ) )"},
		"delete any repository": {"        READ TABLE lt_repos INTO DATA(ls_repo) WITH KEY package = lv_package.\n        IF sy-subrc <> 0.\n          rs_response = err( iv_id = is_message-id iv_code = 'REPO_NOT_FOUND'",
			"        READ TABLE lt_repos INTO DATA(ls_repo) INDEX 1.\n        IF sy-subrc <> 0.\n          rs_response = err( iv_id = is_message-id iv_code = 'REPO_NOT_FOUND'"},
		// Review round 2 (PR #301): every Open SQL write form, each refusal
		// of the import and of delete_repo, removed or weakened.
		"DELETE ... FROM TABLE, no WHERE": {"    COMMIT WORK.\n  ENDMETHOD.",
			"    COMMIT WORK.\n    DELETE tadir FROM TABLE @lt_rows.\n  ENDMETHOD."},
		"DELETE ... WHERE, without FROM": {"    COMMIT WORK.\n  ENDMETHOD.",
			"    COMMIT WORK.\n    DELETE tadir WHERE devclass = @lv_package.\n  ENDMETHOD."},
		"chained DELETE": {"    COMMIT WORK.\n  ENDMETHOD.",
			"    COMMIT WORK.\n    DELETE: FROM tdevc WHERE devclass = @lv_package.\n  ENDMETHOD."},
		"INSERT another table": {"    COMMIT WORK.\n  ENDMETHOD.",
			"    COMMIT WORK.\n    INSERT tadir FROM @ls_row.\n  ENDMETHOD."},
		"MODIFY another table": {"    COMMIT WORK.\n  ENDMETHOD.",
			"    COMMIT WORK.\n    MODIFY tadir FROM TABLE @lt_rows.\n  ENDMETHOD."},
		"UPDATE another table": {"    COMMIT WORK.\n  ENDMETHOD.",
			"    COMMIT WORK.\n    UPDATE tadir SET devclass = @lv_package WHERE obj_name = @lv_name.\n  ENDMETHOD."},
		"DELETE dynamic table": {"    COMMIT WORK.\n  ENDMETHOD.",
			"    COMMIT WORK.\n    DELETE FROM (lv_table) WHERE (lv_where).\n  ENDMETHOD."},
		"DELETE INDX rows of another area": {"DELETE FROM indx WHERE relid = 'ZV' AND srtfd LIKE 'VSPGITR%' AND aedat < @lv_cutoff.",
			"DELETE FROM indx WHERE relid = 'ZV' AND aedat < @lv_cutoff."},
		"EXPORT to another cluster table": {"EXPORT result = lv_json TO DATABASE indx(zv)",
			"EXPORT result = lv_json TO DATABASE zvsp_cluster(zv)"},
		"check_packages: IF 1 = 2 AND": {"      IF lv_package IS NOT INITIAL AND NOT line_exists",
			"      IF 1 = 2 AND lv_package IS NOT INITIAL AND NOT line_exists"},
		"check_packages: every file skipped": {"      IF strlen( ls_file-path ) < lv_start_len.",
			"      IF strlen( ls_file-path ) < lv_start_len OR 1 = 1."},
		"check_packages: creates packages": {"        iv_create_if_not_exists = abap_false ).",
			"        iv_create_if_not_exists = abap_true )."},
		"check_packages: no refusal": {"maps to package { lv_package }, which is not among the packages checked ({ is_params-packages }).|.\n        RETURN.",
			"maps to package { lv_package }, which is not among the packages checked ({ is_params-packages }).|.\n        CLEAR rv_message."},
		"do_import: no REPO_OTHER_PACKAGE": {"          IF li_repo->get_package( ) <> is_params-package.\n            rs_result-outcome = `refused`.\n            rs_result-code = `REPO_OTHER_PACKAGE`.\n            rs_result-message = |{ lv_reason }: the repository of { li_repo->get_package( ) } covers { is_params-package }. Nothing was imported.|.\n            RETURN.\n          ENDIF.\n",
			""},
		"do_import: no REPO_EXISTS": {"          IF is_params-overwrite = abap_false.\n            rs_result-outcome = `refused`.\n            rs_result-code = `REPO_EXISTS`.\n            rs_result-message = |{ lv_reason } (repository { rs_result-repo_key }); it is not overwritten without overwrite = true. Nothing was imported.|.\n            RETURN.\n          ENDIF.\n",
			""},
		"do_import: no PACKAGE_MISSING": {"        IF is_params-package(1) <> '$' AND zcl_abapgit_factory=>get_sap_package( is_params-package )->exists( ) = abap_false.\n          rs_result-outcome = `refused`.\n          rs_result-code = `PACKAGE_MISSING`.\n          rs_result-message = |Package { is_params-package } does not exist; only a local ($) package is created by the import. Nothing was imported.|.\n          RETURN.\n        ENDIF.\n",
			""},
		"do_import: no REPO_ONLINE": {"          IF li_repo->is_offline( ) = abap_false.\n            rs_result-outcome = `refused`.\n            rs_result-code = `REPO_ONLINE`.\n            rs_result-message = |Repository { rs_result-repo_key } of { is_params-package } is an online repository; a zip is imported into an offline one only. Nothing was imported.|.\n            RETURN.\n          ENDIF.\n",
			""},
		"do_import: no zip limits": {"    zip_limits( EXPORTING iv_zip = iv_zip IMPORTING ev_code = lv_code ev_message = lv_message ).\n    IF lv_code IS NOT INITIAL.\n      rs_result-outcome = `refused`.\n      rs_result-code = lv_code.\n      rs_result-message = |{ lv_message } Nothing was imported.|.\n      RETURN.\n    ENDIF.\n",
			""},
		"evaluate_checks: no customizing refusal": {"    IF cs_checks-customizing-required = abap_true.\n      ev_code = `CUSTOMIZING_NOT_SUPPORTED`.\n      ev_message = `The zip carries table content (customizing); that is not imported here.`.\n      RETURN.\n    ENDIF.\n",
			""},
		"evaluate_checks: requirements weakened": {"    IF cs_checks-requirements-met = 'N'.",
			"    IF cs_checks-requirements-met = 'N' AND 1 = 2."},
		"evaluate_checks: any DEVC kept": {"      IF iv_new_repo = abap_true AND <ls_over>-obj_type = 'DEVC' AND <ls_over>-obj_name = is_params-package",
			"      IF iv_new_repo = abap_true AND <ls_over>-obj_type = 'DEVC'"},
		"evaluate_checks: any object of a new repo exempt": {"      IF iv_new_repo = abap_true AND <ls_over>-obj_type = 'DEVC' AND <ls_over>-obj_name = is_params-package",
			"      IF iv_new_repo = abap_true"},
		"evaluate_checks: package entry overwritten": {"        ls_decision-action = `package_kept`.\n        <ls_over>-decision = 'N'.",
			"        ls_decision-action = `package_kept`.\n        <ls_over>-decision = 'Y'."},
		"import_begin: target not among packages": {"    IF NOT line_exists( lt_packages[ table_line = CONV devclass( lv_package ) ] ).\n      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'\n                         iv_message = |packages must list every package the zip may touch, { lv_package } among them| ).\n      RETURN.\n    ENDIF.\n",
			""},
		"import_commit: no SHA-256 check": {"    IF sha256( ls_up-data ) <> ls_up-sha.\n      rs_response = err( iv_id = is_message-id iv_code = 'CHECKSUM_MISMATCH'\n                         iv_message = `The received bytes do not match the SHA-256 declared at begin. Nothing was imported.` ).\n      RETURN.\n    ENDIF.\n",
			""},
		"import_commit: no zip limits": {"    zip_limits( EXPORTING iv_zip = ls_up-data IMPORTING ev_code = lv_code ev_message = lv_message ).\n    IF lv_code IS NOT INITIAL.\n      rs_response = err( iv_id = is_message-id iv_code = lv_code iv_message = |{ lv_message } Nothing was imported.| ).\n      RETURN.\n    ENDIF.\n",
			""},
		"zip limits: entry cap raised": {"CONSTANTS c_max_entries TYPE i VALUE 50000.",
			"CONSTANTS c_max_entries TYPE i VALUE 5000000."},
		"zip limits: size check weakened": {"    IF lv_total > c_max_unzipped.",
			"    IF lv_total > c_max_unzipped * 100."},
		"zip limits: several .abapgit.xml": {"    IF lv_dots <> 1.",
			"    IF lv_dots < 1."},
		"import_status: no NOT_YOUR_JOB by job row": {"    IF lv_found = abap_true AND ls_job-sdluname <> sy-uname.\n      rs_response = err( iv_id = is_message-id iv_code = 'NOT_YOUR_JOB'\n                         iv_message = |Job { lv_job } was scheduled by another user| ).\n      RETURN.\n    ENDIF.\n",
			""},
		"import_status: no NOT_YOUR_JOB by result user": {"    IF lv_has_result = abap_true AND ls_indx-usera <> sy-uname.\n      rs_response = err( iv_id = is_message-id iv_code = 'NOT_YOUR_JOB'\n                         iv_message = |The result of job { lv_job } belongs to another user| ).\n      RETURN.\n    ENDIF.\n",
			""},
		"store_result: no user": {"    ls_indx-usera = sy-uname.\n    ls_indx-pgmid = c_job_name.\n    DATA(lv_json)",
			"    ls_indx-pgmid = c_job_name.\n    DATA(lv_json)"},
		"run_job: aename clause dropped": {" OR ( ls_varid-aename IS NOT INITIAL AND ls_varid-aename <> sy-uname ).",
			"."},
		"run_job: vtext not checked": {"    IF sy-subrc <> 0 OR lv_vtext <> 'vsp git import'.",
			"    IF sy-subrc <> 0."},
		"start_job: job kept after JOB_CLOSE failed": {"were deleted again|.\n      drop_job( lv_jobcount ).",
			"were deleted again|."},
		"drop_job: another job": {"        jobname    = lv_jobname\n        forcedmode",
			"        jobname    = 'SAP_REORG_JOBS'\n        forcedmode"},
		"drop_job: jobname from outside": {"    lv_jobname = c_job_name.\n    lv_report = c_job_name.\n    lv_jobcount = iv_jobcount.",
			"    lv_jobname = iv_jobcount.\n    lv_report = c_job_name.\n    lv_jobcount = iv_jobcount."},
		"delete_repo: online repository unregistered": {"        IF li_repo->is_offline( ) = abap_false.\n          rs_response = err( iv_id = is_message-id iv_code = 'REPO_ONLINE'\n                             iv_message = |Repository { ls_repo-key } of { lv_package } is an online repository; vsp never unregisters one (do it in abapGit). Nothing was deleted| ).\n          RETURN.\n        ENDIF.\n",
			""},
		"delete_repo: package with objects": {"        IF sy-subrc = 0.\n          rs_response = err( iv_id = is_message-id iv_code = 'PACKAGE_NOT_EMPTY'\n                             iv_message = |Package { lv_package } still has objects ({ lv_left } and maybe more); its repository is kept. Nothing was deleted| ).\n          RETURN.\n        ENDIF.\n",
			""},
		"delete_repo: package with subpackages": {"        IF sy-subrc = 0.\n          rs_response = err( iv_id = is_message-id iv_code = 'PACKAGE_NOT_EMPTY'\n                             iv_message = |Package { lv_package } has subpackage { lv_child }; its repository is kept. Nothing was deleted| ).\n          RETURN.\n        ENDIF.\n",
			""},
		"delete_repo: no authority check": {"    IF sy-subrc <> 0.\n      rs_response = err( iv_id = is_message-id iv_code = 'NOT_AUTHORIZED'\n                         iv_message = |No authorization to delete in package { lv_package } (S_DEVELOP, activity 06); nothing was deleted| ).\n      RETURN.\n    ENDIF.\n",
			""},
		"delete_repo: own entry check widened": {"AND NOT ( pgmid = 'R3TR' AND object = 'DEVC' AND obj_name = @lv_obj_name ).",
			"AND NOT ( pgmid = 'R3TR' )."},
		// #409: object_versions and delete_repo's expected name.
		"delete_repo: expected name ignored": {"        IF lv_expect_name IS NOT INITIAL AND li_repo->get_name( ) <> lv_expect_name.\n          rs_response = err( iv_id = is_message-id iv_code = 'REPO_NAME_MISMATCH'\n                             iv_message = |The repository of { lv_package } is { ls_repo-key } { li_repo->get_name( ) }, not { lv_expect_name }; nothing was deleted| ).\n          RETURN.\n        ENDIF.\n",
			""},
		"delete_repo: expected name weakened": {"        IF lv_expect_name IS NOT INITIAL AND li_repo->get_name( ) <> lv_expect_name.",
			"        IF lv_expect_name IS NOT INITIAL AND 1 = 2."},
		"delete_repo: expected name from another param": {"DATA(lv_expect_name) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'name' ).",
			"DATA(lv_expect_name) = zcl_vsp_utils=>extract_param( iv_params = is_message-params iv_name = 'repo_name' )."},
		"object_versions: no authority check": {"      rs_response = err( iv_id = is_message-id iv_code = 'NOT_AUTHORIZED'\n                         iv_message = |No authorization to display package { lv_package } (S_DEVELOP, activity 03)| ).\n      RETURN.\n    ENDIF.\n    SPLIT lv_objects",
			"      CLEAR rs_response.\n    ENDIF.\n    SPLIT lv_objects"},
		"object_versions: bad package accepted": {"iv_name = 'sha256' ) = `true` ).\n    IF valid_package( lv_package ) = abap_false.\n      rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'\n                         iv_message = |package '{ lv_package }' is not a package name| ).\n      RETURN.\n    ENDIF.\n    lv_devclass = lv_package.\n    AUTHORITY-CHECK OBJECT 'S_DEVELOP'\n      ID 'DEVCLASS' FIELD lv_devclass\n      ID 'OBJTYPE'  DUMMY",
			"iv_name = 'sha256' ) = `true` ).\n    lv_devclass = lv_package.\n    AUTHORITY-CHECK OBJECT 'S_DEVELOP'\n      ID 'DEVCLASS' FIELD lv_devclass\n      ID 'OBJTYPE'  DUMMY"},
		"object_versions: no object limit": {"    IF lines( lt_parts ) = 0 OR lines( lt_parts ) > c_max_versions.",
			"    IF lines( lt_parts ) = 0."},
		"object_versions: limit raised": {"CONSTANTS c_max_versions TYPE i VALUE 500.", "CONSTANTS c_max_versions TYPE i VALUE 500000."},
		"object_versions: malformed object accepted": {"        rs_response = err( iv_id = is_message-id iv_code = 'INVALID_PARAM'\n                           iv_message = |object '{ condense( lv_part ) }' is not \"TYPE NAME\"| ).\n        RETURN.",
			"        CONTINUE."},
		"object_versions: another package's object versioned": {"      DATA(lv_in) = xsdbool( sy-subrc = 0 AND ls_tadir-devclass = lv_devclass ).",
			"      DATA(lv_in) = xsdbool( sy-subrc = 0 )."},
		"object_versions: version without TADIR entry": {"      DATA(lv_in) = xsdbool( sy-subrc = 0 AND ls_tadir-devclass = lv_devclass ).",
			"      DATA(lv_in) = abap_true."},
		"object_versions: deleted TADIR rows count": {"AND obj_name = @lv_name AND delflag = @space\n", "AND obj_name = @lv_name\n"},
		"object_versions: commits": {"    ENDLOOP.\n    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(\n      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = lv_package ) )\n      ( |\"objects\"",
			"    ENDLOOP.\n    COMMIT WORK.\n    rs_response = zcl_vsp_utils=>build_success( iv_id = is_message-id iv_data = zcl_vsp_utils=>json_obj( zcl_vsp_utils=>json_join( VALUE #(\n      ( zcl_vsp_utils=>json_str( iv_key = 'package' iv_value = lv_package ) )\n      ( |\"objects\""},
		"object_versions: action rerouted": {"      WHEN 'object_versions'.\n        rs_response = handle_object_versions( is_message ).",
			"      WHEN 'object_versions'.\n        rs_response = handle_package_objects( is_message )."},
		"object_stamp: active rows only": {"        SELECT udat AS d, utime AS t FROM reposrc WHERE progname = @iv_name INTO",
			"        SELECT udat AS d, utime AS t FROM reposrc WHERE progname = @iv_name AND r3state = 'A' INTO"},
		"object_stamp: row count dropped": {"    ev_stamp = |v2:{ lv_tables }:{ lv_newest }:{ lines( lt_rows ) }:{ lv_digest(16) }|.",
			"    ev_stamp = |v2:{ lv_tables }:{ lv_newest }:0:{ lv_digest(16) }|."},
		"object_stamp: digest dropped": {"    ev_stamp = |v2:{ lv_tables }:{ lv_newest }:{ lines( lt_rows ) }:{ lv_digest(16) }|.",
			"    ev_stamp = |v2:{ lv_tables }:{ lv_newest }:{ lines( lt_rows ) }:0000000000000000|."},
		"object_stamp: any include matches": {"          IF strlen( lv_pool ) = 30 AND ls_pool-progname(30) = lv_pool AND ls_pool-progname+30 <> 'CS'.",
			"          IF strlen( lv_pool ) = 30."},
		"object_stamp: CS counted": {"          IF strlen( lv_pool ) = 30 AND ls_pool-progname(30) = lv_pool AND ls_pool-progname+30 <> 'CS'.",
			"          IF strlen( lv_pool ) = 30 AND ls_pool-progname(30) = lv_pool."},
		"object_stamp: class text pool dropped":    {"        SELECT udat AS d, utime AS t FROM repotext WHERE progname = @lv_text APPENDING CORRESPONDING FIELDS OF TABLE @lt_rows.\n", ""},
		"object_stamp: dynpros dropped":            {"        SELECT dgen AS d, tgen AS t FROM d020s WHERE prog = @iv_name APPENDING CORRESPONDING FIELDS OF TABLE @lt_rows.\n", ""},
		"object_stamp: technical settings dropped": {"        SELECT as4date AS d, as4time AS t FROM dd09l WHERE tabname = @iv_name APPENDING CORRESPONDING FIELDS OF TABLE @lt_rows.\n", ""},
		"object_stamp: class descriptions dropped": {"        CALL TRANSFORMATION id SOURCE rows = lt_seotx RESULT XML lv_xml.\n        APPEND sha256( lv_xml ) TO lt_digest.\n", ""},
		"object_inactive: parts ignored": {"      IF ls_inactive-obj_name = iv_name OR ls_inactive-obj_name(30) = iv_name OR",
			"      IF ls_inactive-obj_name = iv_name OR"},
		"object_inactive: never inactive":        {"        rv_inactive = abap_true.\n        RETURN.", "        RETURN."},
		"object_versions: inactive key dropped":  {"        ( zcl_vsp_utils=>json_bool( iv_key = 'inactive' iv_value = lv_inactive ) )\n", ""},
		"object_versions: inactive key renamed":  {"json_bool( iv_key = 'inactive' iv_value = lv_inactive )", "json_bool( iv_key = 'is_inactive' iv_value = lv_inactive )"},
		"object_versions: inactive not reported": {"        lv_inactive = object_inactive( iv_type = lv_type iv_name = lv_name ).\n", ""},
		"object_stamp: unsupported type stamped": {"        ev_error = |no stamp for type { iv_type }: only CLAS, INTF, PROG, TABL, DTEL, DOMA, TTYP and DDLS have one|.\n        RETURN.",
			"        ev_stamp = `v1:NONE:00000000000000:0`.\n        RETURN."},
		"object_stamp: no rows stamped": {"    IF lt_rows IS INITIAL.\n      RETURN.\n    ENDIF.\n    LOOP AT lt_rows",
			"    LOOP AT lt_rows"},
		"object_sha256: translations included": {"                                                         iv_main_language_only = abap_true ) ).",
			"                                                         iv_main_language_only = abap_false ) )."},
		"object_sha256: failure hashed": {"        ev_error = |abapGit could not serialise it: { lx_error->get_text( ) }|.\n        RETURN.",
			"        ev_error = |abapGit could not serialise it: { lx_error->get_text( ) }|."},
		"object_sha256: lines unsorted": {"    SORT lt_lines.\n", ""},
		"package_objects: no authority check": {"    IF sy-subrc <> 0.\n      rs_response = err( iv_id = is_message-id iv_code = 'NOT_AUTHORIZED'\n                         iv_message = |No authorization to display package { lv_package } (S_DEVELOP, activity 03)| ).\n      RETURN.\n    ENDIF.\n",
			""},
		// Review round 3: user scoping of the cleanup, VSPGIT keys only in the
		// shared INDX(ZV), package_objects failing closed, and statements the
		// tokenizer used to miss.
		"housekeeping: any user's stale jobs": {" AND sdluname = @sy-uname AND sdldate < @lv_stale.",
			" AND sdldate < @lv_stale."},
		"housekeeping: stale filter dropped": {" AND sdluname = @sy-uname AND sdldate < @lv_stale.",
			" AND sdluname = @sy-uname."},
		"housekeeping: running jobs' variants too": {"      IF sy-subrc <> 0 OR lv_status = 'F' OR lv_status = 'A'.",
			"      IF 1 = 1."},
		"drop_job: variant from outside": {"        jobname    = lv_jobname\n        forcedmode = 'X'\n      EXCEPTIONS\n        OTHERS     = 1.\n    lv_variant = |VSP{ lv_jobcount }|.",
			"        jobname    = lv_jobname\n        forcedmode = 'X'\n      EXCEPTIONS\n        OTHERS     = 1.\n    lv_variant = iv_jobcount."},
		"start_job: another variant": {"\n    \" not SUBMIT, so SUBMIT ... VIA JOB is not an option.\n    lv_variant = |VSP{ lv_jobcount }|.",
			"\n    \" not SUBMIT, so SUBMIT ... VIA JOB is not an option.\n    lv_variant = |SAP&{ lv_jobcount }|."},
		"drop_job: key from outside": {"    lv_key = |VSPGITZ{ lv_jobcount }|.\n    DELETE FROM DATABASE indx(zv) ID lv_key.\n    COMMIT WORK.\n  ENDMETHOD.",
			"    lv_key = iv_jobcount.\n    DELETE FROM DATABASE indx(zv) ID lv_key.\n    COMMIT WORK.\n  ENDMETHOD."},
		"drop_job: another user's job": {"    IF sy-subrc = 0 AND lv_owner <> sy-uname.",
			"    IF 1 = 2."},
		"INDX key VSPFIX": {"    lv_key = |VSPGITZ{ lv_jobcount }|.\n    IMPORT zip",
			"    lv_key = 'VSPFIX'.\n    IMPORT zip"},
		"INDX key prefix changed": {"    lv_key = |VSPGITR{ iv_jobcount }|.",
			"    lv_key = |VSPFIX{ iv_jobcount }|."},
		"EXPORT under a literal key": {"EXPORT result = lv_json TO DATABASE indx(zv) FROM ls_indx ID lv_key.",
			"EXPORT result = lv_json TO DATABASE indx(zv) FROM ls_indx ID 'VSPFIX'."},
		"lv_key concatenated": {"    lv_key = |VSPGITR{ lv_job }|.",
			"    CONCATENATE 'VSPFIX' lv_job INTO lv_key."},
		"package_objects: list failure swallowed": {"      CATCH zcx_abapgit_exception INTO DATA(lx_list).\n        rs_response = err( iv_id = is_message-id iv_code = 'REPO_LIST_FAILED'\n                           iv_message = |abapGit's repository list cannot be read: { clean( lx_list->get_text( ) ) }| ).\n        RETURN.\n    ENDTRY.\n",
			"      CATCH zcx_abapgit_exception ##NO_HANDLER.\n    ENDTRY.\n"},
		"package_objects: unknown reads as offline": {"      lv_repo_state = `unknown`.",
			"      lv_repo_state = `offline`."},
		"package_objects: row dropped when abapGit throws": {"        CATCH zcx_abapgit_exception ##NO_HANDLER.\n      ENDTRY.\n      lv_repo = ",
			"        CATCH zcx_abapgit_exception.\n          RETURN.\n      ENDTRY.\n      lv_repo = "},
		"zip limits: negative size accepted": {"      IF ls_file-size < 0.",
			"      IF ls_file-size < -1."},
		"statement glued after a period": {"\n    DELETE FROM DATABASE indx(zv) ID lv_key.\n    COMMIT WORK.\n  ENDMETHOD.",
			"\n    DELETE FROM DATABASE indx(zv) ID lv_key.\n    COMMIT WORK.DELETE FROM tadir WHERE devclass = @lv_package.\n  ENDMETHOD."},
		"statement hidden by an escaped template bar": {"\n    DELETE FROM DATABASE indx(zv) ID lv_key.\n    COMMIT WORK.\n  ENDMETHOD.",
			"\n    DELETE FROM DATABASE indx(zv) ID lv_key.\n    COMMIT WORK.\n    DATA(lv_x) = |a\\\\| \". |. DELETE FROM tadir WHERE devclass = @lv_package.\n  ENDMETHOD."},
		"import_begin: second begin discards the upload": {"    IF ms_upload-id IS NOT INITIAL.\n      rs_response = err( iv_id = is_message-id iv_code = 'UPLOAD_IN_PROGRESS'\n                         iv_message = |Upload { ms_upload-id } is in progress in this session; commit or abort it first. Nothing was imported.| ).\n      RETURN.\n    ENDIF.\n",
			"    CLEAR ms_upload.\n"},
		"import_begin: in-progress check removed": {"    IF ms_upload-id IS NOT INITIAL.\n      rs_response = err( iv_id = is_message-id iv_code = 'UPLOAD_IN_PROGRESS'\n                         iv_message = |Upload { ms_upload-id } is in progress in this session; commit or abort it first. Nothing was imported.| ).\n      RETURN.\n    ENDIF.\n",
			""},
		"import_status: owner checked only after IMPORT": {"    SELECT SINGLE usera FROM indx INTO @DATA(lv_result_owner)\n      WHERE relid = 'ZV' AND srtfd = @lv_key AND srtf2 = 0.\n    IF sy-subrc = 0 AND lv_result_owner <> sy-uname.\n      rs_response = err( iv_id = is_message-id iv_code = 'NOT_YOUR_JOB'\n                         iv_message = |The result of job { lv_job } belongs to another user| ).\n      RETURN.\n    ENDIF.\n",
			""},
	}
	for name, m := range mutations {
		if !strings.Contains(src, m.old) {
			t.Errorf("%s: the source no longer contains %q; update the mutation", name, m.old)
			continue
		}
		mutated := strings.Replace(src, m.old, m.new, 1)
		if len(checkGitService(mutated)) == 0 {
			t.Errorf("%s: the guard accepted the mutated source", name)
		}
	}
}

// The job program is a shim: its parameters, and one call of run_job.
func checkGitJobProgram(src string) []string {
	var bad []string
	stmts := abapStatements(src)
	want := []string{
		"REPORT ZVSP_GIT_IMPORT",
		"PARAMETERS: P_PKG TYPE DEVCLASS, P_SHZ TYPE C LENGTH 44 LOWER CASE, P_SHM TYPE C LENGTH 44 LOWER CASE, P_PUSH TYPE C LENGTH 60 LOWER CASE",
		"START-OF-SELECTION",
		"ZCL_VSP_GIT_SERVICE=>RUN_JOB( IV_PACKAGE = P_PKG IV_ZIP_SHA = P_SHZ IV_META_SHA = P_SHM IV_PUSH_ID = P_PUSH )",
	}
	if len(stmts) != len(want) {
		bad = append(bad, "the job program must be exactly REPORT, PARAMETERS, START-OF-SELECTION and the run_job call")
		return bad
	}
	for i, st := range stmts {
		if strings.ToUpper(st) != want[i] {
			bad = append(bad, "unexpected statement: "+st)
		}
	}
	return bad
}

func TestGitJobProgramIsAShim(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "src", "zvsp_git_import.prog.abap"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	if bad := checkGitJobProgram(src); len(bad) > 0 {
		t.Errorf("ZVSP_GIT_IMPORT: %s", strings.Join(bad, "; "))
	}
	mutated := strings.Replace(src, "START-OF-SELECTION.", "START-OF-SELECTION.\n  SUBMIT zother AND RETURN.", 1)
	if len(checkGitJobProgram(mutated)) == 0 {
		t.Error("the guard accepted a job program that submits another report")
	}
}

// The APC handler names neither optional service statically, so ZADT_VSP
// activates and runs where abapGit is missing; and it binds each WebSocket to
// its own extension of ZVSP_GIT /import, going on without push if that fails.
func TestAPCHandlerGitServiceIsOptional(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "src", "zcl_vsp_apc_handler.clas.abap"))
	if err != nil {
		t.Fatal(err)
	}
	up := strings.ToUpper(string(b))
	if strings.Contains(up, "NEW ZCL_VSP_GIT_SERVICE(") || strings.Contains(up, "TYPE REF TO ZCL_VSP_GIT_SERVICE") {
		t.Error("the APC handler names ZCL_VSP_GIT_SERVICE statically; without abapGit it would not activate")
	}
	if !strings.Contains(up, "CREATE OBJECT LO_SERVICE TYPE (LS_CLASS-CLSNAME)") {
		t.Error("the APC handler does not create the services it discovers dynamically")
	}
	start := strings.ToUpper(strings.Join(methodStatements(abapStatements(string(b)), "IF_APC_WSP_EXTENSION~ON_START"), "\n"))
	if !regexp.MustCompile(`BIND_AMC_MESSAGE_CONSUMER\( I_APPLICATION_ID = 'ZVSP_GIT' I_CHANNEL_ID = '/IMPORT' I_CHANNEL_EXTENSION_ID = CONV #\( MV_SESSION_ID \) \)\nLV_GIT_PUSH = ABAP_TRUE\nCATCH CX_ROOT( ##CATCH_ALL)?\nLV_GIT_PUSH = ABAP_FALSE\nENDTRY`).MatchString(start) {
		t.Error("on_start must bind ZVSP_GIT /import on its own extension and go on without push if that fails")
	}
}

func TestGitAMCApplicationDefinition(t *testing.T) {
	d := AMCGitApplicationDefinition
	for _, want := range []string{"<AMC_APPL>ZVSP_GIT</AMC_APPL>", "<CHANNEL_ID>/import</CHANNEL_ID>", "<MESSAGE_TYPE_ID>TEXT</MESSAGE_TYPE_ID>",
		"<OBJ_NAME>ZCL_VSP_GIT_SERVICE</OBJ_NAME><ACTIVITY>S</ACTIVITY>", "<OBJ_NAME>ZCL_VSP_APC_HANDLER</OBJ_NAME><ACTIVITY>C</ACTIVITY>"} {
		if !strings.Contains(d, want) {
			t.Errorf("definition lacks %s", want)
		}
	}
	if n := strings.Count(d, "<AMC_ADTCONTENTAUTHORITIES>"); n != 2 {
		t.Errorf("%d authorities; want exactly two", n)
	}
	src := gitServiceSource(t)
	pub := strings.ToUpper(strings.Join(methodStatements(abapStatements(src), "PUBLISH"), "\n"))
	if !strings.Contains(pub, "I_APPLICATION_ID = C_AMC_APP I_CHANNEL_ID = C_AMC_CHANNEL I_CHANNEL_EXTENSION_ID = CONV #( IV_PUSH_ID )") ||
		!strings.Contains(strings.ToUpper(src), "C_AMC_APP TYPE AMC_APPLICATION_ID VALUE 'ZVSP_GIT'") {
		t.Error("publish must send on ZVSP_GIT /import, on the importing session's extension")
	}
}

// Objects that name abapGit are deployed only where abapGit is.
func TestGitObjectsRequireAbapGit(t *testing.T) {
	for _, o := range GetObjects() {
		want := o.Name == "ZCL_VSP_GIT_SERVICE" || o.Name == "ZVSP_GIT_IMPORT"
		if o.RequiresAbapGit != want {
			t.Errorf("%s: RequiresAbapGit = %t, want %t", o.Name, o.RequiresAbapGit, want)
		}
		if strings.Contains(strings.ToUpper(o.Source), "ZCL_ABAPGIT_") && !o.RequiresAbapGit {
			t.Errorf("%s names abapGit but is deployed without it", o.Name)
		}
	}
}
