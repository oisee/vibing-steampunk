package adt

import (
	"testing"
)

// The mapping from a dump frame to a repository object is where this feature
// is most likely to be quietly wrong: every miss looks like "nobody calls
// this" rather than like an error, so every kind of frame gets a case here.
func TestUnitForFrame(t *testing.T) {
	cases := []struct {
		name  string
		frame DumpFrame
		want  repoUnit
		ok    bool
	}{
		{
			name:  "class pool unwraps to the class",
			frame: DumpFrame{Type: "METHOD", Program: "ZCL_DEMO_SERVICE=====CP", Name: "ZCL_DEMO_SERVICE=>RUN"},
			want:  repoUnit{"ZCL_DEMO_SERVICE", "CLAS", "/sap/bc/adt/oo/classes/zcl_demo_service"},
			ok:    true,
		},
		{
			name:  "interface pool is an interface, not a class",
			frame: DumpFrame{Type: "METHOD", Program: "ZIF_DEMO_SERVICE=====IP"},
			want:  repoUnit{"ZIF_DEMO_SERVICE", "INTF", "/sap/bc/adt/oo/interfaces/zif_demo_service"},
			ok:    true,
		},
		{
			// The whole point of the FUNCTION case: the module has its own,
			// far narrower where-used list than the group it lives in.
			name:  "a FUNCTION frame resolves to the module",
			frame: DumpFrame{Type: "FUNCTION", Program: "SAPLZDEMO_LOG", Include: "LZDEMO_LOGU01", Name: "ZDEMO_LOG_WRITE"},
			want:  repoUnit{"ZDEMO_LOG_WRITE", "FUNC", "/sap/bc/adt/functions/groups/zdemo_log/fmodules/zdemo_log_write"},
			ok:    true,
		},
		{
			name:  "a non-FUNCTION frame in a function pool falls back to the group",
			frame: DumpFrame{Type: "FORM", Program: "SAPLZDEMO_LOG", Include: "LZDEMO_LOGF01", Name: "BUILD_OUTPUT"},
			want:  repoUnit{"ZDEMO_LOG", "FUGR", "/sap/bc/adt/functions/groups/zdemo_log"},
			ok:    true,
		},
		{
			// The bug the old programURI had: this used to become
			// /programs/programs/saplzdemo_log, a 404 that reads as "no callers".
			name:  "a function pool is never a program",
			frame: DumpFrame{Type: "EVENT", Program: "SAPLZDEMO_LOG"},
			want:  repoUnit{"ZDEMO_LOG", "FUGR", "/sap/bc/adt/functions/groups/zdemo_log"},
			ok:    true,
		},
		{
			name:  "the group can come from the include when the program is a piece of the pool",
			frame: DumpFrame{Type: "FORM", Program: "LZDEMO_LOGU02", Include: "LZDEMO_LOGU02"},
			want:  repoUnit{"ZDEMO_LOG", "FUGR", "/sap/bc/adt/functions/groups/zdemo_log"},
			ok:    true,
		},
		{
			name:  "a TOP include is a function pool too",
			frame: DumpFrame{Type: "EVENT", Program: "LZDEMO_LOGTOP"},
			want:  repoUnit{"ZDEMO_LOG", "FUGR", "/sap/bc/adt/functions/groups/zdemo_log"},
			ok:    true,
		},
		{
			name:  "a report is a program",
			frame: DumpFrame{Type: "EVENT", Program: "ZDEMO_REPORT", Name: "START-OF-SELECTION"},
			want:  repoUnit{"ZDEMO_REPORT", "PROG", "/sap/bc/adt/programs/programs/zdemo_report"},
			ok:    true,
		},
		{
			// A module pool starts with SAP but not SAPL, and must not be
			// mistaken for a function group.
			name:  "a module pool stays a program",
			frame: DumpFrame{Type: "MODULE (PBO)", Program: "SAPMSSY1", Include: "SAPMSSY1", Name: "%_RFC_START"},
			want:  repoUnit{"SAPMSSY1", "PROG", "/sap/bc/adt/programs/programs/sapmssy1"},
			ok:    true,
		},
		{
			// A leading L is not enough; LOAD_REPORT is a program.
			name:  "a program that merely starts with L is not a function pool",
			frame: DumpFrame{Type: "EVENT", Program: "ZLOCAL_TEST"},
			want:  repoUnit{"ZLOCAL_TEST", "PROG", "/sap/bc/adt/programs/programs/zlocal_test"},
			ok:    true,
		},
		{
			// In a namespace SAPL and L come after it: /DEMO/SAPLLOG is the
			// pool of /DEMO/LOG, not a program called /DEMO/SAPLLOG.
			name:  "a namespaced FUNCTION frame resolves to the module",
			frame: DumpFrame{Type: "FUNCTION", Program: "/DEMO/SAPLLOG", Include: "/DEMO/LLOGU01", Name: "/DEMO/LOG_WRITE"},
			want:  repoUnit{"/DEMO/LOG_WRITE", "FUNC", "/sap/bc/adt/functions/groups/%2Fdemo%2Flog/fmodules/%2Fdemo%2Flog_write"},
			ok:    true,
		},
		{
			name:  "a namespaced function pool is never a program",
			frame: DumpFrame{Type: "EVENT", Program: "/DEMO/SAPLLOG"},
			want:  repoUnit{"/DEMO/LOG", "FUGR", "/sap/bc/adt/functions/groups/%2Fdemo%2Flog"},
			ok:    true,
		},
		{
			name:  "a namespaced pool include names its group",
			frame: DumpFrame{Type: "FORM", Program: "/DEMO/LLOGF01", Include: "/DEMO/LLOGF01"},
			want:  repoUnit{"/DEMO/LOG", "FUGR", "/sap/bc/adt/functions/groups/%2Fdemo%2Flog"},
			ok:    true,
		},
		{
			name:  "a namespaced name is escaped, not lost",
			frame: DumpFrame{Type: "EVENT", Program: "/DEMO/ZREPORT"},
			want:  repoUnit{"/DEMO/ZREPORT", "PROG", "/sap/bc/adt/programs/programs/%2Fdemo%2Fzreport"},
			ok:    true,
		},
		{
			name:  "an empty frame yields nothing rather than a URI that 404s",
			frame: DumpFrame{Type: "EVENT"},
			ok:    false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := unitForFrame(tc.frame)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v", ok, tc.ok)
			}
			if !tc.ok {
				return
			}
			if got != tc.want {
				t.Errorf("got %+v\nwant %+v", got, tc.want)
			}
		})
	}
}

