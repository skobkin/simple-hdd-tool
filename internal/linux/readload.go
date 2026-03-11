package linux

import (
	"context"
	"errors"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
)

type ReadLoader struct {
	devicePath string
	sizeBytes  uint64
	startedAt  time.Time
	directIO   bool
	directNote string

	cancel context.CancelFunc

	mu      sync.RWMutex
	bytes   uint64
	lastErr error
	stopped bool
}

func StartReadLoad(devicePath string, sizeBytes uint64) (*ReadLoader, error) {
	ctx, cancel := context.WithCancel(context.Background())
	loader := &ReadLoader{
		devicePath: devicePath,
		sizeBytes:  sizeBytes,
		startedAt:  time.Now(),
		cancel:     cancel,
	}
	if err := loader.start(ctx); err != nil {
		cancel()
		return nil, err
	}
	return loader, nil
}

func (r *ReadLoader) start(ctx context.Context) error {
	fd, err := unix.Open(r.devicePath, unix.O_RDONLY|unix.O_DIRECT|unix.O_CLOEXEC, 0)
	if err != nil {
		if !errors.Is(err, syscall.EINVAL) && !errors.Is(err, syscall.EPERM) && !errors.Is(err, syscall.EACCES) {
			return err
		}
		fd, err = unix.Open(r.devicePath, unix.O_RDONLY|unix.O_CLOEXEC, 0)
		if err != nil {
			return err
		}
		r.directNote = "direct I/O unavailable; using buffered reads"
	} else {
		r.directIO = true
	}

	go r.run(ctx, fd)
	return nil
}

func (r *ReadLoader) run(ctx context.Context, fd int) {
	defer unix.Close(fd)
	buf := make([]byte, 1024*1024+4096)
	aligned := align4096(buf)
	var offset int64
	for {
		select {
		case <-ctx.Done():
			r.mu.Lock()
			r.stopped = true
			r.mu.Unlock()
			return
		default:
		}

		n, err := unix.Pread(fd, aligned, offset)
		if err != nil {
			if errors.Is(err, syscall.EINVAL) && r.directIO {
				r.mu.Lock()
				r.lastErr = errors.New("direct I/O read failed")
				r.stopped = true
				r.mu.Unlock()
				return
			}
			r.mu.Lock()
			r.lastErr = err
			r.stopped = true
			r.mu.Unlock()
			return
		}
		if n == 0 {
			offset = 0
			continue
		}
		offset += int64(n)
		if r.sizeBytes > 0 && uint64(offset) >= r.sizeBytes {
			offset = 0
		}
		r.mu.Lock()
		r.bytes += uint64(n)
		r.mu.Unlock()
		if !r.directIO {
			_ = unix.Fadvise(fd, offset-int64(n), int64(n), unix.FADV_DONTNEED)
		}
	}
}

func (r *ReadLoader) Stop() {
	r.cancel()
}

func (r *ReadLoader) Snapshot() domain.ReadLoadSnapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	elapsed := time.Since(r.startedAt)
	speed := 0.0
	if elapsed > 0 {
		speed = float64(r.bytes) / elapsed.Seconds()
	}
	return domain.ReadLoadSnapshot{
		DevicePath:      r.devicePath,
		DirectIO:        r.directIO,
		DirectIOMessage: r.directNote,
		Elapsed:         elapsed,
		BytesRead:       r.bytes,
		BytesPerSecond:  speed,
		LastError:       r.lastErr,
	}
}

func align4096(buf []byte) []byte {
	const block = 4096
	base := uintptr(block - 1)
	ptr := uintptr(unsafe.Pointer(&buf[0]))
	offset := int((base - (ptr-1)%block) % block)
	return buf[offset : offset+1024*1024]
}
