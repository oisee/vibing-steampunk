package embedded

import (
	"regexp"
	"testing"
)

// The classes install deploys run inside the ZADT_VSP APC handler, which is
// stateful and so in non-blocking mode: SUBMIT (VIA JOB too), sRFC with
// DESTINATION and WAIT dump there with APC_ILLEGAL_STATEMENT. The report
// service did SUBMIT ... AND RETURN for months, and every RunReport ended in
// that dump and a client-side timeout (#55). Programs are job steps, not APC
// code, so they may.
var apcForbidden = regexp.MustCompile(`(?im)^[ \t]*(SUBMIT\b|WAIT\s+(UP|FOR)\b|CALL\s+FUNCTION\s+\S+\s+DESTINATION\b)`)

func TestAPCClassesUseNoForbiddenStatements(t *testing.T) {
	for _, obj := range GetObjects() {
		if obj.Type == "PROG" {
			continue
		}
		for _, m := range apcForbidden.FindAllString(obj.Source, -1) {
			t.Errorf("%s: %q is forbidden in APC (APC_ILLEGAL_STATEMENT)", obj.Name, m)
		}
	}
}
