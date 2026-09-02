package protocol

import (
	"strings"
	"testing"
)

func TestValidateURL(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		url  string
		want bool
	}{
		{"https://example.test/docs?q=1", true},
		{"http://example.test", true},
		{"javascript:alert(1)", false},
		{"https://user@example.test", false},
		{"-profile", false},
		{"https://example.test/\n", false},
	} {
		if got := ValidateURL(test.url) == nil; got != test.want {
			t.Errorf("ValidateURL(%q) valid = %v, want %v", test.url, got, test.want)
		}
	}
}

func TestValidateNote(t *testing.T) {
	t.Parallel()
	for _, test := range []struct {
		note string
		want bool
	}{
		{"", true},
		{"Необязательная заметка\nв две строки", true},
		{strings.Repeat("x", 4096), true},
		{strings.Repeat("x", 4097), false},
		{"bad\x00note", false},
	} {
		if got := ValidateNote(test.note) == nil; got != test.want {
			t.Errorf("ValidateNote(%q) valid = %v, want %v", test.note, got, test.want)
		}
	}
}

func TestValidateTags(t *testing.T) {
	t.Parallel()
	if err := ValidateTags([]string{"Работа", "Проект"}); err != nil {
		t.Fatal(err)
	}
	for _, tags := range [][]string{{""}, {" work"}, {"Work", "work"}, {"Σ", "ς"}, {strings.Repeat("x", 65)}, {"a,b"}} {
		if ValidateTags(tags) == nil {
			t.Fatalf("accepted tags %#v", tags)
		}
	}
}

func FuzzValidateURL(f *testing.F) {
	for _, seed := range []string{"https://example.test", "javascript:alert(1)", "https://user@example.test", "\x00"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, raw string) {
		err := ValidateURL(raw)
		if err == nil {
			if len(raw) > 8192 {
				t.Fatalf("accepted oversized URL")
			}
			if raw[:4] != "http" {
				t.Fatalf("accepted non-http URL %q", raw)
			}
		}
	})
}
