package store

import "testing"

// TestFTSQuery covers the sanitiser that stands between a search box and the
// FTS5 parser. Anything that slips through here either reads as an operator or
// produces a syntax error, so the exact output is asserted.
func TestFTSQuery(t *testing.T) {
	cases := map[string]string{
		"hello":            `"hello"*`,
		"hello world":      `"hello"* AND "world"*`,
		"self-hosted":      `"self"* AND "hosted"*`,
		"  padded  ":       `"padded"*`,
		"dots.and,commas":  `"dots"* AND "and"* AND "commas"*`,
		"@alice mentioned": `"alice"* AND "mentioned"*`,
		"AND OR NOT":       `"AND"* AND "OR"* AND "NOT"*`,
		"c++":              `"c"*`,
		"trailing!":        `"trailing"*`,
		"ünïcode wörds":    `"ünïcode"* AND "wörds"*`,
	}

	for input, want := range cases {
		if got := ftsQuery(input); got != want {
			t.Errorf("ftsQuery(%q) = %q, want %q", input, got, want)
		}
	}
}

// TestFTSQueryDropsPunctuationOnly checks that a query made entirely of
// operators becomes empty rather than being handed to the parser.
func TestFTSQueryDropsPunctuationOnly(t *testing.T) {
	for _, input := range []string{"", "   ", "\"", "(*){}[]", "---", "!!!"} {
		if got := ftsQuery(input); got != "" {
			t.Errorf("ftsQuery(%q) = %q, want empty", input, got)
		}
	}
}

// TestFTSQueryCannotInjectOperators proves the quoting survives characters that
// have meaning to FTS5.
func TestFTSQueryCannotInjectOperators(t *testing.T) {
	for _, input := range []string{
		`" OR 1=1 --`,
		`NEAR(a b)`,
		`*`,
		`^start`,
		`a AND (b OR c)`,
	} {
		got := ftsQuery(input)
		// Every term must be fully quoted, so no bare operator can appear.
		for i := 0; i < len(got); i++ {
			if got[i] == '(' || got[i] == ')' || got[i] == '^' {
				t.Fatalf("ftsQuery(%q) = %q, which contains an unquoted operator", input, got)
			}
		}
	}
}
