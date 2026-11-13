package api

import (
	"html"
	"strings"
	"testing"
)

func TestSnippetHTMLEscapesContent(t *testing.T) {
	// The payload from the audit: typed into a code block, it reaches the index as
	// plain text and used to be rendered as markup.
	raw := "`" + snippetMarkOpen + "<img src=x onerror=alert(1)>" + snippetMarkClose + "` written here"

	got := snippetHTML(raw)
	if strings.Contains(got, "<img") {
		t.Fatalf("markup survived: %s", got)
	}
	if !strings.Contains(got, "&lt;img src=x onerror=alert(1)&gt;") {
		t.Errorf("payload was not escaped as text: %s", got)
	}
	if !strings.Contains(got, "<mark>&lt;img") {
		t.Errorf("match was not highlighted: %s", got)
	}
}

func TestSnippetHTMLHighlightsMatches(t *testing.T) {
	raw := "the " + snippetMarkOpen + "zebra" + snippetMarkClose + " escaped, " +
		snippetMarkOpen + "zebra" + snippetMarkClose + " again"

	want := "the <mark>zebra</mark> escaped, <mark>zebra</mark> again"
	if got := snippetHTML(raw); got != want {
		t.Errorf("snippet = %q, want %q", got, want)
	}
}

func TestSnippetHTMLEscapesAttributesAndEntities(t *testing.T) {
	// Quotes and ampersands must not break out of either text or an attribute
	// context; only the tags this function writes may appear.
	raw := `"quoted" & <b>bold</b>`

	got := snippetHTML(snippetMarkOpen + raw + snippetMarkClose)
	if strings.Contains(got, "<b>") || strings.Contains(got, `"`) {
		t.Fatalf("unescaped text in output: %s", got)
	}

	// The text inside the highlight must decode back to exactly what went in,
	// whichever entity form the escaper chose.
	inner := strings.TrimSuffix(strings.TrimPrefix(got, "<mark>"), "</mark>")
	if html.UnescapeString(inner) != raw {
		t.Errorf("round trip through escaping changed the text:\n got %q\nwant %q",
			html.UnescapeString(inner), raw)
	}
}

func TestSnippetHTMLUnbalancedMarkers(t *testing.T) {
	// A closing marker from content alone must not produce a stray closing tag, and
	// an opening marker without a closing one must still yield balanced HTML.
	for _, raw := range []string{
		"stray" + snippetMarkClose + " mark",
		"open" + snippetMarkOpen + "never closed",
		"nested" + snippetMarkOpen + "a" + snippetMarkOpen + "b" + snippetMarkClose,
	} {
		got := snippetHTML(raw)
		if opens, closes := strings.Count(got, "<mark>"), strings.Count(got, "</mark>"); opens != closes {
			t.Errorf("%q produced unbalanced markup: %s", raw, got)
		}
		if strings.Count(got, "<") != strings.Count(got, "</mark>")+strings.Count(got, "<mark>") {
			t.Errorf("%q produced a tag that is not <mark>: %s", raw, got)
		}
	}
}

func TestSnippetHTMLEmpty(t *testing.T) {
	if got := snippetHTML(""); got != "" {
		t.Errorf("empty snippet became %q", got)
	}
}
