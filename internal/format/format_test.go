package format

import "testing"

func TestDurationHours(t *testing.T) {
	v := uint64(24*365 + 24*30 + 24*20 + 10)
	if got := DurationHours(&v); got != "1y 1m 20d 10h" {
		t.Fatalf("unexpected duration: %s", got)
	}
}

func TestSizeBytes(t *testing.T) {
	if got := SizeBytes(1500); got != "1.5 KB" {
		t.Fatalf("unexpected size: %s", got)
	}
}
