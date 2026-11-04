// Package sanitize cleans user-supplied strings before they reach a use case.
//
// DTO fields opt in with a `sanitize:"<mode>"` struct tag; fields without the
// tag (passwords, URLs) are left byte-exact. Struct walks a request value and
// applies each field's mode in place.
package sanitize

import (
	"html"
	"reflect"
	"strings"
	"unicode"

	"github.com/microcosm-cc/bluemonday"
	"golang.org/x/text/unicode/norm"
)

// Field modes, used as the value of the `sanitize` struct tag.
const (
	ModeText      = "text"      // single line: HTML stripped, control characters removed
	ModeMultiline = "multiline" // like text, but keeps newlines and tabs
	ModeEmail     = "email"     // like text, lowercased
)

const tagName = "sanitize"

// strict removes every HTML tag and drops script and style content.
var strict = bluemonday.StrictPolicy()

// Text cleans a single-line value such as a name, slug or technology label.
func Text(s string) string { return clean(s, false) }

// Multiline cleans free text such as a comment or contact message.
func Multiline(s string) string { return clean(s, true) }

// Email cleans and lowercases an address. It does not check the format;
// the `binding:"email"` tag does.
func Email(s string) string { return strings.ToLower(clean(s, false)) }

func clean(s string, keepNewlines bool) string {
	s = norm.NFC.String(s)
	// bluemonday escapes text (& becomes &amp;). Unescape once so "R&D" and
	// "1 < 2" survive as plain text. The result is stored and returned as JSON,
	// never rendered as HTML by this API.
	s = html.UnescapeString(strict.Sanitize(s))
	s = strings.Map(func(r rune) rune {
		switch {
		case keepNewlines && (r == '\n' || r == '\t'):
			return r
		case r == '\r' && keepNewlines:
			return -1
		case unicode.IsControl(r), unicode.Is(unicode.Cf, r), r == ' ', r == ' ':
			// Control characters include CR and LF, which would allow header
			// injection in the contact email. Cf covers zero-width and bidi marks.
			return -1
		}
		return r
	}, s)
	return strings.TrimSpace(s)
}

func apply(mode, s string) string {
	switch mode {
	case ModeText:
		return Text(s)
	case ModeMultiline:
		return Multiline(s)
	case ModeEmail:
		return Email(s)
	}
	return s
}

// Struct sanitizes, in place, every tagged string, *string, []string and
// *[]string field of the struct that v points to. Anything else is ignored.
func Struct(v any) {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Pointer || rv.IsNil() || rv.Elem().Kind() != reflect.Struct {
		return
	}
	rv = rv.Elem()
	rt := rv.Type()
	for i := range rt.NumField() {
		mode, ok := rt.Field(i).Tag.Lookup(tagName)
		if !ok {
			continue
		}
		field := rv.Field(i)
		if field.Kind() == reflect.Pointer {
			if field.IsNil() {
				continue
			}
			field = field.Elem()
		}
		switch {
		case field.Kind() == reflect.String:
			field.SetString(apply(mode, field.String()))
		case field.Kind() == reflect.Slice && field.Type().Elem().Kind() == reflect.String:
			for j := range field.Len() {
				item := field.Index(j)
				item.SetString(apply(mode, item.String()))
			}
		}
	}
}
