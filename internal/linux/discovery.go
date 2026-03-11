package linux

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	smart "github.com/anatol/smart.go"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

type Scanner struct {
	PerDiskTimeout time.Duration
}

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
	readOnly := os.Geteuid() != 0
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
	entries, err := os.ReadDir("/sys/block")
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "sd") {
			continue
		}
		deviceType := strings.TrimSpace(readText(filepath.Join("/sys/block", name, "device/type")))
		if deviceType != "" && deviceType != "0" {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

func (s Scanner) scanDisk(ctx context.Context, name string, usage UsageInfo) domain.Disk {
	sysfsPath := filepath.Join("/sys/block", name)
	devicePath := "/dev/" + name
	sizeSectors, _ := strconv.ParseUint(strings.TrimSpace(readText(filepath.Join(sysfsPath, "size"))), 10, 64)
	sizeBytes := sizeSectors * 512

	disk := domain.Disk{
		ID:         name,
		Name:       name,
		DevicePath: devicePath,
		SysfsPath:  sysfsPath,
		Vendor:     dashIfEmpty(strings.TrimSpace(readText(filepath.Join(sysfsPath, "device/vendor")))),
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
			CanReadLoad: os.Geteuid() == 0,
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
		if ident.Vendor != "" {
			disk.Vendor = ident.Vendor
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

	disk.Health, disk.Problem, disk.ProblemNote = classifyProblem(disk)
	return disk
}

type holderState struct {
	RAIDMember bool
	DMHolder   bool
}

func inspectHolders(name string) (holderState, bool) {
	var out holderState
	entries, err := os.ReadDir(filepath.Join("/sys/block", name, "holders"))
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
	slaves, err := filepath.Glob(filepath.Join("/sys/block/md*", "slaves", name))
	if err == nil && len(slaves) > 0 {
		out.RAIDMember = true
	}
	return out, false
}

func detectTransport(name string) string {
	sysfsPath := filepath.Join("/sys/block", name)
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

	resolved, err := filepath.EvalSymlinks(filepath.Join(sysfsPath, "device"))
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
	Vendor    string
	Model     string
	Serial    string
	SizeBytes uint64
}

func scanSMART(ctx context.Context, devicePath string, timeout time.Duration) (domain.SmartInfo, identity, error) {
	type result struct {
		smart domain.SmartInfo
		id    identity
		err   error
	}
	done := make(chan result, 1)
	go func() {
		sm, id, err := scanSMARTSync(devicePath)
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

func scanSMARTSync(devicePath string) (domain.SmartInfo, identity, error) {
	dev, err := smart.Open(devicePath)
	if err != nil {
		return domain.SmartInfo{}, identity{}, err
	}
	defer dev.Close()

	info := domain.SmartInfo{Available: true}
	id := identity{}
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
			id.Vendor = strings.TrimSpace(string(bytes.TrimSpace(inquiry.VendorIdent[:])))
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
		case 5:
			v := attr.ValueRaw
			info.ReallocatedSectors = &v
		case 9:
			v := attr.ValueRaw
			info.PowerOnHours = &v
		case 12:
			v := attr.ValueRaw
			info.PowerCycleCount = &v
		case 194, 190:
			temp, _, _, _, err := attr.ParseAsTemperature()
			if err == nil {
				v := uint64(temp)
				info.TemperatureC = &v
			}
		case 197:
			v := attr.ValueRaw
			info.PendingSectors = &v
		case 198:
			v := attr.ValueRaw
			info.UncorrectableErrors = &v
		case 4:
			v := attr.ValueRaw
			info.StartStopCount = &v
		}
	}
}

func classifyProblem(d domain.Disk) (domain.Health, string, string) {
	if d.Smart.ReadError != "" {
		return domain.HealthWarning, "SMART read failure", d.Smart.ReadError
	}
	if d.Usage.Mounted || d.Usage.Swap {
		return domain.HealthWarning, "mounted", "device is in active use"
	}
	if d.Usage.RAIDMember || d.Usage.DMHolder {
		return domain.HealthWarning, "raid member", "device has holders and may be in mdraid or device-mapper"
	}

	if severeCount(d.Smart.ReallocatedSectors) || severeCount(d.Smart.PendingSectors) || severeCount(d.Smart.UncorrectableErrors) {
		return domain.HealthFailing, "disk failure", "critical SMART counters are non-zero"
	}
	if warnedCount(d.Smart.ReallocatedSectors) || warnedCount(d.Smart.PendingSectors) || warnedCount(d.Smart.UncorrectableErrors) || warnedCount(d.Smart.ErrorCount) {
		return domain.HealthWarning, "health warning", "SMART counters indicate potential media issues"
	}
	if d.Usage.ChecksPartial {
		return domain.HealthWarning, "unknown", "could not fully verify device usage"
	}
	if d.Smart.Available {
		return domain.HealthHealthy, "—", ""
	}
	return domain.HealthUnknown, "unknown", ""
}

func severeCount(v *uint64) bool {
	return v != nil && *v > 0
}

func warnedCount(v *uint64) bool {
	return v != nil && *v > 0
}

func readText(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

func isWritable(path string) bool {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return false
	}
	_ = f.Close()
	return true
}

func dashIfEmpty(v string) string {
	if strings.TrimSpace(v) == "" {
		return "—"
	}
	return v
}
