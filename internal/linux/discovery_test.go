package linux

import (
	"testing"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

func TestParseSmartctlFamily(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   string
	}{
		{
			name: "extract family line",
			output: `smartctl 7.5

=== START OF INFORMATION SECTION ===
Model Family:     Seagate Archive HDD (SMR)
Device Model:     ST8000AS0002-1NA17Z
Serial Number:    Z123DGBG
`,
			want: "Seagate Archive HDD (SMR)",
		},
		{
			name: "missing family line",
			output: `smartctl 7.5

=== START OF INFORMATION SECTION ===
Device Model:     ST8000AS0002-1NA17Z
`,
			want: "",
		},
		{
			name:   "collapses excess spaces",
			output: "Model Family:   HGST   Ultrastar HC310/320   \n",
			want:   "HGST Ultrastar HC310/320",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseSmartctlFamily([]byte(tt.output))
			if got != tt.want {
				t.Fatalf("parseSmartctlFamily() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyProblemIncludesSmartCounterDetails(t *testing.T) {
	t.Parallel()

	errorCount := uint64(1)

	health, problem, details, note := classifyProblem(domain.Disk{
		Smart: domain.SmartInfo{
			ErrorCount: &errorCount,
		},
	})

	if health != domain.HealthWarning {
		t.Fatalf("health = %q, want %q", health, domain.HealthWarning)
	}
	if problem != "health warning" {
		t.Fatalf("problem = %q, want %q", problem, "health warning")
	}
	if details != "SMART error log count=1" {
		t.Fatalf("details = %q", details)
	}
	if note != "SMART counters indicate potential media issues" {
		t.Fatalf("note = %q", note)
	}
}

func TestClassifyProblemIncludesKernelUsageDetails(t *testing.T) {
	t.Parallel()

	health, problem, details, note := classifyProblem(domain.Disk{
		Usage: domain.UsageFlags{
			Mounted: true,
			Swap:    true,
		},
	})

	if health != domain.HealthWarning {
		t.Fatalf("health = %q, want %q", health, domain.HealthWarning)
	}
	if problem != "mounted" {
		t.Fatalf("problem = %q, want %q", problem, "mounted")
	}
	if details != "mounted; swap active" {
		t.Fatalf("details = %q", details)
	}
	if note != "device is in active use" {
		t.Fatalf("note = %q", note)
	}
}
