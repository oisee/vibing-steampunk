package datacluster

import (
	"strings"
	"testing"
)

// tyAll mirrors the fixture program's TY_ALL as DDIC would describe it.
var tyAll = &Layout{Name: "TY_ALL", Components: []Component{
	{Name: "F_CHAR", Type: "CHAR", Chars: 10},
	{Name: "F_NUMC", Type: "NUMC", Chars: 6},
	{Name: "F_DATS", Type: "DATS", Chars: 8},
	{Name: "F_TIMS", Type: "TIMS", Chars: 6},
	{Name: "F_INT", Type: "INT4", Chars: 10},
	{Name: "F_INT1", Type: "INT1", Chars: 3},
	{Name: "F_INT2", Type: "INT2", Chars: 5},
	{Name: "F_INT8", Type: "INT8", Chars: 19},
	{Name: "F_P", Type: "DEC", Chars: 15, Decimals: 2},
	{Name: "F_FLTP", Type: "FLTP", Chars: 16},
	{Name: "F_RAW", Type: "RAW", Chars: 4},
	{Name: "F_STR", Type: "STRG"},
	{Name: "F_XSTR", Type: "RSTR"},
	{Name: "F_DEC", Type: "D34D", Chars: 34},
	{Name: "F_TS", Type: "DEC", Chars: 21, Decimals: 7},
}}

func TestApplyLayout(t *testing.T) {
	c, err := Parse(loadHex(t, "indx_compressed.hex"))
	if err != nil {
		t.Fatal(err)
	}
	st := c.Object("STRUCT")
	if err := st.Apply(tyAll); err != nil {
		t.Fatal(err)
	}
	rec := st.Records()[0]
	if rec["F_CHAR"] != "ABC" || rec["F_TS"] != "20260904123456.1234567" || rec["F_XSTR"] != "CAFE" {
		t.Errorf("named record: %v", rec)
	}

	nested := c.Object("NESTED")
	tyNested := &Layout{Name: "TY_NESTED", Components: []Component{
		{Name: "HEAD", Type: "CHAR", Chars: 4},
		{Name: "INNER", Kind: SubstructureField, Type: "TY_ALL", Sub: tyAll},
		{Name: "TAIL", Type: "NUMC", Chars: 2},
	}}
	if err := nested.Apply(tyNested); err != nil {
		t.Fatal(err)
	}
	rec = nested.Records()[0]
	if rec["HEAD"] != "HEAD" || rec["INNER.F_INT"] != int64(-7) || rec["TAIL"] != "99" {
		t.Errorf("nested record: %v", rec)
	}
	if nested.Fields[1].Name != "INNER.F_CHAR" || nested.Fields[1].Path != "2.1" {
		t.Errorf("field 2: %+v", nested.Fields[1])
	}

	scalar := c.Object("SCALAR")
	if err := scalar.Apply(&Layout{Name: "X", Components: []Component{{Name: "VALUE", Type: "CHAR", Chars: 23}}}); err != nil || scalar.Fields[0].Name != "VALUE" {
		t.Errorf("scalar: %v %+v", err, scalar.Fields[0])
	}
}

