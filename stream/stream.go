// Package stream is a streaming facade over the html, css, js and xml tokenizers.
//
// Unlike the four format packages, which require the complete document to be
// held in memory, stream accepts the document in arbitrarily sized byte
// chunks. Chunks may cut through tags, attributes, string literals, comments
// and multi-byte encoding units: the token sequence produced from chunked
// input is identical, token by token and byte by byte, to the sequence
// produced by feeding the whole document to the underlying format lexer at
// once.
//
// The package also adds document-relative source positions, malformed-input
// recovery with an audit trail, document-driven encoding detection and
// rule-based structured extraction. The html, css, js and xml packages
// themselves are not modified.
package stream

import "errors"

// errFeedAfterEnd is returned when Feed is called after End.
var errFeedAfterEnd = errors.New("stream: Feed called after End")

// Format selects one of the four underlying tokenizers.
type Format int

const (
	// HTML selects the html tokenizer.
	HTML Format = iota
	// CSS selects the css tokenizer.
	CSS
	// JS selects the js tokenizer.
	JS
	// XML selects the xml tokenizer.
	XML
)

// String returns the lowercase name of the format.
func (f Format) String() string {
	switch f {
	case HTML:
		return "html"
	case CSS:
		return "css"
	case JS:
		return "js"
	case XML:
		return "xml"
	}
	return "unknown"
}

// Kind is the format-independent token classification. Every native token of
// every format is mapped onto one of these kinds so that extraction code does
// not need format-specific branches.
type Kind int

const (
	KindError Kind = iota
	KindText
	KindTagStart       // opening tag, element name is available on Token.Text
	KindTagEnd         // closing tag
	KindTagClose       // the ">" ending an opening tag
	KindTagSelfClose   // "/>", "?>" and similar void/PI closings
	KindAttribute      // attribute key/value pair
	KindComment
	KindDoctype
	KindCDATA
	KindPI             // processing instruction marker
	KindWhitespace
	KindLineTerminator
	KindIdentifier
	KindKeyword
	KindString
	KindNumber
	KindOperator
	KindPunctuator
	KindRegexp
	KindTemplate
	KindOther
)

// String returns a stable human-readable name for the kind.
func (k Kind) String() string {
	names := [...]string{
		"Error", "Text", "TagStart", "TagEnd", "TagClose", "TagSelfClose",
		"Attribute", "Comment", "Doctype", "CDATA", "PI", "Whitespace",
		"LineTerminator", "Identifier", "Keyword", "String", "Number",
		"Operator", "Punctuator", "Regexp", "Template", "Other",
	}
	if 0 <= int(k) && int(k) < len(names) {
		return names[k]
	}
	return "Unknown"
}

// Token is the format-independent token shape emitted by Parser. Byte slices
// are copies: they remain valid after subsequent Feed calls, even though the
// internal buffers they came from have been reused.
type Token struct {
	Format     Format
	Kind       Kind
	Native     uint32 // native TokenType value of the underlying lexer
	NativeName string // native TokenType.String()
	Data       []byte // bytes returned by the native Next() call
	Text       []byte // native Text() when available, nil otherwise
	AttrKey    []byte // attribute name for attribute tokens, nil otherwise
	AttrVal    []byte // attribute value for attribute tokens, nil otherwise
	Start      int    // start byte offset relative to the decoded document
	End        int    // end byte offset relative to the decoded document
	Line       int    // 1-based line number of Start
	Col        int    // 1-based rune column of Start
}

// MalformationKind classifies a recovered malformed-input event.
type MalformationKind int

const (
	// MalUnclosedTag: a tag opened but never closed before end of input.
	MalUnclosedTag MalformationKind = iota + 1
	// MalMissingQuote: an attribute value quote that has no closing quote.
	MalMissingQuote
	// MalUnclosedComment: a comment that runs to end of input.
	MalUnclosedComment
	// MalBadCharRef: an illegal character reference.
	MalBadCharRef
	// MalTruncatedEncodingUnit: an incomplete multi-byte encoding unit at end of input.
	MalTruncatedEncodingUnit
	// MalLexerError: a native lexer error that is not one of the classes above.
	MalLexerError
)

