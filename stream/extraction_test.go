package stream

import "testing"

func TestExtractionRecords(t *testing.T) {
	doc := `<library>
  <book id="1" genre="fiction"><title>One</title><author><name>A</name></author></book>
  <book id="2"><title>Two</title></book>
</library>`
	rules := []Rule{{
		Name:   "book",
		Path:   []string{"library", "book"},
		Fields: []string{"@id", "@genre", "title"},
	}}
	ext := NewExtractor(rules)
	p := NewParser(XML, WithExtractor(ext))
	feedChunks(t, p, doc, 4)
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}

	recs := ext.Records()
	if len(recs) != 2 {
		t.Fatalf("records: got %d want 2: %+v", len(recs), recs)
	}
	if string(recs[0].Fields["@id"]) != "1" || string(recs[0].Fields["@genre"]) != "fiction" {
		t.Fatalf("record 0 attrs wrong: %+v", recs[0].Fields)
	}
	if string(recs[0].Fields["title"]) != "One" {
		t.Fatalf("record 0 title: %q", recs[0].Fields["title"])
	}
	if string(recs[1].Fields["@id"]) != "2" {
		t.Fatalf("record 1 id: %q", recs[1].Fields["@id"])
	}
	// @genre missing on the second book -> exactly one missing field
	if ext.Missing() != 1 {
		t.Fatalf("missing fields: got %d want 1", ext.Missing())
	}
	stats := p.Stats()
	if stats.Records != 2 || stats.MissingFields != 1 {
		t.Fatalf("stats records/missing: %d/%d", stats.Records, stats.MissingFields)
	}
}

func TestExtractionNoAncestorLeak(t *testing.T) {
	// the matched element's own text must not be polluted by its parent's or
	// child's text; only direct text at the matched depth is collected
	doc := `<root>PARENT<item>DIRECT<child>DESCENDANT</child>TAIL</item></root>`
	rules := []Rule{{
		Name:   "item",
		Path:   []string{"root", "item"},
		Fields: []string{"text"},
	}}
	ext := NewExtractor(rules)
	p := NewParser(XML, WithExtractor(ext))
	feedChunks(t, p, doc, 3)
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	recs := ext.Records()
	if len(recs) != 1 {
		t.Fatalf("records: %+v", recs)
	}
	got := string(recs[0].Fields["text"])
	if got != "DIRECTTAIL" {
		t.Fatalf("direct text leaked ancestor/descendant content: got %q want %q", got, "DIRECTTAIL")
	}
}

func TestExtractionHTMLAndXMLSameRules(t *testing.T) {
	htmlDoc := `<Root><Book ID="x">T</Book></Root>` // HTML names are case-folded
	xmlDoc := `<root><book id="x">T</book></root>`  // XML names are case-sensitive
	rules := []Rule{{
		Name:   "book",
		Path:   []string{"root", "book"},
		Fields: []string{"@id", "text"},
	}}
	cases := []struct {
		format Format
		doc    string
	}{
		{HTML, htmlDoc},
		{XML, xmlDoc},
	}
	for _, c := range cases {
		ext := NewExtractor(rules)
		p := NewParser(c.format, WithExtractor(ext))
		feedChunks(t, p, c.doc, 2)
		if err := p.End(); err != nil {
			t.Fatalf("End: %v", err)
		}
		recs := ext.Records()
		if len(recs) != 1 {
			t.Fatalf("format %s: records %+v", c.format, recs)
		}
		if string(recs[0].Fields["@id"]) != "x" {
			t.Fatalf("format %s: id %q", c.format, recs[0].Fields["@id"])
		}
		if string(recs[0].Fields["text"]) != "T" {
			t.Fatalf("format %s: text %q", c.format, recs[0].Fields["text"])
		}
	}
}

func TestExtractionMissingCounted(t *testing.T) {
	doc := `<root><a>1</a><a>2</a></root>`
	rules := []Rule{{
		Name:   "a",
		Path:   []string{"root", "a"},
		Fields: []string{"@missing", "text"},
	}}
	ext := NewExtractor(rules)
	p := NewParser(XML, WithExtractor(ext))
	feedChunks(t, p, doc, 5)
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	if len(ext.Records()) != 2 {
		t.Fatalf("records: %d", len(ext.Records()))
	}
	if ext.Missing() != 2 {
		t.Fatalf("each record misses @missing: want 2, got %d", ext.Missing())
	}
}

func TestStatsRecomputable(t *testing.T) {
	doc := `<?xml version="1.0"?><root a="1"><b>x</b></root>`
	rules := []Rule{{
		Name:   "b",
		Path:   []string{"root", "b"},
		Fields: []string{"text"},
	}}
	p := NewParser(XML, WithExtractor(NewExtractor(rules)))
	raw := []byte(doc)
	chunks := 0
	for off := 0; off < len(raw); off += 7 {
		end := off + 7
		if end > len(raw) {
			end = len(raw)
		}
		if err := p.Feed(raw[off:end]); err != nil {
			t.Fatalf("Feed: %v", err)
		}
		chunks++
	}
	if err := p.End(); err != nil {
		t.Fatalf("End: %v", err)
	}
	s := p.Stats()
	if s.Chunks != chunks {
		t.Fatalf("chunks: stats %d actual %d", s.Chunks, chunks)
	}
	if s.TotalBytes != len(raw) {
		t.Fatalf("total bytes: stats %d actual %d", s.TotalBytes, len(raw))
	}
	if s.Encoding != "utf-8" {
		t.Fatalf("encoding: %q", s.Encoding)
	}
	if s.TokenCounts[XML] <= 0 || s.Malformations != 0 {
		t.Fatalf("token/malformed stats wrong: %+v", s)
	}
	// token counts must sum to the total emitted tokens
	var sum int
	for _, n := range s.TokenCounts {
		sum += n
	}
	var drained int
	for {
		if _, ok := p.Next(); !ok {
			break
		}
		drained++
	}
	if sum != drained {
		t.Fatalf("token count sum %d != drained %d", sum, drained)
	}
	// kind counts must agree with the format counts
	var kindSum int
	for _, n := range s.KindCounts {
		kindSum += n
	}
	if kindSum != sum {
		t.Fatalf("kind count sum %d != format count sum %d", kindSum, sum)
	}
}

func feedChunks(t *testing.T, p *Parser, doc string, size int) {
	t.Helper()
	b := []byte(doc)
	for off := 0; off < len(b); off += size {
		end := off + size
		if end > len(b) {
			end = len(b)
		}
		if err := p.Feed(b[off:end]); err != nil {
			t.Fatalf("Feed: %v", err)
		}
	}
}
