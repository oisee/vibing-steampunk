package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// The CLI commands that write and activate take --call-timeout, and under it
// a request may outlast the client's per-request timeout; without one it may
// not, as before.
func TestCLIWritesHonourCallTimeout(t *testing.T) {
	for _, c := range []struct {
		name string
		has  bool
	}{
		{"deploy", deployCmd.Flags().Lookup("call-timeout") != nil},
		{"copy", copyCmd.Flags().Lookup("call-timeout") != nil},
		{"source write", sourceWriteCmd.Flags().Lookup("call-timeout") != nil},
		{"source edit", sourceEditCmd.Flags().Lookup("call-timeout") != nil},
		{"install zadt-vsp", installZadtVspCmd.Flags().Lookup("call-timeout") != nil},
		{"install abapgit", installAbapGitCmd.Flags().Lookup("call-timeout") != nil},
	} {
		if !c.has {
			t.Errorf("vsp %s has no --call-timeout", c.name)
		}
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-CSRF-Token", "t")
		if strings.Contains(r.URL.Path, "/activation") {
			select {
			case <-time.After(600 * time.Millisecond):
			case <-r.Context().Done():
				return
			}
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?><chkl:messages xmlns:chkl="http://www.sap.com/abapxml/checklist">` +
				`<chkl:properties checkExecuted="true" activationExecuted="true" generationExecuted="true"/></chkl:messages>`))
			return
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	client := adt.NewClient(srv.URL, "u", "p", adt.WithTimeout(150*time.Millisecond))

	ctx, cancel := withWriteBudget(context.Background(), 0)
	_, err := client.Activate(ctx, "/sap/bc/adt/programs/programs/zdemo", "ZDEMO")
	cancel()
	if err == nil {
		t.Fatal("without a budget the per-request timeout must still end a slow activation")
	}

	ctx, cancel = withWriteBudget(context.Background(), 30*time.Second)
	res, err := client.Activate(ctx, "/sap/bc/adt/programs/programs/zdemo", "ZDEMO")
	cancel()
	if err != nil {
		t.Fatalf("under a budget the activation was still cut at the per-request timeout: %v", err)
	}
	if !res.Success {
		t.Fatalf("activation result: %+v", res)
	}
}

func TestPrintActivationMessagesCaps(t *testing.T) {
	act := &adt.ActivationResult{}
	for i := 1; i <= adt.ActivationMessageLimit+5; i++ {
		act.Messages = append(act.Messages, adt.ActivationResultMessage{
			ObjDescr: "Class ZCL_X", Type: "E", Line: i, ShortText: fmt.Sprintf("error %d", i),
		})
	}
	var buf bytes.Buffer
	printActivationMessages(&buf, act)
	out := buf.String()
	if !strings.HasPrefix(out, "Activation messages:\n  [E] Class ZCL_X, line 1: error 1\n") {
		t.Fatalf("got:\n%s", out)
	}
	if !strings.HasSuffix(out, "  ... and 5 more\n") || strings.Count(out, "\n") != adt.ActivationMessageLimit+2 {
		t.Fatalf("want %d messages and a count of the rest, got:\n%s", adt.ActivationMessageLimit, out)
	}

	buf.Reset()
	printActivationMessages(&buf, &adt.ActivationResult{Success: true})
	printActivationMessages(&buf, nil)
	if buf.Len() != 0 {
		t.Fatalf("nothing to print for a successful or absent activation, got %q", buf.String())
	}
}