// String returns a stable human-readable name for the malformation kind.
func (k MalformationKind) String() string {
	switch k {
	case MalUnclosedTag:
		return "unclosed-tag"
	case MalMissingQuote:
		return "missing-attribute-quote"
	case MalUnclosedComment:
		return "unclosed-comment"
	case MalBadCharRef:
		return "bad-character-reference"
	case MalTruncatedEncodingUnit:
		return "truncated-encoding-unit"
	case MalLexerError:
		return "lexer-error"
	}
	return "unknown"
}

// Malformation is a single recovered malformed-input event. Parsing always
// continues after an event is recorded.
type Malformation struct {
	Offset int
	Line   int
	Col    int
	Kind   MalformationKind
	Reason string
}

// EncodingSource records how the effective encoding was decided.
type EncodingSource int

const (
	// EncFromBOM: a byte order mark determined the encoding.
	EncFromBOM EncodingSource = iota + 1
	// EncFromDeclaration: an in-document declaration determined the encoding.
	EncFromDeclaration
	// EncFromSniff: a content heuristic or the utf-8 fallback was used.
	EncFromSniff
)

// String returns a stable human-readable name for the encoding source.
func (s EncodingSource) String() string {
	switch s {
	case EncFromBOM:
		return "bom"
	case EncFromDeclaration:
		return "declaration"
	case EncFromSniff:
		return "sniff"
	}
	return "unknown"
}

// Encoding describes the encoding decision for a document.
type Encoding struct {
	Name       string         // canonical effective encoding name, lowercase
	Source     EncodingSource // what determined the effective encoding
	BOM        string         // encoding advertised by a BOM, empty when absent
	Declared   string         // encoding advertised by a declaration, empty when absent
	Conflicted bool           // BOM and declaration disagree; BOM wins
}

// Stats summarizes a complete parse. All counters are derivable from the
// parsing process and stable across repeated parses of the same input.
type Stats struct {
	Chunks        int            // number of non-final Feed calls
	TotalBytes     int           // raw bytes handed to Feed, including any BOM
	DecodedBytes   int           // bytes handed to the underlying tokenizer
	TokenCounts    map[Format]int // tokens emitted, keyed by the parser's format
	KindCounts     map[Kind]int   // tokens emitted, keyed by unified kind
	Malformations  int            // number of recovered malformed-input events
	Records        int            // structured records produced by the extractor
	MissingFields  int            // rule fields absent on matched records
	Encoding       string         // effective encoding name
}

// Option configures a Parser at construction time.
type Option func(*Parser)

// Parser streams one document through one of the four format tokenizers.
//
// Usage:
//
//	p := stream.NewParser(stream.HTML)
//	for {
//	    chunk, err := reader.Read(buf)
//	    ...
//	    if err := p.Feed(chunk); err != nil { ... }
//	    for {
//	        tok, ok := p.Next()
//	        if !ok { break }
//	        ...
//	    }
//	    if atEOF { break }
//	}
//	if err := p.End(); err != nil { ... }
//	for { tok, ok := p.Next(); ... }
type Parser struct {
	format Format
	opt    options

	drv     *driver
	lines   *lineIndex
	malfs   *malformationTracker
	enc     *detector
	queue   []Token
	closed  bool
	started bool
	err     error
	stats   Stats
}

type options struct {
	bufSize   int
	extractor *Extractor
}

// NewParser returns a streaming parser for the given format.
func NewParser(format Format, opts ...Option) *Parser {
	p := &Parser{
		format: format,
		lines:  newLineIndex(),
		malfs:  newMalformationTracker(),
		enc:    newDetector(),
		stats: Stats{
			TokenCounts: map[Format]int{},
			KindCounts:  map[Kind]int{},
		},
	}
	p.opt.bufSize = 4096
	for _, opt := range opts {
		opt(p)
	}
	p.drv = newDriver(p, format)
	return p
}

// WithBufferSize sets the initial capacity of the internal continuation
// buffer. It only affects allocation behavior, never token output.
func WithBufferSize(n int) Option {
	return func(p *Parser) {
		if 0 < n {
			p.opt.bufSize = n
		}
	}
}

// WithExtractor attaches a rule-based extractor that receives every emitted
// token automatically. Its record and missing-field counters feed Stats.
func WithExtractor(e *Extractor) Option {
	return func(p *Parser) {
		p.opt.extractor = e
	}
}

