package domain

import "time"

type GroupMode string

const (
	GroupByModel  GroupMode = "model"
	GroupBySize   GroupMode = "size"
	GroupByFamily GroupMode = "family"
	GroupByNone   GroupMode = "none"
)

func (m GroupMode) Next() GroupMode {
	switch m {
	case GroupByModel:
		return GroupBySize
	case GroupBySize:
		return GroupByFamily
	case GroupByFamily:
		return GroupByNone
	default:
		return GroupByModel
	}
}

type SortMode string

const (
	SortBySize   SortMode = "size"
	SortBySerial SortMode = "serial"
	SortByHours  SortMode = "hours"
)

func (m SortMode) Next() SortMode {
	switch m {
	case SortBySize:
		return SortBySerial
	case SortBySerial:
		return SortByHours
	default:
		return SortBySize
	}
}

type Health string

const (
	HealthHealthy Health = "healthy"
	HealthWarning Health = "warning"
	HealthFailing Health = "failing"
	HealthUnknown Health = "unknown"
)

type Capabilities struct {
	CanReadSMART bool
	CanReadLoad  bool
	CanRemove    bool
}

type UsageFlags struct {
	Mounted       bool
	Swap          bool
	RAIDMember    bool
	DMHolder      bool
	ChecksPartial bool
}

type SmartInfo struct {
	Available           bool
	ReadError           string
	PowerOnHours        *uint64
	TemperatureC        *uint64
	ReallocatedSectors  *uint64
	PendingSectors      *uint64
	UncorrectableErrors *uint64
	StartStopCount      *uint64
	PowerCycleCount     *uint64
	ErrorCount          *uint64
}

type Disk struct {
	ID          string
	Name        string
	DevicePath  string
	SysfsPath   string
	Family      string
	Model       string
	Serial      string
	SizeBytes   uint64
	Transport   string
	BlockTime   time.Duration
	Problem     string
	ProblemNote string
	Health      Health
	Warnings    []string
	Smart       SmartInfo
	Usage       UsageFlags
	Caps        Capabilities
}

type ScanProgress struct {
	Current int
	Total   int
	Text    string
}

type ScanResult struct {
	Disks    []Disk
	ReadOnly bool
	Err      error
}

type RemovalProgress struct {
	Step string
}

type RemovalResult struct {
	Success bool
	Err     error
}

type ReadLoadSnapshot struct {
	DevicePath      string
	DirectIO        bool
	DirectIOMessage string
	Elapsed         time.Duration
	BytesRead       uint64
	BytesPerSecond  float64
	LastError       error
}
