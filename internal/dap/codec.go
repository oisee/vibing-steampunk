// Package dap is vsp's Debug Adapter Protocol server: it lets any DAP editor
// (VS Code, nvim-dap, JetBrains) drive the ABAP debugger on a real SAP system.
//
// It is a thin adapter. The debugger itself — breakpoints, the listener, attach,
// step, stack and variables over SAP's own /sap/bc/adt/debugger* resources on
// one held stateful session — is saprfc.Debugger, the same layer `vsp debug
// ui`, `vsp adt debug` and the MCP debug tools ride on. Nothing here talks HTTP.
//
// The codec is written by hand rather than taken from github.com/google/go-dap:
// the wire format is a Content-Length header and a JSON body, the adapter uses
// a couple of dozen fields, and a hand-written codec keeps vsp's dependency
// list where it is.
package dap

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// maxFrameBytes bounds one incoming message. A DAP request is a few hundred
// bytes; a header claiming gigabytes is a broken or hostile peer, and
// allocating for it would be how a malformed frame crashes the adapter.
const maxFrameBytes = 8 << 20

// ProtocolMessage is the envelope every DAP message shares. Requests carry
// Command and Arguments; the adapter's own messages are built from the typed
// structs below.
type ProtocolMessage struct {
	Seq       int             `json:"seq"`
	Type      string          `json:"type"`
	Command   string          `json:"command,omitempty"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Response answers one request.
type Response struct {
	Seq        int    `json:"seq"`
	Type       string `json:"type"`
	RequestSeq int    `json:"request_seq"`
	Success    bool   `json:"success"`
	Command    string `json:"command"`
	Message    string `json:"message,omitempty"`
	Body       any    `json:"body,omitempty"`
}

// Event is something the adapter tells the client unprompted.
type Event struct {
	Seq   int    `json:"seq"`
	Type  string `json:"type"`
	Event string `json:"event"`
	Body  any    `json:"body,omitempty"`
}

// errFrame marks a frame that could not be read but after which the stream is
// still usable: the reader skips it and carries on.
var errFrame = errors.New("malformed DAP frame")

// Conn reads requests from a client and writes responses and events to it.
// Writes are serialised, because events come from the background worker
// while responses come from the request loop.
type Conn struct {
	r   *bufio.Reader
	w   io.Writer
	mu  sync.Mutex
	seq int
}

// NewConn wraps a byte stream — stdin/stdout for `vsp dap`.
func NewConn(r io.Reader, w io.Writer) *Conn {
	return &Conn{r: bufio.NewReader(r), w: w}
}

// Read returns the next message. An error wrapping errFrame means that frame
// was unusable and skipped; any other error means the stream is gone.
func (c *Conn) Read() (*ProtocolMessage, error) {
	body, err := c.readFrame()
	if err != nil {
		return nil, err
	}
	var msg ProtocolMessage
	if err := json.Unmarshal(body, &msg); err != nil {
		return nil, fmt.Errorf("%w: %v", errFrame, err)
	}
	return &msg, nil
}

// maxDrainBytes is the largest oversized body that is read and thrown away to
// stay in step with the stream. A length beyond it is not believed: draining
// it would swallow every later message, so the header is dropped instead and
// the reader resynchronises on the next Content-Length it sees.
const maxDrainBytes = 64 << 20

// readFrame returns the body of the next frame.
func (c *Conn) readFrame() ([]byte, error) {
	length := -1
	sawHeader := false
	for {
		line, err := c.r.ReadString('\n')
		if err != nil {
			if err == io.EOF && line != "" {
				return nil, io.ErrUnexpectedEOF
			}
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if !sawHeader {
				continue // stray blank line between frames
			}
			break
		}
		sawHeader = true
		// Found anywhere in the line, so a header glued to the tail of a
		// broken frame's junk is still recognised.
		i := strings.Index(strings.ToLower(line), "content-length:")
		if i < 0 {
			continue // another header, or junk: skipped, not fatal
		}
		n, perr := strconv.Atoi(strings.TrimSpace(line[i+len("content-length:"):]))
		if perr != nil || n < 0 {
			length = -2
			continue
		}
		length = n
	}
	switch {
	case length == -1:
		return nil, fmt.Errorf("%w: no Content-Length header", errFrame)
	case length == -2:
		return nil, fmt.Errorf("%w: unreadable Content-Length", errFrame)
	case length > maxFrameBytes:
		if length <= maxDrainBytes {
			if _, err := io.CopyN(io.Discard, c.r, int64(length)); err != nil {
				return nil, err
			}
		}
		return nil, fmt.Errorf("%w: %d bytes exceeds the %d-byte limit", errFrame, length, maxFrameBytes)
	}
	body := make([]byte, length)
	if _, err := io.ReadFull(c.r, body); err != nil {
		return nil, err
	}
	return body, nil
}

// IsFrameError reports whether err is a skipped malformed frame rather than a
// dead stream.
func IsFrameError(err error) bool { return errors.Is(err, errFrame) }

func (c *Conn) write(v any, stamp func(seq int)) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.seq++
	stamp(c.seq)
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(c.w, "Content-Length: %d\r\n\r\n", len(body)); err != nil {
		return err
	}
	_, err = c.w.Write(body)
	return err
}

// Respond answers req. A nil error is success with body; otherwise the
// response fails with the error's text, which DAP clients show the user.
func (c *Conn) Respond(req *ProtocolMessage, body any, failure error) error {
	res := &Response{Type: "response", RequestSeq: req.Seq, Command: req.Command, Success: failure == nil, Body: body}
	if failure != nil {
		res.Message = failure.Error()
		res.Body = map[string]any{"error": map[string]any{"id": 1, "format": failure.Error(), "showUser": true}}
	}
	return c.write(res, func(seq int) { res.Seq = seq })
}

// Emit sends an event.
func (c *Conn) Emit(event string, body any) error {
	ev := &Event{Type: "event", Event: event, Body: body}
	return c.write(ev, func(seq int) { ev.Seq = seq })
}
