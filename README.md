# Simple HDD tool [![status-badge](https://ci.skobk.in/api/badges/8/status.svg?events=push%2Ctag)](https://ci.skobk.in/repos/8) [![latest release](https://git.skobk.in/skobkin/simple-hdd-tool/badges/release.svg)](https://git.skobk.in/skobkin/simple-hdd-tool/releases)

Interactive terminal UI for inspecting Linux SATA/SAS disks, checking SMART-derived health signals, generating sustained read load to identify the physical drive by its activity LED, and removing devices from the kernel after safety checks.

## How it looks

### Disk list
```
2 disks found
g Group:model  s Sort:size  r Refresh  Enter Details  q Quit  Ctrl+C Quit

size      model              family     serial         block dev  time         health problems
HGST HUS728T8TALE6L4 (1)
8.0 TB    HGST HUS728T8TALE6 HGST Ultra XXXXXXXX       /dev/sdb   4y 5m 15d 15 ●      mounted
ST8000AS0002-1NA17Z (1)
8.0 TB    ST8000AS0002-1NA17 Seagate Ar YYYYYYYY       /dev/sda   4y 11m 6d 14 ●      health warning
```

### Disk details
```
┌──────────────────────────────────────────────────────┐
│ Disk Details                                         │
│                                                      │
│ Family: Seagate Archive HDD (SMR)                    │
│ Model: ST8000AS0002-1NA17Z                           │
│ Size: 8.0 TB                                         │
│ Serial: YYYYYYYY                                     │
│ Block device: /dev/sda                               │
│                                                      │
│ Health                                               │
│ Time: 4y 11m 6d 14h                                  │
│ smartctl health: passed                              │
│ App diagnosis: warning                               │
│ Problem: health warning                              │
│ Problem details: pending sectors=2                   │
│ Note: SMART counters indicate potential media issues │
│ Note: app diagnosis is stricter than smartctl overal │
│ l-health                                             │
│                                                      │
│ SMART                                                │
│ Temperature: 29 C                                    │
│ Reallocated sectors: 0                               │
│ Pending sectors: 2                                   │
│ Reported uncorrectable errors: 0                     │
│ Start/stop count: 12798                              │
│ Power cycle count: 3380                              │
│                                                      │
│  Close  Read load  Remove                            │
│ Left/Right or Tab select  Enter activate  Esc back   │
└──────────────────────────────────────────────────────┘
```

### Disk load generation
```
┌────────────────────┐                                                                                                                                                                                                      [0/60]
│ Read Load          │
│                    │
│ Device: /dev/sda   │
│ Elapsed: 12s       │
│ Speed: 201 MB/s    │
│ Total read: 2.4 GB │
│                    │
│  Stop              │
└────────────────────┘
```

## Features

- Linux TUI built with Bubble Tea.
- Scans `/sys/block` for supported `/dev/sdX` disks and skips USB/unsupported transports.
- Reads disk identity, capacity, uptime, temperature, and selected SMART counters.
- Shows `smartctl` overall SMART pass/fail separately from the app's own diagnosis.
- Classifies disks as `healthy`, `warning`, `failing`, or `unknown`.
- Highlights risky states such as mounted disks, active swap, mdraid holders, and device-mapper holders.
- Groups the disk list by model, size, family, or not at all.
- Sorts disks by size, serial number, or SMART power-on hours.
- Shows per-disk details, including SMART counter breakdown and safety notes.
- Can generate continuous read load on a selected disk, mainly to help locate its tray in a server by watching the activity LED.
- Can request kernel-side device removal through sysfs.
- Falls back to read-only mode when write-required operations are unavailable.
- Supports `--group-by`, `--sort-by`, `--force-remove`, `--no-color`, `--help`, and `--version`.

## Notes

- Linux only.
- The scan targets `/dev/sdX` block devices. NVMe, loop, and USB disks are not part of the current scope.
- Running as root is required for full functionality. Without it, read-load and remove actions are unavailable.
- `smartctl` is optional, but when present it is used to improve disk family detection and to show overall SMART pass/fail status.

## Build

```shell
# regular build
mkdir -p build && go build -o ./build/hddtool ./cmd/simple-hdd-tool

# stripped static build
mkdir -p build && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X github.com/skobkin/simple-hdd-tool/internal/buildinfo.Version=dev' -o ./build/hddtool-static ./cmd/simple-hdd-tool

# stripped static build with explicit version
mkdir -p build && CGO_ENABLED=0 go build -trimpath -ldflags='-s -w -X github.com/skobkin/simple-hdd-tool/internal/buildinfo.Version=v0.1.0' -o ./build/hddtool-static ./cmd/simple-hdd-tool
```
