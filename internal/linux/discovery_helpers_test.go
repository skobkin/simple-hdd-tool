package linux

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	smart "github.com/anatol/smart.go"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

func TestDiscoverBlockDevicesFiltersAndSorts(t *testing.T) {
	oldRoot := sysBlockRoot
	oldReadDir := readDir
	oldReadFile := readFile
	sysBlockRoot = "/sys/block"
	readDir = func(path string) ([]os.DirEntry, error) {
		if path != "/sys/block" {
			return nil, errors.New("unexpected path")
		}

		return []os.DirEntry{
			fakeDirEntry{name: "sdb"},
			fakeDirEntry{name: "sda"},
			fakeDirEntry{name: "sdc"},
			fakeDirEntry{name: "loop0"},
		}, nil
	}
	readFile = func(path string) ([]byte, error) {
		switch path {
		case "/sys/block/sda/device/type", "/sys/block/sdb/device/type", "/sys/block/loop0/device/type":
			return []byte("0\n"), nil
		case "/sys/block/sdc/device/type":
			return []byte("5\n"), nil
		default:
			return nil, os.ErrNotExist
		}
	}
	t.Cleanup(func() {
		sysBlockRoot = oldRoot
		readDir = oldReadDir
		readFile = oldReadFile
	})

	got, err := discoverBlockDevices()
	if err != nil {
		t.Fatalf("discoverBlockDevices() error = %v", err)
	}
	want := []string{"sda", "sdb"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("discoverBlockDevices() = %v, want %v", got, want)
	}
}

func TestInspectHoldersDetectsRaidAndDM(t *testing.T) {
	oldRoot := sysBlockRoot
	oldReadDir := readDir
	oldGlob := globPaths
	sysBlockRoot = "/sys/block"
	readDir = func(path string) ([]os.DirEntry, error) {
		if path != "/sys/block/sda/holders" {
			return nil, errors.New("unexpected path")
		}

		return []os.DirEntry{
			fakeDirEntry{name: "md0"},
			fakeDirEntry{name: "dm-0"},
		}, nil
	}
	globPaths = func(pattern string) ([]string, error) {
		if pattern != "/sys/block/md*/slaves/sda" {
			return nil, errors.New("unexpected pattern")
		}

		return []string{"/sys/block/md127/slaves/sda"}, nil
	}
	t.Cleanup(func() {
		sysBlockRoot = oldRoot
		readDir = oldReadDir
		globPaths = oldGlob
	})

	got, partial := inspectHolders("sda")
	if partial {
		t.Fatalf("inspectHolders() partial = true, want false")
	}
	if !got.RAIDMember || !got.DMHolder {
		t.Fatalf("inspectHolders() = %+v, want RAIDMember and DMHolder", got)
	}
}

func TestInspectHoldersMarksPartialOnReadFailure(t *testing.T) {
	oldReadDir := readDir
	readDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { readDir = oldReadDir })

	_, partial := inspectHolders("sda")
	if !partial {
		t.Fatalf("inspectHolders() partial = false, want true")
	}
}

