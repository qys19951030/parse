package stream

import (
	"io"
	"reflect"
	"unsafe"

	"github.com/tdewolff/parse/v2"
	"github.com/tdewolff/parse/v2/css"
	"github.com/tdewolff/parse/v2/html"
	"github.com/tdewolff/parse/v2/js"
	"github.com/tdewolff/parse/v2/xml"
)

// memmove copies n bytes from src to dst. It bypasses the reflect read-only
// flags on unexported fields so that lexer state from other packages can be
// snapshotted and restored without modifying those packages.
func memmove(dst, src unsafe.Pointer, n uintptr) {
	copy(unsafe.Slice((*byte)(dst), n), unsafe.Slice((*byte)(src), n))
}

// snapshotStruct returns a heap-allocated deep-enough copy of the struct
// pointed to by x. A shallow copy is sufficient: the lexers only carry scalar
// state and slice headers, and speculative appends beyond a restored slice
// length are never observable.
func snapshotStruct(x any) any {
	v := reflect.ValueOf(x).Elem()
	cp := reflect.New(v.Type())
	memmove(unsafe.Pointer(cp.Elem().UnsafeAddr()), unsafe.Pointer(v.UnsafeAddr()), v.Type().Size())
	return cp.Interface()
}

// restoreStruct copies the struct held in snap back into the struct pointed
// to by dst.
func restoreStruct(dst, snap any) {
	d := reflect.ValueOf(dst).Elem()
	s := reflect.ValueOf(snap).Elem()
	memmove(unsafe.Pointer(d.UnsafeAddr()), unsafe.Pointer(s.UnsafeAddr()), d.Type().Size())
}

// replaceInput moves the contents of fresh into the existing *parse.Input
// instance, keeping its address stable. The format lexers hold this address
// in an unexported field, so replacing the instance in place preserves their
// cross-token state while the underlying buffer is swapped.
func replaceInput(in, fresh *parse.Input) {
	d := reflect.ValueOf(in).Elem()
	s := reflect.ValueOf(fresh).Elem()
	memmove(unsafe.Pointer(d.UnsafeAddr()), unsafe.Pointer(s.UnsafeAddr()), d.Type().Size())
}

// formatLexer hides the four concrete lexer types behind one interface.
type formatLexer interface {
	next() (uint32, []byte)
	text() []byte
	attrKey() []byte
	attrVal() []byte
	err() error
	ptr() any
}

type htmlLexer struct{ l *html.Lexer }

func (a htmlLexer) next() (uint32, []byte) {
	tt, b := a.l.Next()
	return uint32(tt), b
}
func (a htmlLexer) text() []byte    { return a.l.Text() }
func (a htmlLexer) attrKey() []byte { return a.l.AttrKey() }
func (a htmlLexer) attrVal() []byte { return a.l.AttrVal() }
func (a htmlLexer) err() error      { return a.l.Err() }
func (a htmlLexer) ptr() any        { return a.l }

type cssLexer struct{ l *css.Lexer }

func (a cssLexer) next() (uint32, []byte) {
	tt, b := a.l.Next()
	return uint32(tt), b
}
func (a cssLexer) text() []byte    { return nil }
func (a cssLexer) attrKey() []byte { return nil }
func (a cssLexer) attrVal() []byte { return nil }
func (a cssLexer) err() error      { return a.l.Err() }
func (a cssLexer) ptr() any        { return a.l }

type jsLexer struct{ l *js.Lexer }

func (a jsLexer) next() (uint32, []byte) {
	tt, b := a.l.Next()
	return uint32(tt), b
}
func (a jsLexer) text() []byte    { return nil }
func (a jsLexer) attrKey() []byte { return nil }
func (a jsLexer) attrVal() []byte { return nil }
func (a jsLexer) err() error      { return a.l.Err() }
func (a jsLexer) ptr() any        { return a.l }

type xmlLexer struct{ l *xml.Lexer }

func (a xmlLexer) next() (uint32, []byte) {
	tt, b := a.l.Next()
	return uint32(tt), b
}
func (a xmlLexer) text() []byte    { return a.l.Text() }
func (a xmlLexer) attrKey() []byte { return nil }
func (a xmlLexer) attrVal() []byte { return a.l.AttrVal() }
func (a xmlLexer) err() error      { return a.l.Err() }
func (a xmlLexer) ptr() any        { return a.l }

// driver holds the chunk continuation machinery. The format lexer is created
// once and lives for the whole document; only the input buffer underneath it
// is swapped as chunks arrive.
type driver struct {
	format Format
	p      *Parser

	in  *parse.Input
	lex formatLexer

	// pending holds the decoded bytes not yet emitted as complete tokens.
	// base is the document-relative byte offset of pending[0].
	pending []byte
	base    int

	initialized bool
	atEOF       bool
}

// newDriver constructs the continuation driver for a format.
func newDriver(p *Parser, format Format) *driver {
	return &driver{format: format, p: p}
}

