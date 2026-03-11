package app

import (
	"strings"
	"testing"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
	"github.com/skobkin/simple-hdd-tool/internal/linux"
)

func TestRenderDiskRowStaysSingleLine(t *testing.T) {
	m := NewModel(Config{})
	m.width = 120

	row := m.renderDiskRow(domain.Disk{
		Model:      "HGST HUS728T8TALE6L4",
		Family:     "Some Very Long Family Name",
		Serial:     "Z840DGBG",
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

	assertSubstringsInOrder(t, out, "Identity", "Health", "smartctl health: passed", "App diagnosis: warning", "Problem details: pending sectors=2; SMART error log count=1")
	if !strings.Contains(out, "Problem details: pending sectors=2; SMART error log count=1") {
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

	assertSubstringsInOrder(t, out, "Health", "smartctl health: passed", "App diagnosis: healthy", "Problem: —", "SMART", "Temperature: 31 C", "Removal / Usage", "Removal obstacle: mounted; swap active")
	if !strings.Contains(out, "Removal obstacle: mounted; swap active") {
		t.Fatalf("renderDetails() did not include removal obstacle:\n%s", out)
	}
	if strings.Contains(out, "Problem: mounted") {
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
	m.readLoader = &linux.ReadLoader{}

	out := m.renderReadLoad()

	if !strings.Contains(out, "Stop") {
		t.Fatalf("renderReadLoad() did not include stop button:\n%s", out)
	}
	if strings.Contains(out, "Enter stop") {
		t.Fatalf("renderReadLoad() still included removed stop hint:\n%s", out)
	}
}
