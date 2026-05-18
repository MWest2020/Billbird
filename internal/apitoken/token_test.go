package apitoken

import (
	"strings"
	"testing"
)

func TestGenerateTokenString(t *testing.T) {
	plaintext, prefix, err := generateTokenString()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		t.Errorf("plaintext %q must start with %q", plaintext, TokenPrefix)
	}
	if len(prefix) != 8 {
		t.Errorf("prefix must be 8 chars, got %d (%q)", len(prefix), prefix)
	}
	body := strings.TrimPrefix(plaintext, TokenPrefix)
	if !strings.HasPrefix(body, prefix) {
		t.Errorf("body %q must start with prefix %q", body, prefix)
	}
}

func TestGenerateUniqueness(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		plaintext, _, err := generateTokenString()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if seen[plaintext] {
			t.Fatalf("duplicate token generated: %q", plaintext)
		}
		seen[plaintext] = true
	}
}

func TestParsePrefix(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantOK    bool
		wantValue string
	}{
		{"valid bb token", "bb_abcdefghijkl", true, "abcdefgh"},
		{"missing prefix", "abcdefghijkl", false, ""},
		{"prefix only", "bb_", false, ""},
		{"prefix and short body", "bb_abc", false, ""},
		{"unrelated prefix", "github_pat_abc", false, ""},
		{"empty", "", false, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parsePrefix(c.input)
			if ok != c.wantOK {
				t.Errorf("ok: got %v, want %v", ok, c.wantOK)
			}
			if got != c.wantValue {
				t.Errorf("value: got %q, want %q", got, c.wantValue)
			}
		})
	}
}
