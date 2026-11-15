package stream

import (
	"testing"
)

func TestEncodingBOM(t *testing.T) {
	cases := []struct {
		bom  []byte
		name string
	}{
		{[]byte{0xEF, 0xBB, 0xBF}, "utf-8"},
		{[]byte{0xFF, 0xFE}, "utf-16le"},
		{[]byte{0xFE, 0xFF}, "utf-16be"},
		{[]byte{0xFF, 0xFE, 0x00, 0x00}, "utf-32le"},
		{[]byte{0x00, 0x00, 0xFE, 0xFF}, "utf-32be"},
	}
	for _, c := range cases {
		p := NewParser(XML)
		var doc []byte
		switch c.name {
		case "utf-16le", "utf-16be":
			doc = append(append([]byte{}, c.bom...), encodeUTF16("<a/>", c.name)...)
		case "utf-32le", "utf-32be":
			doc = append(append([]byte{}, c.bom...), encodeUTF32("<a/>", c.name)...)
		default:
			doc = append(append([]byte{}, c.bom...), []byte("<a/>")...)
		}
		if err := p.Feed(doc); err != nil {
			t.Fatalf("Feed: %v", err)
		}
		if err := p.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		enc := p.Encoding()
		if enc.Name != c.name || enc.Source != EncFromBOM {
			t.Fatalf("BOM %v: got %s/%s, want %s/bom", c.bom, enc.Name, enc.Source, c.name)
		}
		// BOM bytes must not leak into tokenized content
		var n int
		for {
			tok, ok := p.Next()
			if !ok {
				break
			}
			n++
			if string(tok.Data) == "<a/>" || (len(tok.Data) > 0 && tok.Data[0] == '<') {
				if tok.Start != 0 {
					t.Fatalf("first markup token must start at decoded offset 0, got %d", tok.Start)
				}
			}
		}
		if n == 0 {
			t.Fatalf("document after BOM produced no tokens")
		}
	}
}

func TestEncodingDeclaration(t *testing.T) {
	cases := []struct {
		format Format
		doc    string
		name   string
	}{
		{XML, `<?xml version="1.0" encoding="UTF-8"?><a/>`, "utf-8"},
		{HTML, `<html><head><meta charset="utf-16le"></head></html>`, "utf-16le"},
		{HTML, `<meta charset=iso-8859-1>`, "iso-8859-1"},
		{CSS, `@charset "utf-8"; a{}`, "utf-8"},
	}
	for _, c := range cases {
		p := NewParser(c.format)
		if err := p.Feed([]byte(c.doc)); err != nil {
			t.Fatalf("Feed: %v", err)
		}
		if err := p.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		enc := p.Encoding()
		if enc.Name != c.name || enc.Source != EncFromDeclaration {
			t.Fatalf("doc %q: got %s/%s, want %s/declaration", c.doc, enc.Name, enc.Source, c.name)
		}
	}
}

func TestEncodingBOMBeatsDeclaration(t *testing.T) {
	// BOM says UTF-16LE, declaration inside the same bytes says UTF-8:
	// BOM wins and the conflict is reported.
	decl := []byte(`<?xml version="1.0" encoding="UTF-8"?>`)
	encoded := encodeUTF16LE(string(decl) + "<a/>")
	doc := append([]byte{0xFF, 0xFE}, encoded...)

	p := NewParser(XML)
	if err := p.Feed(doc[:7]); err != nil { // split inside the BOM-adjacent unit
		t.Fatalf("Feed1: %v", err)
	}
	if err := p.Feed(doc[7:]); err != nil {
		t.Fatalf("Feed2: %v", err)
	}
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	enc := p.Encoding()
	if enc.Name != "utf-16le" {
		t.Fatalf("BOM/declaration conflict: effective encoding %s, want utf-16le", enc.Name)
	}
	if !enc.Conflicted || enc.BOM != "utf-16le" || enc.Declared != "utf-8" {
		t.Fatalf("conflict not reported correctly: %+v", enc)
	}
}

