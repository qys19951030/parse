package stream

import "io"

// detector implements document-driven encoding detection: BOM first, then an
// in-document declaration, then content sniffing with a deterministic utf-8
// fallback. The decision is made once, on the first bytes seen, and never
// changes, so repeated parses of the same document agree.
//
// This step wires the streaming decode path in identity (utf-8, BOM kept for
// now); BOM stripping, declaration/sniff handling and the UTF-16/32 decoders
// arrive in the next implementation step.
type detector struct {
	dec     io.Reader // reserved for the pluggable decoders
	decided bool
	enc     Encoding
	tail    []byte // incomplete trailing encoding unit retained between feeds
}

// newDetector returns an undecided detector.
func newDetector() *detector {
	return &detector{}
}

// decide freezes the effective encoding from the first raw bytes.
func (dt *detector) decide(firstChunk []byte) error {
	if dt.decided {
		return nil
	}
	dt.decided = true
	dt.enc = Encoding{Name: "utf-8", Source: EncFromSniff}
	return nil
}

// decode passes raw bytes through unchanged while retaining no state. The
// real encoders replace this body in the encoding step without changing the
// Parser wiring.
func (dt *detector) decode(chunk []byte, final bool) (out []byte, truncated bool, err error) {
	if !dt.decided && len(chunk) > 0 {
		if err := dt.decide(chunk); err != nil {
			return nil, false, err
		}
	}
	return chunk, false, nil
}

// sniffBOM returns the encoding advertised by a byte order mark and its byte
// length, or an empty string and zero when the prefix is not a known BOM.
func sniffBOM(b []byte) (name string, size int) {
	return "", 0
}

// sniffDeclaration extracts an encoding from an XML declaration, HTML meta
// charset or CSS @charset rule found within the sniffing prefix.
func sniffDeclaration(format Format, b []byte) string {
	return ""
}

// sniffBytes applies content heuristics when neither BOM nor declaration
// decides the encoding.
func sniffBytes(b []byte) string {
	return ""
}
