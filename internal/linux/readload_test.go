package linux

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

func TestStartReadLoadFallsBackFromDirectIO(t *testing.T) {
	oldOpen := unixOpen
	oldPread := unixPread
	oldClose := unixClose
	oldFadvise := unixFadvise
	oldNow := timeNowRL
	defer func() {
		unixOpen = oldOpen
		unixPread = oldPread
		unixClose = oldClose
		unixFadvise = oldFadvise
		timeNowRL = oldNow
	}()

	openCalls := 0
	unixOpen = func(path string, mode int, perm uint32) (int, error) {
		openCalls++
		if openCalls == 1 {
			return -1, syscall.EINVAL
		}
		return 42, nil
	}
	readCalls := 0
	unixPread = func(fd int, p []byte, off int64) (int, error) {
		readCalls++
		if readCalls == 1 {
			return len(p), nil
		}
		return 0, context.Canceled
	}
	unixFadvise = func(int, int64, int64, int) error { return nil }
	unixClose = func(int) error { return nil }
	timeNowRL = func() time.Time { return time.Unix(0, 0) }

	loader, err := StartReadLoad("/dev/sda", 1024)
	if err != nil {
		t.Fatalf("StartReadLoad() error = %v", err)
	}
	loader.Stop()
	time.Sleep(10 * time.Millisecond)
	snap := loader.Snapshot()
	if snap.DirectIO {
		t.Fatalf("Snapshot().DirectIO = true, want false")
	}
	if snap.DirectIOMessage == "" {
		t.Fatalf("Snapshot().DirectIOMessage = empty")
	}
}

func TestStartReadLoadReturnsOpenError(t *testing.T) {
	oldOpen := unixOpen
	unixOpen = func(string, int, uint32) (int, error) { return -1, errors.New("boom") }
	t.Cleanup(func() { unixOpen = oldOpen })

	_, err := StartReadLoad("/dev/sda", 1)
	if err == nil || err.Error() != "boom" {
		t.Fatalf("StartReadLoad() error = %v", err)
	}
}

func TestReadLoaderSnapshotAlignAndFail(t *testing.T) {
	now := time.Now()
	loader := &ReadLoader{
		devicePath: "/dev/sda",
		startedAt:  now.Add(-2 * time.Second),
		bytes:      2048,
		directIO:   true,
		directNote: "direct",
	}

	oldSince := timeSince
	timeSince = func(time.Time) time.Duration { return 2 * time.Second }
	t.Cleanup(func() { timeSince = oldSince })

	snap := loader.Snapshot()
	if snap.DevicePath != "/dev/sda" || snap.BytesRead != 2048 || snap.BytesPerSecond != 1024 {
		t.Fatalf("Snapshot() = %+v", snap)
	}
	buf := make([]byte, 1024*1024+4096)
	aligned := align4096(buf)
	if len(aligned) != 1024*1024 {
		t.Fatalf("len(align4096()) = %d", len(aligned))
	}
	if uintptr(len(aligned)) == 0 {
		t.Fatalf("unexpected zero length alignment")
	}

	loader.fail(errors.New("first"))
	loader.fail(errors.New("second"))
	if loader.lastErr == nil || loader.lastErr.Error() != "first" {
		t.Fatalf("lastErr = %v", loader.lastErr)
	}
}

func TestReadLoaderRunHandlesErrorBranches(t *testing.T) {
	oldPread := unixPread
	oldClose := unixClose
	oldFadvise := unixFadvise
	defer func() {
		unixPread = oldPread
		unixClose = oldClose
		unixFadvise = oldFadvise
	}()

	t.Run("direct io read failure", func(t *testing.T) {
		loader := &ReadLoader{directIO: true}
		unixPread = func(int, []byte, int64) (int, error) { return 0, syscall.EINVAL }
		unixClose = func(int) error { return nil }
		unixFadvise = func(int, int64, int64, int) error { return nil }

		loader.run(context.Background(), 1)
		if loader.lastErr == nil || loader.lastErr.Error() != "direct I/O read failed" {
			t.Fatalf("lastErr = %v", loader.lastErr)
		}
	})

	t.Run("buffered fadvise fatal error", func(t *testing.T) {
		loader := &ReadLoader{}
		reads := 0
		unixPread = func(int, []byte, int64) (int, error) {
			reads++
			if reads == 1 {
				return 4096, nil
			}
			return 0, context.Canceled
		}
		unixFadvise = func(int, int64, int64, int) error { return syscall.EIO }
		unixClose = func(int) error { return nil }

		loader.run(context.Background(), 1)
		if !errors.Is(loader.lastErr, syscall.EIO) {
			t.Fatalf("lastErr = %v", loader.lastErr)
		}
	})

	t.Run("negative read count", func(t *testing.T) {
		loader := &ReadLoader{}
		unixPread = func(int, []byte, int64) (int, error) { return -1, nil }
		unixFadvise = func(int, int64, int64, int) error { return nil }
		unixClose = func(int) error { return nil }

		loader.run(context.Background(), 1)
		if loader.lastErr == nil || (loader.lastErr.Error() != "pread returned a negative byte count" && loader.lastErr.Error() != "direct I/O read failed") {
			t.Fatalf("lastErr = %v", loader.lastErr)
		}
	})

	t.Run("close error surfaces", func(t *testing.T) {
		loader := &ReadLoader{}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		unixPread = func(int, []byte, int64) (int, error) { return 0, nil }
		unixFadvise = func(int, int64, int64, int) error { return nil }
		unixClose = func(int) error { return syscall.EIO }

		loader.run(ctx, 1)
		if !errors.Is(loader.lastErr, syscall.EIO) {
			t.Fatalf("lastErr = %v", loader.lastErr)
		}
	})

	t.Run("ignored fadvise error", func(t *testing.T) {
		loader := &ReadLoader{}
		ctx, cancel := context.WithCancel(context.Background())
		calls := 0
		unixPread = func(int, []byte, int64) (int, error) {
			calls++
			if calls == 1 {
				cancel()
				return 4096, nil
			}
			return 0, nil
		}
		unixFadvise = func(int, int64, int64, int) error { return unix.ENOSYS }
		unixClose = func(int) error { return nil }

		loader.run(ctx, 1)
		if loader.lastErr != nil {
			t.Fatalf("lastErr = %v, want nil", loader.lastErr)
		}
	})
}
