package money

import "testing"

func TestParseDollarStringToCents(t *testing.T) {
	cases := []struct {
		input    string
		expected int64
		wantErr  bool
	}{
		{"0.30", 30, false},
		{"99.99", 9999, false},
		{"0.01", 1, false},
		{"1000000.00", 100000000, false},
		{"100", 10000, false},
		{"0", 0, false},
		{"1.5", 150, false},
		{"-10.50", -1050, false},
		{"", 0, true},
		{"abc", 0, true},
		{"10.999", 0, true},
		{"10.1.2", 0, true},
	}
	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			result, err := ParseDollarStringToCents(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got %d", tc.input, result)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error for %q: %v", tc.input, err)
			}
			if result != tc.expected {
				t.Fatalf("ParseDollarStringToCents(%q) = %d, want %d", tc.input, result, tc.expected)
			}
		})
	}
}

func TestFloat64ToCents(t *testing.T) {
	cases := []struct {
		input    float64
		expected int64
	}{
		{0.30, 30},
		{99.99, 9999},
		{0.01, 1},
		{100.00, 10000},
	}
	for _, tc := range cases {
		t.Run("", func(t *testing.T) {
			result, err := Float64ToCents(tc.input)
			if err != nil {
				t.Fatalf("unexpected error for %f: %v", tc.input, err)
			}
			if result != tc.expected {
				t.Fatalf("Float64ToCents(%f) = %d, want %d", tc.input, result, tc.expected)
			}
		})
	}
}
