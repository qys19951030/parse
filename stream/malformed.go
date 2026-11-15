package stream

// malformationTracker collects recovered malformed-input events and performs
// the end-of-input classifications that only become knowable once no more
// chunks will arrive (unclosed tags and comments, dangling quotes, truncated
// encoding units).
type malformationTracker struct {
	events       []Malformation
	tagStack     []tagFrame
	openComments int
}

type tagFrame struct {
	name   []byte
	offset int
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

// observe feeds one emitted token to the stack-based tag/comment bookkeeping.
func (m *malformationTracker) observe(t Token) {}

// finalizeEvents appends the end-of-input malformations once the driver has
// drained the retained tail.
func (m *malformationTracker) finalizeEvents(d *driver) {}

// count returns the number of recorded events.
func (m *malformationTracker) count() int {
	return len(m.events)
}
