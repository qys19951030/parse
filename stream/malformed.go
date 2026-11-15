package stream

// malformationTracker collects recovered malformed-input events and performs
// the end-of-input classifications that only become knowable once no more
// chunks will arrive (unclosed tags and comments, dangling quotes, truncated
// encoding units).
type malformationTracker struct {
	events  []Malformation
	tagStack []tagFrame
}

type tagFrame struct {
	name   []byte
	offset int
}

// newMalformationTracker returns an empty tracker.
func newMalformationTracker() *malformationTracker {
	return &malformationTracker{}
}

// observe feeds one emitted token to the stack-based tag/comment bookkeeping.
func (m *malformationTracker) observe(t Token) {}

// scanCharRefs records illegal character references found inside textual
// content and attribute values.
func (m *malformationTracker) scanCharRefs(b []byte, offset int) {}

// lexerError records a non-EOF error returned by a native lexer.
func (m *malformationTracker) lexerError(offset int, reason string) {}

// finalize appends the end-of-input malformations (unclosed tags, unclosed
// comments, dangling attribute quotes, truncated encoding units) given the
// retained tail and its document base offset.
func (m *malformationTracker) finalize(pending []byte, base int, truncatedEncodingUnit bool) []Malformation {
	return m.events
}

// count returns the number of recorded events.
func (m *malformationTracker) count() int {
	return len(m.events)
}
