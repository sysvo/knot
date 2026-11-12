package markdown_test

import (
	"strings"
	"testing"

	"github.com/sysvo/knot/internal/markdown"
	xhtml "golang.org/x/net/html"
)

// assertSafe walks the rendered HTML and fails on anything that could execute.
// Checking the parsed DOM instead of substring-matching the source matters:
// escaped text such as &lt;svg/onload=…&gt; is harmless, and a substring check
// would report it as a hole that does not exist.
func assertSafe(t *testing.T, rendered string) {
	t.Helper()

	context := xhtml.NewTokenizer(strings.NewReader(rendered))
	for {
		tt := context.Next()
		if tt == xhtml.ErrorToken {
			return
		}
		if tt != xhtml.StartTagToken && tt != xhtml.SelfClosingTagToken {
			continue
		}

		token := context.Token()
		name := token.Data
		switch name {
		case "script", "iframe", "object", "embed", "form", "style", "svg", "math", "link", "meta", "base":
			t.Fatalf("rendered HTML contains a <%s> element: %s", name, rendered)
		}

		for _, attr := range token.Attr {
			key := attr.Key
			value := strings.ToLower(attr.Val)

			if strings.HasPrefix(key, "on") {
				t.Fatalf("rendered HTML contains an event handler %q: %s", key, rendered)
			}
			if key == "style" {
				t.Fatalf("rendered HTML contains an inline style: %s", rendered)
			}
			if key == "href" || key == "src" || key == "action" || key == "formaction" {
				for _, scheme := range []string{"javascript:", "vbscript:", "data:"} {
					if strings.HasPrefix(strings.TrimSpace(value), scheme) {
						t.Fatalf("rendered HTML contains a %s URL: %s", scheme, rendered)
					}
				}
			}
		}
	}
}

// TestSanitisesInjectedHTML is the most important test in the project: every
// post body passes through Render before it is stored, so nothing that survives
// here can execute in a reader's browser.
func TestSanitisesInjectedHTML(t *testing.T) {
	r := markdown.New()

	attacks := []string{
		"<script>alert(1)</script>",
		"<img src=x onerror=alert(1)>",
		"<a href=\"javascript:alert(1)\">click</a>",
		"<iframe src=\"https://evil.example\"></iframe>",
		"[link](javascript:alert(1))",
		"<svg/onload=alert(1)>",
		"<div style=\"position:fixed;top:0\">overlay</div>",
		"<form action=\"https://evil.example\"><input name=\"p\"></form>",
		"<math><mtext><script>alert(1)</script></mtext></math>",
		"![x](data:image/svg+xml;base64,PHN2Zz48L3N2Zz4=)",
		"<object data=\"https://evil.example/x.swf\"></object>",
	}

	for _, attack := range attacks {
		t.Run(attack, func(t *testing.T) {
			assertSafe(t, r.Render(attack))
		})
	}
}

func TestRendersGFM(t *testing.T) {
	r := markdown.New()

	table := r.Render("| a | b |\n| - | - |\n| 1 | 2 |")
	if !strings.Contains(table, "<table>") {
		t.Errorf("GFM table not rendered: %s", table)
	}

	strike := r.Render("~~gone~~")
	if !strings.Contains(strike, "<del>") {
		t.Errorf("strikethrough not rendered: %s", strike)
	}

	code := r.Render("```go\nfmt.Println(1)\n```")
	if !strings.Contains(code, "<code") {
		t.Errorf("fenced code not rendered: %s", code)
	}
}

// TestExternalLinksAreHardened stops a forum post from doing reverse-tabnabbing
// or leaking the reader's referrer to an arbitrary site.
func TestExternalLinksAreHardened(t *testing.T) {
	r := markdown.New()
	out := r.Render("[example](https://example.com/page)")

	if !strings.Contains(out, "nofollow") {
		t.Errorf("external link is missing nofollow: %s", out)
	}
	if !strings.Contains(out, "noopener") && !strings.Contains(out, "noreferrer") {
		t.Errorf("external link is missing opener protection: %s", out)
	}
}

func TestExcerptStripsMarkdown(t *testing.T) {
	r := markdown.New()

	excerpt := r.Excerpt("## Heading\n\nThis is **bold** and `code` with a [link](https://example.com).")
	for _, unwanted := range []string{"##", "**", "`", "](", "https://", "http"} {
		if strings.Contains(excerpt, unwanted) {
			t.Errorf("excerpt still contains %q: %q", unwanted, excerpt)
		}
	}
	for _, wanted := range []string{"Heading", "bold", "code", "link"} {
		if !strings.Contains(excerpt, wanted) {
			t.Errorf("excerpt lost %q: %q", wanted, excerpt)
		}
	}
}

func TestExcerptIsTruncated(t *testing.T) {
	r := markdown.New()
	excerpt := r.Excerpt(strings.Repeat("word ", 200))

	if len([]rune(excerpt)) > 230 {
		t.Errorf("excerpt is %d runes, expected at most 230", len([]rune(excerpt)))
	}
	if !strings.HasSuffix(excerpt, "…") {
		t.Errorf("truncated excerpt does not end with an ellipsis: %q", excerpt)
	}
}

func TestExcerptOfImageOnlyPost(t *testing.T) {
	r := markdown.New()
	if got := r.Excerpt("![alt](https://example.com/a.png)"); got != "" {
		t.Errorf("image-only excerpt = %q, want empty", got)
	}
}
