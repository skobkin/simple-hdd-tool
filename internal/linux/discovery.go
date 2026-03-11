package linux

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	smart "github.com/anatol/smart.go"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

var (
	sysBlockRoot      = "/sys/block"
	readDir           = os.ReadDir
	readFile          = os.ReadFile
	globPaths         = filepath.Glob
	evalSymlinks      = filepath.EvalSymlinks
	openFileWritable  = func(path string) (*os.File, error) { return os.OpenFile(path, os.O_WRONLY, 0) }
	geteuid           = os.Geteuid
	lookPath          = exec.LookPath
	commandContext    = exec.CommandContext
	openSmartDevice   = smart.Open
	scanSMARTSyncFunc = scanSMARTSync
)

// Scanner discovers disks and collects SMART and usage metadata for them.
type Scanner struct {
	PerDiskTimeout time.Duration
}

const (
	ataAttrStartStopCount     = 4
	ataAttrReallocatedSectors = 5
	ataAttrPowerOnHours       = 9
	ataAttrPowerCycleCount    = 12
	ataAttrTemperatureAirflow = 190
	ataAttrTemperature        = 194
	ataAttrPendingSectors     = 197
	ataAttrUncorrectable      = 198
)

// Scan discovers disks and streams progress while collecting metadata.
func (s Scanner) Scan(ctx context.Context, progress chan<- domain.ScanProgress) domain.ScanResult {
	names, err := discoverBlockDevices()
	if err != nil {
		return domain.ScanResult{Err: err, ReadOnly: true}
	}
	devices := make(map[string]struct{}, len(names))
	for _, name := range names {
		devices["/dev/"+name] = struct{}{}
	}
	usage := CollectUsageInfo(devices)

	disks := make([]domain.Disk, 0, len(names))
	readOnly := geteuid() != 0
	for idx, name := range names {
		select {
		case <-ctx.Done():
			return domain.ScanResult{Err: ctx.Err(), ReadOnly: readOnly}
		default:
		}
		progress <- domain.ScanProgress{Current: idx + 1, Total: len(names), Text: "Collecting metadata for /dev/" + name}
		disk := s.scanDisk(ctx, name, usage["/dev/"+name])
		if disk.Transport == "usb" || disk.Transport == "unsupported" {
			continue
		}
		if !disk.Caps.CanReadLoad || !disk.Caps.CanRemove {
			readOnly = true
		}
		disks = append(disks, disk)
	}

	sort.Slice(disks, func(i, j int) bool { return disks[i].DevicePath < disks[j].DevicePath })

	return domain.ScanResult{Disks: disks, ReadOnly: readOnly}
}

func discoverBlockDevices() ([]string, error) {
	entries, err := readDir(sysBlockRoot)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "sd") {
			continue
		}
		deviceType := strings.TrimSpace(readText(filepath.Join(sysBlockRoot, name, "device/type")))
		if deviceType != "" && deviceType != "0" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	return names, nil
}

func (s Scanner) scanDisk(ctx context.Context, name string, usage UsageInfo) domain.Disk {
	sysfsPath := filepath.Join(sysBlockRoot, name)
	devicePath := "/dev/" + name
	sizeSectors, _ := strconv.ParseUint(strings.TrimSpace(readText(filepath.Join(sysfsPath, "size"))), 10, 64)
	sizeBytes := sizeSectors * 512

	disk := domain.Disk{
		ID:         name,
		Name:       name,
		DevicePath: devicePath,
		SysfsPath:  sysfsPath,
		Family:     "-",
		Model:      dashIfEmpty(strings.TrimSpace(readText(filepath.Join(sysfsPath, "device/model")))),
		Serial:     dashIfEmpty(strings.TrimSpace(readText(filepath.Join(sysfsPath, "device/serial")))),
		SizeBytes:  sizeBytes,
		Transport:  detectTransport(name),
		Usage: domain.UsageFlags{
			Mounted:       usage.Mounted,
			Swap:          usage.Swap,
			ChecksPartial: usage.ChecksPart,
		},
		Caps: domain.Capabilities{
			CanReadLoad: geteuid() == 0,
			CanRemove:   isWritable(filepath.Join(sysfsPath, "device/delete")),
		},
		Health:  domain.HealthUnknown,
		Problem: "unknown",
	}

	holders, partial := inspectHolders(name)
	disk.Usage.RAIDMember = holders.RAIDMember
	disk.Usage.DMHolder = holders.DMHolder
	disk.Usage.ChecksPartial = disk.Usage.ChecksPartial || partial

	timeout := s.PerDiskTimeout
	if timeout == 0 {
		timeout = 3 * time.Second
	}
	smartInfo, ident, err := scanSMART(ctx, devicePath, timeout)
	if err == nil {
		disk.Smart = smartInfo
		if ident.Family != "" {
			disk.Family = ident.Family
		}
		if ident.Model != "" {
			disk.Model = ident.Model
		}
		if ident.Serial != "" {
			disk.Serial = ident.Serial
		}
		if ident.SizeBytes > 0 {
			disk.SizeBytes = ident.SizeBytes
		}
		disk.Caps.CanReadSMART = true
	} else {
		disk.Smart.ReadError = err.Error()
		disk.Warnings = append(disk.Warnings, "SMART read failed: "+err.Error())
	}
	if family, err := scanSmartctlFamily(ctx, devicePath, timeout); err == nil && family != "" {
		disk.Family = family
	} else if err != nil {
		disk.Warnings = append(disk.Warnings, "smartctl info read failed: "+err.Error())
	}
	if overallHealth, err := scanSmartctlHealth(ctx, devicePath, timeout); err == nil {
		disk.Smart.OverallHealth = overallHealth
	} else {
		disk.Smart.OverallHealthNote = err.Error()
		disk.Warnings = append(disk.Warnings, "smartctl health read failed: "+err.Error())
	}

	disk.Health, disk.Problem, disk.ProblemDetails, disk.ProblemNote = classifyProblem(disk)
	disk.RemovalObstacle = removalObstacleDetails(disk.Usage)

	return disk
}

