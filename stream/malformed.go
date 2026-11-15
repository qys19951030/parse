package stream

import "bytes"

// malformationTracker collects recovered malformed-input events. Tag,
// attribute, comment and character-reference conditions are observed token
// by token; conditions only knowable at end of input (unclosed tags and
// comments, dangling quotes, truncated encoding units) are emitted by
// finalizeEvents. Parsing always continues after an event.
type malformationTracker struct {
	events   []Malformation
	tagStack []tagFrame
}

type tagFrame struct {
	name   []byte
	offset int
	line   int
	col    int
	format Format
}

// newMalformationTracker returns an empty tracker.
func newMalformationTracker() *malformationTracker {
	return &malformationTracker{}
}

// eventsRef returns a pointer to the event slice so the driver can append
// lexer-level recoveries.
func (m *malformationTracker) eventsRef() *[]Malformation {
	return &m.events
}

// observe feeds one emitted token to the tag/comment/quote/char-ref checks.
func (m *malformationTracker) observe(t Token) {
	switch t.Kind {
	case KindComment:
		m.observeComment(t)
	case KindTagStart:
		name := tagName(t)
		if holdsOpen(t, name) {
			m.tagStack = append(m.tagStack, tagFrame{
				name:   copyBytes(name),
				offset: t.Start,
				line:   t.Line,
				col:    t.Col,
				format: t.Format,
			})
		}
		m.scanCharRefs(t, t.Data)
	case KindTagEnd:
		m.popTag(t)
		m.scanCharRefs(t, t.Data)
	case KindTagSelfClose:
		// "/>", "?>" and similar void/PI closings finish the open tag
		// immediately; an ordinary ">" (KindTagClose) leaves it on the stack
		// until the matching end tag arrives
		if len(m.tagStack) > 0 {
			m.tagStack = m.tagStack[:len(m.tagStack)-1]
		}
		m.scanCharRefs(t, t.Data)
	case KindPI:
		m.scanCharRefs(t, t.Data)
	case KindAttribute:
		m.observeAttribute(t)
	case KindText, KindCDATA:
		m.scanCharRefs(t, t.Data)
	}
}

// observeComment reports a comment token whose lexeme lacks the closing
// marker, which happens when a comment runs to end of input.
func (m *malformationTracker) observeComment(t Token) {
	if t.Format != HTML && t.Format != XML {
		return
	}
	trimmed := bytes.TrimRight(t.Data, "\r\n")
	if bytes.HasSuffix(trimmed, []byte("-->")) {
		return
	}
	m.events = append(m.events, Malformation{
		Offset: t.Start,
		Line:   t.Line,
		Col:    t.Col,
		Kind:   MalUnclosedComment,
		Reason: "comment is not closed before end of input",
	})
}

// observeAttribute reports an attribute whose quoted value has no closing
// quote. Native lexers return the value including the opening quote, and an
// unterminated value reaches EOF still inside the quotes.
func (m *malformationTracker) observeAttribute(t Token) {
	val := t.AttrVal
	if len(val) == 0 {
		m.scanCharRefs(t, t.Data)
		return
	}
	if (val[0] == '"' || val[0] == '\'') && (len(val) < 2 || val[len(val)-1] != val[0]) {
		m.events = append(m.events, Malformation{
			Offset: t.Start,
			Line:   t.Line,
			Col:    t.Col,
			Kind:   MalMissingQuote,
			Reason: "attribute value has no closing quote",
		})
	}
	m.scanCharRefs(t, t.Data)
}

// popTag matches a closing tag against the stack and reports the first
// unclosed ancestor when tags are mismatched.
func (m *malformationTracker) popTag(t Token) {
	name := tagName(t)
	for i := len(m.tagStack) - 1; i >= 0; i-- {
		if tagNameEqual(m.tagStack[i].name, name, t.Format) {
			m.tagStack = m.tagStack[:i]
			return
		}
	}
	// closing tag without a matching opener: record but keep recovering
	m.events = append(m.events, Malformation{
		Offset: t.Start,
		Line:   t.Line,
		Col:    t.Col,
		Kind:   MalUnclosedTag,
		Reason: "closing tag </" + string(name) + "> has no matching opening tag",
	})
}

// tagName returns the bare element name carried by a start/end tag token.
// HTML exposes it through both Text and AttrKey; fall back to AttrKey and
// finally to stripping the angle brackets from Data.
func tagName(t Token) []byte {
	if len(t.Text) > 0 {
		return t.Text
	}
	if len(t.AttrKey) > 0 {
		return t.AttrKey
	}
	b := t.Data
	b = bytes.TrimPrefix(b, []byte("</"))
	b = bytes.TrimSuffix(b, []byte(">"))
	return b
}

