package logger

import "testing"

func TestRedactEmail(t *testing.T) {
	cases := []struct {
		input    string
		expected string
	}{
		{"mallory@mail.example", "m***@mail.example"},
		{"a@example.io", "a***@example.io"},
		{"invalid", "***"},
		{"", "***"},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			result := RedactEmail(tc.input)
			if result != tc.expected {
				t.Fatalf("RedactEmail(%q) = %q, want %q", tc.input, result, tc.expected)
			}
		})
	}
}

func TestRedactEmails(t *testing.T) {
	got := RedactEmails([]string{"mallory@mail.example", "a@example.io"})
	want := "m***@mail.example,a***@example.io"
	if got != want {
		t.Fatalf("RedactEmails(...) = %q, want %q", got, want)
	}
	if RedactEmails(nil) != "" {
		t.Fatalf("RedactEmails(nil) = %q, want empty", RedactEmails(nil))
	}
}
