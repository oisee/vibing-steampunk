package mcp

// What SAP() says when it is called with nothing.
//
// The universal tool answered `action is required` and named one thing to try.
// That is correct and it is the least useful correct answer available: the
// caller who sends no arguments is exactly the caller who does not yet know
// what this connects to, whether it is authenticated, or which build is
// answering. Every one of those is a round trip away and none of them were
// offered.
//
// So an empty call is a question — "what am I talking to?" — and this answers
// it. Four things, in the order somebody needs them:
//
//  1. Which build. An agent reporting a defect against "vsp" names nothing.
//  2. Whether the connection is alive and authenticated, which is the one
//     fact that decides whether any other call is worth making.
//  3. Which system, so a caller cannot act on production believing it is a
//     sandbox.
//  4. What to call next.
//
// Every field is best-effort and none of them can fail the card. A system that
// cannot be reached still gets an answer, and the answer says it cannot be
// reached — which is the most useful thing this can do at that moment.

import (
	"context"
	"fmt"
	"strings"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"
)

// handleInfo answers an empty call, and action="info".
func (s *Server) handleInfo(ctx context.Context) *mcp.CallToolResult {
	var b strings.Builder

	build := s.config.Build
	if build == "" {
		build = "unknown build — this answer cannot be dated against a fix"
	}
	fmt.Fprintf(&b, "vsp %s\n\n", build)

	// Reachability first, because everything below is a claim about a system
	// and this says whether the claims were checked or guessed.
	alive, reason := s.adtReachable(ctx)
	switch {
	case alive:
		fmt.Fprintf(&b, "  connection   authenticated as %s\n", s.userLabel())
	default:
		fmt.Fprintf(&b, "  connection   NOT usable — %s\n", reason)
	}

	if cs := s.adtClient.CacheStats(); cs.Enabled {
		fmt.Fprintf(&b, "  cache        %d hits, %d misses, %d entries (GET answers kept %s, dropped on any write)\n", cs.Hits, cs.Misses, cs.Entries, cs.TTL)
	}

	host, sysnr := saprfc.SysnrFromURL(s.config.BaseURL)
	if host != "" {
		fmt.Fprintf(&b, "  host         %s\n", host)
	}
	if sysnr != "" {
		// Said to be derived. It is a convention about ICM ports, not something
		// the system was asked, and a landscape that does not follow the
		// convention would be misreported by a card that stated it as fact.
		fmt.Fprintf(&b, "  instance     %s (derived from the port, not read from the system)\n", sysnr)
	} else {
		fmt.Fprintf(&b, "  instance     unknown — the port does not follow the 80NN/443NN/5NN00 convention\n")
	}

	// The system's own identity costs SQL, so it is asked for only when the
	// connection works, and its absence is reported rather than left blank.
	if alive {
		if info, err := s.adtClient.GetSystemInfo(ctx); err == nil {
			fmt.Fprintf(&b, "  system       %s client %s\n", info.SystemID, info.Client)
			if rel := strings.TrimSpace(info.SAPRelease); rel != "" {
				line := "  release      " + rel
				if db := strings.TrimSpace(info.DatabaseSystem); db != "" {
					line += " on " + db
				}
				b.WriteString(line + "\n")
			}
		} else {
			fmt.Fprintf(&b, "  system       could not be read: %v\n", err)
		}
	}

	if s.config.ReadOnly {
		b.WriteString("  mode         read-only — every write is refused before it is sent\n")
	}

	// What to do next depends on whether anything works. Handing five object
	// calls to a caller whose session is dead is five failures and no
	// diagnosis, and it is the caller least able to work out why.
	if alive {
		b.WriteString(`
Next call

  SAP(action="help")                         every action, with examples
  SAP(action="help", target="read")          one action in detail
  SAP(action="read", target="CLAS ZCL_X")    source, with dependency contracts
  SAP(action="search", target="ZCL_*")       find objects by name
  SAP(action="system", target="INFO")        the full system report

`)
		b.WriteString(validActionsLine)
		return mcp.NewToolResultText(b.String())
	}

	b.WriteString(`
Nothing else will work until that is fixed. The reason above is the whole
diagnosis; these are what it usually means:

  no such host / dial tcp     the URL or the network, not the credentials
  401                         user or password, or a locked account — do not
                              retry, a wrong password counts toward the lock
  not an ADT document         the session expired; SSO answers 200 with a
                              logon page, so re-authenticate rather than retry
  connection refused          ADT is on another port; "vsp detect" finds it

  SAP(action="help")          still works — it needs no system

`)
	return mcp.NewToolResultText(b.String())
}

// adtReachable asks the cheapest question only an authenticated session can
// answer.
//
// Deliberately not GetSystemInfo: that runs free SQL, which a system with
// --block-free-sql or a restrictive policy refuses — so a perfectly usable
// connection would report itself dead. CheckSession needs authentication and
// no SQL, and it already knows that an expired SSO session answers 200 with a
// logon page rather than 401.
func (s *Server) adtReachable(ctx context.Context) (bool, string) {
	if s.adtClient == nil {
		return false, "no SAP connection is configured"
	}
	if err := s.adtClient.CheckSession(ctx); err != nil {
		return false, err.Error()
	}
	return true, ""
}

// userLabel names who the session is, without ever printing a credential.
func (s *Server) userLabel() string {
	if u := strings.TrimSpace(s.config.Username); u != "" {
		return u
	}
	if len(s.config.Cookies) > 0 {
		return "a cookie session (no user name configured)"
	}
	return "an unnamed session"
}
