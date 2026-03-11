package app

import (
	"testing"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

func TestParseConfigDefaults(t *testing.T) {
	cfg, err := ParseConfig(nil)
	if err != nil {
		t.Fatalf("ParseConfig returned error: %v", err)
	}
	if cfg.GroupBy != domain.GroupByModel {
		t.Fatalf("unexpected default group-by: %s", cfg.GroupBy)
	}
	if cfg.SortBy != domain.SortByHours {
		t.Fatalf("unexpected default sort-by: %s", cfg.SortBy)
	}
}

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

func TestParseConfigRejectsInvalidGroupBy(t *testing.T) {
	if _, err := ParseConfig([]string{"--group-by=bogus"}); err == nil {
		t.Fatalf("expected invalid group-by error")
	}
}