// scanCharRefs finds ampersands that do not start a well-formed named or
// numeric character reference in textual content and attribute data.
func (m *malformationTracker) scanCharRefs(t Token, b []byte) {
	if t.Format != HTML && t.Format != XML {
		return
	}
	line, col := t.Line, t.Col
	for i := 0; i < len(b); i++ {
		if b[i] != '&' {
			continue
		}
		if !validCharRef(b[i:]) {
			m.appendEvent(Malformation{
				Offset: t.Start + i,
				Line:   line,
				Col:    col + i,
				Kind:   MalBadCharRef,
				Reason: "illegal character reference",
			})
		}
	}
}

// appendEvent keeps the event list ordered by document offset. Streaming
// events already arrive in order; end-of-input events are inserted.
func (m *malformationTracker) appendEvent(ev Malformation) {
	pos := len(m.events)
	for pos > 0 && m.events[pos-1].Offset > ev.Offset {
		pos--
	}
	m.events = append(m.events, Malformation{})
	copy(m.events[pos+1:], m.events[pos:])
	m.events[pos] = ev
}

// finalizeEvents emits the end-of-input conditions: tags still on the stack.
// HTML elements that may be closed implicitly are tolerated.
func (m *malformationTracker) finalizeEvents(_ *driver) {
	for _, frame := range m.tagStack {
		if frame.format == HTML && htmlOptionalClose[string(bytes.ToLower(frame.name))] {
			continue
		}
		m.appendEvent(Malformation{
			Offset: frame.offset,
			Line:   frame.line,
			Col:    frame.col,
			Kind:   MalUnclosedTag,
			Reason: "element <" + string(frame.name) + "> is not closed before end of input",
		})
	}
	m.tagStack = nil
}

// count returns the number of recorded events.
func (m *malformationTracker) count() int {
	return len(m.events)
}

// htmlVoidElements never have an end tag in HTML5.
var htmlVoidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true,
	"hr": true, "img": true, "input": true, "link": true, "meta": true,
	"param": true, "source": true, "track": true, "wbr": true,
}

// htmlOptionalClose elements may legally be closed implicitly. They are
// still tracked while open so an explicit end tag matches, but an
// end-of-input remainder of these elements is not reported as malformed.
var htmlOptionalClose = map[string]bool{
	"html": true, "head": true, "body": true, "p": true, "li": true,
	"dt": true, "dd": true, "tr": true, "td": true, "th": true,
	"thead": true, "tbody": true, "tfoot": true, "option": true,
	"optgroup": true, "colgroup": true, "caption": true,
}

// tagNameEqual compares an opener name with a closer name, case-insensitive
// for HTML and exact otherwise.
func tagNameEqual(a, b []byte, format Format) bool {
	if format == HTML {
		return bytes.EqualFold(a, b)
	}
	return bytes.Equal(a, b)
}

// holdsOpen reports whether an opening tag belongs on the stack. HTML void
// elements have no end tag at all; everything else is tracked, though
// implicitly-closable HTML elements are tolerated at end of input.
func holdsOpen(t Token, name []byte) bool {
	if t.Format != HTML {
		return true // XML requires every element to be closed
	}
	return !htmlVoidElements[string(bytes.ToLower(name))]
}

// validCharRef reports whether b starts with a syntactically well-formed
// character reference: "&name;", "&#digits;" or "&#xhex;". A bare "&" or an
// unterminated reference is malformed.
func validCharRef(b []byte) bool {
	if len(b) < 3 || b[0] != '&' {
		return false
	}
	semi := bytes.IndexByte(b, ';')
	if semi < 0 {
		return false
	}
	body := b[1:semi]
	if len(body) == 0 {
		return false
	}
	if body[0] == '#' {
		if len(body) < 2 {
			return false
		}
		if body[1] == 'x' || body[1] == 'X' {
			if len(body) == 2 {
				return false
			}
			for _, c := range body[2:] {
				if !isHex(c) {
					return false
				}
			}
			return true
		}
		for _, c := range body[1:] {
			if c < '0' || '9' < c {
				return false
			}
		}
		return len(body) > 1
	}
	// named reference: letters and digits, must start with a letter
	if !isAlpha(body[0]) {
		return false
	}
	for _, c := range body[1:] {
		if !isAlnum(c) {
			return false
		}
	}
	return true
}

func isAlpha(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func isAlnum(c byte) bool {
	return isAlpha(c) || '0' <= c && c <= '9'
}

func isHex(c byte) bool {
	return '0' <= c && c <= '9' || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F'
}
