package stream

import (
	"testing"
)

// TestSkeletonSmoke keeps `go test ./stream/` green while the package is
// implemented incrementally; the real behavior tests arrive with each
// implementation step.
func TestSkeletonSmoke(t *testing.T) {
	p := NewParser(HTML)
	if p.format != HTML {
		t.Fatalf("expected HTML format")
	}
	if err := p.End(); err != nil {
		t.Fatalf("empty document End: %v", err)
	}
	if _, ok := p.Next(); ok {
		t.Fatalf("empty document must not emit tokens")
	}
	if got := p.Malformations(); len(got) != 0 {
		t.Fatalf("empty document malformations: %v", got)
	}
}
