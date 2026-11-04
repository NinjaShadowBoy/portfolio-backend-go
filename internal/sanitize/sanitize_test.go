package sanitize

import "testing"

func TestText(t *testing.T) {
	for name, tc := range map[string]struct{ in, want string }{
		"plain":              {"Alex Abena", "Alex Abena"},
		"trims":              {"  Alex  ", "Alex"},
		"strips tags":        {"<b>Alex</b>", "Alex"},
		"drops script":       {"Al<script>alert(1)</script>ex", "Alex"},
		"keeps ampersand":    {"R&D", "R&D"},
		"keeps less-than":    {"1 < 2", "1 < 2"},
		"removes newline":    {"a\r\nBcc: x@y.z", "aBcc: x@y.z"},
		"removes zero-width": {"Al" + string(rune(0x200b)) + "ex", "Alex"},
		"composes to NFC":    {"e" + string(rune(0x301)), string(rune(0xe9))},
		"keeps accents":      {string(rune(0xe9)), string(rune(0xe9))},
	} {
		t.Run(name, func(t *testing.T) {
			if got := Text(tc.in); got != tc.want {
				t.Errorf("Text(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestMultilineKeepsNewlines(t *testing.T) {
	got := Multiline("line one\r\nline <i>two</i>\n\tindented\x00")
	if want := "line one\nline two\n\tindented"; got != want {
		t.Errorf("Multiline = %q, want %q", got, want)
	}
}

func TestEmailLowercases(t *testing.T) {
	if got := Email("  Alex@Example.COM "); got != "alex@example.com" {
		t.Errorf("Email = %q", got)
	}
}

func TestStruct(t *testing.T) {
	type req struct {
		Email    string `sanitize:"email"`
		Password string
		Name     *string   `sanitize:"text"`
		Skipped  *string   `sanitize:"text"`
		Body     string    `sanitize:"multiline"`
		Tags     *[]string `sanitize:"text"`
	}
	name := " <b>Al</b> "
	tags := []string{" Go ", "<i>SQL</i>"}
	r := req{Email: " A@B.C ", Password: "  <keep> me  ", Name: &name, Body: "a\n<b>b</b>", Tags: &tags}
	Struct(&r)

	if r.Email != "a@b.c" || *r.Name != "Al" || r.Body != "a\nb" {
		t.Errorf("tagged fields not cleaned: %+v", r)
	}
	if r.Password != "  <keep> me  " {
		t.Errorf("untagged password changed: %q", r.Password)
	}
	if tags[0] != "Go" || tags[1] != "SQL" {
		t.Errorf("tags = %q", tags)
	}
	if r.Skipped != nil {
		t.Error("nil pointer field was allocated")
	}
}
