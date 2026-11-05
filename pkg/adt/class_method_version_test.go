package adt

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// 2026-11-02, on 7.50: a method read sent no version, so SAP read the structure
// inactive (CL_SRC_ADT_RES_OBJ_STRUC) whoever asked, and rewrote a stale
// inactive ==CS cache under the reader, while source/main gives the inactive
// version only to the user holding it. The line range must come from the
// version source/main gave this reader.
func TestClassMethodSourceTakesLinesFromTheVersionItCuts(t *testing.T) {
	structure := func(start, end int) string {
		return fmt.Sprintf(`<abapsource:objectStructureElement adtcore:name="ZCL_T" adtcore:type="CLAS/OC" xmlns:adtcore="http://www.sap.com/adt/core" xmlns:abapsource="http://www.sap.com/adt/abapsource" xmlns:atom="http://www.w3.org/2005/Atom">`+
			`<abapsource:objectStructureElement adtcore:type="CLAS/OM" adtcore:name="RUN" level="instance" visibility="public">`+
			`<atom:link rel="http://www.sap.com/adt/relations/source/implementationBlock" href="./../x/source/main#start=%d,2;end=%d,11"/>`+
			`</abapsource:objectStructureElement></abapsource:objectStructureElement>`, start, end)
	}
	activeSrc := "CLASS zcl_t IMPLEMENTATION.\n  METHOD run.\n    a = 1.\n  ENDMETHOD.\nENDCLASS."
	// Someone's unactivated edit added a method above RUN: two lines down.
	inactiveSrc := "CLASS zcl_t IMPLEMENTATION.\n  METHOD new.\n  ENDMETHOD.\n  METHOD run.\n    a = 2.\n  ENDMETHOD.\nENDCLASS."
	activeRun, inactiveRun := "  METHOD run.\n    a = 1.\n  ENDMETHOD.", "  METHOD run.\n    a = 2.\n  ENDMETHOD."

	for _, tc := range []struct {
		name          string
		readerHolds   bool // source/main without a version gives this reader the inactive version
		neverActive   bool
		wantStructure string
		want          string
	}{
		{name: "another user holds it inactive", wantStructure: "active", want: activeRun},
		{name: "the reader's own unactivated edit", readerHolds: true, wantStructure: "inactive", want: inactiveRun},
		{name: "never activated", readerHolds: true, neverActive: true, wantStructure: "inactive", want: inactiveRun},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base := "/sap/bc/adt/oo/classes/ZCL_T"
			structureVersion := "none sent"
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("X-CSRF-Token", "test-token")
				version := r.URL.Query().Get("version")
				switch r.URL.Path {
				case base + "/objectstructure":
					structureVersion = version
					if version == "active" {
						w.Write([]byte(structure(2, 4)))
					} else { // SAP: no version and "inactive" both read inactive
						w.Write([]byte(structure(4, 6)))
					}
				case base + "/source/main":
					switch {
					case version == "active" && tc.neverActive:
						http.Error(w, "not found", http.StatusNotFound)
					case version == "active", version == "" && !tc.readerHolds:
						w.Write([]byte(activeSrc))
					default:
						w.Write([]byte(inactiveSrc))
					}
				default:
					http.Error(w, "not found", http.StatusNotFound)
				}
			}))
			t.Cleanup(srv.Close)

			got, err := NewClient(srv.URL, "user", "pass").GetClassMethodSource(context.Background(), "ZCL_T", "RUN")
			if err != nil || got != tc.want {
				t.Fatalf("want %q, got %q, %v", tc.want, got, err)
			}
			if structureVersion != tc.wantStructure {
				t.Fatalf("structure read as %q, want %q", structureVersion, tc.wantStructure)
			}
		})
	}
}
