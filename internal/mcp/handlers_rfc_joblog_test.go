package mcp

import (
	"testing"

	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// The job log is readable while the job runs, so a job that has begun is asked
// for it. It used to be asked only once the job had ended, which left a long
// job invisible for its whole run.
//
// The gate is the status letter, not JobRun.Started: on op "run" that flag is
// set by BAPI_XBP_JOB_START_ASAP before any status is known, so a wait-0 run has
// Started=true and Status="" -- and asking XBP for the log of a job with no
// status returns an error that lands in job_log as if the job had failed.
func TestJobLogWantedReadsTheStatusLetter(t *testing.T) {
	tests := []struct {
		name   string
		params map[string]any
		run    *saprfc.JobRun
		want   bool
	}{
		{"running, nothing said", nil, &saprfc.JobRun{Status: "R", Started: true}, true},
		{"finished, nothing said", nil, &saprfc.JobRun{Status: "F", Started: true}, true},
		{"no status yet -- op run, wait 0", nil, &saprfc.JobRun{Status: "", Started: true}, false},
		{"scheduled", nil, &saprfc.JobRun{Status: "P", Started: true}, false},
		{"released", nil, &saprfc.JobRun{Status: "S", Started: true}, false},
		{"running, asked for", map[string]any{"joblog": true}, &saprfc.JobRun{Status: "R", Started: true}, true},
		{"running, refused", map[string]any{"joblog": false}, &saprfc.JobRun{Status: "R", Started: true}, false},
		{"scheduled, asked for", map[string]any{"joblog": true}, &saprfc.JobRun{Status: "S"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := jobLogWanted(tt.params, tt.run); got != tt.want {
				t.Fatalf("jobLogWanted = %t, want %t", got, tt.want)
			}
		})
	}
}