// A method name on a FUNCTION frame would build /fmodules/zcl_x=>run, which is
// not a resource. Refusing is better than asking.
func TestFunctionModuleOfRefusesMethodNames(t *testing.T) {
	if got := functionModuleOf(DumpFrame{Type: "FUNCTION", Name: "ZCL_DEMO=>RUN"}); got != "" {
		t.Errorf("a method name was accepted as a module: %q", got)
	}
	if got := functionModuleOf(DumpFrame{Type: "METHOD", Name: "ZDEMO_LOG_WRITE"}); got != "" {
		t.Errorf("a METHOD frame gave a module name: %q", got)
	}
	if got := functionModuleOf(DumpFrame{Type: "FUNCTION", Name: "ZDEMO_LOG_WRITE"}); got != "ZDEMO_LOG_WRITE" {
		t.Errorf("got %q, want ZDEMO_LOG_WRITE", got)
	}
}

// whereUsedFixture is the shape SAP actually returns: a flat list where a row
// with no usageInformation is the container for the rows that follow it.
func whereUsedFixture() []UsageReference {
	return []UsageReference{
		{
			URI:  "/sap/bc/adt/oo/classes/zcl_demo_caller",
			Name: "ZCL_DEMO_CALLER", Type: "CLAS/OC", PackageName: "$ZDEMO",
		},
		{
			URI:              "/sap/bc/adt/oo/classes/zcl_demo_caller/source/main#type=CLAS%2FOM;name=RUN;start=1",
			ParentURI:        "/sap/bc/adt/oo/classes/zcl_demo_caller",
			Name:             "RUN",
			UsageInformation: "gradeDirect,includeProductive",
			PackageName:      "$ZDEMO",
		},
		{
			// The same caller referencing the target twice: one object, not two.
			URI:              "/sap/bc/adt/oo/classes/zcl_demo_caller/source/main#type=CLAS%2FOM;name=RETRY;start=1",
			ParentURI:        "/sap/bc/adt/oo/classes/zcl_demo_caller",
			Name:             "RETRY",
			UsageInformation: "gradeDirect,includeProductive",
			PackageName:      "$ZDEMO",
		},
		{
			URI:  "/sap/bc/adt/oo/classes/zcl_demo_service_test",
			Name: "ZCL_DEMO_SERVICE_TEST", Type: "CLAS/OC", PackageName: "$ZDEMO",
		},
		{
			URI:              "/sap/bc/adt/oo/classes/zcl_demo_service_test/source/main#type=CLAS%2FOM;name=FIRST_TEST;start=1",
			ParentURI:        "/sap/bc/adt/oo/classes/zcl_demo_service_test",
			Name:             "FIRST_TEST",
			UsageInformation: "gradeDirect,includeTest",
			PackageName:      "$ZDEMO",
		},
		{
			URI:  "/sap/bc/adt/oo/classes/zcl_demo_service",
			Name: "ZCL_DEMO_SERVICE", Type: "CLAS/OC", PackageName: "$ZDEMO",
		},
		{
			// The target describing its own parts. Counting this is how an
			// impact query reports a class as its own blast radius.
			URI:              "/sap/bc/adt/oo/classes/zcl_demo_service/source/main#type=CLAS%2FOM;name=BUILD_ERROR;start=1",
			ParentURI:        "/sap/bc/adt/oo/classes/zcl_demo_service",
			Name:             "BUILD_ERROR",
			UsageInformation: "gradeComponent,includeProductive",
			PackageName:      "$ZDEMO",
		},
		{
			// A package is a container in this list too, with its interfaces
			// underneath marked gradeDirect. A package cannot call anything.
			URI:  "/sap/bc/adt/packages/%24zdemo",
			Name: "$ZDEMO", Type: "DEVC/K", PackageName: "$ZDEMO",
		},
		{
			URI:              "/sap/bc/adt/packages/%24zdemo#interface=ZDEMO_PUBLIC",
			ParentURI:        "/sap/bc/adt/packages/%24zdemo",
			Name:             "ZDEMO_PUBLIC",
			UsageInformation: "gradeDirect,includeProductive",
			PackageName:      "$ZDEMO",
		},
	}
}

