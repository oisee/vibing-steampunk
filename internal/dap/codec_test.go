package dap

import (
	"bytes"
	"io"
	"strings"
	"testing"
)

// FuzzReadFrame feeds the reader arbitrary bytes. Whatever arrives, it must
// return a frame or an error, never panic, never allocate past its limits,
// and always make progress.
func FuzzReadFrame(f *testing.F) {
	for _, seed := range []string{
		"0\x91ConteNt-Length:\n", // the reduced crash: an invalid UTF-8 byte before the header
		"\x91\x92\x93Content-Length: 2\r\n\r\n{}",
		"Content-Length: 2\r\n\r\n{}",
		"Content-Length: 99999999999\r\n\r\n",
		"Content-Length: -1\r\n\r\n",
		"content-length:3\n\n{}}",
		"X: y\r\n\r\n",
		"\r\n\r\n\r\n",
		strings.Repeat("A", 70000) + "\r\nContent-Length: 2\r\n\r\n{}",
		strings.Repeat("H: v\r\n", 300) + "Content-Length: 2\r\n\r\n{}",
		"Content-Length: 5\r\n\r\n{\"a\":",
		"Content-Length: 99999999999\r\n\r\n" + strings.Repeat("x", 5000), // a claimed giant body, sent without newlines
	} {
		f.Add([]byte(seed))
	}
	f.Fuzz(func(t *testing.T, data []byte) {
		conn := NewConn(bytes.NewReader(data), io.Discard)
		// Every Read consumes at least one byte or ends the stream, so the
		// input length bounds the loop.
		for i := 0; i <= len(data)+1; i++ {
			_, err := conn.Read()
			if err != nil && !IsFrameError(err) {
				return
			}
		}
		t.Fatalf("the reader made no progress on %q", data)
	})
}

// The fuzz crash, as a plain test.
func TestReadFrameInvalidUTF8BeforeHeader(t *testing.T) {
	conn := NewConn(strings.NewReader("0\x91ConteNt-Length:\n"), io.Discard)
	for i := 0; i < 5; i++ {
		if _, err := conn.Read(); err != nil && !IsFrameError(err) {
			return
		}
	}
}

// A header line or header block too long to be a header is junk: refused
// without being held in memory, and the next frame still reads.
func TestReadFrameBoundsTheHeader(t *testing.T) {
	long := strings.Repeat("A", 1<<20) + "\r\n\r\n" + frame(`{"seq":1,"type":"request","command":"threads"}`)
	conn := NewConn(strings.NewReader(long), io.Discard)
	if _, err := conn.Read(); !IsFrameError(err) {
		t.Fatalf("a 1 MB header line: %v", err)
	}
	msg, err := conn.Read()
	if err != nil || msg.Command != "threads" {
		t.Fatalf("the next frame: %+v %v", msg, err)
	}

	many := strings.Repeat("H: v\r\n", 1000) + "\r\n" + frame(`{"seq":2,"type":"request","command":"threads"}`)
	conn = NewConn(strings.NewReader(many), io.Discard)
	sawFrameErr := false
	for {
		msg, err := conn.Read()
		if IsFrameError(err) {
			sawFrameErr = true
			continue
		}
		if err != nil {
			t.Fatalf("a thousand header lines: %v (frame error seen: %v)", err, sawFrameErr)
		}
		if msg.Command == "threads" {
			break
		}
	}
	if !sawFrameErr {
		t.Error("a thousand header lines were accepted as one header block")
	}
}

// A body claimed beyond what can be drained is not a DAP peer: the stream is
// ended with an error rather than read on, header-hunting through gigabytes.
func TestAbsurdLengthEndsTheStream(t *testing.T) {
	conn := NewConn(strings.NewReader("Content-Length: 99999999999\r\n\r\n"+strings.Repeat("x", 100000)+"Content-Length: 2\r\n\r\n{}"), io.Discard)
	_, err := conn.Read()
	if err == nil || IsFrameError(err) {
		t.Fatalf("an absurd length should end the stream, got %v", err)
	}
}
