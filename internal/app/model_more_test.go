package app

import (
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
	"github.com/skobkin/simple-hdd-tool/internal/linux"
)

func TestHelpTextContainsUsageAndFlags(t *testing.T) {
	t.Parallel()

	text := HelpText()
	if !strings.Contains(text, "Usage:") || !strings.Contains(text, "--group-by") || !strings.Contains(text, "--sort-by") {
		t.Fatalf("HelpText() = %q", text)
	}
}

func TestParseConfigMoreBranches(t *testing.T) {
	t.Parallel()

	if _, err := ParseConfig([]string{"--sort-by=bogus"}); err == nil {
		t.Fatalf("expected invalid sort-by error")
	}
	if _, err := ParseConfig([]string{"extra"}); err == nil {
		t.Fatalf("expected positional args error")
	}
	cfg, err := ParseConfig([]string{"--help"})
	if err != nil || !cfg.ShowHelp {
		t.Fatalf("ParseConfig(--help) = %+v, %v", cfg, err)
	}
}

func TestViewDispatchAndTableNavigation(t *testing.T) {
	t.Parallel()

	m := NewModel(Config{NoColor: true, GroupBy: domain.GroupByNone, SortBy: domain.SortBySize})
	m.disks = []domain.Disk{
		{ID: "a", Model: "Alpha", DevicePath: "/dev/sda", SizeBytes: 2},
		{ID: "b", Model: "Beta", DevicePath: "/dev/sdb", SizeBytes: 1},
	}
	m.rebuildRows()

	if got := m.View().Content; !strings.Contains(got, "Scanning SATA/SAS disks") {
		t.Fatalf("View() scanning = %q", got)
	}

	m.mode = viewTable
	if got := m.View().Content; !strings.Contains(got, "disks found") {
		t.Fatalf("View() table = %q", got)
	}

	model, _ := m.handleKey(keyPress(tea.KeyDown, "", 0))
	m = model.(*Model)
	if disk := m.selectedDisk(); disk == nil || disk.ID != "a" {
		t.Fatalf("selectedDisk() = %+v, want a", disk)
	}

	model, _ = m.handleKey(keyPress('g', "g", 0))
	m = model.(*Model)
	if m.cfg.GroupBy != domain.GroupByModel {
		t.Fatalf("GroupBy = %q, want %q", m.cfg.GroupBy, domain.GroupByModel)
	}

	model, _ = m.handleKey(keyPress('s', "s", 0))
	m = model.(*Model)
	if m.cfg.SortBy != domain.SortBySerial {
		t.Fatalf("SortBy = %q, want %q", m.cfg.SortBy, domain.SortBySerial)
	}
}

func TestUpdateWindowAndResultMessages(t *testing.T) {
	t.Parallel()

	m := NewModel(Config{NoColor: true, GroupBy: domain.GroupByNone, SortBy: domain.SortBySize})
	m.mode = viewDetails
	m.width = 40
	m.height = 8
	m.disks = []domain.Disk{{
		ID:             "disk-a",
		Model:          "Model",
		DevicePath:     "/dev/sda",
		Problem:        "warning",
		ProblemDetails: strings.Repeat("overflow ", 12),
	}}
	m.rebuildRows()
	m.detailScroll = 100
	m.scanRunner = &scanRunner{progress: make(chan domain.ScanProgress), result: make(chan domain.ScanResult, 1)}
	m.removeRunner = &removeRunner{progress: make(chan domain.RemovalProgress), result: make(chan domain.RemovalResult, 1)}

	model, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	m = model.(*Model)
	if m.width != 60 || m.height != 12 || m.detailScroll > m.maxDetailScroll() {
		t.Fatalf("unexpected window update state: %+v", m)
	}

	model, _ = m.Update(scanProgressMsg(domain.ScanProgress{Current: 1, Total: 3, Text: "one"}))
	m = model.(*Model)
	if m.scanProgress.Text != "one" {
		t.Fatalf("scanProgress = %+v", m.scanProgress)
	}

	model, _ = m.Update(scanCompleteMsg(domain.ScanResult{
		Disks:    []domain.Disk{{ID: "disk-b", Model: "Beta", DevicePath: "/dev/sdb"}},
		ReadOnly: true,
	}))
	m = model.(*Model)
	if m.mode != viewTable || !m.readOnly || len(m.rows) == 0 {
		t.Fatalf("unexpected scan complete state: mode=%v readOnly=%v rows=%d", m.mode, m.readOnly, len(m.rows))
	}

	model, _ = m.Update(scanCompleteMsg(domain.ScanResult{Err: errors.New("scan failed")}))
	m = model.(*Model)
	if m.mode != viewInfo || !strings.Contains(m.infoText, "Scan failed") {
		t.Fatalf("unexpected scan failure state: mode=%v info=%q", m.mode, m.infoText)
	}

	model, _ = m.Update(removeProgressMsg(domain.RemovalProgress{Step: "checking"}))
	m = model.(*Model)
	if m.removeProgress != "checking" {
		t.Fatalf("removeProgress = %q", m.removeProgress)
	}

	model, _ = m.Update(removeCompleteMsg(domain.RemovalResult{Err: errors.New("remove failed")}))
	m = model.(*Model)
	if m.mode != viewInfo || !strings.Contains(m.infoText, "Remove failed") {
		t.Fatalf("unexpected remove failure state: mode=%v info=%q", m.mode, m.infoText)
	}
}