type holderState struct {
	RAIDMember bool
	DMHolder   bool
}

func inspectHolders(name string) (holderState, bool) {
	var out holderState
	entries, err := readDir(filepath.Join(sysBlockRoot, name, "holders"))
	if err != nil {
		return out, true
	}
	for _, entry := range entries {
		target := entry.Name()
		if strings.HasPrefix(target, "md") {
			out.RAIDMember = true
		}
		if strings.HasPrefix(target, "dm-") {
			out.DMHolder = true
		}
	}
	slaves, err := globPaths(filepath.Join(sysBlockRoot, "md*", "slaves", name))
	if err == nil && len(slaves) > 0 {
		out.RAIDMember = true
	}

	return out, false
}

func detectTransport(name string) string {
	sysfsPath := filepath.Join(sysBlockRoot, name)
	protocol := strings.ToLower(strings.TrimSpace(readText(filepath.Join(sysfsPath, "device/protocol"))))
	if protocol != "" {
		switch protocol {
		case "ata", "sata":
			return "sata"
		case "sas":
			return "sas"
		case "usb":
			return "usb"
		default:
			return protocol
		}
	}

	resolved, err := evalSymlinks(filepath.Join(sysfsPath, "device"))
	if err == nil {
		if strings.Contains(resolved, "/usb") {
			return "usb"
		}
		if strings.Contains(resolved, "/ata") {
			return "sata"
		}
		if strings.Contains(resolved, "/sas") {
			return "sas"
		}
	}

	return "scsi"
}

type identity struct {
	Family    string
	Model     string
	Serial    string
	SizeBytes uint64
}

type smartctlInfo struct {
	Family        string
	OverallHealth domain.SmartctlHealth
}

func scanSmartctlFamily(ctx context.Context, devicePath string, timeout time.Duration) (string, error) {
	info, err := scanSmartctlInfo(ctx, devicePath, timeout, "-i")
	if err != nil {
		return "", err
	}

	return info.Family, nil
}

func scanSmartctlHealth(ctx context.Context, devicePath string, timeout time.Duration) (domain.SmartctlHealth, error) {
	info, err := scanSmartctlInfo(ctx, devicePath, timeout, "-H")
	if err != nil {
		return domain.SmartctlHealthUnknown, err
	}

	return info.OverallHealth, nil
}

func scanSmartctlInfo(ctx context.Context, devicePath string, timeout time.Duration, mode string) (smartctlInfo, error) {
	if _, err := lookPath("smartctl"); err != nil {
		return smartctlInfo{}, nil
	}
	if !isTrustedDevicePath(devicePath) {
		return smartctlInfo{}, errors.New("untrusted device path")
	}

	cmdCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := commandContext(cmdCtx, "smartctl", mode, devicePath) // #nosec G204 -- devicePath is restricted to discovered /dev/sdX block devices
	output, err := cmd.CombinedOutput()
	if err != nil {
		var exitErr *exec.ExitError
		switch {
		case errors.As(err, &exitErr):
			// smartctl returns bitmask exit codes for health and capability states.
			// The information section may still be valid, so keep parsing output.
		case cmdCtx.Err() != nil:
			return smartctlInfo{}, cmdCtx.Err()
		default:
			return smartctlInfo{}, err
		}
	}

	return smartctlInfo{
		Family:        parseSmartctlFamily(output),
		OverallHealth: parseSmartctlHealth(output),
	}, nil
}

