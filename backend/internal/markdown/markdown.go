// Package markdown renders user-written Markdown to HTML once, on write, and
// stores both forms. Rendering at write time means readers pay nothing, and the
// sanitiser is applied to everything that ever reaches the database.
package markdown

import (
	"strings"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	xhtml "golang.org/x/net/html"
)

// Renderer converts Markdown to sanitised HTML.
type Renderer struct {
	md  goldmark.Markdown
	pol *bluemonday.Policy
}

// New builds a renderer with a sanitiser policy that allows ordinary
// formatting, links, code, tables, and images — but no scripts, styles or event
// handlers.
func New() *Renderer {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,         // tables, strikethrough, task lists, autolinks
			extension.Typographer, // curly quotes and dashes
		),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithHardWraps()),
	)

	p := bluemonday.UGCPolicy()
	p.AllowElements("del", "s", "input", "details", "summary")
	p.AllowAttrs("type", "checked", "disabled").OnElements("input")
	p.AllowAttrs("class").Matching(bluemonday.SpaceSeparatedTokens).OnElements("code", "pre", "span", "li", "ul")
	p.AllowAttrs("align").OnElements("th", "td")
	p.AllowAttrs("id").Matching(bluemonday.SpaceSeparatedTokens).OnElements("h1", "h2", "h3", "h4", "h5", "h6")
	p.RequireNoFollowOnLinks(true)
	p.RequireNoReferrerOnLinks(true)
	p.AddTargetBlankToFullyQualifiedLinks(true)

	return &Renderer{md: md, pol: p}
}

// Render returns sanitised HTML for a Markdown document.
func (r *Renderer) Render(src string) string {
	var buf strings.Builder
	if err := r.md.Convert([]byte(src), &buf); err != nil {
		// goldmark does not fail on ordinary input; if it ever does, degrade to
		// escaped plain text rather than serving a half-rendered document.
		return "<p>" + bluemonday.StrictPolicy().Sanitize(src) + "</p>"
	}
	return r.pol.Sanitize(buf.String())
}

// Excerpt returns a short plain-text preview for thread lists and
// notifications. It renders through the same pipeline as a post and then reads
// the text out of the resulting HTML, so link targets and Markdown syntax never
// leak into the preview.
func (r *Renderer) Excerpt(src string) string {
	text := htmlToText(r.Render(src))

	const max = 220
	runes := []rune(text)
	if len(runes) > max {
		return strings.TrimSpace(string(runes[:max])) + "…"
	}
	return strings.TrimSpace(text)
}

// htmlToText collects the text nodes of an HTML fragment and collapses
// whitespace. Using the tokenizer means entities are decoded for free.
func htmlToText(fragment string) string {
	tok := xhtml.NewTokenizer(strings.NewReader(fragment))
	var b strings.Builder
	for {
		switch tok.Next() {
		case xhtml.ErrorToken:
			return strings.Join(strings.Fields(b.String()), " ")
		case xhtml.TextToken:
			b.WriteString(tok.Token().Data)
			b.WriteString(" ")
		}
	}
}
