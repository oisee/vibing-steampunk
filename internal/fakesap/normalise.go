package fakesap

import (
	"regexp"
)

var (
	progressRun = regexp.MustCompile(`\r[^\r\n]*`)
	ageDays     = regexp.MustCompile(`("age_days":\s*)\d+|(age_days=)\d+|\(\d+ days?\)|\d+ days ago|(Age \(days\)\s*\|\s*)\d+|(age_days:\s*)\d+`)
	digits      = regexp.MustCompile(`\d+`)
	serverURL   = regexp.MustCompile(`http://127\.0\.0\.1:\d+`)
)

// Normalise removes from an output what changes between runs and is not part
// of the answer:
//
//   - the progress counter concurrent reads write to stderr, in any order;
//   - the age of a fixed date, which grows by one every day;
//   - the fake's port.
//
// Nothing is reordered. The boundary crossings, the "Packages crossed" lines
// and the lists of what could not be searched used to be sorted here, because
// the code under test printed them in map order. It now promises an order of
// its own (graph.CrossingDirectionOrder, BoundaryReport.SortedCrossedPackages,
// adtsource.Resolution.FailedLookups), and a golden that sorted them would
// hide a regression of that promise.
func Normalise(s string) string {
	s = progressRun.ReplaceAllString(s, "")
	s = ageDays.ReplaceAllStringFunc(s, func(m string) string {
		return digits.ReplaceAllString(m, "N")
	})
	return serverURL.ReplaceAllString(s, "http://fake")
}
