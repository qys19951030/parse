package stream

import (
	"bytes"
	"unicode/utf16"
	"unicode/utf8"
)

// sniffPrefix is the number of leading bytes scanned for an in-document
// encoding declaration, matching the size used by HTML's prescan algorithm.
const sniffPrefix = 1024

// runeErrorBytes is the UTF-8 encoding of the replacement rune, emitted when
// a dangling multi-byte unit reaches end of input.
var runeErrorBytes = []byte(string(utf8.RuneError))

// detector implements document-driven encoding detection. Precedence is
// fixed: byte order mark, then an in-document declaration, then content
// sniffing with a deterministic utf-8 fallback. The decision is made once and
// never changes, so repeated parses of the same document always agree.
//
// Before the decision, raw leading bytes are buffered (at most sniffPrefix).
// The declaration is scanned on the *decoded* prefix, because an XML
// declaration inside a UTF-16 document is itself UTF-16 encoded.
type detector struct {
	format  Format
	decided bool
	enc     Encoding
	bomSize int

	raw  []byte // raw bytes accumulated before the decision
	tail []byte // raw unconsumed bytes (incomplete trailing unit) after it
}

// newDetector returns an undecided detector.
func newDetector(format Format) *detector {
	return &detector{format: format}
}

// decode transforms one raw chunk into tokenizer-ready bytes, retaining an
// incomplete trailing multi-byte unit for the next call. On the final call a
// dangling unit is reported through truncated instead of being retained.
func (dt *detector) decode(chunk []byte, final bool) (out []byte, truncated bool, err error) {
	if !dt.decided {
		dt.raw = append(dt.raw, chunk...)
		if !dt.canDecide(final) {
			return nil, false, nil
		}
		dt.decide()
		// decide() moved the BOM-stripped prefix into tail, including an
		// incomplete trailing encoding unit that decodeAll will retain
	} else if len(chunk) > 0 {
		dt.tail = append(dt.tail, chunk...)
	}

	remainder, decoded := decodeAll(dt.enc.Name, dt.tail, final)
	if final {
		// a non-empty remainder at end of input is a dangling encoding unit;
		// decodeAll already emitted replacement bytes for it
		truncated = len(remainder) > 0
		dt.tail = dt.tail[:0]
	} else {
		dt.tail = append(dt.tail[:0], remainder...)
	}
	return decoded, truncated, nil
}

// canDecide reports whether the buffered prefix is sufficient to freeze the
// encoding. The sniffing prefix is always collected first so a declaration
// inside the prefix is seen even when a BOM already selected the decoding;
// the BOM still wins, but a disagreeing declaration must be detectable. At
// end of input whatever has arrived is enough.
func (dt *detector) canDecide(final bool) bool {
	if final {
		return len(dt.raw) > 0
	}
	return len(dt.raw) >= sniffPrefix
}

// decide freezes the effective encoding and prepares the BOM-stripped raw
// tail for the first decode pass.
func (dt *detector) decide() {
	dt.decided = true
	bomName, bomSize := sniffBOM(dt.raw)
	dt.bomSize = bomSize
	dt.enc.BOM = bomName

	// first candidate: BOM, else content sniff, else utf-8
	candidate := bomName
	source := EncFromBOM
	if candidate == "" {
		if sniffed := sniffBytes(dt.raw); sniffed != "" {
			candidate, source = sniffed, EncFromSniff
		} else {
			candidate, source = "utf-8", EncFromSniff
		}
	}

	// decode the prefix under the candidate encoding and read the declaration
	prefix := dt.raw[bomSize:]
	if len(prefix) > sniffPrefix {
		prefix = prefix[:sniffPrefix]
	}
	_, decoded := decodeAll(candidate, prefix, true)
	declName := sniffDeclaration(dt.format, decoded)
	dt.enc.Declared = declName

	if bomName != "" {
		// BOM always wins; a disagreeing declaration is recorded as conflict
		dt.enc.Name, dt.enc.Source = bomName, EncFromBOM
		dt.enc.Conflicted = declName != "" && declName != bomName
	} else if declName != "" {
		// the declaration overrides the sniffed candidate: re-decode the raw
		// prefix later under the declared encoding
		dt.enc.Name, dt.enc.Source = declName, EncFromDeclaration
	} else {
		dt.enc.Name, dt.enc.Source = candidate, source
	}
	dt.tail = append(dt.tail[:0], dt.raw[bomSize:]...)
	dt.raw = nil
}

