package stream

// Rule selects structured fields from the token stream.
//
// Path matches an element by its tag-name path from the document root, for
// example []string{"library", "book"}. Names are compared case-insensitively
// for HTML and exactly for the other formats. Field names follow the same
// comparison rules.
//
// A Field starting with "@" extracts an attribute of the matched element;
// otherwise the field collects the element's direct text content. Text of
// ancestor and descendant elements never leaks into a field: only text at the
// exact depth of the matched element is collected.
type Rule struct {
	Name   string   // record name used in Record.Name
	Path   []string // tag-name path from the root
	Fields []string // field selectors, "@attr" for attributes
}

// Record is one structured extraction result.
type Record struct {
	Name   string
	Fields map[string][]byte
}

// Extractor applies a fixed set of rules to the unified token stream. It is
// format-independent: the same rules run on HTML and XML because both share
// the unified Token shape and position conventions.
type Extractor struct {
	rules   []Rule
	records []Record
	missing int
}

// NewExtractor builds an extractor for the given rules.
func NewExtractor(rules []Rule) *Extractor {
	return &Extractor{rules: rules}
}

// observe feeds one token into the rule engine.
func (e *Extractor) observe(t Token) {}

// Records returns the records extracted so far.
func (e *Extractor) Records() []Record {
	return e.records
}

// Missing returns the number of absent rule fields across emitted records.
func (e *Extractor) Missing() int {
	return e.missing
}