// Feed supplies one chunk of the raw document. A chunk may end at any byte
// boundary, including inside a token or a multi-byte encoding unit; complete
// tokens become available through Next, incomplete tails are retained until
// the next Feed. Feed must not be called after End.
func (p *Parser) Feed(chunk []byte) error {
	if p.err != nil {
		return p.err
	}
	if p.closed {
		p.err = errFeedAfterEnd
		return p.err
	}
	if len(chunk) > 0 {
		p.stats.Chunks++
		p.stats.TotalBytes += len(chunk)
	}

	decoded, truncated, err := p.enc.decode(chunk, false)
	if err != nil {
		p.err = err
		return err
	}
	if truncated {
		p.malfs.pendingTruncation = true
	}

	p.drv.feed(decoded)
	p.started = true
	p.drainToQueue()
	return nil
}

// End signals that the whole document has been fed. It flushes the retained
// tail, finalizes malformed-input bookkeeping and freezes the statistics.
func (p *Parser) End() error {
	if p.err != nil {
		return p.err
	}
	if p.closed {
		return nil
	}
	p.closed = true

	// flush any bytes still held by the decoder
	decoded, truncated, err := p.enc.decode(nil, true)
	if err != nil {
		p.err = err
		return err
	}
	p.stats.DecodedBytes += len(decoded)
	if truncated {
		p.malfs.pendingTruncation = true
	}
	p.drv.feed(decoded)
	p.started = true

	tokens := p.drv.finish(p.malfs.eventsRef())
	for i := range tokens {
		p.accept(tokens[i])
	}
	p.malfs.finalizeEvents(p.drv)
	p.freezeStats()
	return nil
}

// drainToQueue runs the continuation driver over the retained bytes and
// queues every token that is fully determined at the current chunk boundary.
func (p *Parser) drainToQueue() {
	tokens := p.drv.drain(p.malfs.eventsRef())
	for i := range tokens {
		p.accept(tokens[i])
	}
}

// accept registers one emitted token with statistics, the malformation
// tracker and the attached extractor, then queues it for Next.
func (p *Parser) accept(t Token) {
	p.stats.TokenCounts[t.Format]++
	p.stats.KindCounts[t.Kind]++
	p.stats.DecodedBytes += t.End - t.Start
	p.malfs.observe(t)
	if p.opt.extractor != nil {
		p.opt.extractor.observe(t)
	}
	p.queue = append(p.queue, t)
}

// Next returns the next unified token and true, or a zero Token and false
// when no fully determined token is currently available. After End, false
// means the document is exhausted.
func (p *Parser) Next() (Token, bool) {
	if len(p.queue) == 0 {
		return Token{}, false
	}
	t := p.queue[0]
	p.queue = p.queue[1:]
	return t, true
}

// Drain returns all currently available tokens at once, emptying the queue.
func (p *Parser) Drain() []Token {
	out := p.queue
	p.queue = nil
	return out
}

// Malformations returns the recovered malformed-input events in document
// order. The slice is frozen at End.
func (p *Parser) Malformations() []Malformation {
	return p.malfs.events
}

// Encoding returns the document-driven encoding decision. The decision is
// made on the first non-empty Feed and does not change afterwards.
func (p *Parser) Encoding() Encoding {
	return p.enc.enc
}

// Stats returns the parse statistics accumulated so far.
func (p *Parser) Stats() Stats {
	stats := p.stats
	stats.Malformations = p.malfs.count()
	if p.opt.extractor != nil {
		stats.Records = len(p.opt.extractor.Records())
		stats.MissingFields = p.opt.extractor.Missing()
	}
	if p.enc.enc.Name != "" {
		stats.Encoding = p.enc.enc.Name
	}
	return stats
}

// Err returns the first fatal error, if any. Malformed-input recoveries are
// not errors and are reported through Malformations instead.
func (p *Parser) Err() error {
	return p.err
}

func (p *Parser) freezeStats() {
	p.stats.Malformations = p.malfs.count()
	p.stats.Encoding = p.enc.enc.Name
	if p.opt.extractor != nil {
		p.stats.Records = len(p.opt.extractor.Records())
		p.stats.MissingFields = p.opt.extractor.Missing()
	}
}
