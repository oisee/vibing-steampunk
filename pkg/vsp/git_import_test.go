package vsp

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

func writeCLIDemoZip(t *testing.T) string {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	for n, c := range map[string]string{
		".abapgit.xml": `<?xml version="1.0" encoding="utf-8"?><asx:abap xmlns:asx="http://www.sap.com/abapxml" version="1.0"><asx:values><DATA>` +
			`<STARTING_FOLDER>/src/</STARTING_FOLDER><FOLDER_LOGIC>PREFIX</FOLDER_LOGIC></DATA></asx:values></asx:abap>`,
		"src/zdemo_report.prog.abap":  "REPORT zdemo_report.",
		"src/sub/zdemo_sub.prog.abap": "REPORT zdemo_sub.",
	} {
		f, _ := w.Create(n)
		_, _ = f.Write([]byte(c))
	}
	_ = w.Close()
	p := filepath.Join(t.TempDir(), "demo.zip")
	if err := os.WriteFile(p, b.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

// Every refusal of vsp git import-zip and delete-objects comes before the
// system is contacted at all.
func TestGitCLI_RefusesBeforeContact(t *testing.T) {
	cases := map[string]struct {
		extra   string
		env     map[string]string
		pkg     string
		del     bool
		wantErr string
	}{
		"read_only system":                        {`,"read_only":true`, nil, "$ZDEMO", false, "read-only"},
		"SAP_READ_ONLY":                           {``, map[string]string{"SAP_READ_ONLY": "true"}, "$ZDEMO", false, "read-only"},
		"outside allowed":                         {`,"allowed_packages":["$ZOTHER"]`, nil, "$ZDEMO", false, "blocked"},
		"subpackage not allowed":                  {`,"allowed_packages":["$ZDEMO"]`, nil, "$ZDEMO", false, "$ZDEMO_SUB"},
		"transportable":                           {``, nil, "ZDEMO", false, "transportable"},
		"transportable, choice off, no transport": {`,"allow_transportable_edits":true`, map[string]string{"SAP_TRANSPORT_CHOICE": "off"}, "ZDEMO", false, "name the transport"},
		"delete read_only":                        {`,"read_only":true`, nil, "$ZDEMO", true, "read-only"},
		"delete outside allowed":                  {`,"allowed_packages":["$ZOTHER"]`, nil, "$ZDEMO", true, "blocked"},
		"delete transportable":                    {`,"allow_transportable_edits":true`, nil, "ZDEMO", true, "needs a transport"},
		"delete SAP_READ_ONLY":                    {``, map[string]string{"SAP_READ_ONLY": "true"}, "$ZDEMO", true, "read-only"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			hits, _, _ := uploadCLIEnv(t, c.extra)
			for k, v := range c.env {
				t.Setenv(k, v)
			}
			zipPath := writeCLIDemoZip(t)
			var err error
			if c.del {
				_ = gitDeleteObjectsCmd.Flags().Set("package", c.pkg)
				t.Cleanup(func() { _ = gitDeleteObjectsCmd.Flags().Set("package", "") })
				err = gitDeleteObjectsCmd.RunE(gitDeleteObjectsCmd, []string{"PROG ZDEMO_REPORT"})
			} else {
				_ = gitImportZipCmd.Flags().Set("package", c.pkg)
				t.Cleanup(func() { _ = gitImportZipCmd.Flags().Set("package", "") })
				err = gitImportZipCmd.RunE(gitImportZipCmd, []string{zipPath})
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Errorf("got %v, want %q", err, c.wantErr)
			}
			if n := hits(); n != 0 {
				t.Errorf("the system was contacted %d times before the refusal", n)
			}
		})
	}
}

// Waiting that ends in a refusal, a lost connection or Ctrl-C fails with
// the job number; --wait running out is a pending result.
func TestGitWaitError(t *testing.T) {
	if err := gitWaitError("ZVSP_GIT_IMPORT", "12345678", nil, nil); err != nil {
		t.Errorf("no error: %v", err)
	}
	if err := gitWaitError("ZVSP_GIT_IMPORT", "12345678", context.DeadlineExceeded, nil); err != nil {
		t.Errorf("--wait ran out: %v", err)
	}
	for _, c := range []struct{ werr, parent error }{
		{context.Canceled, context.Canceled},
		{context.DeadlineExceeded, context.Canceled},
		{errors.New("git.import_status: NOT_YOUR_JOB: no"), nil},
		// The connection lost while waiting: a failure, not a timeout.
		{fmt.Errorf("the push channel failed while waiting for job ZVSP_GIT_IMPORT 12345678: %w", adt.ErrWebSocketClosed), nil},
		{errors.New("reading the state of job ZVSP_GIT_IMPORT 12345678: git.import_status: not connected"), nil},
	} {
		err := gitWaitError("ZVSP_GIT_IMPORT", "12345678", c.werr, c.parent)
		if err == nil || !strings.Contains(err.Error(), "12345678") || !errors.Is(err, c.werr) {
			t.Errorf("%v / %v: %v", c.werr, c.parent, err)
		}
	}
}
