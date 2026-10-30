package postgres

import "testing"

func TestMoneyToKopecks(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  int64
		ok    bool
	}{
		{"zero", "0.00", 0, true},
		{"minimum", "1.00", 100, true},
		{"fraction", "23.46", 2346, true},
		{"maximum", "1000.00", 100000, true},
		{"negative", "-1.00", 0, false},
		{"no cents", "1", 0, false},
		{"one cent digit", "1.0", 0, false},
		{"too many digits", "1.000", 0, false},
		{"comma", "12,34", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := MoneyToKopecks(tt.value)
			if (err == nil) != tt.ok || (tt.ok && actual != tt.want) {
				t.Fatalf("MoneyToKopecks(%q) = %d, %v", tt.value, actual, err)
			}
		})
	}
}
