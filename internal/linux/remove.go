package linux

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

// Remover removes a disk from the kernel after safety checks.
type Remover struct{}

// Remove requests kernel-side device removal and waits for the device nodes to disappear.
func (Remover) Remove(ctx context.Context, disk domain.Disk, force bool, progress chan<- domain.RemovalProgress) domain.RemovalResult {
	progress <- domain.RemovalProgress{Step: "Checking mount status"}
	if !force {
		if disk.Usage.Mounted || disk.Usage.Swap {
			return domain.RemovalResult{Err: errors.New("device is mounted or used as swap")}
		}
		if disk.Usage.RAIDMember || disk.Usage.DMHolder {
			return domain.RemovalResult{Err: errors.New("device has RAID or device-mapper holders")}
		}
	}

	deletePath := filepath.Join(disk.SysfsPath, "device/delete")
	if _, err := os.Stat(deletePath); err != nil {
		return domain.RemovalResult{Err: errors.New("kernel delete path is unavailable")}
	}

	progress <- domain.RemovalProgress{Step: "Removing " + disk.DevicePath + " from kernel"}
	if err := os.WriteFile(deletePath, []byte("1"), 0); err != nil {
		return domain.RemovalResult{Err: err}
	}

	progress <- domain.RemovalProgress{Step: "Verifying removal"}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return domain.RemovalResult{Err: ctx.Err()}
		default:
		}
		_, sysErr := os.Stat(disk.SysfsPath)
		_, devErr := os.Stat(disk.DevicePath)
		if os.IsNotExist(sysErr) && os.IsNotExist(devErr) {
			return domain.RemovalResult{Success: true}
		}
		time.Sleep(200 * time.Millisecond)
	}

	return domain.RemovalResult{Err: errors.New("removal verification timed out")}
}
