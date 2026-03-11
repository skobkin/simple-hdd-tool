package app

import (
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
	"github.com/skobkin/simple-hdd-tool/internal/linux"
)

func TestRenderDiskRowStaysSingleLine(t *testing.T) {
	m := NewModel(Config{})
	m.width = 120

	row := m.renderDiskRow(domain.Disk{
		Model:      "HGST HUS728T8TALE6L4",
		Family:     "Some Very Long Family Name",
		Serial:     "SERIAL01",
		DevicePath: "/dev/sdb",
		Problem:    "health warning",
		SizeBytes:  8001563222016,
		Smart: domain.SmartInfo{
			OverallHealth: domain.SmartctlHealthPassed,
		},
	})

	if strings.Contains(row, "\n") {
		t.Fatalf("renderDiskRow produced a multiline row: %q", row)
	}
}

func TestRenderDiskRowKeepsHourSuffixVisible(t *testing.T) {
	m := NewModel(Config{})
	m.width = 120
	hours := uint64(6*24*365 + 1*24*30 + 20*24 + 12)

	row := m.renderDiskRow(domain.Disk{
		Model:      "Model",
		Family:     "Family",
		Serial:     "Serial",
		DevicePath: "/dev/sdb",
		Problem:    "healthy",
		SizeBytes:  8001563222016,
		Smart: domain.SmartInfo{
			PowerOnHours: &hours,
		},
	})

	if !strings.Contains(row, "6y 1m 20d 12h") {
		t.Fatalf("renderDiskRow truncated hour suffix: %q", row)
	}
}

func TestRenderDetailsShowsProblemDetails(t *testing.T) {
	m := NewModel(Config{})
	m.mode = viewDetails
	m.disks = []domain.Disk{{
		ID:             "disk-1",
		Family:         "Family",
		Model:          "Model",
		Serial:         "Serial",
		DevicePath:     "/dev/sdb",
		Health:         domain.HealthWarning,
		Problem:        "health warning",
		ProblemDetails: "pending sectors=2; SMART error log count=1",
		Smart: domain.SmartInfo{
			OverallHealth: domain.SmartctlHealthPassed,
		},
	}}
	m.rebuildRows()

	out := m.renderDetails()
	plain := ansi.Strip(out)

	assertSubstringsInOrder(t, plain, "Identity", "Health", "smartctl health: passed", "App diagnosis: warning", "Problem details: pending sectors=2; SMART error log count=1")
	if !strings.Contains(plain, "Problem details: pending sectors=2; SMART error log count=1") {
		t.Fatalf("renderDetails() did not include problem details:\n%s", out)
	}
}

func TestRenderDetailsShowsRemovalObstacleSeparately(t *testing.T) {
	temp := uint64(31)
	m := NewModel(Config{})
	m.mode = viewDetails
	m.disks = []domain.Disk{{
		ID:              "disk-1",
		Family:          "Family",
		Model:           "Model",
		Serial:          "Serial",
		DevicePath:      "/dev/sdb",
		Health:          domain.HealthHealthy,
		Problem:         "—",
		RemovalObstacle: "mounted; swap active",
		Smart: domain.SmartInfo{
			TemperatureC:  &temp,
			OverallHealth: domain.SmartctlHealthPassed,
		},
	}}
	m.rebuildRows()

	out := m.renderDetails()
	plain := ansi.Strip(out)

	assertSubstringsInOrder(t, plain, "Health", "smartctl health: passed", "App diagnosis: healthy", "Problem: —", "SMART", "Temperature: 31 C", "Removal / Usage", "Removal obstacle: mounted; swap active")
	if !strings.Contains(plain, "Removal obstacle: mounted; swap active") {
		t.Fatalf("renderDetails() did not include removal obstacle:\n%s", out)
	}
	if strings.Contains(plain, "Problem: mounted") {
		t.Fatalf("renderDetails() rendered removal obstacle as problem:\n%s", out)
	}
}

func TestRenderColumnsIncludesHealthBeforeProblems(t *testing.T) {
	m := NewModel(Config{})

	columns := m.renderColumns()

	assertSubstringsInOrder(t, columns, "time", "health", "problems")
}

