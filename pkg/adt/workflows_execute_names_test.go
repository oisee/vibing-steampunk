package adt

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

// programPoolServer stands in for SAP's repository as far as the ExecuteABAP
// name pool depends on it: creating a name that exists is refused the way SAP
// refuses it, and a DELETE frees the name again.
//
// With holdRuns set, each test run waits until that many programs exist at
// the same time, so that parallel runs really are in flight together rather
// than one after the other.
type programPoolServer struct {
	existing map[string]bool
	holdRuns int

	url string

	mu      sync.Mutex
	created []string
	refused []string
	locked  []string
	deleted []string
	enough  chan struct{}
}

// The program's own name comes first in the create body; the package
// reference ($TMP) is skipped in case it ever does not.
var createdNameAttr = regexp.MustCompile(`adtcore:name="([^"$][^"]*)"`)

func (s *programPoolServer) start(t *testing.T) *Client {
	t.Helper()
	if s.existing == nil {
		s.existing = make(map[string]bool)
	}
	s.enough = make(chan struct{})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/discovery"):
			w.Header().Set("X-CSRF-Token", "TOKEN")
			w.WriteHeader(http.StatusOK)

		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/programs/programs"):
			body, _ := io.ReadAll(r.Body)
			match := createdNameAttr.FindSubmatch(body)
			if match == nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			s.create(w, strings.ToUpper(string(match[1])))

		case r.Method == http.MethodPost && r.URL.Query().Get("_action") == "LOCK":
			s.note(&s.locked, strings.ToUpper(path.Base(r.URL.Path)))
			w.Header().Set("Content-Type", "application/vnd.sap.as+xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<asx:abap xmlns:asx="http://www.sap.com/abapxml"><asx:values><DATA>
<LOCK_HANDLE>HANDLE-1</LOCK_HANDLE><MODIFICATION_SUPPORT>Modification</MODIFICATION_SUPPORT>
</DATA></asx:values></asx:abap>`))

		case strings.Contains(r.URL.Path, "/abapunit/testruns"):
			if s.holdRuns > 0 {
				select {
				case <-s.enough:
				case <-time.After(5 * time.Second):
				}
			}
			w.Header().Set("Content-Type", "application/xml")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(successfulExecuteRunResult))

		case r.Method == http.MethodDelete:
			name := strings.ToUpper(path.Base(r.URL.Path))
			s.mu.Lock()
			delete(s.existing, name)
			s.deleted = append(s.deleted, name)
			s.mu.Unlock()
			w.WriteHeader(http.StatusOK)

		default:
			// Package check, activation, source PUT, UNLOCK: all accepted.
			w.WriteHeader(http.StatusOK)
		}
	}))
	t.Cleanup(srv.Close)
	s.url = srv.URL
	return s.client()
}

// client is another client of the same server: another vsp process, with a
// pool of its own, against the same SAP system.
func (s *programPoolServer) client() *Client {
	cfg := NewConfig(s.url, "TESTUSER", "secret")
	return NewClientWithTransport(cfg, NewTransport(cfg))
}

func (s *programPoolServer) create(w http.ResponseWriter, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.existing[name] {
		s.refused = append(s.refused, name)
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprintf(w, `<?xml version="1.0" encoding="utf-8"?><exc:exception xmlns:exc="http://www.sap.com/abapxml/types/communicationframework"><namespace id="com.sap.adt"/><type id="ExceptionResourceAlreadyExists"/><message lang="EN">Program %s does already exist</message></exc:exception>`, name)
		return
	}
	s.existing[name] = true
	s.created = append(s.created, name)
	if s.holdRuns > 0 && len(s.existing) == s.holdRuns {
		close(s.enough)
	}
	w.WriteHeader(http.StatusOK)
}

func (s *programPoolServer) note(list *[]string, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	*list = append(*list, name)
}

func (s *programPoolServer) snapshot(list *[]string) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), (*list)...)
}

func (s *programPoolServer) exists(name string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.existing[name]
}

// --- the name of a slot ---

func TestPooledProgramName(t *testing.T) {
	cases := []struct {
		prefix, user string
		slot         int
		want         string
	}{
		{"ZTEMP_EXEC_", "TESTUSER", 0, "ZTEMP_EXEC_TESTUSER_0"},
		{"ztemp_exec_", "testuser", 9, "ZTEMP_EXEC_TESTUSER_9"},
		// SSO or a cookie file: no user to put in the name.
		{"ZTEMP_EXEC_", "", 3, "ZTEMP_EXEC_3"},
		// Only what a program name may hold is kept from the user.
		{"ZTEMP_EXEC_", "j.doe@corp-1", 1, "ZTEMP_EXEC_JDOECORP1_1"},
		// Cut short to fit 30 characters: 11 + 17 + "_" + 1.
		{"ZTEMP_EXEC_", "ABCDEFGHIJKLMNOPQRSTUVWXYZ", 2, "ZTEMP_EXEC_ABCDEFGHIJKLMNOPQ_2"},
		// A prefix that leaves no room for the user goes without it.
		{"Z" + strings.Repeat("X", 27), "DEV", 4, "Z" + strings.Repeat("X", 27) + "4"},
		{"Z" + strings.Repeat("X", 28), "DEV", 5, "Z" + strings.Repeat("X", 28) + "5"},
	}
	for _, tc := range cases {
		got, err := pooledProgramName(tc.prefix, tc.user, tc.slot)
		if err != nil {
			t.Fatalf("pooledProgramName(%q, %q, %d): %v", tc.prefix, tc.user, tc.slot, err)
		}
		if got != tc.want {
			t.Errorf("pooledProgramName(%q, %q, %d) = %q, want %q", tc.prefix, tc.user, tc.slot, got, tc.want)
		}
		if len(got) > maxProgramNameLength {
			t.Errorf("%q has %d characters, more than a program name may have", got, len(got))
		}
	}

	if name, err := pooledProgramName(strings.Repeat("Z", 30), "DEV", 0); err == nil {
		t.Fatalf("a 30-character prefix gave %q, want an error: the slot number no longer fits", name)
	}
}

func TestTemporaryNameTaken(t *testing.T) {
	alreadyExists := &APIError{StatusCode: http.StatusBadRequest, Message: `<type id="ExceptionResourceAlreadyExists"/><message lang="EN">Program ZTEMP_EXEC_TESTUSER_0 does already exist</message>`}
	lockConflict := &APIError{StatusCode: http.StatusForbidden, Message: "User OTHER is currently editing ZTEMP_EXEC_TESTUSER_0"}
	serverError := &APIError{StatusCode: http.StatusInternalServerError, Message: "boom"}

	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"already exists", fmt.Errorf("creating object: %w", alreadyExists), true},
		{"locked by another session, then found in place", &PartialCreateError{LeftInPlace: true, OriginalErr: fmt.Errorf("creating object: %w", lockConflict)}, true},
		// A failure that may have created the program anyway is not "taken":
		// moving on could leave a second half-created program behind.
		{"failed, then found in place", &PartialCreateError{LeftInPlace: true, OriginalErr: fmt.Errorf("creating object: %w", serverError)}, false},
		{"failed", fmt.Errorf("creating object: %w", serverError), false},
		{"cancelled", context.Canceled, false},
	}
	for _, tc := range cases {
		if got := temporaryNameTaken(tc.err); got != tc.want {
			t.Errorf("%s: temporaryNameTaken = %v, want %v", tc.name, got, tc.want)
		}
	}
}

// --- the pool through ExecuteABAP ---

func TestExecuteABAPReusesOneNameRunAfterRun(t *testing.T) {
	srv := &programPoolServer{}
	client := srv.start(t)

	for run := 1; run <= 3; run++ {
		result, err := client.ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
		if err != nil {
			t.Fatalf("run %d: %v", run, err)
		}
		if !result.Success || !result.CleanedUp {
			t.Fatalf("run %d: result = %+v, want success and cleanup", run, result)
		}
		if result.ProgramName != "ZTEMP_EXEC_TESTUSER_0" {
			t.Fatalf("run %d: program = %q, want ZTEMP_EXEC_TESTUSER_0 every time", run, result.ProgramName)
		}
	}

	// Another process of the same user, once the first is done, gets the
	// same name too: the pool is the user's, not the process's.
	result, err := srv.client().ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
	if err != nil || result.ProgramName != "ZTEMP_EXEC_TESTUSER_0" {
		t.Fatalf("other process: program = %q, err = %v, want ZTEMP_EXEC_TESTUSER_0", result.ProgramName, err)
	}
	if refused := srv.snapshot(&srv.refused); len(refused) != 0 {
		t.Fatalf("creates refused = %v, want none when each run deletes its program", refused)
	}
}

func TestExecuteABAPMovesPastANameThatExists(t *testing.T) {
	// Slot 0 is someone else's run, or a program a failed cleanup left.
	srv := &programPoolServer{existing: map[string]bool{"ZTEMP_EXEC_TESTUSER_0": true}}
	result, err := srv.start(t).ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
	if err != nil {
		t.Fatalf("ExecuteABAP: %v", err)
	}
	if !result.Success || !result.CleanedUp || result.ProgramName != "ZTEMP_EXEC_TESTUSER_1" {
		t.Fatalf("result = %+v, want success in ZTEMP_EXEC_TESTUSER_1, cleaned up", result)
	}
	locked, deleted := srv.snapshot(&srv.locked), srv.snapshot(&srv.deleted)
	for _, name := range append(append([]string(nil), locked...), deleted...) {
		if name == "ZTEMP_EXEC_TESTUSER_0" {
			t.Fatalf("locked %v, deleted %v: the program in slot 0 is not this run's to touch", locked, deleted)
		}
	}
	if !srv.exists("ZTEMP_EXEC_TESTUSER_0") {
		t.Fatal("the program in slot 0 is gone")
	}
}

func TestExecuteABAPKeptProgramHoldsItsSlot(t *testing.T) {
	srv := &programPoolServer{}
	client := srv.start(t)

	kept, err := client.ExecuteABAP(context.Background(), "lv_result = 'ok'.", &ExecuteABAPOptions{KeepProgram: true})
	if err != nil || kept.CleanedUp || kept.ProgramName != "ZTEMP_EXEC_TESTUSER_0" {
		t.Fatalf("kept run: %+v, err = %v, want ZTEMP_EXEC_TESTUSER_0 left in place", kept, err)
	}
	for run := 1; run <= 2; run++ {
		result, err := client.ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
		if err != nil || !result.CleanedUp || result.ProgramName != "ZTEMP_EXEC_TESTUSER_1" {
			t.Fatalf("run %d after the kept one: %+v, err = %v, want ZTEMP_EXEC_TESTUSER_1, cleaned up", run, result, err)
		}
	}
	if !srv.exists("ZTEMP_EXEC_TESTUSER_0") {
		t.Fatal("the kept program was deleted")
	}
}

func TestExecuteABAPReportsAFullPool(t *testing.T) {
	srv := &programPoolServer{existing: map[string]bool{}}
	for slot := 0; slot < temporaryProgramSlots; slot++ {
		srv.existing[fmt.Sprintf("ZTEMP_EXEC_TESTUSER_%d", slot)] = true
	}
	result, err := srv.start(t).ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
	if err != nil {
		t.Fatalf("ExecuteABAP: %v", err)
	}
	if result.Success || result.ProgramName != "" {
		t.Fatalf("result = %+v, want a failure with no program of its own", result)
	}
	if !strings.Contains(result.Message, "all 10 temporary program names are in use") ||
		!strings.Contains(result.Message, "ZTEMP_EXEC_TESTUSER_0") || !strings.Contains(result.Message, "ZTEMP_EXEC_TESTUSER_9") {
		t.Fatalf("message = %q, want the full pool named", result.Message)
	}
	if created, locked, deleted := srv.snapshot(&srv.created), srv.snapshot(&srv.locked), srv.snapshot(&srv.deleted); len(created)+len(locked)+len(deleted) != 0 {
		t.Fatalf("created %v, locked %v, deleted %v: nothing of anyone else's may be touched", created, locked, deleted)
	}
}

func TestExecuteABAPStopsAtAFailureThatIsNotATakenName(t *testing.T) {
	// A create that fails for another reason may still have created the
	// program; trying the next slot could leave a second one behind.
	srv := &executeCleanupServer{createStatus: http.StatusInternalServerError}
	result, err := srv.start(t).ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
	if err != nil {
		t.Fatalf("ExecuteABAP: %v", err)
	}
	if got := srv.count("create"); got != 1 {
		t.Fatalf("creates = %d, want 1: the search stops at the first failure that is not a taken name", got)
	}
	if result.ProgramName != "ZTEMP_EXEC_TESTUSER_0" {
		t.Fatalf("program = %q, want the slot whose create failed", result.ProgramName)
	}
}

func TestExecuteABAPParallelRunsTakeDifferentNames(t *testing.T) {
	run := func(t *testing.T, clients ...*Client) {
		t.Helper()
		names := make([]string, len(clients))
		errs := make([]error, len(clients))
		var wg sync.WaitGroup
		for i, client := range clients {
			wg.Add(1)
			go func(i int, client *Client) {
				defer wg.Done()
				result, err := client.ExecuteABAP(context.Background(), "lv_result = 'ok'.", nil)
				switch {
				case err != nil:
					errs[i] = err
				case !result.Success || !result.CleanedUp:
					errs[i] = errors.New(result.Message)
				default:
					names[i] = result.ProgramName
				}
			}(i, client)
		}
		wg.Wait()
		for i, err := range errs {
			if err != nil {
				t.Fatalf("run %d: %v", i, err)
			}
		}
		if names[0] == names[1] {
			t.Fatalf("both runs used %s", names[0])
		}
		for _, name := range names {
			if name != "ZTEMP_EXEC_TESTUSER_0" && name != "ZTEMP_EXEC_TESTUSER_1" {
				t.Fatalf("names = %v, want slots 0 and 1", names)
			}
		}
	}

	t.Run("same process", func(t *testing.T) {
		srv := &programPoolServer{holdRuns: 2}
		client := srv.start(t)
		run(t, client, client)
		// Within one process the names in use are set aside before the
		// create, so SAP never has to refuse one.
		if refused := srv.snapshot(&srv.refused); len(refused) != 0 {
			t.Fatalf("creates refused = %v, want none within one process", refused)
		}
	})

	t.Run("two processes", func(t *testing.T) {
		srv := &programPoolServer{holdRuns: 2}
		first := srv.start(t)
		run(t, first, srv.client())
		// Across processes, SAP's refusal is what sends one of them on.
		if refused := srv.snapshot(&srv.refused); len(refused) != 1 || refused[0] != "ZTEMP_EXEC_TESTUSER_0" {
			t.Fatalf("creates refused = %v, want ZTEMP_EXEC_TESTUSER_0 once", refused)
		}
	})
}