func TestApplyLayoutRefuses(t *testing.T) {
	c, err := Parse(loadHex(t, "indx_compressed.hex"))
	if err != nil {
		t.Fatal(err)
	}
	st := c.Object("STRUCT")
	clone := func(edit func(l *Layout)) *Layout {
		l := &Layout{Name: "BAD", Components: append([]Component(nil), tyAll.Components...)}
		edit(l)
		return l
	}
	cases := map[string]*Layout{
		"too short":      clone(func(l *Layout) { l.Components = l.Components[:14] }),
		"too long":       clone(func(l *Layout) { l.Components = append(l.Components, Component{Name: "X", Type: "CHAR", Chars: 1}) }),
		"wrong length":   clone(func(l *Layout) { l.Components[0].Chars = 11 }),
		"wrong type":     clone(func(l *Layout) { l.Components[4].Type = "FLTP" }),
		"wrong decimals": clone(func(l *Layout) { l.Components[8].Decimals = 3 }),
		"field for struct": clone(func(l *Layout) {
			l.Components[0] = Component{Name: "S", Kind: SubstructureField, Sub: &Layout{Components: []Component{{Name: "A", Type: "CHAR", Chars: 5}}}}
		}),
		"table type": clone(func(l *Layout) { l.Components[0].Kind = TableField }),
	}
	for name, l := range cases {
		if err := st.Apply(l); err == nil {
			t.Errorf("%s: accepted", name)
		}
		for _, f := range st.Fields {
			if f.Name != "" {
				t.Fatalf("%s: a refused layout still named field %s", name, f.Path)
			}
		}
	}
}

func TestApplyLayoutIncludes(t *testing.T) {
	// The BAL message bucket row: number, a substructure of four variables,
	// an include of two flags, a substructure of four flags, an include that
	// itself includes three flags after one, and the message substructure.
	c, err := Parse(loadHex(t, "baldat_a4h.hex"))
	if err != nil {
		t.Fatal(err)
	}
	flag := func(n string) Component { return Component{Name: n, Type: "CHAR", Chars: 1} }
	l := &Layout{Name: "BUCKET", Components: []Component{
		{Name: "MSGNUMBER", Type: "NUMC", Chars: 6},
		{Name: "VARS", Kind: SubstructureField, Sub: &Layout{Components: []Component{
			{Name: "MSGV1", Type: "CHAR", Chars: 50}, {Name: "MSGV2", Type: "CHAR", Chars: 50},
			{Name: "MSGV3", Type: "CHAR", Chars: 50}, {Name: "MSGV4", Type: "CHAR", Chars: 50},
		}}},
		{Name: ".INCLUDE", Kind: IncludeField, Sub: &Layout{Components: []Component{flag("CTX1"), flag("CTX2")}}},
		{Name: "FLAGS", Kind: SubstructureField, Sub: &Layout{Components: []Component{flag("A"), flag("B"), flag("C"), flag("D")}}},
		{Name: ".INCLUDE", Kind: IncludeField, Sub: &Layout{Components: []Component{
			flag("E"),
			{Name: ".INCLUDE", Kind: IncludeField, Sub: &Layout{Components: []Component{flag("F"), flag("G"), flag("H")}}},
		}}},
		{Name: "MSG", Kind: SubstructureField, Sub: &Layout{Components: []Component{
			flag("MSGTY"), {Name: "MSGID", Type: "CHAR", Chars: 20}, {Name: "MSGNO", Type: "NUMC", Chars: 3},
			flag("DETLEVEL"), flag("PROBCLASS"), {Name: "ALSORT", Type: "CHAR", Chars: 3},
			{Name: "TIME_STMP", Type: "DEC", Chars: 21, Decimals: 7}, {Name: "MSG_COUNT", Type: "INT4", Chars: 10},
		}}},
	}}
	obj := c.Object("T_2000")
	if err := obj.Apply(l); err != nil {
		t.Fatal(err)
	}
	rec := obj.Records()[0]
	if rec["MSGNUMBER"] != "000001" || rec["VARS.MSGV1"] != "Periodic mode - checking all active runs" || rec["MSG.MSGID"] != "BL" || rec["H"] != "" {
		t.Errorf("record: %v", rec)
	}
	var names []string
	for _, f := range obj.Fields {
		names = append(names, f.Name)
	}
	if got := strings.Join(names, ","); !strings.Contains(got, "CTX1,CTX2,FLAGS.A") || !strings.Contains(got, "E,F,G,H,MSG.MSGTY") {
		t.Errorf("names: %s", got)
	}
}

