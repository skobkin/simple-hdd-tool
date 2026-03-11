package linux

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectUsageInfoMarksMountsAndSwap(t *testing.T) {
	dir := t.TempDir()
	mounts := filepath.Join(dir, "mountinfo")
	swaps := filepath.Join(dir, "swaps")
	if err := os.WriteFile(mounts, []byte("31 22 8:1 / /data rw,relatime - ext4 /dev/sda1 rw\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(swaps, []byte("Filename Type Size Used Priority\n/dev/sdb2 partition 1 0 -2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	oldMounts := procMountInfoPath
	oldSwaps := procSwapsPath
	oldOpen := openProcFile
	procMountInfoPath = mounts
	procSwapsPath = swaps
	openProcFile = os.Open
	t.Cleanup(func() {
		procMountInfoPath = oldMounts
		procSwapsPath = oldSwaps
		openProcFile = oldOpen
	})

	got := CollectUsageInfo(map[string]struct{}{
		"/dev/sda": {},
		"/dev/sdb": {},
	})

	if !got["/dev/sda"].Mounted {
		t.Fatalf("/dev/sda Mounted = false, want true")
	}
	if !got["/dev/sdb"].Swap {
		t.Fatalf("/dev/sdb Swap = false, want true")
	}
}

func TestCollectUsageInfoMarksPartialOnProcErrors(t *testing.T) {
	oldOpen := openProcFile
	openProcFile = func(string) (*os.File, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { openProcFile = oldOpen })

	got := CollectUsageInfo(map[string]struct{}{"/dev/sda": {}})
	if !got["/dev/sda"].ChecksPart {
		t.Fatalf("ChecksPart = false, want true")
	}
}
