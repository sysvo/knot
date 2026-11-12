package policy

import (
	"net/url"
	"strings"

	xhtml "golang.org/x/net/html"
)

// ExternalLinks returns the distinct hosts an HTML fragment links or embeds
// from that are not in allowed.
//
// The check runs over the rendered HTML rather than over the Markdown source,
// because the rendered HTML is exactly what will be displayed. That also covers
// bare URLs, which the Markdown renderer turns into links, and image tags, which
// are as good a way to track readers as a link is.
func ExternalLinks(fragment string, allowed ...string) []string {
	permitted := make(map[string]bool, len(allowed))
	for _, host := range allowed {
		if host = strings.ToLower(strings.TrimSpace(host)); host != "" {
			permitted[host] = true
		}
	}

	tok := xhtml.NewTokenizer(strings.NewReader(fragment))
	seen := map[string]bool{}
	out := []string{}

	for {
		switch tok.Next() {
		case xhtml.ErrorToken:
			return out
		case xhtml.StartTagToken, xhtml.SelfClosingTagToken:
		default:
			continue
		}

		token := tok.Token()
		if token.Data != "a" && token.Data != "img" {
			continue
		}

		for _, attr := range token.Attr {
			if attr.Key != "href" && attr.Key != "src" {
				continue
			}
			value := strings.TrimSpace(attr.Val)
			if value == "" {
				continue
			}

			parsed, err := url.Parse(value)
			if err != nil {
				continue
			}
			// A relative URL points at our own uploads or site.
			if parsed.Scheme == "" {
				continue
			}

			host := strings.ToLower(parsed.Hostname())
			if host == "" {
				// Non-hierarchical schemes such as mailto:.
				scheme := strings.ToLower(parsed.Scheme)
				if scheme != "http" && scheme != "https" && !seen[scheme] {
					seen[scheme] = true
					out = append(out, scheme+":")
				}
				continue
			}
			if permitted[host] || seen[host] {
				continue
			}
			seen[host] = true
			out = append(out, host)
		}
	}
}
