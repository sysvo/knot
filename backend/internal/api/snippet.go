package api

import (
	"html"
	"strings"
)

// Snippet markers written by Store.Search. They are ASCII control characters:
// nothing a member types on purpose is likely to contain them, and they cannot
// survive html.EscapeString, so they can never be smuggled in by content.
const (
	snippetMarkOpen  = "\x02"
	snippetMarkClose = "\x03"
)

// snippetHTML turns a search snippet into HTML that is safe to render.
//
// Every piece of text is escaped with html.EscapeString before any markup is
// written, so the only tags in the result are the <mark> pair this function emits
// itself. That matters because a snippet is built from text a member typed: the
// index holds plain text (not HTML), and FTS5 copies it into the snippet verbatim,
// so treating the raw snippet as HTML executes whatever was typed into a code
// block.
func snippetHTML(raw string) string {
	if raw == "" {
		return ""
	}

	var b strings.Builder
	b.Grow(len(raw) + 16)

	segments := strings.Split(raw, snippetMarkOpen)
	// Everything before the first marker is unmatched context.
	b.WriteString(html.EscapeString(segments[0]))

	for _, segment := range segments[1:] {
		// An unterminated marker is closed here anyway, so the output is always
		// balanced HTML whatever the snippet contained.
		marked, rest, closed := strings.Cut(segment, snippetMarkClose)
		b.WriteString("<mark>")
		b.WriteString(html.EscapeString(marked))
		b.WriteString("</mark>")
		if closed {
			b.WriteString(html.EscapeString(rest))
		}
	}

	return b.String()
}
