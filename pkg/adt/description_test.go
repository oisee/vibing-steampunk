package adt

import "testing"

func TestDescriptionOf(t *testing.T) {
	body := `<program:abapProgram adtcore:name="ZDEMO" adtcore:description="A &amp; B &quot;c&quot;" adtcore:descriptionTextLimit="70" xmlns:adtcore="x"/>`
	got, limit, err := descriptionOf(body)
	if err != nil || got != `A & B "c"` || limit != 70 {
		t.Errorf("got %q %d %v", got, limit, err)
	}
	if _, _, err := descriptionOf(`<x adtcore:name="Z"/>`); err == nil {
		t.Error("missing attribute accepted")
	}
	replaced := descriptionAttr.ReplaceAllLiteralString(body, ` adtcore:description="New &lt;text&gt;"`)
	if got, _, _ := descriptionOf(replaced); got != "New <text>" {
		t.Errorf("replaced: %q", got)
	}
	if u, err := DescriptionObjectURL("FUNC", "ZDEMO_FM", "ZDEMO_FG"); err != nil || u != "/sap/bc/adt/functions/groups/zdemo_fg/fmodules/zdemo_fm" {
		t.Errorf("func url: %s %v", u, err)
	}
	if _, err := DescriptionObjectURL("MSAG", "ZX", ""); err == nil {
		t.Error("unknown type accepted")
	}
}
