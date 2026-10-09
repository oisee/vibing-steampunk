package adt

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPrepareMutation_MarksOnlyAnObjectItAccepted(t *testing.T) {
	rec := &adtRecorder{}
	client := newStubbedClient(t, rec, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "informationsystem/search") {
			pkg := "$TMP"
			if strings.Contains(r.URL.RawQuery, "ZDEMO_FOREIGN") {
				pkg = "SAPFOREIGN"
			}
			name := "ZDEMO_PREP"
			if pkg != "$TMP" {
				name = "ZDEMO_FOREIGN"
			}
			_, _ = io.WriteString(w, searchXMLFor("/sap/bc/adt/programs/programs/"+strings.ToLower(name), name, pkg))
			return
		}
		w.WriteHeader(http.StatusOK)
	}, WithAllowedPackages("$TMP"))
	ctx := context.Background()

	const ok = "/sap/bc/adt/programs/programs/ZDEMO_PREP"
	prepared, err := client.PrepareMutation(ctx, MutationContext{Op: OpUpdate, OpName: "ExtWrite", ObjectURL: ok})
	if err != nil {
		t.Fatalf("PrepareMutation: %v", err)
	}
	before := len(rec.snapshot())
	if err := client.CheckMutation(prepared, MutationContext{Op: OpUpdate, OpName: "ExtWrite", ObjectURL: ok}); err != nil {
		t.Fatalf("the prepared object was refused: %v", err)
	}
	if after := len(rec.snapshot()); after != before {
		t.Errorf("%d package lookups for an object PrepareMutation had checked", after-before)
	}

	// A refusal hands back no mark: the context returned with the error must
	// not let the same object through.
	const foreign = "/sap/bc/adt/programs/programs/ZDEMO_FOREIGN"
	refused, err := client.PrepareMutation(ctx, MutationContext{Op: OpUpdate, OpName: "ExtWrite", ObjectURL: foreign})
	if err == nil {
		t.Fatal("an object outside the allowed packages was accepted")
	}
	if err := client.CheckMutation(refused, MutationContext{Op: OpUpdate, OpName: "ExtWrite", ObjectURL: foreign}); err == nil {
		t.Error("the context of a refused PrepareMutation let the object through")
	}
}
