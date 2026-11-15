package stream

// driver holds the chunk continuation machinery: the retained tail, the
// format lexer, the snapshot/rollback mechanism and the document position
// counter. It is intentionally separate from Parser so the continuation
// logic can be implemented and tested on its own.
type driver struct {
	format Format
	p      *Parser

	// pending holds the not-yet-fully-emitted bytes of the decoded document.
	// base is the document offset of pending[0].
	pending []byte
	base    int

	// input and lex are recreated/repointed whenever pending is rebuilt.
	input    lexerInput
	lex      formatLexer
	snapshot lexerSnapshot

	// bookkeeping
	initialized bool
	atEOF       bool
}

// lexerInput is the subset of *parse.Input used by the driver. The concrete
// value is built via parse.NewInputBytes and re-pointed through reflection in
// state.go, so no existing package needs to change.
type lexerInput struct {
	ptr any // *parse.Input
}

// formatLexer hides the four concrete lexer types behind one interface.
// Snapshot and restore copy the whole (unexported) lexer state through
// reflection so that a speculative Next() against a partial buffer can be
// fully undone when more bytes arrive.
type formatLexer interface {
	next() (uint32, []byte)
	text() []byte
	attrKey() []byte
	attrVal() []byte
	err() error
	snapshot() lexerSnapshot
	restore(lexerSnapshot)
	input() any
}

// lexerSnapshot is an opaque, heap-allocated copy of a format lexer taken
// immediately before a speculative Next() call.
type lexerSnapshot struct {
	value any
}

// newDriver constructs the continuation driver for a format.
func newDriver(p *Parser, format Format) *driver {
	return &driver{format: format, p: p}
}

// init builds the concrete parse.Input and format lexer on first use.
func (d *driver) init() error {
	return nil
}

// feed appends a decoded chunk to the retained tail and repoints the lexer at
// the rebuilt buffer.
func (d *driver) feed(decoded []byte) error {
	return nil
}

// drain runs the lexer until the next token cannot be completed with the
// bytes currently available, returning the tokens emitted in this pass. At
// final EOF the retained tail is fully drained.
func (d *driver) drain() ([]Token, error) {
	return nil, nil
}

// finish is called once End() marks the document complete and runs the final
// drain plus end-of-input malformed-input checks.
func (d *driver) finish() ([]Token, []Malformation, error) {
	return nil, nil, nil
}

// rebuild recreates the parse.Input over pending and restores the format
// lexer's position state, keeping the lexer instance (and its cross-token
// state) alive for the whole document.
func (d *driver) rebuild() error {
	return nil
}

// markEOFEdge reports whether the lexer consumed every retained byte, meaning
// the last token abuts the (possibly provisional) end of input.
func (d *driver) markEOFEdge() bool {
	return false
}

// copyBytes returns an independent copy of b so emitted tokens outlive the
// next buffer rebuild.
func copyBytes(b []byte) []byte {
	if len(b) == 0 {
		return nil
	}
	out := make([]byte, len(b))
	copy(out, b)
	return out
}
