package sapcompress

import (
	"errors"
	"fmt"
)

// LZC is compress(1): LZW with codes that grow from nine bits to a limit the
// header states, a clear code of 256 in block mode, and the quirk that codes
// are read in chunks of codeWidth bytes so that a width change or a clear
// starts on a chunk boundary and the tail of the previous chunk is discarded.
// The dictionary is the usual prefix-code-plus-byte table; the special case
// where a code names the entry being defined is the KwKwK case of every LZW.

const (
	lzcMinWidth  = 9
	lzcMaxWidth  = 16
	lzcLiterals  = 256
	lzcClearCode = 256
)

type lzcReader struct {
	src   []byte
	pos   int
	chunk []byte
	cpos  int // bit position inside chunk
}

// nextChunk takes the next width bytes of input as the chunk codes are read
// from. At the end of the stream a shorter chunk is fine; codes stop when the
// bits do.
func (r *lzcReader) nextChunk(width int) {
	n := width
	if left := len(r.src) - r.pos; left < n {
		n = left
	}
	r.chunk = r.src[r.pos : r.pos+n]
	r.pos += n
	r.cpos = 0
}

func (r *lzcReader) bitsLeft() int { return len(r.chunk)*8 - r.cpos }

func (r *lzcReader) readBits(n int) int {
	v := 0
	for i := 0; i < n; i++ {
		bit := int(r.chunk[r.cpos/8]>>(r.cpos%8)) & 1
		v |= bit << i
		r.cpos++
	}
	return v
}

type lzcEntry struct {
	prefix int32 // code this entry extends
	last   byte  // byte it appends
	length int32 // bytes in the expanded string
}

func lzcDecode(body []byte, h Header) ([]byte, error) {
	blockMode := h.Extra&0x80 != 0
	limit := int(h.Extra & 0x1f)
	if limit < lzcMinWidth || limit > lzcMaxWidth {
		return nil, fmt.Errorf("code width limit %d outside %d..%d", limit, lzcMinWidth, lzcMaxWidth)
	}
	firstFree := lzcLiterals
	if blockMode {
		firstFree++
	}
	table := make([]lzcEntry, 1<<limit)
	for i := 0; i < lzcLiterals; i++ {
		table[i] = lzcEntry{prefix: -1, last: byte(i), length: 1}
	}

	r := &lzcReader{src: body}
	width := lzcMinWidth
	maxCode := (1 << width) - 1
	setWidth := func(w int) {
		width = w
		if w == limit {
			maxCode = 1 << limit
		} else {
			maxCode = (1 << w) - 1
		}
	}
	nextFree := firstFree
	r.nextChunk(width)

	readCode := func() (int, bool) {
		if r.bitsLeft() < width || nextFree > maxCode {
			if nextFree > maxCode {
				setWidth(width + 1)
			}
			r.nextChunk(width)
		}
		if r.bitsLeft() < width {
			return 0, false
		}
		return r.readBits(width), true
	}

	out := make([]byte, 0, h.Length)
	scratch := make([]byte, 0, 1<<limit)
	expand := func(code int) []byte {
		scratch = scratch[:table[code].length]
		for i := len(scratch) - 1; i >= 0; i-- {
			e := table[code]
			scratch[i] = e.last
			code = int(e.prefix)
		}
		return scratch
	}

	prev := -1
	for len(out) < h.Length {
		code, ok := readCode()
		if !ok {
			// The original library stops here too: the header's length is the
			// contract, and a stream that ends early is reported by the
			// length check in Decompress.
			break
		}
		if blockMode && code == lzcClearCode {
			nextFree = firstFree
			setWidth(lzcMinWidth)
			r.nextChunk(width)
			prev = -1
			continue
		}
		var chain []byte
		switch {
		case code < nextFree && (code < lzcLiterals || table[code].length > 0):
			chain = expand(code)
		case code == nextFree && prev >= 0:
			// KwKwK: the string is the previous one plus its own first byte.
			p := expand(prev)
			chain = append(p, p[0])
		default:
			return nil, fmt.Errorf("unknown code %d", code)
		}
		out = append(out, chain...)
		if prev >= 0 && nextFree < len(table) {
			table[nextFree] = lzcEntry{prefix: int32(prev), last: chain[0], length: table[prev].length + 1}
			nextFree++
		}
		prev = code
	}
	if len(out) > h.Length {
		return nil, errors.New("stream expanded past the header's length")
	}
	return out, nil
}