func TestDetectTransportUsesProtocolAndFallbacks(t *testing.T) {
	oldRoot := sysBlockRoot
	oldReadFile := readFile
	oldEval := evalSymlinks
	sysBlockRoot = "/sys/block"
	readFile = func(path string) ([]byte, error) {
		switch path {
		case "/sys/block/sata0/device/protocol":
			return []byte("ata\n"), nil
		case "/sys/block/sas0/device/protocol":
			return []byte("sas\n"), nil
		case "/sys/block/usb0/device/protocol":
			return []byte("usb\n"), nil
		case "/sys/block/fc0/device/protocol":
			return []byte("fc\n"), nil
		default:
			return nil, os.ErrNotExist
		}
	}
	evalSymlinks = func(path string) (string, error) {
		switch path {
		case "/sys/block/symlink/device":
			return "/devices/pci/ata1", nil
		case "/sys/block/default/device":
			return "/devices/pci/unknown", nil
		default:
			return "", errors.New("missing")
		}
	}
	t.Cleanup(func() {
		sysBlockRoot = oldRoot
		readFile = oldReadFile
		evalSymlinks = oldEval
	})

	cases := map[string]string{
		"sata0":   "sata",
		"sas0":    "sas",
		"usb0":    "usb",
		"fc0":     "fc",
		"symlink": "sata",
		"default": "scsi",
	}
	for name, want := range cases {
		if got := detectTransport(name); got != want {
			t.Fatalf("detectTransport(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestScanSmartctlInfoHandlesCommonBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("smartctl missing", func(t *testing.T) {
		oldLookPath := lookPath
		lookPath = func(string) (string, error) { return "", exec.ErrNotFound }
		t.Cleanup(func() { lookPath = oldLookPath })

		got, err := scanSmartctlInfo(ctx, "/dev/sda", time.Second, "-H")
		if err != nil {
			t.Fatalf("scanSmartctlInfo() error = %v", err)
		}
		if got != (smartctlInfo{}) {
			t.Fatalf("scanSmartctlInfo() = %+v, want zero value", got)
		}
	})

	t.Run("untrusted device path", func(t *testing.T) {
		oldLookPath := lookPath
		lookPath = func(string) (string, error) { return "/usr/bin/smartctl", nil }
		t.Cleanup(func() { lookPath = oldLookPath })

		_, err := scanSmartctlInfo(ctx, "/tmp/sda", time.Second, "-H")
		if err == nil || !strings.Contains(err.Error(), "untrusted") {
			t.Fatalf("scanSmartctlInfo() error = %v, want untrusted path error", err)
		}
	})
}

func TestScanSmartctlInfoExecBranches(t *testing.T) {
	oldLookPath := lookPath
	oldCmd := commandContext
	lookPath = func(string) (string, error) { return "/usr/bin/smartctl", nil }
	t.Cleanup(func() {
		lookPath = oldLookPath
		commandContext = oldCmd
	})

	t.Run("generic exec error", func(t *testing.T) {
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "/definitely/missing/binary")
		}

		_, err := scanSmartctlInfo(context.Background(), "/dev/sda", 10*time.Millisecond, "-H")
		if err == nil {
			t.Fatalf("scanSmartctlInfo() error = nil, want exec error")
		}
	})

	t.Run("exit error still parses output", func(t *testing.T) {
		commandContext = func(ctx context.Context, name string, args ...string) *exec.Cmd {
			return exec.CommandContext(ctx, "sh", "-c", "printf 'Model Family: Test Family\nSMART overall-health self-assessment test result: FAILED\n'; exit 4")
		}

		got, err := scanSmartctlInfo(context.Background(), "/dev/sda", time.Second, "-H")
		if err != nil {
			t.Fatalf("scanSmartctlInfo() error = %v", err)
		}
		if got.Family != "Test Family" || got.OverallHealth != domain.SmartctlHealthFailed {
			t.Fatalf("scanSmartctlInfo() = %+v", got)
		}
	})
	for _, code := range []int{0, 1, 2, 4, 8, 12, 16, 32, 64, 128, 255} {
		for _, output := range []string{"", "SMART Health Status: OK"} {
			t.Run(fmt.Sprintf("exit %d output %q", code, output), func(t *testing.T) {
				commandContext = func(ctx context.Context, _ string, _ ...string) *exec.Cmd {
					// #nosec G204 -- output and exit code are fixed test fixtures passed as positional arguments.
					return exec.CommandContext(ctx, "sh", "-c", `printf '%s\n' "$1"; exit "$2"`, "sh", output, fmt.Sprint(code))
				}
				got, err := scanSmartctlInfo(context.Background(), "/dev/sda", time.Second, "-H")
				want := domain.SmartctlHealthUnknown
				if output != "" {
					want = domain.SmartctlHealthPassed
				}
				if code&(1<<3) != 0 {
					want = domain.SmartctlHealthFailed
				}
				if err != nil || got.OverallHealth != want {
					t.Fatalf("scanSmartctlInfo() = %+v, %v; want %q", got, err, want)
				}
			})
		}
	}

}

