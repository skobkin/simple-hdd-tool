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
