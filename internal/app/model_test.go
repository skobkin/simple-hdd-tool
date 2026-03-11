package app

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

func TestDetailsScrollClampsAndPages(t *testing.T) {
	m := NewModel(Config{NoColor: true})
	m.mode = viewDetails
	m.width = 48
	m.height = 10
	m.disks = []domain.Disk{{
		ID:             "disk-a",
		Family:         "Family",
		Model:          "Model",
		Serial:         "Serial",
		DevicePath:     "/dev/sda",
		Health:         domain.HealthWarning,
		Problem:        "health warning",
		ProblemDetails: "pending sectors=2; SMART error log count=1; note is intentionally long to force wrapping in a very narrow viewport",
		ProblemNote:    "keep watching SMART counters over time",
	}}
	m.rebuildRows()

	maxScroll := m.maxDetailScroll()
	if maxScroll < 1 {
		t.Fatalf("expected overflowing details body, got max scroll %d", maxScroll)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m.detailScroll != 1 {
		t.Fatalf("unexpected detail scroll after down: got %d want 1", m.detailScroll)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.detailScroll <= 1 {
		t.Fatalf("expected page down to advance scroll, got %d", m.detailScroll)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyEnd})
	if m.detailScroll != maxScroll {
		t.Fatalf("unexpected detail scroll after end: got %d want %d", m.detailScroll, maxScroll)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.detailScroll != maxScroll {
		t.Fatalf("detail scroll exceeded max: got %d want %d", m.detailScroll, maxScroll)
	}

	m.handleKey(tea.KeyMsg{Type: tea.KeyHome})
	if m.detailScroll != 0 {
		t.Fatalf("unexpected detail scroll after home: got %d want 0", m.detailScroll)
	}
}

func TestEnteringDetailsResetsScroll(t *testing.T) {
	m := NewModel(Config{NoColor: true})
	m.mode = viewTable
	m.detailScroll = 7
	m.disks = []domain.Disk{{
		ID:         "disk-a",
		Model:      "Model",
		DevicePath: "/dev/sda",
	}}
	m.rebuildRows()

	model, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	got := model.(*Model)

	if got.mode != viewDetails {
		t.Fatalf("unexpected mode after enter: got %v want %v", got.mode, viewDetails)
	}
	if got.detailScroll != 0 {
		t.Fatalf("expected detail scroll reset on enter, got %d", got.detailScroll)
	}
}
