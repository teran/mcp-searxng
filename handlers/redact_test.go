package handlers

import "testing"

// TestRedactURL verifies the S02 redaction helper strips embedded URL
// credentials (user:pass@ or token@) so they never reach logs or output.
func TestRedactURL(t *testing.T) {
	t.Parallel()

	cases := []struct {
		in   string
		want string
	}{
		{"http://user:pass@host/search?q=x", "http://host/search?q=x"},
		{"https://token@host/x", "https://host/x"},
		{"prefix http://a:b@c.example/y suffix", "prefix http://c.example/y suffix"},
		{"plain text with no url", "plain text with no url"},
		{"query http://example.com/a", "query http://example.com/a"},
	}

	for _, testCase := range cases {
		if got := RedactURL(testCase.in); got != testCase.want {
			t.Errorf("RedactURL(%q) = %q, want %q", testCase.in, got, testCase.want)
		}
	}
}
