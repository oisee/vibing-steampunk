// Package sapcompress decodes the two compression formats SAP applies to its
// own byte streams: the ones behind the 1F 9D signature in RFC and DIAG
// traffic, in SAPCAR archives, and in every EXPORT ... TO DATABASE cluster —
// BALDAT, INDX, STXL and their kin.
//
// SAP calls them LZH and LZC. Neither is the thing its name suggests. LZH is
// raw DEFLATE (RFC 1951) — the same literal/length alphabet, the same thirty
// distance codes, the same fixed Huffman trees, the same code-length order —
// wearing an eight-byte SAP header and a two-bit prefix that says how many
// junk bits follow it before the first block. Once that prefix is stripped the
// standard library inflates it. LZC is the LZW variant of compress(1), with
// block mode and the code-width padding quirk of that program, and has no
// standard-library equivalent, so it is implemented here.
//
// Only decompression is provided. Nothing in this repository writes a cluster.
package sapcompress

import (
	"bytes"
	"compress/flate"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Algorithm identifies the compression scheme named in a SAP header.
type Algorithm byte

const (
	// LZC is the compress(1)-style LZW variant. Header algorithm byte 0x10.
	LZC Algorithm = 1
	// LZH is DEFLATE with a SAP prefix. Header algorithm byte 0x12.
	LZH Algorithm = 2
)

func (a Algorithm) String() string {
	switch a {
	case LZC:
		return "LZC"
	case LZH:
		return "LZH"
	}
	return fmt.Sprintf("algorithm %d", byte(a))
}

// HeaderSize is the length of the SAP compression header.
const HeaderSize = 8

// Header is the eight bytes every SAP-compressed stream starts with.
type Header struct {
	// Length is the size of the uncompressed data, which the stream states up
	// front and which the decoder holds it to.
	Length int
	// Algorithm and Version come from one byte: the low nibble is the
	// algorithm, the high nibble its version.
	Algorithm Algorithm
	Version   byte
	// Extra is the eighth byte. LZC keeps its block mode and code-width limit
	// there; LZH does not use it.
	Extra byte
}

var magic = []byte{0x1f, 0x9d}

// ErrNotCompressed is returned when the 1F 9D signature is missing, which is
// what an uncompressed cluster body looks like.
var ErrNotCompressed = errors.New("sapcompress: no SAP compression signature")

// ParseHeader reads the header without decompressing anything.
func ParseHeader(data []byte) (Header, error) {
	if len(data) < HeaderSize {
		return Header{}, fmt.Errorf("sapcompress: %d bytes is shorter than the %d-byte header", len(data), HeaderSize)
	}
	if !bytes.Equal(data[5:7], magic) {
		return Header{}, ErrNotCompressed
	}
	return Header{
		Length:    int(binary.LittleEndian.Uint32(data[:4])),
		Algorithm: Algorithm(data[4] & 0x0f),
		Version:   data[4] >> 4,
		Extra:     data[7],
	}, nil
}

// Decompress decodes a complete SAP-compressed stream, header included, and
// returns exactly the number of bytes the header promised.
func Decompress(data []byte) ([]byte, error) {
	h, err := ParseHeader(data)
	if err != nil {
		return nil, err
	}
	var out []byte
	switch h.Algorithm {
	case LZH:
		out, err = inflate(data[HeaderSize:], h.Length)
	case LZC:
		out, err = lzcDecode(data[HeaderSize:], h)
	default:
		return nil, fmt.Errorf("sapcompress: unknown %s", h.Algorithm)
	}
	if err != nil {
		return nil, fmt.Errorf("sapcompress: %s: %w", h.Algorithm, err)
	}
	if len(out) != h.Length {
		return nil, fmt.Errorf("sapcompress: %s: header promised %d bytes, stream held %d", h.Algorithm, h.Length, len(out))
	}
	return out, nil
}

// inflate decodes the LZH body: a two-bit count of noise bits, that many
// noise bits, then DEFLATE blocks. DEFLATE is read least-significant-bit
// first, so dropping the prefix is a right shift of the whole buffer by that
// many bits, after which compress/flate reads it as a raw stream. SAP never
// emits stored blocks, which are the only part of DEFLATE that would care
// about byte alignment.
func inflate(body []byte, length int) ([]byte, error) {
	if len(body) == 0 {
		return nil, errors.New("empty body")
	}
	prefix := uint(2 + body[0]&0x03)
	shifted := make([]byte, len(body))
	for i := range body {
		v := body[i] >> prefix
		if i+1 < len(body) {
			v |= body[i+1] << (8 - prefix)
		}
		shifted[i] = v
	}
	r := flate.NewReader(bytes.NewReader(shifted))
	defer r.Close()
	out := make([]byte, 0, length)
	buf := bytes.NewBuffer(out)
	// One byte more than promised, so an overlong stream is detected rather
	// than silently truncated to the header's figure.
	if _, err := io.CopyN(buf, r, int64(length)+1); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	return buf.Bytes(), nil
}
