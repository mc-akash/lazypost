package history

import (
	"bytes"
	"strings"
	"testing"
)

func TestRedactBodyMasksSecretAcrossLimit(t *testing.T) {
	secret := "SECRETabcdefghijklmnopqrstuvwxyz0123456789"
	half := secret[:len(secret)/2]
	cases := map[string][]byte{
		"straddle": []byte(strings.Repeat("a", MaxStoredBody-len(half)) + secret + strings.Repeat("b", 1<<20)),
		"shrink": []byte(strings.Repeat(secret, 50) + strings.Repeat("a", MaxStoredBody-50*len(secret)+len(secret)/2) +
			secret + strings.Repeat("b", 1<<20)),
	}
	for name, body := range cases {
		out, truncated := RedactBody(body, []string{secret})
		if bytes.Contains(out, []byte(half)) {
			t.Errorf("%s: stored body holds part of the secret", name)
		}
		if !truncated || len(out) > MaxStoredBody {
			t.Errorf("%s: len = %d truncated = %v", name, len(out), truncated)
		}
	}
}

func TestRedactBodyShortBodyUntouched(t *testing.T) {
	out, truncated := RedactBody([]byte("token=abc123 ok"), []string{"abc123"})
	if string(out) != "token="+redactMask+" ok" || truncated {
		t.Fatalf("out = %q truncated = %v", out, truncated)
	}
}