func TestScanSMARTReturnsContextAndTimeoutErrors(t *testing.T) {
	oldScanSMARTSync := scanSMARTSyncFunc
	t.Cleanup(func() { scanSMARTSyncFunc = oldScanSMARTSync })

	t.Run("context canceled", func(t *testing.T) {
		blocked := make(chan struct{})
		scanSMARTSyncFunc = func(string) (domain.SmartInfo, identity, error) {
			<-blocked

			return domain.SmartInfo{}, identity{}, nil
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		_, _, err := scanSMART(ctx, "/dev/sda", time.Second)
		close(blocked)
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("scanSMART() error = %v, want canceled", err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		blocked := make(chan struct{})
		scanSMARTSyncFunc = func(string) (domain.SmartInfo, identity, error) {
			<-blocked

			return domain.SmartInfo{}, identity{}, nil
		}

		_, _, err := scanSMART(context.Background(), "/dev/sda", 10*time.Millisecond)
		close(blocked)
		if err == nil || !strings.Contains(err.Error(), "timed out") {
			t.Fatalf("scanSMART() error = %v, want timed out", err)
		}
	})
}

func TestFillGenericAndAtaSMART(t *testing.T) {
	info := domain.SmartInfo{}
	fillGeneric(&info, &smart.GenericAttributes{
		Temperature:  31,
		PowerOnHours: 99,
		PowerCycles:  7,
	})

	if info.TemperatureC == nil || *info.TemperatureC != 31 {
		t.Fatalf("TemperatureC = %v", info.TemperatureC)
	}
	if info.PowerOnHours == nil || *info.PowerOnHours != 99 {
		t.Fatalf("PowerOnHours = %v", info.PowerOnHours)
	}
	if info.PowerCycleCount == nil || *info.PowerCycleCount != 7 {
		t.Fatalf("PowerCycleCount = %v", info.PowerCycleCount)
	}

	info = domain.SmartInfo{}
	fillAtaSMART(&info, &smart.AtaSmartPage{
		Attrs: map[uint8]smart.AtaSmartAttr{
			ataAttrReallocatedSectors: {ValueRaw: 2},
			ataAttrPowerOnHours:       {ValueRaw: 120},
			ataAttrPowerCycleCount:    {ValueRaw: 5},
			ataAttrPendingSectors:     {ValueRaw: 3},
			ataAttrUncorrectable:      {ValueRaw: 4},
			ataAttrStartStopCount:     {ValueRaw: 8},
			ataAttrTemperature: {
				Type:     smart.AtaDeviceAttributeTypeTempMinMax,
				ValueRaw: 30,
			},
		},
	})

	if info.ReallocatedSectors == nil || *info.ReallocatedSectors != 2 {
		t.Fatalf("ReallocatedSectors = %v", info.ReallocatedSectors)
	}
	if info.PendingSectors == nil || *info.PendingSectors != 3 {
		t.Fatalf("PendingSectors = %v", info.PendingSectors)
	}
	if info.UncorrectableErrors == nil || *info.UncorrectableErrors != 4 {
		t.Fatalf("UncorrectableErrors = %v", info.UncorrectableErrors)
	}
	if info.StartStopCount == nil || *info.StartStopCount != 8 {
		t.Fatalf("StartStopCount = %v", info.StartStopCount)
	}
	if info.TemperatureC == nil || *info.TemperatureC != 30 {
		t.Fatalf("TemperatureC = %v", info.TemperatureC)
	}
}

func TestClassifyProblemAdditionalBranches(t *testing.T) {
	t.Run("smart read error", func(t *testing.T) {
		health, problem, details, note := classifyProblem(domain.Disk{
			Smart: domain.SmartInfo{ReadError: "no smart"},
		})
		if health != domain.HealthWarning || problem != "SMART read failure" || details != "no smart" || note != "no smart" {
			t.Fatalf("unexpected classifyProblem result: %q %q %q %q", health, problem, details, note)
		}
	})

	t.Run("checks partial", func(t *testing.T) {
		health, problem, details, note := classifyProblem(domain.Disk{
			Usage: domain.UsageFlags{ChecksPartial: true},
		})
		if health != domain.HealthWarning || problem != "unknown" || details != "usage verification incomplete" || note != "could not fully verify device usage" {
			t.Fatalf("unexpected classifyProblem result: %q %q %q %q", health, problem, details, note)
		}
	})

	t.Run("unknown when smart unavailable", func(t *testing.T) {
		health, problem, details, note := classifyProblem(domain.Disk{})
		if health != domain.HealthUnknown || problem != "unknown" || details != "" || note != "" {
			t.Fatalf("unexpected classifyProblem result: %q %q %q %q", health, problem, details, note)
		}
	})
}

func TestUsageAndSmartProblemDetails(t *testing.T) {
	if got := usageProblemDetails(domain.UsageFlags{Mounted: true, RAIDMember: true, DMHolder: true}); got != "mounted; mdraid holder present; device-mapper holder present" {
		t.Fatalf("usageProblemDetails() = %q", got)
	}

	one := uint64(1)
	two := uint64(2)
	if got := smartProblemDetails(domain.SmartInfo{
		ReallocatedSectors:  &one,
		PendingSectors:      &two,
		UncorrectableErrors: &one,
		ErrorCount:          &two,
	}); got != "reallocated sectors=1; pending sectors=2; uncorrectable errors=1; SMART error log count=2" {
		t.Fatalf("smartProblemDetails() = %q", got)
	}
}

func TestTrustedPathHelpersAndReadText(t *testing.T) {
	if !isTrustedFSPath("/sys/block/sda") || !isTrustedFSPath("/proc/swaps") {
		t.Fatalf("expected trusted fs paths")
	}
	if isTrustedFSPath("relative/path") || isTrustedFSPath("/tmp/file") {
		t.Fatalf("unexpected trusted fs path")
	}
	if !isTrustedDevicePath("/dev/sda") || isTrustedDevicePath("/dev/nvme0n1") {
		t.Fatalf("unexpected trusted device path result")
	}
	if got := dashIfEmpty(" \t "); got != "—" {
		t.Fatalf("dashIfEmpty() = %q", got)
	}

	root := t.TempDir()
	path := filepath.Join(root, "value")
	writeText(t, path, "ignored")

	oldReadFile := readFile
	readFile = os.ReadFile
	t.Cleanup(func() { readFile = oldReadFile })

	if got := readText(path); got != "" {
		t.Fatalf("readText() for untrusted path = %q, want empty", got)
	}

	oldReadFile = readFile
	readFile = func(string) ([]byte, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { readFile = oldReadFile })
	if got := readText("/sys/block/sda"); got != "" {
		t.Fatalf("readText() on read error = %q, want empty", got)
	}
}

func TestIsWritableUsesTrustedPaths(t *testing.T) {
	oldOpen := openFileWritable
	t.Cleanup(func() { openFileWritable = oldOpen })

	openFileWritable = func(path string) (*os.File, error) {
		f, err := os.CreateTemp(t.TempDir(), "writable-*")
		if err != nil {
			t.Fatal(err)
		}

		return f, nil
	}
	if !isWritable("/sys/block/sda/device/delete") {
		t.Fatalf("isWritable() = false, want true")
	}

	openFileWritable = func(string) (*os.File, error) { return nil, errors.New("nope") }
	if isWritable("/sys/block/sda/device/delete") {
		t.Fatalf("isWritable() = true, want false")
	}
	if isWritable("/tmp/delete") {
		t.Fatalf("isWritable() trusted untrusted path")
	}
}

func writeText(t *testing.T, path, content string) {
	t.Helper()

	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

type fakeDirEntry struct {
	name string
}

func (f fakeDirEntry) Name() string               { return f.name }
func (f fakeDirEntry) IsDir() bool                { return true }
func (f fakeDirEntry) Type() os.FileMode          { return os.ModeDir }
func (f fakeDirEntry) Info() (os.FileInfo, error) { return fakeFileInfo(f.name), nil }