// decodeAll decodes as many complete units of enc as possible from b. It
// returns the unconsumed tail and the decoded UTF-8 bytes. Unknown encodings
// fall back to the utf-8 byte stream.
func decodeAll(enc string, b []byte, final bool) (remainder, decoded []byte) {
	switch enc {
	case "utf-16le", "utf-16be":
		n := len(b) &^ 1
		if n < len(b) {
			decoded = decodeUTF16(enc, b[:n])
			if !final {
				return b[n:], decoded
			}
			return nil, append(decoded, runeErrorBytes...)
		}
		return nil, decodeUTF16(enc, b)
	case "utf-32le", "utf-32be":
		n := (len(b) / 4) * 4
		if n < len(b) {
			decoded = decodeUTF32(enc, b[:n])
			if !final {
				return b[n:], decoded
			}
			return nil, append(decoded, runeErrorBytes...)
		}
		return nil, decodeUTF32(enc, b)
	default: // utf-8 and any unsupported declared encoding: byte-preserving
		if i := incompleteUTF8Start(b); i >= 0 {
			if final {
				// keep the dangling unit as remainder so the caller flags it,
				// and emit replacement bytes in its place
				return b[i:], append(b[:i:i], runeErrorBytes...)
			}
			return b[i:], b[:i]
		}
		return nil, b
	}
}

// incompleteUTF8Start returns the start index of a trailing incomplete UTF-8
// sequence (a lead byte whose continuation bytes have not all arrived), or
// -1 when the buffer ends on a complete boundary. Invalid but length-complete
// sequences are not retained.
func incompleteUTF8Start(b []byte) int {
	i := len(b) - 1
	if i < 0 {
		return -1
	}
	if b[i]&0x80 == 0 {
		// the final byte is ASCII: walk back over any dangling continuation
		// bytes to find their (arrived) lead byte
		for i >= 0 && b[i]&0xC0 == 0x80 {
			i--
		}
		if i < 0 || b[i]&0x80 == 0 {
			return -1 // complete boundary
		}
	}
	for j := i; j >= 0 && j >= len(b)-utf8.UTFMax; j-- {
		c := b[j]
		if c&0xC0 == 0xC0 { // lead byte
			want := 0
			switch {
			case c&0xE0 == 0xC0:
				want = 2
			case c&0xF0 == 0xE0:
				want = 3
			case c&0xF8 == 0xF0:
				want = 4
			default:
				return -1 // invalid lead, length-complete handling stays with the tokenizer
			}
			if len(b)-j < want {
				return j
			}
			return -1
		}
		if c&0x80 == 0 {
			return -1
		}
	}
	return -1
}

func decodeUTF16(enc string, b []byte) []byte {
	u16 := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		var v uint16
		if enc == "utf-16le" {
			v = uint16(b[i]) | uint16(b[i+1])<<8
		} else {
			v = uint16(b[i+1]) | uint16(b[i])<<8
		}
		u16 = append(u16, v)
	}
	rs := utf16.Decode(u16)
	out := make([]byte, 0, len(b))
	for _, r := range rs {
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], r)
		out = append(out, buf[:n]...)
	}
	return out
}

func decodeUTF32(enc string, b []byte) []byte {
	out := make([]byte, 0, len(b))
	for i := 0; i+3 < len(b); i += 4 {
		var v uint32
		if enc == "utf-32le" {
			v = uint32(b[i]) | uint32(b[i+1])<<8 | uint32(b[i+2])<<16 | uint32(b[i+3])<<24
		} else {
			v = uint32(b[i+3]) | uint32(b[i+2])<<8 | uint32(b[i+1])<<16 | uint32(b[i])<<24
		}
		r := rune(v)
		if r > utf8.MaxRune || (0xD800 <= r && r <= 0xDFFF) {
			r = utf8.RuneError
		}
		var buf [utf8.UTFMax]byte
		n := utf8.EncodeRune(buf[:], r)
		out = append(out, buf[:n]...)
	}
	return out
}