// init builds the concrete parse.Input and format lexer on first use.
func (d *driver) init() {
	if d.initialized {
		return
	}
	size := 4096
	if d.p != nil && d.p.opt.bufSize > 0 {
		size = d.p.opt.bufSize
	}
	d.pending = make([]byte, 0, size)
	d.in = parse.NewInputBytes(nil)
	switch d.format {
	case HTML:
		d.lex = htmlLexer{html.NewLexer(d.in)}
	case CSS:
		d.lex = cssLexer{css.NewLexer(d.in)}
	case JS:
		d.lex = jsLexer{js.NewLexer(d.in)}
	case XML:
		d.lex = xmlLexer{xml.NewLexer(d.in)}
	}
	d.initialized = true
}

// feed appends a decoded chunk to the retained tail and repoints the lexer at
// the rebuilt buffer. The chunk is copied: the caller's buffer is owned by
// the decoder and may be reused.
func (d *driver) feed(decoded []byte) {
	d.init()
	if d.initialized && d.in.Offset() > 0 {
		// compact the bytes already emitted before appending
		consumed := d.in.Offset()
		d.pending = d.pending[consumed:]
		d.base += consumed
	}
	if len(decoded) > 0 {
		d.pending = append(d.pending, decoded...)
	}
	d.rebuild()
}

// rebuild recreates the parse.Input over the retained tail in place. At the
// entry of every token the native lexers have input start == pos, so a fresh
// input at offset zero matches the restored lexer state.
func (d *driver) rebuild() {
	fresh := parse.NewInputBytes(d.pending)
	replaceInput(d.in, fresh)
}

// eofEdge reports whether the lexer currently stands exactly at the end of
// the retained bytes, meaning the last Next() decision was bounded by the
// provisional end of input.
func (d *driver) eofEdge() bool {
	return d.in.Offset() == len(d.pending)
}

// lookaheadWindow is the number of trailing bytes that must remain beyond the
// current lexer position before a token is accepted on provisional input.
// CSS and JS decide token shapes from bytes past the token's end (for example
// "1" followed by ".5em" is a dimension, not a number), so speculative tokens
// whose decision could still be changed by arriving bytes are withheld.
const lookaheadWindow = 8

// drain runs the lexer until the next token cannot be completed with the
// bytes currently available. Tokens whose shape depends on bytes still to
// arrive are speculatively produced and then fully rolled back, because the
// native lexers treat end-of-buffer as end-of-document; the same bytes are
// re-lexed once the next chunk extends the buffer. At final EOF the whole
// retained tail is drained. Recovered lexer errors are appended to malfs.
func (d *driver) drain(malfs *[]Malformation) []Token {
	d.init()
	tokens := make([]Token, 0)
	for {
		off := d.in.Offset()
		if !d.atEOF && len(d.pending)-off < lookaheadWindow {
			// inside the reserved lookahead window: stop while every emitted
			// token is backed by enough unseen bytes to be final
			return tokens
		}
		snap := snapshotStruct(d.lex.ptr())
		native, data := d.lex.next()

		if native == 0 { // native ErrorToken is the zero value in every format
			err := d.lex.err()
			if !d.atEOF || err == io.EOF {
				// clean end of document, or an error at a provisional edge:
				// undo the speculative call and wait for more bytes
				restoreStruct(d.lex.ptr(), snap)
				rewindInput(d.in, off)
				return tokens
			}
			// genuine lexer error after the document was closed
			*malfs = append(*malfs, Malformation{
				Offset: d.base + off,
				Kind:   MalLexerError,
				Reason: err.Error(),
			})
			if d.in.Offset() == off {
				return tokens // no progress: the native lexer cannot continue
			}
			continue // JS-style recovery: Next() already skipped the bad rune
		}

		if !d.atEOF && d.eofEdge() {
			// provisional end of input: the token shape can change when more
			// bytes arrive, so re-lex it later
			restoreStruct(d.lex.ptr(), snap)
			rewindInput(d.in, off)
			return tokens
		}

		t := mapToken(d.format, native, data, d.lex)
		start := d.base + off
		end := d.base + d.in.Offset()
		t.Start, t.End = start, end
		if d.p != nil {
			t.Line, t.Col = d.p.lines.position()
			d.p.lines.consume(d.pending[off:d.in.Offset()])
		}
		tokens = append(tokens, t)

		if d.atEOF && d.eofEdge() {
			return tokens
		}
	}
}

// rewindInput moves the input position back to a token boundary captured
// before a speculative Next().
func rewindInput(in *parse.Input, offset int) {
	in.Reset()
	in.Move(offset)
	in.Skip()
}

// finish marks the document complete and drains the retained tail for the
// last time.
func (d *driver) finish(malfs *[]Malformation) []Token {
	d.atEOF = true
	return d.drain(malfs)
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
