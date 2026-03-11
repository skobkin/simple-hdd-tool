package app

import (
	"strings"
	"testing"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
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
	})

	if strings.Contains(row, "\n") {
		t.Fatalf("renderDiskRow produced a multiline row: %q", row)
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
	}}
	m.rebuildRows()

	out := m.renderDetails()

	if !strings.Contains(out, "Problem details: pending sectors=2; SMART error log count=1") {
		t.Fatalf("renderDetails() did not include problem details:\n%s", out)
	}
}
