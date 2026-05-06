package format

import "testing"

func TestMarkdownEmbedded(t *testing.T) {
	if len(Markdown) < 100 {
		t.Fatalf("expected embedded FORMAT.md, got len %d", len(Markdown))
	}
}