func TestExposedCallersFiltersGradeAndSelf(t *testing.T) {
	callers := exposedCallers(whereUsedFixture(), "ZCL_DEMO_SERVICE")

	if len(callers) != 2 {
		t.Fatalf("got %d callers, want 2 (the package interface is not one): %+v", len(callers), callers)
	}
	for _, c := range callers {
		if c.Name == "$ZDEMO" {
			t.Fatalf("a package was reported as a caller: %+v", c)
		}
	}
	// Productive first, tests after: a failing test is exposure, but not the
	// exposure anybody is paged about.
	if callers[0].Name != "ZCL_DEMO_CALLER" || callers[0].IsTest {
		t.Errorf("first caller is %+v, want the productive ZCL_DEMO_CALLER", callers[0])
	}
	if callers[1].Name != "ZCL_DEMO_SERVICE_TEST" || !callers[1].IsTest {
		t.Errorf("second caller is %+v, want the test class flagged as one", callers[1])
	}
	if callers[0].Type != "CLAS/OC" || callers[0].Package != "$ZDEMO" {
		t.Errorf("the container's identity did not reach the caller: %+v", callers[0])
	}
	// Two references from one object collapse into one caller, both routines named.
	if callers[0].Component != "RUN, RETRY" {
		t.Errorf("components = %q, want both routines on one caller", callers[0].Component)
	}
}