func TestHandleKeyInfoAndDetailsBranches(t *testing.T) {
	t.Parallel()

	m := NewModel(Config{NoColor: true, GroupBy: domain.GroupByNone, SortBy: domain.SortBySize})
	m.mode = viewInfo
	m.infoText = "status"
	model, _ := m.handleKey(keyPress(tea.KeyEnter, "", 0))
	m = model.(*Model)
	if m.mode != viewTable || m.infoText != "" {
		t.Fatalf("info dismiss state: mode=%v info=%q", m.mode, m.infoText)
	}

	m.mode = viewDetails
	m.disks = []domain.Disk{{ID: "a", Model: "Model", DevicePath: "/dev/sda"}}
	m.rebuildRows()
	model, _ = m.handleKey(keyPress(tea.KeyTab, "", 0))
	m = model.(*Model)
	if m.detailAction != detailActionReadLoad {
		t.Fatalf("detailAction = %v, want read load", m.detailAction)
	}
	model, _ = m.handleKey(keyPress(tea.KeyTab, "", tea.ModShift))
	m = model.(*Model)
	if m.detailAction != detailActionClose {
		t.Fatalf("detailAction = %v, want close", m.detailAction)
	}
	model, _ = m.handleKey(keyPress(tea.KeyEsc, "", 0))
	m = model.(*Model)
	if m.mode != viewTable {
		t.Fatalf("mode = %v, want table", m.mode)
	}
}

