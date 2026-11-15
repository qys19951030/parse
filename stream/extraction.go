package stream

import "bytes"

// Rule selects structured fields from the token stream.
//
// Path matches an element by its tag-name path from the document root, for
// example []string{"library", "book"}. Names are compared case-insensitively
// for HTML and exactly for the other formats.
//
// A Field starting with "@" extracts an attribute of the matched element;
// otherwise the field collects the element's direct text content. Only text
// at the exact depth of the matched element is collected, so values of
// ancestor and descendant elements never leak into a field.
type Rule struct {
	Name   string
	Path   []string
	Fields []string
}

// Record is one structured extraction result.
type Record struct {
	Name   string
	Fields map[string][]byte
}

// extractFrame is one element on the extractor's depth stack.
type extractFrame struct {
	name   []byte
	depth  int
	rule   *Rule       // rule whose path matches this element, nil otherwise
	fields map[string][]byte
	seen   map[string]bool
	text   []byte // direct text of this element only
}

// Extractor applies a fixed set of rules to the unified token stream. It is
// format-independent: the same rules run on HTML and XML because both share
// the unified Token shape and position conventions.
type Extractor struct {
	rules   []Rule
	records []Record
	missing int

	stack []extractFrame
}

// NewExtractor builds an extractor for the given rules.
func NewExtractor(rules []Rule) *Extractor {
	return &Extractor{rules: rules}
}

// observe feeds one token into the rule engine.
func (e *Extractor) observe(t Token) {
	switch t.Kind {
	case KindTagStart, KindPI:
		e.push(t)
	case KindTagSelfClose:
		// void/PI element: if a rule matched it, emit immediately
		if len(e.stack) > 0 && e.stack[len(e.stack)-1].rule != nil {
			e.emitTop()
		}
		if len(e.stack) > 0 {
			e.stack = e.stack[:len(e.stack)-1]
		}
	case KindTagClose:
		// ordinary ">" does not pop; element lifetime ends on its end tag or
		// self-close. Nothing to do here.
	case KindTagEnd:
		e.pop(t)
	case KindAttribute:
		e.attribute(t)
	case KindText, KindCDATA:
		e.text(t)
	}
}

// push opens an element and checks whether its tag path matches any rule.
func (e *Extractor) push(t Token) {
	name := tagName(t)
	depth := len(e.stack)
	frame := extractFrame{
		name:  copyBytes(name),
		depth: depth,
	}
	path := e.currentPath()
	path = append(path, name)
	for i := range e.rules {
		if pathMatch(e.rules[i].Path, path, t.Format) {
			r := e.rules[i]
			frame.rule = &r
			frame.fields = map[string][]byte{}
			frame.seen = map[string]bool{}
			break
		}
	}
	e.stack = append(e.stack, frame)
}

// pop closes the topmost matching element, or balances an unmatched end tag.
func (e *Extractor) pop(t Token) {
	if len(e.stack) == 0 {
		return
	}
	top := &e.stack[len(e.stack)-1]
	if tagNameEqual(top.name, tagName(t), t.Format) {
		if top.rule != nil {
			e.emitTop()
		}
		e.stack = e.stack[:len(e.stack)-1]
	}
}

// attribute stores a matched "@field" attribute on the rule element.
func (e *Extractor) attribute(t Token) {
	if len(e.stack) == 0 {
		return
	}
	top := &e.stack[len(e.stack)-1]
	if top.rule == nil {
		return
	}
	key := string(t.AttrKey)
	if len(key) == 0 {
		key = string(t.Text)
	}
	want := "@" + key
	for _, f := range top.rule.Fields {
		if f == want && !top.seen[f] {
			top.fields[f] = unquote(copyBytes(t.AttrVal))
			top.seen[f] = true
		}
	}
}

// text records direct textual content. When the current element is a named
// child field of a matched element below it, the text is stored on that
// record; otherwise only text directly inside a matched element counts, so
// ancestor and descendant values cannot leak into a field.
func (e *Extractor) text(t Token) {
	if len(e.stack) == 0 || len(t.Data) == 0 {
		return
	}
	top := &e.stack[len(e.stack)-1]
	if top.rule != nil {
		top.text = append(top.text, t.Data...)
		return
	}
	// walk down to the nearest matched ancestor and only accept the text
	// when the element directly holding it is one of that rule's fields
	for i := len(e.stack) - 2; i >= 0; i-- {
		frame := &e.stack[i]
		if frame.rule == nil {
			continue
		}
		field := string(top.name)
		if e.ruleHasTextField(frame.rule, field) {
			frame.text = append(frame.text, t.Data...)
		}
		return
	}
}

// ruleHasTextField reports whether rule declares a non-attribute field name.
func (e *Extractor) ruleHasTextField(rule *Rule, name string) bool {
	for _, f := range rule.Fields {
		if f[0] != '@' && f == name {
			return true
		}
	}
	return false
}

// emitTop finalizes the record on top of the stack. Each non-attribute field
// receives that element's direct text; every rule field not found is counted
// as missing.
func (e *Extractor) emitTop() {
	top := &e.stack[len(e.stack)-1]
	rule := top.rule
	fields := map[string][]byte{}
	for k, v := range top.fields {
		fields[k] = v
	}
	for _, f := range rule.Fields {
		if f[0] == '@' {
			if !top.seen[f] {
				e.missing++
			}
			continue
		}
		if top.seen[f] || len(top.text) > 0 {
			fields[f] = copyBytes(top.text)
			top.seen[f] = true
		} else {
			e.missing++
		}
	}
	e.records = append(e.records, Record{Name: rule.Name, Fields: fields})
}

// currentPath returns the tag names of the open elements.
func (e *Extractor) currentPath() [][]byte {
	out := make([][]byte, len(e.stack))
	for i, f := range e.stack {
		out[i] = f.name
	}
	return out
}

// pathMatch compares a rule path against the actual element path by suffix,
// so a rule written as {"library","book"} matches the same element whether
// or not an XML declaration or other non-element nodes precede it.
func pathMatch(want []string, got [][]byte, format Format) bool {
	if len(want) > len(got) {
		return false
	}
	got = got[len(got)-len(want):]
	for i := range want {
		if format == HTML {
			if !bytes.EqualFold([]byte(want[i]), got[i]) {
				return false
			}
		} else if want[i] != string(got[i]) {
			return false
		}
	}
	return true
}

// unquote removes a single pair of surrounding quotes from an attribute
// value; unquoted values pass through unchanged.
func unquote(b []byte) []byte {
	if len(b) >= 2 && (b[0] == '"' || b[0] == '\'') && b[len(b)-1] == b[0] {
		return b[1 : len(b)-1]
	}
	return b
}

// Records returns the records extracted so far.
func (e *Extractor) Records() []Record {
	return e.records
}

// Missing returns the number of absent rule fields across emitted records.
func (e *Extractor) Missing() int {
	return e.missing
}
