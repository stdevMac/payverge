package webhookhmac

import "testing"

func TestVerifyHexSHA256(t *testing.T) {
	secret := "whsec_test_secret"
	payload := []byte("1700000000.{\"id\":\"evt_1\"}")
	good := ComputeHexSHA256(secret, payload)

	cases := []struct {
		name       string
		candidates []string
		want       bool
	}{
		{"exact match", []string{good}, true},
		{"uppercased+padded match", []string{"  " + upper(good) + "  "}, true},
		{"one of several matches", []string{"deadbeef", good}, true},
		{"no match", []string{"deadbeef"}, false},
		{"empty candidate list", nil, false},
		{"wrong secret signature", []string{ComputeHexSHA256("other", payload)}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := VerifyHexSHA256(secret, payload, c.candidates); got != c.want {
				t.Errorf("VerifyHexSHA256(%v) = %v, want %v", c.candidates, got, c.want)
			}
		})
	}
}

func upper(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'a' && c <= 'f' {
			b[i] = c - 32
		}
	}
	return string(b)
}

func BenchmarkVerifyHexSHA256(b *testing.B) {
	secret := "whsec_test_secret"
	payload := []byte("1700000000.{\"id\":\"evt_1\",\"type\":\"checkout.session.completed\"}")
	sig := ComputeHexSHA256(secret, payload)
	candidates := []string{sig}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if !VerifyHexSHA256(secret, payload, candidates) {
			b.Fatal("expected match")
		}
	}
}