// sniffBOM returns the canonical encoding name advertised by a byte order
// mark and the mark's byte length.
func sniffBOM(b []byte) (name string, size int) {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return "utf-8", 3
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE, 0x00, 0x00}):
		return "utf-32le", 4
	case bytes.HasPrefix(b, []byte{0x00, 0x00, 0xFE, 0xFF}):
		return "utf-32be", 4
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return "utf-16le", 2
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return "utf-16be", 2
	}
	return "", 0
}

// sniffDeclaration extracts an encoding from an XML declaration, HTML meta
// charset or CSS @charset rule within the decoded sniffing prefix. The three
// syntaxes are recognizable on their own, so all of them are accepted for
// every format; the earliest occurrence in the prefix wins.
func sniffDeclaration(_ Format, b []byte) string {
	if len(b) > sniffPrefix {
		b = b[:sniffPrefix]
	}
	lower := make([]byte, len(b))
	for i, c := range b {
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		lower[i] = c
	}

	type match struct {
		offset int
		name   string
	}
	var matches []match
	if i := bytes.Index(lower, []byte("@charset")); i >= 0 {
		if n := quotedValue(lower[i+len("@charset"):]); n != "" {
			matches = append(matches, match{i, canonicalCharset(n)})
		}
	}
	if i := bytes.Index(lower, []byte("encoding")); i >= 0 {
		if n := quotedValue(lower[i+len("encoding"):]); n != "" {
			matches = append(matches, match{i, canonicalCharset(n)})
		}
	}
	if i := bytes.Index(lower, []byte("charset")); i >= 0 {
		if n := charsetValue(lower[i+len("charset"):]); n != "" {
			matches = append(matches, match{i, canonicalCharset(n)})
		}
	}
	if len(matches) == 0 {
		return ""
	}
	best := matches[0]
	for _, m := range matches[1:] {
		if m.offset < best.offset {
			best = m
		}
	}
	return best.name
}

// quotedValue returns the content of the first quoted string inside b.
func quotedValue(b []byte) string {
	eq := bytes.IndexByte(b, '=')
	if eq < 0 {
		return ""
	}
	b = b[eq+1:]
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	if len(b) == 0 || (b[0] != '"' && b[0] != '\'') {
		return ""
	}
	quote := b[0]
	end := bytes.IndexByte(b[1:], quote)
	if end < 0 {
		return ""
	}
	return string(b[1 : 1+end])
}

// charsetValue handles both quoted and unquoted HTML charset values.
func charsetValue(b []byte) string {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '=' || b[0] == '\r' || b[0] == '\n') {
		b = b[1:]
	}
	if len(b) == 0 {
		return ""
	}
	if b[0] == '"' || b[0] == '\'' {
		quote := b[0]
		end := bytes.IndexByte(b[1:], quote)
		if end < 0 {
			return ""
		}
		return string(b[1 : 1+end])
	}
	end := 0
	for end < len(b) && b[end] != ' ' && b[end] != '\t' && b[end] != '>' &&
		b[end] != '\r' && b[end] != '\n' && b[end] != '/' && b[end] != ';' {
		end++
	}
	return string(b[:end])
}

// sniffBytes applies content heuristics: UTF-16 documents with no BOM show up
// as interleaved NUL bytes within ASCII markup.
func sniffBytes(b []byte) string {
	if len(b) < 4 {
		return ""
	}
	nulEven, nulOdd := 0, 0
	limit := len(b)
	if limit > sniffPrefix {
		limit = sniffPrefix
	}
	for i := 0; i+1 < limit; i += 2 {
		if b[i] == 0 {
			nulEven++
		}
		if b[i+1] == 0 {
			nulOdd++
		}
	}
	if nulEven > 0 && nulOdd == 0 {
		return "utf-16be"
	}
	if nulOdd > 0 && nulEven == 0 {
		return "utf-16le"
	}
	return ""
}

// canonicalCharset normalizes the spellings accepted by the sniffers.
func canonicalCharset(name string) string {
	canonical := map[string]string{
		"utf8":     "utf-8",
		"utf-8":    "utf-8",
		"utf16":    "utf-16le",
		"utf-16":   "utf-16le",
		"utf-16le": "utf-16le",
		"utf-16be": "utf-16be",
		"utf32":    "utf-32le",
		"utf-32":   "utf-32le",
		"utf-32le": "utf-32le",
		"utf-32be": "utf-32be",
	}
	if c, ok := canonical[name]; ok {
		return c
	}
	return name
}
