package format

import (
	"testing"
	"time"
)

func TestSizeBytesAndRateAndElapsedShort(t *testing.T) {
	t.Parallel()

	if got := SizeBytes(0); got != "—" {
		t.Fatalf("SizeBytes(0) = %q", got)
	}
	if got := SizeBytes(999); got != "999 B" {
		t.Fatalf("SizeBytes(999) = %q", got)
	}
	if got := SizeBytes(150_000_000); got != "150 MB" {
		t.Fatalf("SizeBytes(150M) = %q", got)
	}
	if got := RateBytes(2048); got != "2.0 KB/s" {
		t.Fatalf("RateBytes() = %q", got)
	}
	if got := ElapsedShort(12 * time.Second); got != "12s" {
		t.Fatalf("ElapsedShort(seconds) = %q", got)
	}
	if got := ElapsedShort(2*time.Minute + 5*time.Second); got != "2m 5s" {
		t.Fatalf("ElapsedShort(minutes) = %q", got)
	}
	if got := ElapsedShort(2*time.Hour + 3*time.Minute + 4*time.Second); got != "2h 3m 4s" {
		t.Fatalf("ElapsedShort(hours) = %q", got)
	}
}

func TestFitDurationPartsBranches(t *testing.T) {
	t.Parallel()

	if got := fitDurationParts([]string{"1y", "2m"}, 0); got != "1y 2m" {
		t.Fatalf("fitDurationParts(no width) = %q", got)
	}
	if got := fitDurationParts([]string{"1000h", "2m"}, 2); got != "1000h" {
		t.Fatalf("fitDurationParts(narrow) = %q", got)
	}
}
