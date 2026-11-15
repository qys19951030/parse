package stream

import "unicode/utf8"

// lineIndex tracks the 1-based line and rune column of the next byte to be
// consumed. It advances incrementally as tokens are emitted, so source
// positions never require rescanning earlier chunks. Newlines are "\n", "\r"
// and "\r\n" (matching parse.Position); U+2028 and U+2029 are also treated as
// line separators for consistency with parse.Position's rune handling.
type lineIndex struct {
	line   int  // line number of the current position, 1-based
	col    int  // rune column of the current position, 1-based
	prevCR bool // previous byte was '\r', so the next '\n' is not a new line
}

// newLineIndex returns an index positioned at line 1, column 1.
func newLineIndex() *lineIndex {
	return &lineIndex{line: 1, col: 1}
}

// consume records one emitted byte sequence, in document order.
func (li *lineIndex) consume(b []byte) {
	for i := 0; i < len(b); {
		c := b[i]
		if c == '\r' {
			li.line++
			li.col = 1
			li.prevCR = true
			i++
			continue
		}
		if c == '\n' {
			if li.prevCR {
				// part of the preceding "\r\n", already counted
				li.prevCR = false
				i++
				continue
			}
			li.line++
			li.col = 1
			i++
			continue
		}
		li.prevCR = false
		if c < 0x80 {
			li.col++
			i++
			continue
		}
		r, n := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && n == 1 {
			// invalid byte: count it as one column, keep streaming
			li.col++
			i++
			continue
		}
		if r == 0x2028 || r == 0x2029 {
			li.line++
			li.col = 1
		} else {
			li.col++
		}
		i += n
	}
}

// position returns the line and column at which the next consumed byte will
// start.
func (li *lineIndex) position() (line, col int) {
	return li.line, li.col
}