func parseSmartctlFamily(output []byte) string {
	for _, line := range strings.Split(string(output), "\n") {
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.TrimSpace(key) != "Model Family" {
			continue
		}
		family := strings.Join(strings.Fields(value), " ")

		return family
	}

	return ""
}

func parseSmartctlHealth(output []byte) domain.SmartctlHealth {
	for _, line := range strings.Split(string(output), "\n") {
		trimmed := strings.TrimSpace(line)
		if !strings.Contains(trimmed, "SMART overall-health self-assessment test result:") {
			continue
		}
		switch {
		case strings.HasSuffix(trimmed, "PASSED"):
			return domain.SmartctlHealthPassed
		case strings.HasSuffix(trimmed, "FAILED"):
			return domain.SmartctlHealthFailed
		default:
			return domain.SmartctlHealthUnknown
		}
	}

	return domain.SmartctlHealthUnknown
}

func scanSMART(ctx context.Context, devicePath string, timeout time.Duration) (domain.SmartInfo, identity, error) {
	type result struct {
		smart domain.SmartInfo
		id    identity
		err   error
	}
	done := make(chan result, 1)
	go func() {
		sm, id, err := scanSMARTSyncFunc(devicePath)
		done <- result{smart: sm, id: id, err: err}
	}()

	select {
	case <-ctx.Done():
		return domain.SmartInfo{}, identity{}, ctx.Err()
	case <-time.After(timeout):
		return domain.SmartInfo{}, identity{}, errors.New("timed out")
	case res := <-done:
		return res.smart, res.id, res.err
	}
}

