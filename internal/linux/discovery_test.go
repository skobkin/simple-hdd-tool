package linux

import (
	"fmt"
	"strings"
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
Serial Number:    TEST-SN-01
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
		{name: "ATA trailing explanation", output: "SMART overall-health self-assessment test result: FAILED! Drive failure expected", want: domain.SmartctlHealthFailed},
		{name: "ATA failed trailing text", output: "SMART overall-health self-assessment test result: FAILED (attributes)", want: domain.SmartctlHealthFailed},
		{name: "SCSI passed", output: "SMART Health Status: OK", want: domain.SmartctlHealthPassed},
		{name: "SCSI unrecognized", output: "SMART Health Status: unavailable", want: domain.SmartctlHealthUnknown},
		{name: "ATA unknown", output: "SMART overall-health self-assessment test result: UNKNOWN", want: domain.SmartctlHealthUnknown},
		{name: "ATA empty", output: "SMART overall-health self-assessment test result:", want: domain.SmartctlHealthUnknown},
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

func TestClassifyProblemExplainsCounterWarning(t *testing.T) {
	t.Parallel()

	pending := uint64(2)

	health, problem, details, note := classifyProblem(domain.Disk{
		Smart: domain.SmartInfo{
			Available:      true,
			OverallHealth:  domain.SmartctlHealthPassed,
			PendingSectors: &pending,
		},
	})

	if health != domain.HealthWarning {
		t.Fatalf("health = %q, want %q", health, domain.HealthWarning)
	}
	if problem != "health warning" {
		t.Fatalf("problem = %q, want %q", problem, "health warning")
	}
	if details != "pending sectors=2" {
		t.Fatalf("details = %q", details)
	}
	wantNote := "SMART counters indicate wear or potential media issues; non-zero counters alone do not establish SMART failure"
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

func TestClassifyProblemMediaCounters(t *testing.T) {
	t.Parallel()
	zero, one, large := uint64(0), uint64(1), ^uint64(0)
	for _, status := range []domain.SmartctlHealth{domain.SmartctlHealthPassed, domain.SmartctlHealthUnknown, ""} {
		for _, counter := range []string{"reallocated sectors", "pending sectors", "uncorrectable errors"} {
			for _, value := range []*uint64{nil, &zero, &one, &large} {
				name := "absent"
				if value != nil {
					name = fmt.Sprint(*value)
				}
				t.Run(string(status)+"/"+counter+"/"+name, func(t *testing.T) {
					t.Parallel()
					info := domain.SmartInfo{Available: true, OverallHealth: status}
					switch counter {
					case "reallocated sectors":
						info.ReallocatedSectors = value
					case "pending sectors":
						info.PendingSectors = value
					case "uncorrectable errors":
						info.UncorrectableErrors = value
					}
					health, problem, details, note := classifyProblem(domain.Disk{Smart: info})
					if value != nil && *value > 0 {
						if health != domain.HealthWarning || problem != "health warning" || details != counter+"="+name || !strings.Contains(note, "non-zero counters alone do not establish SMART failure") {
							t.Fatalf("unexpected warning: %q %q %q %q", health, problem, details, note)
						}
					} else if health != domain.HealthHealthy || problem != "—" || details != "" || note != "" {
						t.Fatalf("unexpected healthy result: %q %q %q %q", health, problem, details, note)
					}
				})
			}
		}
	}
}

func TestClassifyProblemReportedFailureTakesPrecedence(t *testing.T) {
	t.Parallel()
	zero, one := uint64(0), uint64(1)
	for _, tt := range []struct {
		name string
		info domain.SmartInfo
	}{
		{name: "unavailable counters"},
		{name: "zero counters", info: domain.SmartInfo{Available: true, ReallocatedSectors: &zero, PendingSectors: &zero, UncorrectableErrors: &zero}},
		{name: "read error", info: domain.SmartInfo{ReadError: "read failed"}},
		{name: "media and historical errors", info: domain.SmartInfo{ReallocatedSectors: &one, ErrorCount: &one}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			info := tt.info
			info.OverallHealth = domain.SmartctlHealthFailed
			health, problem, details, note := classifyProblem(domain.Disk{Smart: info, Usage: domain.UsageFlags{ChecksPartial: true}})
			if health != domain.HealthFailing || problem != "SMART health failed" || !strings.Contains(note, "smartctl reports failing SMART health") {
				t.Fatalf("unexpected failure: %q %q %q %q", health, problem, details, note)
			}
			if info.ErrorCount != nil && (details != "reallocated sectors=1; SMART error log count=1" || !strings.Contains(note, "historical entries")) {
				t.Fatalf("lost counter details: %q %q", details, note)
			}
		})
	}
}
