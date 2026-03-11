package app

import "testing"

func TestParseConfig(t *testing.T) {
	cfg, err := ParseConfig([]string{"--group-by=size", "--sort-by=hours", "--force-remove"})
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if string(cfg.GroupBy) != "size" {
		t.Fatalf("unexpected group-by: %s", cfg.GroupBy)
	}
	if string(cfg.SortBy) != "hours" {
		t.Fatalf("unexpected sort-by: %s", cfg.SortBy)
	}
	if !cfg.ForceRemove {
		t.Fatalf("expected force-remove")
	}
}