func TestSAPscriptText(t *testing.T) {
	c := &Cluster{Objects: []Object{{
		Name: "TLINE", Kind: Table, RowLength: 268, charBytes: 2,
		Type: &Node{Length: 268, Children: []*Node{
			{Path: "1", TypeCode: typeChar, Length: 4}, {Path: "2", TypeCode: typeChar, Length: 264},
		}},
		Fields: []Field{{Path: "1", Type: "CHAR", Length: 4}, {Path: "2", Type: "CHAR", Length: 264}},
		Rows:   [][]any{{"*", "Dear customer,"}, {"", "your order"}, {"=", " 4711 shipped."}, {"/:", "INCLUDE ZFOOTER"}},
	}}}
	lines, text, err := c.SAPscriptText()
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 4 || lines[0].Format != "*" {
		t.Errorf("lines: %+v", lines)
	}
	if text != "Dear customer,\nyour order 4711 shipped.\nINCLUDE ZFOOTER" {
		t.Errorf("text: %q", text)
	}
	if c.Objects[0].Fields[1].Name != "TDLINE" {
		t.Errorf("TLINE not named: %+v", c.Objects[0].Fields)
	}
	if _, _, err := (&Cluster{}).SAPscriptText(); err == nil {
		t.Error("cluster without TLINE accepted")
	}
}

// indx_ddic.hex was written by a program exporting a BAPIRET2 structure, a
// TLINE table and a BAL_S_CONT structure: flat DDIC types, which the kernel
// marks with the flat object kinds 02 and 03.
func TestApplyDDICLayouts(t *testing.T) {
	c, err := Parse(loadHex(t, "indx_ddic.hex"))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Objects) != 3 {
		t.Fatalf("cluster: %d objects", len(c.Objects))
	}
	ch := func(n string, l int) Component { return Component{Name: n, Type: "CHAR", Chars: l} }
	bapiret2 := &Layout{Name: "BAPIRET2", Components: []Component{
		ch("TYPE", 1), ch("ID", 20), {Name: "NUMBER", Type: "NUMC", Chars: 3}, ch("MESSAGE", 220),
		ch("LOG_NO", 20), {Name: "LOG_MSG_NO", Type: "NUMC", Chars: 6},
		ch("MESSAGE_V1", 50), ch("MESSAGE_V2", 50), ch("MESSAGE_V3", 50), ch("MESSAGE_V4", 50),
		ch("PARAMETER", 32), {Name: "ROW", Type: "INT4", Chars: 10}, ch("FIELD", 30), ch("SYSTEM", 10),
	}}
	ret := c.Object("RET")
	if ret == nil || ret.Kind != Structure {
		t.Fatalf("RET: %+v", ret)
	}
	if err := ret.Apply(bapiret2); err != nil {
		t.Fatal(err)
	}
	rec := ret.Records()[0]
	if rec["TYPE"] != "E" || rec["ID"] != "ZDEMO" || rec["NUMBER"] != "017" || rec["ROW"] != int64(3) || rec["FIELD"] != "VBELN" {
		t.Errorf("BAPIRET2 record: %v", rec)
	}
	lines := c.Object("LINES")
	if lines == nil || lines.Kind != Table || len(lines.Rows) != 3 {
		t.Fatalf("LINES: %+v", lines)
	}
	if err := lines.Apply(TLINELayout); err != nil {
		t.Fatal(err)
	}
	if lines.Records()[2]["TDLINE"] != " 4711 shipped." {
		t.Errorf("TLINE row 3: %v", lines.Records()[2])
	}
	cont := c.Object("CONT")
	if err := cont.Apply(&Layout{Name: "BAL_S_CONT", Components: []Component{ch("TABNAME", 30), ch("VALUE", 256)}}); err != nil {
		t.Fatal(err)
	}
	if cont.Records()[0]["TABNAME"] != "ZDEMO_ORDER_KEY" {
		t.Errorf("BAL_S_CONT: %v", cont.Records()[0])
	}
}
