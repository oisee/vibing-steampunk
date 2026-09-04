package sapcompress

import (
	"bytes"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The fixtures were produced by an independent implementation (pysap's
// pysapcompress, itself a port of the MaxDB code) compressing synthetic
// inputs: prose, UTF-16 text of the shape a cluster holds, random bytes that
// do not compress, three bytes, five thousand zeros, and a 55 KB repeating
// pattern that spans many DEFLATE blocks and forces LZC through every code
// width up to its limit.
var fixtures = []string{"text", "utf16", "random", "short", "zeros", "big"}

func load(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if strings.HasSuffix(name, ".hex") {
		data, err = hex.DecodeString(strings.TrimSpace(string(data)))
		if err != nil {
			t.Fatal(err)
		}
	}
	return data
}

func TestDecompressFixtures(t *testing.T) {
	for _, name := range fixtures {
		want := load(t, name+".bin")
		for _, alg := range []string{"lzh", "lzc"} {
			t.Run(name+"/"+alg, func(t *testing.T) {
				comp := load(t, name+"."+alg+".hex")
				h, err := ParseHeader(comp)
				if err != nil {
					t.Fatal(err)
				}
				if h.Length != len(want) {
					t.Fatalf("header length %d, fixture %d", h.Length, len(want))
				}
				got, err := Decompress(comp)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(got, want) {
					t.Fatalf("%s: output differs (%d bytes, want %d)", alg, len(got), len(want))
				}
			})
		}
	}
}

func TestHeader(t *testing.T) {
	lzh := load(t, "text.lzh.hex")
	h, err := ParseHeader(lzh)
	if err != nil {
		t.Fatal(err)
	}
	if h.Algorithm != LZH || h.Algorithm.String() != "LZH" || h.Version != 1 {
		t.Errorf("LZH header parsed as %+v", h)
	}
	lzc := load(t, "text.lzc.hex")
	h, err = ParseHeader(lzc)
	if err != nil {
		t.Fatal(err)
	}
	if h.Algorithm != LZC || h.Algorithm.String() != "LZC" {
		t.Errorf("LZC header parsed as %+v", h)
	}
}

func TestRejects(t *testing.T) {
	if _, err := ParseHeader([]byte{1, 2, 3}); err == nil {
		t.Error("short input accepted")
	}
	plain := []byte("this is not compressed at all")
	if _, err := Decompress(plain); err != ErrNotCompressed {
		t.Errorf("uncompressed input: got %v, want ErrNotCompressed", err)
	}
	// A header that promises more than the stream holds.
	comp := load(t, "text.lzh.hex")
	lying := append([]byte(nil), comp...)
	lying[0]++
	if _, err := Decompress(lying); err == nil {
		t.Error("length mismatch accepted")
	}
	// A truncated stream.
	if _, err := Decompress(comp[:len(comp)/2]); err == nil {
		t.Error("truncated LZH stream accepted")
	}
	lzc := load(t, "big.lzc.hex")
	if _, err := Decompress(lzc[:len(lzc)/2]); err == nil {
		t.Error("truncated LZC stream accepted")
	}
	// Unknown algorithm nibble.
	odd := append([]byte(nil), comp...)
	odd[4] = 0x13
	if _, err := Decompress(odd); err == nil || !strings.Contains(err.Error(), "algorithm 3") {
		t.Errorf("unknown algorithm: %v", err)
	}
}
