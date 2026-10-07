package saprfc

import (
	"slices"
	"testing"
)

// A poller has only the status letter to go on -- it is not the process that
// scheduled the job -- so "has it started" has to come from that letter. A flag
// held in the scheduling process is true there and false everywhere else, which
// is how a running job came to be reported as "started": false and its log
// withheld.
func TestJobStartedReadsTheStatusLetter(t *testing.T) {
	tests := map[string]bool{
		"R":   true,  // active
		"F":   true,  // finished
		"A":   true,  // cancelled
		"P":   false, // scheduled
		"S":   false, // released
		"Y":   false, // ready
		"":    false,
		" r ": true,
		"f":   true,
	}
	for status, want := range tests {
		if got := JobStarted(status); got != want {
			t.Errorf("JobStarted(%q) = %t, want %t", status, got, want)
		}
	}
}

// A poller builds its JobRun through PolledJob, so Started, StatusFor and the
// letter cannot disagree: all three are read from the one read in the one place.
func TestPolledJobDerivesTheWholeRunFromTheLetter(t *testing.T) {
	tests := []struct {
		status      string
		wantStarted bool
		wantFor     string
	}{
		{"R", true, "running"},
		{"F", true, "finished"},
		{"A", true, "cancelled"},
		{"P", false, "scheduled"},
		{"S", false, "released"},
		{"Y", false, "ready"},
		{"", false, ""},
		{" r ", true, "running"},
	}
	for _, tt := range tests {
		run := PolledJob(" zdemo_nightly ", "12345678", PolledJobStatus{Status: tt.status})
		if run.Started != tt.wantStarted {
			t.Errorf("status %q: Started = %t, want %t", tt.status, run.Started, tt.wantStarted)
		}
		if run.StatusFor != tt.wantFor {
			t.Errorf("status %q: StatusFor = %q, want %q", tt.status, run.StatusFor, tt.wantFor)
		}
		if run.JobName != "ZDEMO_NIGHTLY" || run.JobCount != "12345678" {
			t.Errorf("status %q: run identifies %q/%q", tt.status, run.JobName, run.JobCount)
		}
	}
}

// #353 asked for "started: true (and ideally the start time)". The stamp comes
// from the same TBTCO read as the letter and rides through the constructor as
// the system wrote it -- not as a time.Time, which would claim a zone the row
// does not carry.
func TestPolledJobCarriesTheStartStamp(t *testing.T) {
	const stamp = "20261003 215908"
	run := PolledJob("ZDEMO_NIGHTLY", "12345678", PolledJobStatus{Status: "R", StartStamp: stamp})
	if run.StartStamp != stamp {
		t.Errorf("StartStamp = %q, want %q", run.StartStamp, stamp)
	}
	if run.StatusFor != "running" {
		t.Errorf("StatusFor = %q, want %q derived from the letter", run.StatusFor, "running")
	}
	// A job that has not begun has no stamp, and PolledJob must not invent one.
	if notStarted := PolledJob("ZDEMO_NIGHTLY", "1", PolledJobStatus{Status: "P"}); notStarted.StartStamp != "" {
		t.Errorf("scheduled job carries StartStamp = %q, want empty", notStarted.StartStamp)
	}
}

// The letter set and the wait loop's rule have to agree for the letters TBTCO
// writes: begun, or still to wait on, never both. What the loop does with a
// letter it does not know is not fixed here.
func TestStatusLettersCoverTheAlphabet(t *testing.T) {
	for _, tt := range []struct {
		letter                string
		started, stillWaiting bool
	}{
		{"P", false, true},
		{"S", false, true},
		{"Y", false, true},
		{"R", true, true},
		{"F", true, false},
		{"A", true, false},
		{"", false, true}, // nothing read yet: keep asking
		{" r ", true, true},
	} {
		waiting := tt.letter == "" || slices.Contains(notEndedLetters, statusLetter(tt.letter))
		if got := JobStarted(tt.letter); got != tt.started {
			t.Errorf("JobStarted(%q) = %t, want %t", tt.letter, got, tt.started)
		}
		if waiting != tt.stillWaiting {
			t.Errorf("letter %q waits = %t, want %t", tt.letter, waiting, tt.stillWaiting)
		}
	}
}
