package format

import (
	"fmt"
	"strings"
	"time"
)

// SizeBytes formats a byte size using decimal units.
func SizeBytes(v uint64) string {
	if v == 0 {
		return "—"
	}
	units := []string{"B", "KB", "MB", "GB", "TB", "PB"}
	n := float64(v)
	idx := 0
	for n >= 1000 && idx < len(units)-1 {
		n /= 1000
		idx++
	}
	if idx >= len(units) {
		idx = len(units) - 1
	}
	unit := units[idx]

	if idx == 0 {
		return fmt.Sprintf("%d %s", v, unit)
	}
	if n >= 100 {
		return fmt.Sprintf("%.0f %s", n, unit)
	}

	return fmt.Sprintf("%.1f %s", n, unit)
}

// DurationHours formats a SMART power-on-hours value in compact units.
func DurationHours(hours *uint64) string {
	return durationHours(hours, 0)
}

// DurationHoursCompact formats a SMART power-on-hours value to fit a target width.
func DurationHoursCompact(hours *uint64, width int) string {
	return durationHours(hours, width)
}

func durationHours(hours *uint64, width int) string {
	if hours == nil {
		return "—"
	}
	total := *hours
	if total < 24 {
		return fitDurationParts([]string{fmt.Sprintf("%dh", total)}, width)
	}
	years := total / (24 * 365)
	total %= 24 * 365
	months := total / (24 * 30)
	total %= 24 * 30
	days := total / 24
	hoursOnly := total % 24

	parts := make([]string, 0, 4)
	if years > 0 {
		parts = append(parts, fmt.Sprintf("%dy", years))
	}
	if months > 0 {
		parts = append(parts, fmt.Sprintf("%dm", months))
	}
	if days > 0 {
		parts = append(parts, fmt.Sprintf("%dd", days))
	}
	if hoursOnly > 0 || len(parts) == 0 {
		parts = append(parts, fmt.Sprintf("%dh", hoursOnly))
	}

	return fitDurationParts(parts, width)
}

func fitDurationParts(parts []string, width int) string {
	if len(parts) == 0 {
		return "—"
	}
	if width <= 0 {
		return strings.Join(parts, " ")
	}
	for n := len(parts); n > 0; n-- {
		out := strings.Join(parts[:n], " ")
		if len(out) <= width {
			return out
		}
	}

	return parts[0]
}

// RateBytes formats a byte-per-second rate using the same units as SizeBytes.
func RateBytes(v float64) string {
	return fmt.Sprintf("%s/s", SizeBytes(uint64(v)))
}

// ElapsedShort formats a duration for the read-load status view.
func ElapsedShort(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm %ds", int(d.Minutes()), int(d.Seconds())%60)
	}

	return fmt.Sprintf("%dh %dm %ds", int(d.Hours()), int(d.Minutes())%60, int(d.Seconds())%60)
}
