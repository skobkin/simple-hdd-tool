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

func TestParseSmartctlHealth(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		output string
		want   domain.SmartctlHealth
	}{
		{
			name: "passed",
			output: `smartctl 7.5

=== START OF READ SMART DATA SECTION ===
SMART overall-health self-assessment test result: PASSED
`,
			want: domain.SmartctlHealthPassed,
		},
		{
			name: "failed",
			output: `smartctl 7.5

=== START OF READ SMART DATA SECTION ===
SMART overall-health self-assessment test result: FAILED
`,
			want: domain.SmartctlHealthFailed,
		},
		{
			name: "missing line",
			output: `smartctl 7.5

=== START OF READ SMART DATA SECTION ===
SMART Attributes Data Structure revision number: 10
`,
			want: domain.SmartctlHealthUnknown,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := parseSmartctlHealth([]byte(tt.output))
			if got != tt.want {
				t.Fatalf("parseSmartctlHealth() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestClassifyProblemDoesNotWarnOnSmartErrorLogAlone(t *testing.T) {
	t.Parallel()

	errorCount := uint64(1)

	health, problem, details, note := classifyProblem(domain.Disk{
		Smart: domain.SmartInfo{
			Available:  true,
			ErrorCount: &errorCount,
		},
	})

	if health != domain.HealthHealthy {
		t.Fatalf("health = %q, want %q", health, domain.HealthHealthy)
	}
	if problem != "—" {
		t.Fatalf("problem = %q, want %q", problem, "—")
	}
	if details != "SMART error log count=1" {
		t.Fatalf("details = %q", details)
	}
	if note != "SMART error log contains historical entries" {
		t.Fatalf("note = %q", note)
	}
}

func TestClassifyProblemExplainsSmartctlDisagreement(t *testing.T) {
	t.Parallel()

	pending := uint64(2)

	health, problem, details, note := classifyProblem(domain.Disk{
		Smart: domain.SmartInfo{
			Available:      true,
			OverallHealth:  domain.SmartctlHealthPassed,
			PendingSectors: &pending,
		},
	})

	if health != domain.HealthFailing {
		t.Fatalf("health = %q, want %q", health, domain.HealthFailing)
	}
	if problem != "disk failure" {
		t.Fatalf("problem = %q, want %q", problem, "disk failure")
	}
	if details != "pending sectors=2" {
		t.Fatalf("details = %q", details)
	}
	wantNote := "critical SMART counters are non-zero; app diagnosis is stricter than smartctl overall-health"
	if note != wantNote {
		t.Fatalf("note = %q, want %q", note, wantNote)
	}
}

func TestClassifyProblemIncludesKernelUsageDetails(t *testing.T) {
	t.Parallel()

	health, problem, details, note := classifyProblem(domain.Disk{
		Smart: domain.SmartInfo{
			Available: true,
		},
		Usage: domain.UsageFlags{
			Mounted: true,
			Swap:    true,
		},
	})

	if health != domain.HealthHealthy {
		t.Fatalf("health = %q, want %q", health, domain.HealthHealthy)
	}
	if problem != "—" {
		t.Fatalf("problem = %q, want %q", problem, "—")
	}
	if details != "" {
		t.Fatalf("details = %q", details)
	}
	if note != "" {
		t.Fatalf("note = %q", note)
	}
}

func TestRemovalObstacleDetailsIncludesKernelUsageDetails(t *testing.T) {
	t.Parallel()

	details := removalObstacleDetails(domain.UsageFlags{
		Mounted: true,
		Swap:    true,
	})

	if details != "mounted; swap active" {
		t.Fatalf("details = %q", details)
	}
}
