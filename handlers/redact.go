package handlers

import (
	"net/url"
	"reflect"
	"regexp"
)

// secretTag is the struct tag that marks a field as possibly holding sensitive
// data. The annotation-driven redaction helper (S02) masks exactly these
// fields — the struct is authoritative, no regex "guessing" about what is or
// is not a secret.
const secretTag = "secret"

// redactedMarker replaces the value of a non-URL secret field in redacted
// output so the underlying data can never reach logs or tool output.
const redactedMarker = "<redacted>"

// urlUserinfoRe matches the userinfo (credentials) part of an http(s) URL,
// e.g. "http://user:pass@" or "https://token@". It is a defensive complement
// to the annotation-driven redaction: it strips URL credentials that appear
// inside arbitrary free-text (such as a search query that happens to embed a
// URL) where there is no struct to carry the secret:"true" annotation.
var urlUserinfoRe = regexp.MustCompile(`(https?://)[^/@\s]+@`)

// RedactURL strips embedded URL credentials (user:pass@ or token@) from any
// text so a secret can never reach logs or output (S02). Non-URL text is
// returned unchanged.
func RedactURL(s string) string {
	return urlUserinfoRe.ReplaceAllString(s, "${1}")
}

// RedactValue masks a single value that lives in a secret:"true" field (S02).
// HTTP(S) URL strings are redacted to keep the host visible while hiding any
// userinfo credentials (matching the startup-banner contract); every other
// value is replaced by a fixed marker so nothing sensitive can leak.
func RedactValue(v any) any {
	s, ok := v.(string)
	if !ok {
		return redactedMarker
	}

	parsed, err := url.Parse(s)
	if err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != "" {
		if parsed.User != nil {
			parsed.User = nil // drop the userinfo (user:pass@ / token@) entirely
		}

		return parsed.String()
	}

	return redactedMarker
}

// RedactStruct returns a redacted copy of a struct value: every exported field
// annotated secret:"true" is masked via RedactValue, nested structs are
// redacted recursively, and non-secret fields are copied through unchanged.
// This is the single annotation-driven redaction helper for S02 — callers never
// guess at what is sensitive, the struct tag is authoritative.
//
// Non-struct values are returned unchanged so the helper is safe to apply to
// arbitrary tool input (maps, scalars, functions).
func RedactStruct(v any) any {
	value := reflect.ValueOf(v)
	if value.Kind() != reflect.Struct {
		return v
	}

	structType := value.Type()
	out := reflect.New(structType).Elem()

	for i := range structType.NumField() {
		f := structType.Field(i)
		if !f.IsExported() {
			continue
		}

		fieldValue := value.Field(i)
		switch {
		case f.Tag.Get(secretTag) == "true":
			redacted := reflect.ValueOf(RedactValue(fieldValue.Interface()))
			if redacted.Type().AssignableTo(f.Type) {
				out.Field(i).Set(redacted)
			} else {
				// Non-string secret field: replace with the zero value so the
				// real data cannot leak (e.g. a secret int/[]byte).
				out.Field(i).Set(reflect.Zero(f.Type))
			}
		case fieldValue.Kind() == reflect.Struct:
			out.Field(i).Set(reflect.ValueOf(RedactStruct(fieldValue.Interface())))
		default:
			out.Field(i).Set(fieldValue)
		}
	}

	return out.Interface()
}
