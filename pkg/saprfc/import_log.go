package saprfc

import (
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/oisee/open-rfc-go/rfc"
)

// TPALOG is the system's own record of what tp did with a request there: one
// row per step (import, activation, main import, generation, ...) and client,
// with the step's return code and time. It answers "has this request been
// imported here, and how did it go" without STMS, and it changes nothing.

// ImportStep is one tp step, as TPALOG records it.
type ImportStep struct {
	Client  string `json:"client"`
	Step    string `json:"step"`
	RetCode string `json:"retcode"`
	Time    string `json:"time"`
}

// ImportLog is what TPALOG holds for one request: its tp steps, oldest first,
// and the worst return code among them. No steps means tp has not touched the
// request in this system.
type ImportLog struct {
	Request string       `json:"request"`
	Steps   []ImportStep `json:"steps"`
	MaxRC   string       `json:"maxRc,omitempty"`
}

var (
	logRequestPattern = regexp.MustCompile(`^[A-Z0-9][A-Z0-9_-]{0,19}$`)
	logSincePattern   = regexp.MustCompile(`^([0-9]{8}|[0-9]{14})$`)
)

// ReadImportLog reads the tp steps TPALOG holds for the requests in the
// connected system, optionally only those at or after since (YYYYMMDD or
// YYYYMMDDhhmmss).
func ReadImportLog(ctx context.Context, c *rfc.Client, requests []string, since string) ([]ImportLog, error) {
	return importLogs(ctx, clientCall(c), requests, since)
}

// CheckImportLogArgs validates what ReadImportLog takes, without calling
// anything, so a caller can refuse before it logs on.
func CheckImportLogArgs(requests []string, since string) error {
	_, _, err := importLogArgs(requests, since)
	return err
}

func importLogArgs(requests []string, since string) ([]string, string, error) {
	var reqs []string
	for _, r := range requests {
		r = strings.ToUpper(strings.TrimSpace(r))
		// The number goes into the WHERE clause: only a plain request number.
		if !logRequestPattern.MatchString(r) {
			return nil, "", fmt.Errorf("%q is not a request number", r)
		}
		reqs = append(reqs, r)
	}
	if len(reqs) == 0 {
		return nil, "", fmt.Errorf("at least one request is required")
	}
	since = strings.TrimSpace(since)
	if since != "" && !logSincePattern.MatchString(since) {
		return nil, "", fmt.Errorf("since %q: want YYYYMMDD or YYYYMMDDhhmmss", since)
	}
	return reqs, since, nil
}

func importLogs(ctx context.Context, call callFn, requests []string, since string) ([]ImportLog, error) {
	reqs, since, err := importLogArgs(requests, since)
	if err != nil {
		return nil, err
	}

	quoted := make([]string, len(reqs))
	for i, r := range reqs {
		quoted[i] = "'" + r + "'"
	}
	// Separated by ", ": RFC_READ_TABLE's 72-character OPTIONS lines break
	// only between tokens, and an unbroken list of six requests is one token
	// too long.
	rows, err := readTable(ctx, call, "TPALOG", "TRKORR IN ( "+strings.Join(quoted, ", ")+" )",
		[]string{"TRKORR", "TRCLI", "TRSTEP", "RETCODE", "TRTIME"}, 0)
	if err != nil {
		return nil, fmt.Errorf("reading TPALOG: %w", err)
	}
	byReq := map[string][]ImportStep{}
	for _, row := range rows {
		byReq[row["TRKORR"]] = append(byReq[row["TRKORR"]],
			ImportStep{Client: row["TRCLI"], Step: row["TRSTEP"], RetCode: row["RETCODE"], Time: row["TRTIME"]})
	}

	out := make([]ImportLog, 0, len(reqs))
	for _, r := range reqs {
		steps := byReq[r]
		sort.SliceStable(steps, func(i, j int) bool { return steps[i].Time < steps[j].Time })
		l := ImportLog{Request: r, Steps: []ImportStep{}}
		for _, st := range steps {
			// A date alone compares as the start of that day.
			if since != "" && st.Time < since {
				continue
			}
			l.Steps = append(l.Steps, st)
			// RETCODE is a fixed-width number, so the strings order as the values do.
			if st.RetCode > l.MaxRC {
				l.MaxRC = st.RetCode
			}
		}
		out = append(out, l)
	}
	return out, nil
}
