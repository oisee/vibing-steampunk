package adt

import (
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/datacluster"
)

// The rows below are DD03L for BAL_S_MSGR and the structures it reaches, as a
// 7.58 system returns them: an include whose fields follow at the same
// depth, a structured component whose fields are nested one deeper, a
// table-typed component, and a string field with an empty COMPTYPE.
func dd03lFixture() map[string][]dd03lRow {
	e := func(pos, depth int, field, datatype string, leng, dec int) dd03lRow {
		return dd03lRow{Position: pos, Depth: depth, Field: field, Comptype: "E", Datatype: datatype, Leng: leng, Decimals: dec}
	}
	balSCont := []dd03lRow{e(1, 0, "TABNAME", "CHAR", 30, 0), e(2, 0, "VALUE", "CHAR", 256, 0)}
	balSClbk := []dd03lRow{e(1, 0, "USEREXITP", "CHAR", 40, 0), e(2, 0, "USEREXITF", "CHAR", 30, 0), e(3, 0, "USEREXITT", "CHAR", 1, 0)}
	balSParm := []dd03lRow{
		{Position: 1, Field: "T_PAR", Comptype: "L", Rollname: "BAL_T_PAR", Datatype: "TTYP"},
		{Position: 2, Field: "CALLBACK", Comptype: "S", Rollname: "BAL_S_CLBK", Datatype: "STRU"},
		e(3, 1, "USEREXITP", "CHAR", 40, 0), e(4, 1, "USEREXITF", "CHAR", 30, 0), e(5, 1, "USEREXITT", "CHAR", 1, 0),
		e(6, 0, "ALTEXT", "CHAR", 28, 0),
	}
	balSMsg := []dd03lRow{
		e(1, 0, "MSGTY", "CHAR", 1, 0), e(2, 0, "MSGID", "CHAR", 20, 0), e(3, 0, "MSGNO", "NUMC", 3, 0),
		e(4, 0, "MSGV1", "CHAR", 50, 0), e(5, 0, "MSGV2", "CHAR", 50, 0), e(6, 0, "MSGV3", "CHAR", 50, 0), e(7, 0, "MSGV4", "CHAR", 50, 0),
		e(8, 0, "DETLEVEL", "CHAR", 1, 0), e(9, 0, "PROBCLASS", "CHAR", 1, 0), e(10, 0, "ALSORT", "CHAR", 3, 0),
		e(11, 0, "TIME_STMP", "DEC", 21, 7), e(12, 0, "MSG_COUNT", "INT4", 10, 0),
		{Position: 13, Field: "CONTEXT", Comptype: "S", Rollname: "BAL_S_CONT", Datatype: "STRU"},
		e(14, 1, "TABNAME", "CHAR", 30, 0), e(15, 1, "VALUE", "CHAR", 256, 0),
		{Position: 16, Field: "PARAMS", Comptype: "S", Rollname: "BAL_S_PARM", Datatype: "STRU"},
		{Position: 17, Depth: 1, Field: "T_PAR", Comptype: "L", Rollname: "BAL_T_PAR", Datatype: "TTYP"},
		{Position: 18, Depth: 1, Field: "CALLBACK", Comptype: "S", Rollname: "BAL_S_CLBK", Datatype: "STRU"},
		e(19, 2, "USEREXITP", "CHAR", 40, 0), e(20, 2, "USEREXITF", "CHAR", 30, 0), e(21, 2, "USEREXITT", "CHAR", 1, 0),
		e(22, 1, "ALTEXT", "CHAR", 28, 0),
	}
	balSMsgr := append([]dd03lRow{
		e(1, 0, "MSGNUMBER", "NUMC", 6, 0),
		{Position: 2, Field: ".INCLUDE", Comptype: "S", Precfield: "BAL_S_MSG"},
	}, balSMsg...)
	balSMsgr = append(balSMsgr, dd03lRow{Position: 25, Field: "MSG_TXT", Datatype: "STRG"})
	return map[string][]dd03lRow{
		"BAL_S_CONT": balSCont, "BAL_S_CLBK": balSClbk, "BAL_S_PARM": balSParm, "BAL_S_MSG": balSMsg, "BAL_S_MSGR": balSMsgr,
	}
}

func TestBuildLayout(t *testing.T) {
	fx := dd03lFixture()
	resolve := func(n string) ([]dd03lRow, error) { return fx[n], nil }
	l, err := buildLayout("BAL_S_MSGR", fx["BAL_S_MSGR"], resolve, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Components) != 3 {
		t.Fatalf("%d top-level components: %+v", len(l.Components), names(l))
	}
	inc := l.Components[1]
	if inc.Kind != datacluster.IncludeField || inc.Type != "BAL_S_MSG" || inc.Sub == nil || len(inc.Sub.Components) != 14 {
		t.Fatalf("include: %+v", inc)
	}
	if l.Components[2].Name != "MSG_TXT" || l.Components[2].Kind != datacluster.ElementaryField || l.Components[2].Type != "STRG" {
		t.Errorf("after the include: %+v", l.Components[2])
	}
	ctx := inc.Sub.Components[12]
	if ctx.Name != "CONTEXT" || ctx.Kind != datacluster.SubstructureField || len(ctx.Sub.Components) != 2 || ctx.Sub.Components[1].Chars != 256 {
		t.Errorf("CONTEXT: %+v", ctx)
	}
	params := inc.Sub.Components[13]
	if params.Name != "PARAMS" || len(params.Sub.Components) != 3 || params.Sub.Components[0].Kind != datacluster.TableField ||
		params.Sub.Components[1].Kind != datacluster.SubstructureField || len(params.Sub.Components[1].Sub.Components) != 3 || params.Sub.Components[2].Name != "ALTEXT" {
		t.Errorf("PARAMS: %+v", params)
	}
	if ts := inc.Sub.Components[10]; ts.Name != "TIME_STMP" || ts.Type != "DEC" || ts.Chars != 21 || ts.Decimals != 7 {
		t.Errorf("TIME_STMP: %+v", ts)
	}
}

func TestBuildLayoutRefuses(t *testing.T) {
	fx := dd03lFixture()
	resolve := func(n string) ([]dd03lRow, error) { return fx[n], nil }
	// A row one level too deep with no structure above it.
	bad := []dd03lRow{{Position: 1, Field: "A", Comptype: "E", Datatype: "CHAR", Leng: 1}, {Position: 2, Depth: 1, Field: "B", Comptype: "E", Datatype: "CHAR", Leng: 1}}
	if _, err := buildLayout("BAD", bad, resolve, 0); err == nil || !strings.Contains(err.Error(), "depth") {
		t.Errorf("orphan depth: %v", err)
	}
	// An include cycle.
	cyc := []dd03lRow{{Position: 1, Field: ".INCLUDE", Comptype: "S", Precfield: "CYC"}}
	if _, err := buildLayout("CYC", cyc, func(string) ([]dd03lRow, error) { return cyc, nil }, 0); err == nil || !strings.Contains(err.Error(), "cycle") {
		t.Errorf("cycle: %v", err)
	}
	// A reference component.
	ref := []dd03lRow{{Position: 1, Field: "R", Comptype: "R", Rollname: "REF"}}
	if _, err := buildLayout("REF", ref, resolve, 0); err == nil {
		t.Error("reference component accepted")
	}
}

func names(l *datacluster.Layout) []string {
	out := make([]string, len(l.Components))
	for i, c := range l.Components {
		out[i] = c.Name
	}
	return out
}
