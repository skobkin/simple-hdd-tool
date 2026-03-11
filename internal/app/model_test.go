package app

import (
	"testing"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

func TestRebuildRowsKeepsSelectedDiskWhenGroupingChanges(t *testing.T) {
	m := NewModel(Config{
		GroupBy: domain.GroupByModel,
		SortBy:  domain.SortBySize,
	})
	m.disks = []domain.Disk{
		{ID: "disk-a", Model: "Alpha", DevicePath: "/dev/sda", SizeBytes: 1},
		{ID: "disk-b", Model: "Beta", DevicePath: "/dev/sdb", SizeBytes: 2},
	}

	m.rebuildRows()
	m.selected = 3

	m.cfg.GroupBy = domain.GroupByNone
	m.rebuildRows()

	if len(m.rows) != 2 {
		t.Fatalf("unexpected row count: got %d want 2", len(m.rows))
	}
	if m.selected != 1 {
		t.Fatalf("unexpected selection index: got %d want 1", m.selected)
	}
	if disk := m.selectedDisk(); disk == nil || disk.ID != "disk-b" {
		t.Fatalf("unexpected selected disk after regroup: %+v", disk)
	}
}
