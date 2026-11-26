package service

import (
	"strings"
	"testing"
)

func TestMailName(t *testing.T) {
	tests := []struct{ in, want string }{
		{"Ana", "Ana"},
		{"  Ana  Marija ", "Ana Marija"},
		{"Ana\r\n\r\nVaš nalog je blokiran", "Ana Vaš nalog je blokiran"},
		{"Ana‮txt.exe", "Anatxt.exe"},
		{"A\u0000n\u0007a", "Ana"},
		{"\n\t", "korisniče"},
		{strings.Repeat("ž", 80), strings.Repeat("ž", 50)},
	}
	for _, tc := range tests {
		if got := mailName(tc.in); got != tc.want {
			t.Errorf("mailName(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestMailNameHasNoLineBreaks(t *testing.T) {
	got := mailName("Ana\nSubject: lažno\r\nBcc: x@y.z")
	if strings.ContainsAny(got, "\r\n") {
		t.Errorf("ime u mejlu ne sme da ima prelom reda: %q", got)
	}
}