func TestRenderDiskRowShowsSmartctlHealthIndicator(t *testing.T) {
	m := NewModel(Config{})
	m.width = 120

	row := m.renderDiskRow(domain.Disk{
		Model:      "Model",
		Family:     "Family",
		Serial:     "Serial",
		DevicePath: "/dev/sdb",
		Problem:    "healthy",
		SizeBytes:  8001563222016,
		Smart: domain.SmartInfo{
			OverallHealth: domain.SmartctlHealthPassed,
		},
	})

	if !strings.Contains(row, "●") {
		t.Fatalf("renderDiskRow() did not include smartctl health indicator:\n%s", row)
	}
}

func TestRenderDiskRowExpandsFamilyBeforeLeavingSerialPadding(t *testing.T) {
	m := NewModel(Config{})
	m.width = 120
	disk := domain.Disk{
		Model:      "ST10000NM0086-2AA101",
		Family:     "Seagate Enterprise Capacity",
		Serial:     "SERIAL02",
		DevicePath: "/dev/sdb",
		Problem:    "healthy",
		SizeBytes:  10000000000000,
	}
	m.disks = []domain.Disk{disk}

	row := ansi.Strip(m.renderDiskRow(disk))

	if !strings.Contains(row, "Seagate Enterprise") {
		t.Fatalf("renderDiskRow() did not allocate enough family width:\n%s", row)
	}
	if strings.Contains(row, "SERIAL02      ") {
		t.Fatalf("renderDiskRow() kept oversized serial padding:\n%s", row)
	}
}

func TestRenderDiskRowPadsHealthColumnBeforeProblems(t *testing.T) {
	m := NewModel(Config{})
	m.width = 120

	row := ansi.Strip(m.renderDiskRow(domain.Disk{
		Model:      "Model",
		Family:     "Family",
		Serial:     "Serial",
		DevicePath: "/dev/sdb",
		Problem:    "disk failure",
		SizeBytes:  8001563222016,
		Smart: domain.SmartInfo{
			OverallHealth: domain.SmartctlHealthPassed,
		},
	}))

	if !regexp.MustCompile(`●\s{2,}disk failure`).MatchString(row) {
		t.Fatalf("renderDiskRow() did not pad the health column before problems:\n%s", row)
	}
}

func TestRenderDetailsOmitsEmptySmartSection(t *testing.T) {
	m := NewModel(Config{})
	m.mode = viewDetails
	m.disks = []domain.Disk{{
		ID:         "disk-1",
		Family:     "Family",
		Model:      "Model",
		Serial:     "Serial",
		DevicePath: "/dev/sdb",
		Health:     domain.HealthHealthy,
		Problem:    "—",
	}}
	m.rebuildRows()

	out := m.renderDetails()

	if strings.Contains(out, "SMART") {
		t.Fatalf("renderDetails() rendered empty SMART section:\n%s", out)
	}
}

func TestRenderDetailsKeepsActionsVisibleWhenBodyOverflows(t *testing.T) {
	m := NewModel(Config{NoColor: true})
	m.mode = viewDetails
	m.width = 48
	m.height = 10
	m.disks = []domain.Disk{{
		ID:             "disk-1",
		Family:         "Family",
		Model:          "Model",
		Serial:         "Serial",
		DevicePath:     "/dev/sdb",
		Health:         domain.HealthWarning,
		Problem:        "health warning",
		ProblemDetails: "pending sectors=2; SMART error log count=1; this text is intentionally long to force wrapping in a tiny terminal",
		ProblemNote:    "watch this disk closely and plan replacement",
	}}
	m.rebuildRows()

	out := m.renderDetails()

	if !strings.Contains(out, "Close") {
		t.Fatalf("renderDetails() did not keep action buttons visible:\n%s", out)
	}
	if !strings.Contains(out, "Up/Down/PgUp/PgDn scroll") {
		t.Fatalf("renderDetails() did not render scroll help for overflowing content:\n%s", out)
	}
}