func scanSMARTSync(devicePath string) (info domain.SmartInfo, id identity, err error) {
	dev, err := openSmartDevice(devicePath)
	if err != nil {
		return domain.SmartInfo{}, identity{}, err
	}
	defer func() {
		if closeErr := dev.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	info = domain.SmartInfo{Available: true, OverallHealth: domain.SmartctlHealthUnknown}
	id = identity{}
	switch d := dev.(type) {
	case *smart.SataDevice:
		ident, err := d.Identify()
		if err == nil {
			id.Model = ident.ModelNumber()
			id.Serial = ident.SerialNumber()
			_, capBytes, _, _, _ := ident.Capacity()
			id.SizeBytes = capBytes
		}
		generic, err := d.ReadGenericAttributes()
		if err == nil {
			fillGeneric(&info, generic)
		}
		page, err := d.ReadSMARTData()
		if err == nil {
			fillAtaSMART(&info, page)
		}
		log, err := d.ReadSMARTErrorLogSummary()
		if err == nil {
			v := uint64(log.ErrorCount)
			info.ErrorCount = &v
		}
	case *smart.ScsiDevice:
		inquiry, err := d.Inquiry()
		if err == nil {
			id.Family = strings.TrimSpace(string(bytes.TrimSpace(inquiry.VendorIdent[:])))
			id.Model = strings.TrimSpace(string(bytes.TrimSpace(inquiry.ProductIdent[:])))
		}
		serial, err := d.SerialNumber()
		if err == nil && serial != "" {
			id.Serial = serial
		}
		capacity, err := d.Capacity()
		if err == nil {
			id.SizeBytes = capacity
		}
		generic, err := d.ReadGenericAttributes()
		if err == nil {
			fillGeneric(&info, generic)
		}
	default:
		generic, err := dev.ReadGenericAttributes()
		if err == nil {
			fillGeneric(&info, generic)
		}
	}

	return info, id, nil
}

func fillGeneric(info *domain.SmartInfo, generic *smart.GenericAttributes) {
	poh := generic.PowerOnHours
	info.PowerOnHours = &poh
	if generic.Temperature > 0 {
		t := generic.Temperature
		info.TemperatureC = &t
	}
	if generic.PowerCycles > 0 {
		pc := generic.PowerCycles
		info.PowerCycleCount = &pc
	}
}

func fillAtaSMART(info *domain.SmartInfo, page *smart.AtaSmartPage) {
	for id, attr := range page.Attrs {
		switch id {
		case ataAttrReallocatedSectors:
			v := attr.ValueRaw
			info.ReallocatedSectors = &v
		case ataAttrPowerOnHours:
			v := attr.ValueRaw
			info.PowerOnHours = &v
		case ataAttrPowerCycleCount:
			v := attr.ValueRaw
			info.PowerCycleCount = &v
		case ataAttrTemperature, ataAttrTemperatureAirflow:
			temp, _, _, _, err := attr.ParseAsTemperature()
			if err == nil && temp >= 0 {
				v := uint64(temp)
				info.TemperatureC = &v
			}
		case ataAttrPendingSectors:
			v := attr.ValueRaw
			info.PendingSectors = &v
		case ataAttrUncorrectable:
			v := attr.ValueRaw
			info.UncorrectableErrors = &v
		case ataAttrStartStopCount:
			v := attr.ValueRaw
			info.StartStopCount = &v
		}
	}
}

func classifyProblem(d domain.Disk) (domain.Health, string, string, string) {
	if d.Smart.ReadError != "" {
		return domain.HealthWarning, "SMART read failure", d.Smart.ReadError, d.Smart.ReadError
	}

	if severeCount(d.Smart.ReallocatedSectors) || severeCount(d.Smart.PendingSectors) || severeCount(d.Smart.UncorrectableErrors) {
		return domain.HealthFailing, "disk failure", smartProblemDetails(d.Smart), problemNote(d.Smart, "critical SMART counters are non-zero")
	}
	if warnedCount(d.Smart.ReallocatedSectors) || warnedCount(d.Smart.PendingSectors) || warnedCount(d.Smart.UncorrectableErrors) {
		return domain.HealthWarning, "health warning", smartProblemDetails(d.Smart), problemNote(d.Smart, "SMART counters indicate potential media issues")
	}
	if d.Usage.ChecksPartial {
		return domain.HealthWarning, "unknown", "usage verification incomplete", "could not fully verify device usage"
	}
	if d.Smart.Available {
		return domain.HealthHealthy, "—", smartProblemDetails(d.Smart), problemNote(d.Smart, "")
	}

	return domain.HealthUnknown, "unknown", "", ""
}

func severeCount(v *uint64) bool {
	return v != nil && *v > 0
}

func warnedCount(v *uint64) bool {
	return v != nil && *v > 0
}

func problemNote(info domain.SmartInfo, base string) string {
	notes := make([]string, 0, 3)
	if base != "" {
		notes = append(notes, base)
	}
	if info.ErrorCount != nil && *info.ErrorCount > 0 {
		notes = append(notes, "SMART error log contains historical entries")
	}
	if info.OverallHealth == domain.SmartctlHealthPassed &&
		(warnedCount(info.ReallocatedSectors) || warnedCount(info.PendingSectors) || warnedCount(info.UncorrectableErrors)) {
		notes = append(notes, "app diagnosis is stricter than smartctl overall-health")
	}

	return strings.Join(notes, "; ")
}

func usageProblemDetails(usage domain.UsageFlags) string {
	parts := make([]string, 0, 4)
	if usage.Mounted {
		parts = append(parts, "mounted")
	}
	if usage.Swap {
		parts = append(parts, "swap active")
	}
	if usage.RAIDMember {
		parts = append(parts, "mdraid holder present")
	}
	if usage.DMHolder {
		parts = append(parts, "device-mapper holder present")
	}

	return strings.Join(parts, "; ")
}

func removalObstacleDetails(usage domain.UsageFlags) string {
	return usageProblemDetails(usage)
}

func smartProblemDetails(info domain.SmartInfo) string {
	parts := make([]string, 0, 4)
	if info.ReallocatedSectors != nil && *info.ReallocatedSectors > 0 {
		parts = append(parts, "reallocated sectors="+strconv.FormatUint(*info.ReallocatedSectors, 10))
	}
	if info.PendingSectors != nil && *info.PendingSectors > 0 {
		parts = append(parts, "pending sectors="+strconv.FormatUint(*info.PendingSectors, 10))
	}
	if info.UncorrectableErrors != nil && *info.UncorrectableErrors > 0 {
		parts = append(parts, "uncorrectable errors="+strconv.FormatUint(*info.UncorrectableErrors, 10))
	}
	if info.ErrorCount != nil && *info.ErrorCount > 0 {
		parts = append(parts, "SMART error log count="+strconv.FormatUint(*info.ErrorCount, 10))
	}

	return strings.Join(parts, "; ")
}

func readText(path string) string {
	if !isTrustedFSPath(path) {
		return ""
	}

	b, err := readFile(path) // #nosec G304 -- path is restricted to trusted procfs/sysfs roots
	if err != nil {
		return ""
	}

	return string(b)
}

func isWritable(path string) bool {
	if !isTrustedFSPath(path) {
		return false
	}

	f, err := openFileWritable(path) // #nosec G304 -- path is restricted to trusted procfs/sysfs roots
	if err != nil {
		return false
	}
	_ = f.Close()

	return true
}

func isTrustedFSPath(path string) bool {
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return false
	}

	return strings.HasPrefix(cleanPath, "/sys/") || strings.HasPrefix(cleanPath, "/proc/")
}

func isTrustedDevicePath(path string) bool {
	cleanPath := filepath.Clean(path)
	if !filepath.IsAbs(cleanPath) {
		return false
	}

	return strings.HasPrefix(cleanPath, "/dev/sd")
}

func dashIfEmpty(v string) string {
	if strings.TrimSpace(v) == "" {
		return "—"
	}

	return v
}
