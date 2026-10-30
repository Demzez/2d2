package common

import "testing"

func TestMonth(t *testing.T) {
	for _, month := range []string{"2026-01", "2026-10", "2030-12"} {
		actual, err := Month(month, "2026-10")
		if err != nil || actual != month {
			t.Fatalf("month %q rejected: %v", month, err)
		}
	}
	for _, month := range []string{"2026-00", "2026-13", "2026-1", "2026-0x", "2026- 1"} {
		if _, err := Month(month, "2026-10"); err == nil {
			t.Errorf("invalid month %q accepted", month)
		}
	}
}

func TestIntRange(t *testing.T) {
	v, err := IntRange("", 12, 1, 12)
	if err != nil || v != 12 {
		t.Fatalf("default: %d %v", v, err)
	}
	for _, value := range []string{"0", "13", "x", "-1"} {
		if _, err := IntRange(value, 12, 1, 12); err == nil {
			t.Errorf("invalid input %q accepted", value)
		}
	}
}
