package saprfc

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/oisee/vibing-steampunk/pkg/adt"
)

// An ADT request is an HTTP envelope, and two very different things can carry
// it: the classic-RFC tunnel through SADT_REST_RFC_ENDPOINT, and an ordinary
// HTTPS session. The debugger flow does not care which — it is SAP's own ADT
// resources either way — so the choice belongs here and nowhere else.
//
// The distinction that matters is not the protocol but the *session*. ADT keeps
// a debug session, and a lock, in an ABAP roll area; a caller that cannot get
// back to the same roll area cannot use either. Over RFC the roll area is the
// pinned conversation. Over HTTP it is the stateful ICF session that
// `sap-contextid` selects, which is why the HTTP transport below forces
// stateful mode on every request and why it must live as long as the loop does.
type ADTTransport interface {
	Do(ctx context.Context, req ADTRequest) (*ADTResponse, error)
}

// rfcTunnel carries ADT requests over a pinned RFC conversation.
type rfcTunnel struct{ caller rfcCaller }

// RFCTunnel returns the transport that tunnels ADT through classic RFC. Pass a
// pinned session, not the pooled client, or the ABAP session changes underneath
// the caller.
func RFCTunnel(caller rfcCaller) ADTTransport { return rfcTunnel{caller: caller} }

func (t rfcTunnel) Do(ctx context.Context, req ADTRequest) (*ADTResponse, error) {
	return CallADTOn(ctx, t.caller, req)
}

// httpSession carries ADT requests over a stateful HTTPS session — the route
// for systems where there is no RFC channel at all: a cookie, a single sign-on,
// no gateway port and no RFC password.
type httpSession struct {
	transport *adt.Transport
	// stateless sends each request outside any session; see HTTPStateless.
	stateless bool
}

// HTTPSession returns the transport that speaks to ADT directly. The caller
// keeps it for the whole conversation: a new transport is a new session, and a
// new session has no debuggee attached and no lock held.
func HTTPSession(transport *adt.Transport) ADTTransport {
	return httpSession{transport: transport}
}

// HTTPStateless returns a transport whose requests belong to no session. It
// is for the side requests a session cannot carry while it is busy: removing
// the listener while the listener request is still open on the session, where
// SAP would queue the removal behind the very request it is meant to end.
// Build it on an adt.Transport of its own, configured stateless, so that it
// carries no sap-contextid of the session's.
func HTTPStateless(transport *adt.Transport) ADTTransport {
	return httpSession{transport: transport, stateless: true}
}

func (t httpSession) Do(ctx context.Context, req ADTRequest) (*ADTResponse, error) {
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = "GET"
	}
	path, rawQuery, _ := strings.Cut(req.URI, "?")
	if !strings.HasPrefix(path, "/") {
		return nil, fmt.Errorf("ADT URI must be absolute, got %q", req.URI)
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil, fmt.Errorf("parsing the query of %q: %w", req.URI, err)
	}

	opts := &adt.RequestOptions{
		Method: method,
		Query:  query,
		Body:   req.Body,
		// Everything the debugger does is session-bound; so is a lock. Stateless
		// would work for a single read and fail for every sequence.
		Stateful: !t.stateless,
		Headers:  map[string]string{},
	}
	for _, h := range req.Headers {
		switch strings.ToLower(h.Name) {
		case "accept":
			opts.Accept = h.Value
		case "content-type":
			opts.ContentType = h.Value
		default:
			opts.Headers[h.Name] = h.Value
		}
	}
	if opts.Accept == "" {
		// Same rule as the RFC tunnel: ADT refuses a request with no Accept, and
		// a concrete type gives 406 on resources that answer another one.
		opts.Accept = "*/*"
	}

	res, err := t.transport.Request(ctx, path, opts)
	if err != nil {
		// A refused request still had an answer: ADT reports "no" as a status
		// and an exception document, and the transport turns that pair into an
		// error. Handing the answer back alongside the error loses nothing for
		// callers that only check err, and it lets anything wrapping this
		// transport — a recorder, a diagnostic — see what the server actually
		// said rather than a flattened string.
		var apiErr *adt.APIError
		if errors.As(err, &apiErr) {
			return &ADTResponse{
				Status:       apiErr.StatusCode,
				ReasonPhrase: http.StatusText(apiErr.StatusCode),
				Body:         []byte(apiErr.Message),
			}, err
		}
		return nil, err
	}
	out := &ADTResponse{Status: res.StatusCode, Body: res.Body}
	for name, values := range res.Headers {
		for _, v := range values {
			out.Headers = append(out.Headers, ADTHeader{Name: name, Value: v})
		}
	}
	return out, nil
}