func TestEncodingDeterministic(t *testing.T) {
	docs := [][]byte{
		[]byte(`<?xml encoding="UTF-8"?><a/>`),
		append([]byte{0xFF, 0xFE}, encodeUTF16LE("<a/>")...),
		[]byte("<a>plain</a>"),
	}
	for _, doc := range docs {
		run := func() Encoding {
			p := NewParser(XML)
			for off := 0; off < len(doc); off += 3 {
				end := off + 3
				if end > len(doc) {
					end = len(doc)
				}
				if err := p.Feed(doc[off:end]); err != nil {
					t.Fatalf("Feed: %v", err)
				}
			}
			if err := p.End(); err != nil {
				t.Fatalf("End: %v", err)
			}
			return p.Encoding()
		}
		a, b := run(), run()
		if a != b {
			t.Fatalf("encoding decision not deterministic: %+v vs %+v", a, b)
		}
	}
}

func TestEncodingUTF16RoundTrip(t *testing.T) {
	doc := `<root a="v"><child>text</child></root>`
	for _, c := range []struct {
		bom  []byte
		name string
		enc  string
	}{
		{[]byte{0xFF, 0xFE}, "utf-16le", "utf-16le"},
		{[]byte{0xFE, 0xFF}, "utf-16be", "utf-16be"},
	} {
		raw := append(append([]byte{}, c.bom...), encodeUTF16(doc, c.enc)...)
		// feed one byte at a time so every chunk boundary cuts a code unit
		p := NewParser(XML)
		for i := 0; i < len(raw); i++ {
			if err := p.Feed(raw[i : i+1]); err != nil {
				t.Fatalf("Feed: %v", err)
			}
		}
		if err := p.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		if p.Encoding().Name != c.name {
			t.Fatalf("encoding: got %s want %s", p.Encoding().Name, c.name)
		}
		got := p.Drain()
		ref := referenceLex(t, XML, doc)
		if len(got) != len(ref) {
			t.Fatalf("%s: token count got %d want %d", c.name, len(got), len(ref))
		}
		for i := range ref {
			if got[i].Native != ref[i].native || string(got[i].Data) != ref[i].data ||
				got[i].Start != ref[i].start || got[i].End != ref[i].end {
				t.Fatalf("%s token %d mismatch: got %q %d-%d want %q %d-%d",
					c.name, i, got[i].Data, got[i].Start, got[i].End,
					ref[i].data, ref[i].start, ref[i].end)
			}
		}
	}
}

func TestTruncatedEncodingUnit(t *testing.T) {
	// a valid 2-byte UTF-8 rune (é = 0xC3 0xA9) split so only its lead byte
	// has arrived when End is called
	doc := []byte("<a>é</a>")
	lead := bytesIndexOf(doc, 0xC3) // offset of the rune's lead byte
	p := NewParser(XML)
	if err := p.Feed(doc[:lead]); err != nil {
		t.Fatalf("Feed1: %v", err)
	}
	if err := p.Feed(doc[lead : lead+1]); err != nil { // only the lead byte
		t.Fatalf("Feed2: %v", err)
	}
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	var found bool
	for _, m := range p.Malformations() {
		if m.Kind == MalTruncatedEncodingUnit {
			found = true
		}
	}
	if !found {
		t.Fatalf("truncated encoding unit not recorded: %+v", p.Malformations())
	}
	// parsing continues and the closing tag is still emitted
	if n := p.Stats().TokenCounts[XML]; n == 0 {
		t.Fatalf("no tokens after truncated unit recovery")
	}
}

func bytesIndexOf(b []byte, want byte) int {
	for i, c := range b {
		if c == want {
			return i
		}
	}
	return -1
}

func encodeUTF16LE(s string) []byte { return encodeUTF16(s, "utf-16le") }

func encodeUTF16(s, enc string) []byte {
	out := make([]byte, 0, 2*len(s))
	for _, r := range s {
		v := uint16(r)
		lo, hi := byte(v), byte(v>>8)
		if enc == "utf-16be" {
			lo, hi = hi, lo
		}
		out = append(out, lo, hi)
	}
	return out
}

func encodeUTF32(s, enc string) []byte {
	out := make([]byte, 0, 4*len(s))
	for _, r := range s {
		v := uint32(r)
		b := []byte{byte(v), byte(v >> 8), byte(v >> 16), byte(v >> 24)}
		if enc == "utf-32be" {
			b[0], b[1], b[2], b[3] = b[3], b[2], b[1], b[0]
		}
		out = append(out, b...)
	}
	return out
}
