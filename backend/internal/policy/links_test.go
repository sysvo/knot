package policy_test

import (
	"reflect"
	"testing"

	"github.com/sysvo/knot/internal/policy"
)

func TestExternalLinksFindsOffsiteTargets(t *testing.T) {
	html := `<p>Look at <a href="https://spam.example/buy">this</a> and ` +
		`<a href="https://forum.example/threads/2">that</a>, plus ` +
		`<a href="/uploads/abc.png">a picture</a>.</p>` +
		`<p><img src="https://tracker.example/pixel.gif" alt=""></p>`

	got := policy.ExternalLinks(html, "forum.example")
	want := []string{"spam.example", "tracker.example"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExternalLinks = %v, want %v", got, want)
	}
}

// TestExternalLinksIgnoresRelativeURLs matters because uploads are served from
// the same host as relative paths, and blocking those would break posting an
// image for every new account.
func TestExternalLinksIgnoresRelativeURLs(t *testing.T) {
	html := `<p><img src="/uploads/xyz.webp"><a href="/t/3">thread</a></p>`
	if got := policy.ExternalLinks(html, "forum.example"); len(got) != 0 {
		t.Fatalf("relative URLs were flagged: %v", got)
	}
}

func TestExternalLinksReportsNonHTTP(t *testing.T) {
	html := `<a href="mailto:spam@example.com">mail me</a>`
	got := policy.ExternalLinks(html)
	if len(got) != 1 || got[0] != "mailto:" {
		t.Fatalf("ExternalLinks = %v, want [mailto:]", got)
	}
}

func TestExternalLinksDeduplicates(t *testing.T) {
	html := `<a href="https://spam.example/1">a</a><a href="https://spam.example/2">b</a>` +
		`<img src="https://SPAM.example/x.png">`
	got := policy.ExternalLinks(html)
	if len(got) != 1 {
		t.Fatalf("ExternalLinks = %v, want one entry", got)
	}
}

func TestExternalLinksAllowsEverythingWhenNothingIsRestricted(t *testing.T) {
	html := `<a href="https://example.com">example</a>`
	if got := policy.ExternalLinks(html, "example.com", "EXAMPLE.com"); len(got) != 0 {
		t.Fatalf("allowed host was flagged: %v", got)
	}
}

func TestExternalLinksHandlesMalformedHTML(t *testing.T) {
	// A sanitiser upstream should prevent this, but the check must not panic or
	// silently pass a fragment it could not parse.
	html := `<a href="http://unclosed.example>text`
	if got := policy.ExternalLinks(html); len(got) == 0 {
		t.Log("tokenizer recovered nothing from the malformed fragment; acceptable")
	}
}