// SAPMSSY1's one direct reference on a live system is a package interface. A
// kernel dispatcher does not have exactly one caller, and that caller is not a
// package.
func TestExposedCallersDropPackageInterfaces(t *testing.T) {
	refs := []UsageReference{
		{URI: "/sap/bc/adt/packages/basis", Name: "BASIS", Type: "DEVC/K"},
		{URI: "/sap/bc/adt/packages/srcx", ParentURI: "/sap/bc/adt/packages/basis", Name: "SRCX", Type: "DEVC/K"},
		{
			URI:              "/sap/bc/adt/vit/wb/object_type/pinfki/object_name/SRCX_INTERNAL",
			ParentURI:        "/sap/bc/adt/packages/srcx",
			Name:             "SRCX_INTERNAL",
			Type:             "PINF/KI",
			UsageInformation: "gradeDirect,includeProductive",
		},
	}
	if got := exposedCallers(refs, "SAPMSSY1"); len(got) != 0 {
		t.Errorf("a package interface was reported as a caller: %+v", got)
	}
}

func TestExposedCallersOnEmptyResult(t *testing.T) {
	if got := exposedCallers(nil, "ZCL_DEMO_SERVICE"); len(got) != 0 {
		t.Errorf("got %d callers from an empty where-used list", len(got))
	}
	// A system that answers with containers only — every reference being the
	// object's own components — is an object nobody calls, not an error.
	onlySelf := []UsageReference{
		{URI: "/sap/bc/adt/oo/classes/zcl_demo_service", Name: "ZCL_DEMO_SERVICE", Type: "CLAS/OC"},
		{
			URI:              "/sap/bc/adt/oo/classes/zcl_demo_service/source/main#start=1,0",
			ParentURI:        "/sap/bc/adt/oo/classes/zcl_demo_service",
			Name:             "Public Section",
			UsageInformation: "gradeComponent,includeProductive",
		},
	}
	if got := exposedCallers(onlySelf, "ZCL_DEMO_SERVICE"); len(got) != 0 {
		t.Errorf("the object's own components were reported as callers: %+v", got)
	}
}

// The empty answer that is not an answer. Asking a function group returns 200
// and zero results whatever the truth is, and without this the command would
// print "nothing else calls this code" about code with hundreds of callers.
func TestAFunctionGroupIsNotAskable(t *testing.T) {
	if note := unanswerable(ImpactUnit{Object: "SBAL_DB", Type: "FUGR"}); note == "" {
		t.Fatal("a function group unit was treated as answerable")
	}
	for _, answerable := range []string{"CLAS", "INTF", "PROG", "FUNC"} {
		if note := unanswerable(ImpactUnit{Object: "X", Type: answerable}); note != "" {
			t.Errorf("%s was refused: %s", answerable, note)
		}
	}
}

func TestAnswerableSeparatesEmptyFromUnaskable(t *testing.T) {
	unaskable := &DumpImpactResult{Units: []ImpactUnit{
		{Type: "FUGR", Note: "cannot answer"},
		{Type: "PROG", Err: "404"},
	}}
	if unaskable.Answerable() {
		t.Error("a result with nothing askable reported itself answerable")
	}
	// Zero callers from a unit that *was* asked is a real finding.
	asked := &DumpImpactResult{Units: []ImpactUnit{
		{Type: "FUGR", Note: "cannot answer"},
		{Type: "CLAS", Total: 0},
	}}
	if !asked.Answerable() {
		t.Error("a unit that answered with zero callers was treated as unaskable")
	}
}

func TestImpactUnitsOrderAndDedup(t *testing.T) {
	dump := Dump{Program: "ZCL_DEMO_SERVICE=====CP"}
	stack := []DumpFrame{
		{Position: 3, Type: "METHOD", Program: "ZCL_DEMO_SERVICE=====CP", Name: "ZCL_DEMO_SERVICE=>RUN"},
		{Position: 2, Type: "METHOD", Program: "ZCL_DEMO_CALLER======CP", Name: "ZCL_DEMO_CALLER=>GO"},
		{Position: 1, Type: "EVENT", Program: "ZDEMO_REPORT", Name: "START-OF-SELECTION"},
	}

	units := impactUnits(dump, stack, 5)
	if len(units) != 3 {
		t.Fatalf("got %d units, want 3 (the dump program is also frame 3): %+v", len(units), units)
	}
	want := []string{"ZCL_DEMO_SERVICE", "ZCL_DEMO_CALLER", "ZDEMO_REPORT"}
	for i, name := range want {
		if units[i].Object != name {
			t.Errorf("unit %d is %s, want %s", i, units[i].Object, name)
		}
		if units[i].Distance != i {
			t.Errorf("unit %d has distance %d", i, units[i].Distance)
		}
	}
	// The dump's own program is not a frame, and the deduplication must not
	// let it steal frame 3's frame pointer.
	if units[0].Frame != nil {
		t.Errorf("the leading unit came from the dump header and should carry no frame")
	}
}

