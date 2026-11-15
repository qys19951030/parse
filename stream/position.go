package stream

// lineIndex maps document-relative byte offsets to 1-based line and rune
// column numbers. It grows incrementally as bytes are emitted, so offsets
// never require rescanning the document. Newlines are "\n", "\r" and "\r\n",
// matching parse.Position.
type lineIndex struct {
	lineStarts []int // byte offset at which each line starts, line 1 at 0
	line       int   // line of the last consumed position
	colBytes   int   // bytes consumed on the current line
}

// newLineIndex returns an empty line index positioned at line 1, column 1.
func newLineIndex() *lineIndex {
	return &lineIndex{lineStarts: []int{0}, line: 1}
}

// consume records one decoded byte and updates the line counters. It must be
// called with every byte in order.
func (li *lineIndex) consume(b byte) {}

// positionAt returns line and 1-based rune column for a document byte offset.
// Columns count runes, matching parse.Position semantics.
func (li *lineIndex) positionAt(offset int, doc []byte) (line, col int) {
	return 1, 1
}

// addToken fills Start, End, Line and Col on t using the pending buffer and
// document base offset.
func (d *driver) position(t *Token) {}
