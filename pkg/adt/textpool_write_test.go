package adt

import (
	"reflect"
	"testing"
)

func TestSelectionTextsFromSource(t *testing.T) {
	src := `REPORT zdemo.
TABLES tadir.
PARAMETERS: p_devc TYPE tadir-devclass DEFAULT '$TMP', "~t: Package to scan
            p_deep TYPE abap_bool AS CHECKBOX.      "~~t:Follow includes
SELECT-OPTIONS s_obj FOR tadir-obj_name. "~t: Object names
PARAMETER p_old TYPE c. " an ordinary comment, no text
SELECTION-SCREEN COMMENT 3(60) gv_hint FOR FIELD p_devc. "~t: Where to look
SELECTION-SCREEN COMMENT /1(30) gv_two. "~t: Second line
  p_cont TYPE i. "~t: In a chained statement
WRITE 'not a declaration'. "~t: ignored? no — a token is a token
"~t: a comment on its own line is nothing
`
	got := SelectionTextsFromSource(src)
	want := map[string]string{
		"P_DEVC": "Package to scan", "P_DEEP": "Follow includes", "S_OBJ": "Object names",
		"GV_HINT": "Where to look", "GV_TWO": "Second line", "P_CONT": "In a chained statement",
		"WRITE": "ignored? no — a token is a token",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v\nwant %v", got, want)
	}
}

func TestFormatTextPool(t *testing.T) {
	got := FormatTextPool(map[string]string{"S_OBJ": "Object names", "P_DEVC": "Package to scan", "P_DEEP": ""})
	want := "P_DEEP  =\nP_DEVC  =Package to scan\nS_OBJ   =Object names\n"
	if got != want {
		t.Errorf("got %q want %q", got, want)
	}
}