// An RFC refused at the door dumps with a dispatcher on the stack and names the
// module it could not reach only in the header. Leading with the dump's own
// program is what makes that case answerable at all.
func TestImpactUnitsLeadWithTheDumpProgram(t *testing.T) {
	dump := Dump{Program: "SAPLSBAL_DB"}
	stack := []DumpFrame{{Position: 1, Type: "MODULE (PBO)", Program: "SAPMSSY1", Name: "%_RFC_START"}}

	units := impactUnits(dump, stack, 3)
	if len(units) != 2 {
		t.Fatalf("got %d units, want 2", len(units))
	}
	if units[0].Object != "SBAL_DB" || units[0].Type != "FUGR" {
		t.Errorf("unit 0 is %+v, want the function group from the dump header", units[0])
	}
	if units[1].Object != "SAPMSSY1" {
		t.Errorf("unit 1 is %+v, want the dispatcher frame", units[1])
	}
}

func TestImpactUnitsRespectMax(t *testing.T) {
	dump := Dump{Program: "ZDEMO_A"}
	stack := []DumpFrame{
		{Position: 3, Program: "ZDEMO_B"},
		{Position: 2, Program: "ZDEMO_C"},
		{Position: 1, Program: "ZDEMO_D"},
	}
	if got := impactUnits(dump, stack, 2); len(got) != 2 {
		t.Errorf("got %d units, want the cap of 2", len(got))
	}
	if got := impactUnits(dump, stack, 0); len(got) != 0 {
		t.Errorf("a cap of zero produced %d units", len(got))
	}
}

func TestRankExposureSplitsOffTheStack(t *testing.T) {
	dump := Dump{Program: "ZCL_DEMO_SERVICE=====CP"}
	stack := []DumpFrame{
		{Position: 2, Program: "ZCL_DEMO_SERVICE=====CP"},
		{Position: 1, Program: "ZCL_DEMO_CALLER======CP"},
	}
	units := []ImpactUnit{
		{Object: "ZCL_DEMO_SERVICE", Distance: 0, Callers: []ExposedCaller{
			// On the stack: this is the route that failed, not extra exposure.
			{Name: "ZCL_DEMO_CALLER", Distance: 0, Via: "ZCL_DEMO_SERVICE"},
			{Name: "ZCL_DEMO_BATCH", Distance: 0, Via: "ZCL_DEMO_SERVICE"},
		}},
		{Object: "ZCL_DEMO_CALLER", Distance: 1, Callers: []ExposedCaller{
			// Seen already one unit closer to the failure; the nearer sighting wins.
			{Name: "ZCL_DEMO_BATCH", Distance: 1, Via: "ZCL_DEMO_CALLER"},
			{Name: "ZDEMO_REPORT", Distance: 1, Via: "ZCL_DEMO_CALLER"},
		}},
	}

	exposed, onPath := rankExposure(units, dump, stack)

	if len(onPath) != 1 || onPath[0].Name != "ZCL_DEMO_CALLER" {
		t.Fatalf("onPath = %+v, want only the caller that is on the stack", onPath)
	}
	if len(exposed) != 2 {
		t.Fatalf("exposed = %+v, want two distinct objects", exposed)
	}
	if exposed[0].Name != "ZCL_DEMO_BATCH" || exposed[0].Distance != 0 {
		t.Errorf("exposed[0] = %+v, want ZCL_DEMO_BATCH kept at its nearest distance", exposed[0])
	}
	if exposed[1].Name != "ZDEMO_REPORT" || exposed[1].Distance != 1 {
		t.Errorf("exposed[1] = %+v, want ZDEMO_REPORT one unit further out", exposed[1])
	}
}
