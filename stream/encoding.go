package stream

import "io"

// detector implements document-driven encoding detection: BOM first, then an
// in-document declaration, then content sniffing with a deterministic utf-8
// fallback. The decision is made once, on the first bytes seen, and never
// changes, so repeated parses of the same document agree.
type detector struct {
	dec     io.Reader // streaming decoder for the effective encoding
	decided bool
	enc     Encoding
}

// newDetector returns an undecided detector.
func newDetector() *detector {
	return &detector{}
}

// decide inspects the first raw chunk (plus a retained prefix when needed)
// and freezes the effective encoding. Subsequent calls are no-ops.
func (dt *detector) decide(firstChunk []byte) error {
	return nil
}

// decode transforms one raw chunk into bytes understood by the format
// tokenizers, retaining incomplete trailing encoding units for the next call.
// On the final call (final == true) a dangling unit is reported rather than
// retained.
func (dt *detector) decode(chunk []byte, final bool) (out []byte, truncated bool, err error) {
	return nil, false, nil
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
