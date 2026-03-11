package linux

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

func TestRemoverRemoveGuardsAndSuccess(t *testing.T) {
	t.Run("rejects mounted unless forced", func(t *testing.T) {
		progress := make(chan domain.RemovalProgress, 4)
		res := (Remover{}).Remove(context.Background(), domain.Disk{
			Usage: domain.UsageFlags{Mounted: true},
		}, false, progress)
		if res.Err == nil || res.Err.Error() != "device is mounted or used as swap" {
			t.Fatalf("Remove() error = %v", res.Err)
		}
	})

	t.Run("rejects holders unless forced", func(t *testing.T) {
		progress := make(chan domain.RemovalProgress, 4)
		res := (Remover{}).Remove(context.Background(), domain.Disk{
			Usage: domain.UsageFlags{RAIDMember: true},
		}, false, progress)
		if res.Err == nil || res.Err.Error() != "device has RAID or device-mapper holders" {
			t.Fatalf("Remove() error = %v", res.Err)
		}
	})

	t.Run("success", func(t *testing.T) {
		oldStat := statPath
		oldWrite := writeFile
		oldNow := timeNow
		oldSleep := sleep
		oldTimeout := removeTimout
		defer func() {
			statPath = oldStat
			writeFile = oldWrite
			timeNow = oldNow
			sleep = oldSleep
			removeTimout = oldTimeout
		}()

		var stats int
		statPath = func(path string) (os.FileInfo, error) {
			stats++
			if stats <= 3 {
				return fakeFileInfo("exists"), nil
			}
			return nil, os.ErrNotExist
		}
		writeFile = func(string, []byte, os.FileMode) error { return nil }
		now := time.Unix(0, 0)
		timeNow = func() time.Time {
			now = now.Add(100 * time.Millisecond)
			return now
		}
		sleep = func(time.Duration) {}
		removeTimout = time.Second

		progress := make(chan domain.RemovalProgress, 8)
		res := (Remover{}).Remove(context.Background(), domain.Disk{
			SysfsPath:  "/sys/block/sda",
			DevicePath: "/dev/sda",
			Usage:      domain.UsageFlags{Mounted: true},
		}, true, progress)
		if res.Err != nil || !res.Success {
			t.Fatalf("Remove() = %+v", res)
		}
	})
}

func TestRemoverRemoveErrorBranches(t *testing.T) {
	t.Run("missing delete path", func(t *testing.T) {
		oldStat := statPath
		statPath = func(string) (os.FileInfo, error) { return nil, os.ErrNotExist }
		t.Cleanup(func() { statPath = oldStat })

		res := (Remover{}).Remove(context.Background(), domain.Disk{
			SysfsPath:  "/sys/block/sda",
			DevicePath: "/dev/sda",
		}, true, make(chan domain.RemovalProgress, 4))
		if res.Err == nil || res.Err.Error() != "kernel delete path is unavailable" {
			t.Fatalf("Remove() error = %v", res.Err)
		}
	})

	t.Run("write failure", func(t *testing.T) {
		oldStat := statPath
		oldWrite := writeFile
		statPath = func(string) (os.FileInfo, error) { return fakeFileInfo("exists"), nil }
		writeFile = func(string, []byte, os.FileMode) error { return errors.New("denied") }
		t.Cleanup(func() {
			statPath = oldStat
			writeFile = oldWrite
		})

		res := (Remover{}).Remove(context.Background(), domain.Disk{
			SysfsPath:  "/sys/block/sda",
			DevicePath: "/dev/sda",
		}, true, make(chan domain.RemovalProgress, 4))
		if res.Err == nil || res.Err.Error() != "denied" {
			t.Fatalf("Remove() error = %v", res.Err)
		}
	})

	t.Run("context cancel during verify", func(t *testing.T) {
		oldStat := statPath
		oldWrite := writeFile
		oldNow := timeNow
		oldSleep := sleep
		oldTimeout := removeTimout
		defer func() {
			statPath = oldStat
			writeFile = oldWrite
			timeNow = oldNow
			sleep = oldSleep
			removeTimout = oldTimeout
		}()

		statPath = func(string) (os.FileInfo, error) { return fakeFileInfo("exists"), nil }
		writeFile = func(string, []byte, os.FileMode) error { return nil }
		now := time.Unix(0, 0)
		timeNow = func() time.Time {
			now = now.Add(100 * time.Millisecond)
			return now
		}
		sleep = func(time.Duration) {}
		removeTimout = time.Second

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		res := (Remover{}).Remove(ctx, domain.Disk{
			SysfsPath:  "/sys/block/sda",
			DevicePath: "/dev/sda",
		}, true, make(chan domain.RemovalProgress, 4))
		if !errors.Is(res.Err, context.Canceled) {
			t.Fatalf("Remove() error = %v, want canceled", res.Err)
		}
	})

	t.Run("timeout", func(t *testing.T) {
		oldStat := statPath
		oldWrite := writeFile
		oldNow := timeNow
		oldSleep := sleep
		oldTimeout := removeTimout
		defer func() {
			statPath = oldStat
			writeFile = oldWrite
			timeNow = oldNow
			sleep = oldSleep
			removeTimout = oldTimeout
		}()

		statPath = func(string) (os.FileInfo, error) { return fakeFileInfo("exists"), nil }
		writeFile = func(string, []byte, os.FileMode) error { return nil }
		now := time.Unix(0, 0)
		timeNow = func() time.Time {
			now = now.Add(2 * time.Second)
			return now
		}
		sleep = func(time.Duration) {}
		removeTimout = time.Second

		res := (Remover{}).Remove(context.Background(), domain.Disk{
			SysfsPath:  "/sys/block/sda",
			DevicePath: "/dev/sda",
		}, true, make(chan domain.RemovalProgress, 4))
		if res.Err == nil || res.Err.Error() != "removal verification timed out" {
			t.Fatalf("Remove() error = %v", res.Err)
		}
	})
}

type fakeFileInfo string

func (f fakeFileInfo) Name() string       { return string(f) }
func (f fakeFileInfo) Size() int64        { return 0 }
func (f fakeFileInfo) Mode() os.FileMode  { return 0 }
func (f fakeFileInfo) ModTime() time.Time { return time.Time{} }
func (f fakeFileInfo) IsDir() bool        { return false }
func (f fakeFileInfo) Sys() any           { return nil }