func TestRunDetailActionBranches(t *testing.T) {
	t.Parallel()

	oldStart := startReadLoad
	defer func() { startReadLoad = oldStart }()

	m := NewModel(Config{NoColor: true})
	model, _ := m.runDetailAction()
	if model.(*Model).mode != viewTable {
		t.Fatalf("nil selected disk should return table mode")
	}

	m = NewModel(Config{NoColor: true})
	m.mode = viewDetails
	m.disks = []domain.Disk{{
		ID:         "a",
		Model:      "Model",
		DevicePath: "/dev/sda",
		SizeBytes:  1024,
		Caps:       domain.Capabilities{CanReadLoad: true, CanRemove: true},
	}}
	m.rebuildRows()

	m.detailAction = detailActionClose
	model, _ = m.runDetailAction()
	if model.(*Model).mode != viewTable {
		t.Fatalf("close action mode = %v", model.(*Model).mode)
	}

	mReadOnly := NewModel(Config{NoColor: true, GroupBy: domain.GroupByNone, SortBy: domain.SortBySize})
	mReadOnly.mode = viewDetails
	mReadOnly.disks = []domain.Disk{{
		ID:         "a",
		Model:      "Model",
		DevicePath: "/dev/sda",
		SizeBytes:  1024,
		Caps:       domain.Capabilities{CanReadLoad: false},
	}}
	mReadOnly.rebuildRows()
	mReadOnly.detailAction = detailActionReadLoad
	model, _ = mReadOnly.runDetailAction()
	if model.(*Model).mode != viewInfo || !strings.Contains(model.(*Model).infoText, "Read load is unavailable") {
		t.Fatalf("read-only read load branch failed: mode=%v info=%q", model.(*Model).mode, model.(*Model).infoText)
	}

	mStartErr := NewModel(Config{NoColor: true, GroupBy: domain.GroupByNone, SortBy: domain.SortBySize})
	mStartErr.mode = viewDetails
	mStartErr.disks = []domain.Disk{{
		ID:         "a",
		Model:      "Model",
		DevicePath: "/dev/sda",
		SizeBytes:  1024,
		Caps:       domain.Capabilities{CanReadLoad: true},
	}}
	mStartErr.rebuildRows()
	mStartErr.detailAction = detailActionReadLoad
	startReadLoad = func(string, uint64) (*linux.ReadLoader, error) { return nil, errors.New("boom") }
	model, _ = mStartErr.runDetailAction()
	if model.(*Model).mode != viewInfo || !strings.Contains(model.(*Model).infoText, "Read load failed") {
		t.Fatalf("read load error branch failed: mode=%v info=%q", model.(*Model).mode, model.(*Model).infoText)
	}

	mRemove := NewModel(Config{NoColor: true, GroupBy: domain.GroupByNone, SortBy: domain.SortBySize})
	mRemove.mode = viewDetails
	mRemove.disks = []domain.Disk{{
		ID:         "a",
		Model:      "Model",
		DevicePath: "/dev/sda",
		SizeBytes:  1024,
		Caps:       domain.Capabilities{CanRemove: true},
	}}
	mRemove.rebuildRows()
	mRemove.detailAction = detailActionRemove
	mRemove.readOnly = true
	model, _ = mRemove.runDetailAction()
	if model.(*Model).mode != viewInfo || !strings.Contains(model.(*Model).infoText, "Remove is unavailable") {
		t.Fatalf("remove unavailable branch failed: mode=%v info=%q", model.(*Model).mode, model.(*Model).infoText)
	}

	mRemove.mode = viewDetails
	mRemove.readOnly = false
	model, _ = mRemove.runDetailAction()
	if model.(*Model).mode != viewConfirmRemove {
		t.Fatalf("remove confirm mode = %v", model.(*Model).mode)
	}
}

func TestHelperBranches(t *testing.T) {
	t.Parallel()

	if got := groupKey(domain.Disk{SizeBytes: 1500}, domain.GroupBySize); got != "1.5 KB" {
		t.Fatalf("groupKey(size) = %q", got)
	}
	if got := groupKey(domain.Disk{Family: "Fam"}, domain.GroupByFamily); got != "Fam" {
		t.Fatalf("groupKey(family) = %q", got)
	}
	if got := groupKey(domain.Disk{}, domain.GroupByModel); got != "—" {
		t.Fatalf("groupKey(model) = %q", got)
	}

	disks := []domain.Disk{
		{ID: "a", DevicePath: "/dev/sdb", Serial: "B"},
		{ID: "b", DevicePath: "/dev/sda"},
	}
	sortDisks(disks, domain.SortBySerial)
	if disks[0].ID != "a" {
		t.Fatalf("sortDisks(serial) first = %q", disks[0].ID)
	}

	h1 := uint64(100)
	h2 := uint64(5)
	disks = []domain.Disk{
		{ID: "a", DevicePath: "/dev/sdb", Smart: domain.SmartInfo{PowerOnHours: &h1}},
		{ID: "b", DevicePath: "/dev/sda", Smart: domain.SmartInfo{PowerOnHours: &h2}},
	}
	sortDisks(disks, domain.SortByHours)
	if disks[0].ID != "b" {
		t.Fatalf("sortDisks(hours) first = %q", disks[0].ID)
	}

	if got := sortString("", "B", "/dev/sda", "/dev/sdb"); got {
		t.Fatalf("sortString() = true, want false")
	}
	if got := progressBar(3, 2, 4); !strings.Contains(got, "] 2/2") {
		t.Fatalf("progressBar() = %q", got)
	}
	if got := smartctlHealthIndicator(newStyles(true), domain.SmartctlHealthFailed); !strings.Contains(got, "✕") {
		t.Fatalf("smartctlHealthIndicator() = %q", got)
	}
	if got := renderSmartctlHealthText(newStyles(true), domain.SmartctlHealthUnknown); !strings.Contains(got, "unknown") {
		t.Fatalf("renderSmartctlHealthText() = %q", got)
	}
	if got := maxInt(1, 2); got != 2 {
		t.Fatalf("maxInt() = %d", got)
	}
}
