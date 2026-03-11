package linux

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

type UsageInfo struct {
	Mounted    bool
	Swap       bool
	ChecksPart bool
}

func CollectUsageInfo(baseDevices map[string]struct{}) map[string]UsageInfo {
	out := make(map[string]UsageInfo, len(baseDevices))
	for dev := range baseDevices {
		out[dev] = UsageInfo{}
	}

	if err := collectMountedUsage(out); err != nil {
		for dev, info := range out {
			info.ChecksPart = true
			out[dev] = info
		}
	}

	if err := collectSwapUsage(out); err != nil {
		for dev, info := range out {
			info.ChecksPart = true
			out[dev] = info
		}
	}

	return out
}

func baseBlockDevice(device string) string {
	if !strings.HasPrefix(device, "/dev/") {
		return ""
	}
	base := filepath.Base(device)
	if strings.HasPrefix(base, "sd") {
		return "/dev/" + strings.TrimRightFunc(base, func(r rune) bool {
			return r >= '0' && r <= '9'
		})
	}
	return ""
}

func collectMountedUsage(out map[string]UsageInfo) (err error) {
	mounts, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := mounts.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	s := bufio.NewScanner(mounts)
	for s.Scan() {
		fields := strings.Fields(s.Text())
		if len(fields) < 10 {
			continue
		}
		sep := -1
		for i, f := range fields {
			if f == "-" {
				sep = i
				break
			}
		}
		if sep == -1 || sep+2 >= len(fields) {
			continue
		}
		base := baseBlockDevice(fields[sep+2])
		if base == "" {
			continue
		}
		info := out[base]
		info.Mounted = true
		out[base] = info
	}

	return s.Err()
}

func collectSwapUsage(out map[string]UsageInfo) (err error) {
	swaps, err := os.Open("/proc/swaps")
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := swaps.Close(); err == nil && closeErr != nil {
			err = closeErr
		}
	}()

	s := bufio.NewScanner(swaps)
	first := true
	for s.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.Fields(s.Text())
		if len(fields) < 1 {
			continue
		}
		base := baseBlockDevice(fields[0])
		if base == "" {
			continue
		}
		info := out[base]
		info.Swap = true
		out[base] = info
	}

	return s.Err()
}
