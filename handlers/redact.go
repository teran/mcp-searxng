package handlers

import "regexp"

// urlUserinfoRe matches the userinfo (credentials) part of an http(s) URL,
// e.g. "http://user:pass@" or "https://token@".
var urlUserinfoRe = regexp.MustCompile(`(https?://)[^/@\s]+@`)

// RedactURL strips embedded URL credentials (user:pass@ or token@) from any
// text so a secret can never reach logs or output (S02). Non-URL text is
// returned unchanged.
func RedactURL(s string) string {
	return urlUserinfoRe.ReplaceAllString(s, "${1}")
}
