package stream_test

import (
	"fmt"

	"github.com/tdewolff/parse/v2/stream"
)

// Example shows chunked parsing: the document arrives in arbitrary pieces
// (here one byte at a time), complete tokens are drained after each feed,
// positions are document-relative and statistics summarize the run.
func ExampleParser_chunked() {
	doc := []byte(`<root><a href="x">text</a></root>`)
	p := stream.NewParser(stream.XML)
	const chunk = 8
	for off := 0; off < len(doc); off += chunk {
		end := off + chunk
		if end > len(doc) {
			end = len(doc)
		}
		if err := p.Feed(doc[off:end]); err != nil {
			panic(err)
		}
		for {
			tok, ok := p.Next()
			if !ok {
				break
			}
			fmt.Printf("%s %q @%d-%d\n", tok.NativeName, tok.Data, tok.Start, tok.End)
		}
	}
	if err := p.End(); err != nil {
		panic(err)
	}
	for {
		tok, ok := p.Next()
		if !ok {
			break
		}
		fmt.Printf("%s %q @%d-%d\n", tok.NativeName, tok.Data, tok.Start, tok.End)
	}
	stats := p.Stats()
	fmt.Printf("chunks=%d bytes=%d tokens=%d malformed=%d encoding=%s\n",
		stats.Chunks, stats.TotalBytes, stats.TokenCounts[stream.XML],
		stats.Malformations, stats.Encoding)
	// Output:
	// StartTag "<root" @0-5
	// StartTagClose ">" @5-6
	// StartTag "<a" @6-8
	// Attribute " href=\"x\"" @8-17
	// StartTagClose ">" @17-18
	// Text "text" @18-22
	// EndTag "</a>" @22-26
	// EndTag "</root>" @26-33
	// chunks=5 bytes=33 tokens=8 malformed=0 encoding=utf-8
}

// ExampleExtractor shows rule-based extraction over a chunked feed,
// including missing-field accounting.
func ExampleExtractor() {
	doc := []byte(`<library><book id="1"><title>One</title></book><book id="2"></book></library>`)
	rules := []stream.Rule{{
		Name:   "book",
		Path:   []string{"library", "book"},
		Fields: []string{"@id", "title"},
	}}
	ext := stream.NewExtractor(rules)
	p := stream.NewParser(stream.XML, stream.WithExtractor(ext))
	if err := p.Feed(doc); err != nil {
		panic(err)
	}
	if err := p.End(); err != nil {
		panic(err)
	}
	for _, rec := range ext.Records() {
		fmt.Printf("%s id=%q title=%q\n", rec.Name, rec.Fields["@id"], rec.Fields["title"])
	}
	fmt.Printf("missing=%d\n", ext.Missing())
	// Output:
	// book id="1" title="One"
	// book id="2" title=""
	// missing=1
}