func assertSubstringsInOrder(t *testing.T, text string, substrings ...string) {
	t.Helper()

	offset := 0
	for _, substring := range substrings {
		idx := strings.Index(text[offset:], substring)
		if idx < 0 {
			t.Fatalf("expected %q after offset %d in output:\n%s", substring, offset, text)
		}
		offset += idx + len(substring)
	}
}

func TestRenderReadLoadShowsStopButton(t *testing.T) {
	m := NewModel(Config{})
	m.mode = viewReadLoad
	m.width = 64
	m.readLoader = &linux.ReadLoader{}
	oldSnapshotReadLoad := snapshotReadLoad
	snapshotReadLoad = func(*linux.ReadLoader) domain.ReadLoadSnapshot {
		return domain.ReadLoadSnapshot{
			DevicePath:     "/dev/sda",
			DirectIO:       true,
			Elapsed:        12 * time.Second,
			BytesRead:      2400000000,
			BytesPerSecond: 201000000,
		}
	}
	t.Cleanup(func() { snapshotReadLoad = oldSnapshotReadLoad })

	out := m.renderReadLoad()
	plain := ansi.Strip(out)

	if !strings.Contains(plain, "Stop") {
		t.Fatalf("renderReadLoad() did not include stop button:\n%s", out)
	}
	assertSubstringsInOrder(t, plain,
		"Read Load",
		"Generating sustained read activity",
		"Read rate: 201 MB/s",
		"Elapsed: 12s  Data read: 2.4 GB",
		"Device: /dev/sda",
		"Mode: direct I/O",
		"Enter/S/Esc/Q stop",
	)
}

func TestRenderReadLoadShowsBufferedFallbackWarning(t *testing.T) {
	m := NewModel(Config{})
	m.mode = viewReadLoad
	m.width = 72
	m.readLoader = &linux.ReadLoader{}
	oldSnapshotReadLoad := snapshotReadLoad
	snapshotReadLoad = func(*linux.ReadLoader) domain.ReadLoadSnapshot {
		return domain.ReadLoadSnapshot{
			DevicePath:      "/dev/sdb",
			Elapsed:         3 * time.Second,
			BytesRead:       32000000,
			BytesPerSecond:  11000000,
			DirectIOMessage: "direct I/O unavailable; using buffered reads",
		}
	}
	t.Cleanup(func() { snapshotReadLoad = oldSnapshotReadLoad })

	out := m.renderReadLoad()
	plain := ansi.Strip(out)

	if !strings.Contains(plain, "Warning: direct I/O unavailable; using buffered reads") {
		t.Fatalf("renderReadLoad() did not include buffered fallback warning:\n%s", out)
	}
	if strings.Contains(plain, "Mode: direct I/O") {
		t.Fatalf("renderReadLoad() showed direct I/O mode during buffered fallback:\n%s", out)
	}
}

func TestRenderReadLoadWrapsSummaryInNarrowWidth(t *testing.T) {
	m := NewModel(Config{NoColor: true})
	m.mode = viewReadLoad
	m.width = 32
	m.readLoader = &linux.ReadLoader{}
	oldSnapshotReadLoad := snapshotReadLoad
	snapshotReadLoad = func(*linux.ReadLoader) domain.ReadLoadSnapshot {
		return domain.ReadLoadSnapshot{
			DevicePath:     "/dev/sdc",
			DirectIO:       true,
			Elapsed:        2*time.Minute + 5*time.Second,
			BytesRead:      9876543210,
			BytesPerSecond: 123456789,
		}
	}
	t.Cleanup(func() { snapshotReadLoad = oldSnapshotReadLoad })

	out := m.renderReadLoad()
	plain := ansi.Strip(out)

	if !strings.Contains(plain, "Elapsed: 2m 5s") || !strings.Contains(plain, "Data read: 9.9 GB") {
		t.Fatalf("renderReadLoad() did not split summary for narrow width:\n%s", out)
	}
	if !strings.Contains(plain, "Read rate: 123 MB/s") {
		t.Fatalf("renderReadLoad() lost the primary read-rate signal:\n%s", out)
	}
}
