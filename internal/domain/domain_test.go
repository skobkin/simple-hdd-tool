package domain

import "testing"

func TestGroupModeNextCycle(t *testing.T) {
	t.Parallel()

	m := GroupByModel
	m = m.Next()
	if m != GroupBySize {
		t.Fatalf("first Next() = %q", m)
	}
	m = m.Next()
	if m != GroupByFamily {
		t.Fatalf("second Next() = %q", m)
	}
	m = m.Next()
	if m != GroupByNone {
		t.Fatalf("third Next() = %q", m)
	}
	m = m.Next()
	if m != GroupByModel {
		t.Fatalf("fourth Next() = %q", m)
	}
}

func TestSortModeNextCycle(t *testing.T) {
	t.Parallel()

	m := SortBySize
	if m.Next() != SortBySerial {
		t.Fatalf("SortBySize.Next() = %q", m.Next())
	}
	if SortBySerial.Next() != SortByHours {
		t.Fatalf("SortBySerial.Next() = %q", SortBySerial.Next())
	}
	if SortByHours.Next() != SortBySize {
		t.Fatalf("SortByHours.Next() = %q", SortByHours.Next())
	}
}
