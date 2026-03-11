package domain

import "time"

// GroupMode controls how disks are grouped in the table view.
type GroupMode string

const (
	// GroupByModel groups disks by model string.
	GroupByModel GroupMode = "model"
	// GroupBySize groups disks by formatted size.
	GroupBySize GroupMode = "size"
	// GroupByFamily groups disks by SMART model family.
	GroupByFamily GroupMode = "family"
	// GroupByNone disables grouping.
	GroupByNone GroupMode = "none"
)

// Next returns the next grouping mode in the UI cycle.
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

// SortMode controls disk sorting in the table view.
type SortMode string

const (
	// SortBySize sorts disks by capacity.
	SortBySize SortMode = "size"
	// SortBySerial sorts disks by serial number.
	SortBySerial SortMode = "serial"
	// SortByHours sorts disks by SMART power-on hours.
	SortByHours SortMode = "hours"
)

// Next returns the next sort mode in the UI cycle.
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

// Health summarizes the disk health classification.
type Health string

const (
	// HealthHealthy marks a disk without known issues.
	HealthHealthy Health = "healthy"
	// HealthWarning marks a disk with usage or SMART warnings.
	HealthWarning Health = "warning"
	// HealthFailing marks a disk with failing SMART indicators.
	HealthFailing Health = "failing"
	// HealthUnknown marks a disk whose health could not be determined.
	HealthUnknown Health = "unknown"
)

// Capabilities describes operations supported for a disk in the current environment.
type Capabilities struct {
	CanReadSMART bool
	CanReadLoad  bool
	CanRemove    bool
}

// UsageFlags describes whether a disk is in active system use.
type UsageFlags struct {
	Mounted       bool
	Swap          bool
	RAIDMember    bool
	DMHolder      bool
	ChecksPartial bool
}

// SmartInfo contains SMART-derived metrics and collection status.
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

// Disk describes one discovered block device.
type Disk struct {
	ID              string
	Name            string
	DevicePath      string
	SysfsPath       string
	Family          string
	Model           string
	Serial          string
	SizeBytes       uint64
	Transport       string
	BlockTime       time.Duration
	Problem         string
	ProblemDetails  string
	ProblemNote     string
	RemovalObstacle string
	Health          Health
	Warnings        []string
	Smart           SmartInfo
	Usage           UsageFlags
	Caps            Capabilities
}

// ScanProgress reports incremental scanner progress.
type ScanProgress struct {
	Current int
	Total   int
	Text    string
}

// ScanResult returns the discovered disks and scan status.
type ScanResult struct {
	Disks    []Disk
	ReadOnly bool
	Err      error
}

// RemovalProgress reports the current remove step.
type RemovalProgress struct {
	Step string
}

// RemovalResult reports the outcome of a remove request.
type RemovalResult struct {
	Success bool
	Err     error
}

// ReadLoadSnapshot reports read-load activity for a disk.
type ReadLoadSnapshot struct {
	DevicePath      string
	DirectIO        bool
	DirectIOMessage string
	Elapsed         time.Duration
	BytesRead       uint64
	BytesPerSecond  float64
	LastError       error
}
