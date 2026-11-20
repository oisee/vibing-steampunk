package adt

import "testing"

func TestPackageRefOf(t *testing.T) {
	const head = `<program:abapProgram xmlns:program="http://www.sap.com/adt/programs/programs" xmlns:adtcore="http://www.sap.com/adt/core" adtcore:name="ZDEMO_REPORT">`
	const tail = `</program:abapProgram>`

	tests := []struct {
		name string
		body string
		want string
	}{
		{
			name: "program",
			body: head + `<adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/zpkg" adtcore:type="DEVC/K" adtcore:name="ZPKG"/>` + tail,
			want: "ZPKG",
		},
		{
			name: "lower case name",
			body: head + `<adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/zpkg" adtcore:type="DEVC/K" adtcore:name="zpkg"/>` + tail,
			want: "ZPKG",
		},
		{
			name: "local package",
			body: head + `<adtcore:packageRef adtcore:uri="/sap/bc/adt/packages/%24tmp" adtcore:type="DEVC/K" adtcore:name="$TMP"/>` + tail,
			want: "$TMP",
		},
		{
			name: "nested only",
			body: head + `<program:child><adtcore:packageRef adtcore:name="ZOTHER"/></program:child>` + tail,
			want: "",
		},
		{
			name: "no packageRef",
			body: head + tail,
			want: "",
		},
		{
			name: "not xml",
			body: `REPORT zdemo_report.`,
			want: "",
		},
		{
			name: "empty",
			body: ``,
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := packageRefOf([]byte(tt.body)); got != tt.want {
				t.Errorf("packageRefOf() = %q, want %q", got, tt.want)
			}
		})
	}
}
