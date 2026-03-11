package format

import "testing"

func TestDurationHours(t *testing.T) {
	v := uint64(24*365 + 24*30 + 24*20 + 10)
	if got := DurationHours(&v); got != "1y 1m 20d 10h" {
		t.Fatalf("unexpected duration: %s", got)
	}
}

func TestDurationHoursCompactDropsTrailingUnitsToFitWidth(t *testing.T) {
	v := uint64(10*24*365 + 24*30 + 24*20 + 12)
	if got := DurationHoursCompact(&v, 13); got != "10y 1m 20d" {
		t.Fatalf("unexpected compact duration: %s", got)
	}
}

func TestDurationHoursCompactKeepsFullValueWhenItFits(t *testing.T) {
	v := uint64(6*24*365 + 24*30 + 24*20 + 12)
	if got := DurationHoursCompact(&v, 13); got != "6y 1m 20d 12h" {
		t.Fatalf("unexpected compact duration: %s", got)
	}
}

func TestSizeBytes(t *testing.T) {
	if got := SizeBytes(1500); got != "1.5 KB" {
		t.Fatalf("unexpected size: %s", got)
	}
}
