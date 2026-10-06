// handlers_debug_session.go gives the MCP server a debug session that outlives a
// single tool call.
//
// This is the piece the debugger was missing. Attaching to a debuggee hands back
// a reference that lives in an ABAP roll area, and every later operation hangs
// off it — so a client that opens a connection per call attaches in one and
// finds nothing in the next. That is why the debugger tools shipped disabled and
// why a WebSocket to ZADT_VSP was built to hold the session for them.
//
// An MCP server is a long-lived process, so it can simply hold the session
// itself: one pinned RFC conversation, or one stateful ADT session where there
// is no RFC channel, kept from the first debug call to the detach. No daemon and
// no Z code.
package mcp

import (
	"context"
	"fmt"
	"time"

	"github.com/oisee/vibing-steampunk/pkg/adt"
	"github.com/oisee/vibing-steampunk/pkg/saprfc"

	openrfc "github.com/oisee/open-rfc-go/rfc"
)

// debugCallTimeout has to outlast a blocking listener, which is a request that
// deliberately does not answer until a debuggee stops.
const debugCallTimeout = 5 * time.Minute

// debugSession is the server's one debugger and whatever carries it.
type debugSession struct {
	dbg *saprfc.Debugger
	// conn is the RFC client whose connection the session pinned, kept so it can
	// be closed with the session. Nil on the HTTPS route.
	conn *openrfc.Client
	// route names the transport in every answer, because the two differ in what
	// they can do — bp_list reads the server's own breakpoint table only over
	// RFC — and a caller that cannot see which one it got cannot know that.
	route string
	user  string
	// http is the session's own ADT transport on the HTTPS route, closed with
	// the session: behind a transport command it owns a helper process. Nil
	// on the RFC route.
	http *adt.Transport
}

// debugger returns the server's debug session, opening one on first use.
//
// RFC is preferred when the system has a gateway: the tunnel carries the same
// ADT resources, and a pinned conversation additionally reaches the ZADT_DEBUG
// facade where it is installed. HTTPS is the fallback and is not a lesser one —
// the conformance test requires both to answer identically.
func (s *Server) debugger(ctx context.Context) (*debugSession, error) {
	s.debugMu.Lock()
	defer s.debugMu.Unlock()
	if s.debugSess != nil {
		return s.debugSess, nil
	}

	user := s.config.Username
	if c, err := s.dialRFC(ctx, nil); err == nil {
		dbg, derr := saprfc.NewDebugger(ctx, c, user)
		if derr == nil {
			s.debugSess = &debugSession{dbg: dbg, conn: c, route: "rfc", user: user}
			return s.debugSess, nil
		}
		_ = c.Close(ctx)
	}

	transport, httpTransport, err := s.statefulADTSession()
	if err != nil {
		return nil, fmt.Errorf("no debug session: neither an RFC channel nor a stateful ADT session could be opened: %w", err)
	}
	s.debugSess = &debugSession{
		dbg:   saprfc.NewADTDebugger(transport, user),
		route: "https",
		user:  user,
		http:  httpTransport,
	}
	return s.debugSess, nil
}

// statefulADTTransport builds the HTTPS session the debugger holds for its whole
// life. It is deliberately not the server's ordinary ADT client: that one is
// stateless by design, and a stateless session is exactly what cannot hold a
// debuggee.
func (s *Server) statefulADTTransport() (saprfc.ADTTransport, error) {
	t, _, err := s.statefulADTSession()
	return t, err
}

// statefulADTSession is statefulADTTransport plus the adt.Transport under it,
// which the session closes when it ends.
func (s *Server) statefulADTSession() (saprfc.ADTTransport, *adt.Transport, error) {
	if s.config.BaseURL == "" {
		return nil, nil, fmt.Errorf("no system URL configured")
	}
	opts := []adt.Option{
		adt.WithClient(s.config.Client),
		adt.WithLanguage(s.config.Language),
		adt.WithSessionType(adt.SessionStateful),
		// The listener is a request that does not answer until something stops.
		adt.WithTimeout(debugCallTimeout),
	}
	if s.config.InsecureSkipVerify {
		opts = append(opts, adt.WithInsecureSkipVerify())
	}
	// The debug session is its own connection; it carries the server's pin.
	if s.adtClient != nil {
		if pin, _, _ := s.adtClient.IdentityStatus(); pin != nil {
			opts = append(opts, adt.WithExpect(*pin))
		}
	}
	// A transport command is a connection of its own too: the debug session
	// gets its own helper process, and so its own session behind it.
	if len(s.config.TransportCmd) > 0 {
		opts = append(opts, adt.WithTransportCmd(s.config.TransportCmd))
	}
	user, password := s.config.Username, s.config.Password
	if len(s.config.Cookies) > 0 {
		opts = append(opts, adt.WithCookies(s.config.Cookies))
		user, password = "", ""
	}
	cfg := adt.NewConfig(s.config.BaseURL, user, password, opts...)
	t := adt.NewTransport(cfg)
	return saprfc.HTTPSession(t), t, nil
}

// closeDebugSession releases the debuggee, removes the listener registration and
// gives the connection back. Leaving a debuggee attached suspends somebody's
// work process until their call times out, so this runs on detach and on server
// shutdown, not only when a caller remembers.
func (s *Server) closeDebugSession(ctx context.Context) {
	s.debugMu.Lock()
	sess := s.debugSess
	s.debugSess = nil
	s.debugMu.Unlock()
	if sess == nil {
		return
	}
	_ = sess.dbg.ADTDetach(ctx)
	_ = sess.dbg.Close(ctx)
	if sess.conn != nil {
		_ = sess.conn.Close(ctx)
	}
	// Last, after the detach above has gone over it: a transport command's
	// helper is told to exit.
	if sess.http != nil {
		_ = sess.http.CloseTransport()
	}
}
