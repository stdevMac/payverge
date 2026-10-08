package utils

import "testing"

func TestVerifySignature_shortSignatures(t *testing.T) {
	cases := []struct {
		name string
		sig  string
	}{
		{"empty", "0x"},
		{"1 byte", "0xaa"},
		{"64 bytes", "0x" + repeatHex("bb", 64)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("VerifySignature panicked on %s signature: %v", tc.name, r)
				}
			}()
			result := VerifySignature("0x1234567890abcdef1234567890abcdef12345678", "test message", tc.sig, "1")
			if result {
				t.Fatal("expected false for short signature")
			}
		})
	}
}

func repeatHex(s string, n int) string {
	result := ""
	for i := 0; i < n; i++ {
		result += s
	}
	return result
}
