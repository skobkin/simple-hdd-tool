package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/skobkin/simple-hdd-tool/internal/buildinfo"
	"github.com/skobkin/simple-hdd-tool/internal/domain"
	"github.com/skobkin/simple-hdd-tool/internal/format"
)

// View renders the current Bubble Tea screen.
func (m *Model) View() string {
	switch m.mode {
	case viewScanning:
		return m.renderScan()
	case viewDetails:
		return m.renderDetails()
	case viewConfirmRemove:
		return m.wrapModal("Remove Disk", []string{
			"Remove " + m.selectedDisk().DevicePath + " from the kernel?",
			"Default answer is No. Press y to confirm, n or Esc to cancel.",
		})
	case viewRemoving:
		return m.wrapModal("Removing", []string{m.removeProgress})
	case viewInfo:
		return m.wrapModal("Status", []string{m.infoText, "Press Enter, Esc, or q to close."})
	case viewReadLoad:
		return m.renderReadLoad()
	default:
		return m.renderTable()
	}
}

func (m *Model) renderScan() string {
	lines := []string{
		m.styles.header.Render(buildinfo.AppName),
		"",
		"Scanning SATA/SAS disks",
		progressBar(m.scanProgress.Current, m.scanProgress.Total, maxInt(24, m.width-10)),
	}
	if m.scanProgress.Text != "" {
		lines = append(lines, m.scanProgress.Text)
	}
	lines = append(lines, "", "q Quit  Ctrl+C Quit")

	return strings.Join(lines, "\n")
}

func (m *Model) renderTable() string {
	lines := []string{
		m.styles.header.Render(fmt.Sprintf("%d disks found", len(m.disks))),
	}
	if m.readOnly {
		lines = append(lines, m.styles.banner.Render("READ-ONLY MODE: remove is disabled; write-required operations are unavailable"))
	}
	lines = append(lines, fmt.Sprintf("g Group:%s  s Sort:%s  r Refresh  Enter Details  q Quit  Ctrl+C Quit", m.cfg.GroupBy, m.cfg.SortBy))
	lines = append(lines, "")
	lines = append(lines, m.renderColumns())
	for idx, row := range m.rows {
		var text string
		if row.HasDisk {
			text = m.renderDiskRow(row.Disk)
		} else {
			text = m.styles.header.Render(row.Header)
		}
		if idx == m.selected {
			text = m.styles.selected.Render(text)
		}
		lines = append(lines, text)
	}
	if len(m.rows) == 0 {
		lines = append(lines, "No supported SATA/SAS /dev/sdX disks found.")
	}

	return strings.Join(lines, "\n")
}

func (m *Model) renderColumns() string {
	return fmt.Sprintf("%-9s %-18s %-10s %-14s %-10s %-13s %s",
		"size", "model", "family", "serial", "dev", "time", "problems")
}

func (m *Model) renderDiskRow(d domain.Disk) string {
	return fmt.Sprintf("%-9s %-18s %-10s %-14s %-10s %-13s %s",
		trunc(format.SizeBytes(d.SizeBytes), 9),
		trunc(d.Model, 18),
		trunc(d.Family, 10),
		trunc(d.Serial, 14),
		trunc(d.DevicePath, 10),
		format.DurationHoursCompact(d.Smart.PowerOnHours, 13),
		trunc(d.Problem, maxInt(8, m.width-80)),
	)
}

func (m *Model) renderDetails() string {
	disk := m.selectedDisk()
	if disk == nil {
		return m.renderTable()
	}
	health := string(disk.Health)
	switch disk.Health {
	case domain.HealthHealthy:
		health = m.styles.good.Render(health)
	case domain.HealthWarning:
		health = m.styles.warn.Render(health)
	case domain.HealthFailing:
		health = m.styles.bad.Render(health)
	}
	lines := []string{m.styles.header.Render("Disk Details")}

	sections := []detailSection{
		{
			Title: "Identity",
			Lines: []string{
				"Family: " + disk.Family,
				"Model: " + disk.Model,
				"Size: " + format.SizeBytes(disk.SizeBytes),
				"Serial: " + disk.Serial,
				"Block device: " + disk.DevicePath,
			},
		},
		{
			Title: "Health",
			Lines: []string{
				"Time: " + format.DurationHours(disk.Smart.PowerOnHours),
				"Health: " + health,
				"Problem: " + fallback(disk.Problem),
			},
		},
		{
			Title: "SMART",
			Lines: []string{
				metricLine("Temperature", disk.Smart.TemperatureC, " C"),
				metricLine("Reallocated sectors", disk.Smart.ReallocatedSectors, ""),
				metricLine("Pending sectors", disk.Smart.PendingSectors, ""),
				metricLine("Reported uncorrectable errors", disk.Smart.UncorrectableErrors, ""),
				metricLine("Start/stop count", disk.Smart.StartStopCount, ""),
				metricLine("Power cycle count", disk.Smart.PowerCycleCount, ""),
			},
		},
		{
			Title: "Removal / Usage",
			Lines: []string{
				optionalLine("Removal obstacle", disk.RemovalObstacle),
			},
		},
	}

	if disk.ProblemDetails != "" {
		sections[1].Lines = append(sections[1].Lines, "Problem details: "+disk.ProblemDetails)
	}
	if disk.ProblemNote != "" {
		sections[1].Lines = append(sections[1].Lines, "Note: "+disk.ProblemNote)
	}
	if disk.Usage.ChecksPartial {
		sections[3].Lines = append(sections[3].Lines, "Warning: Could not fully verify device usage; proceed carefully")
	}

	for _, section := range sections {
		rendered := m.renderDetailSection(section)
		if len(rendered) == 0 {
			continue
		}
		lines = append(lines, "")
		lines = append(lines, rendered...)
	}

	lines = append(lines, "", m.renderDetailActions(disk), "Left/Right or Tab select  Enter activate  Esc back")

	return m.styles.box.Render(strings.Join(lines, "\n"))
}

type detailSection struct {
	Title string
	Lines []string
}

func (m *Model) renderDetailSection(section detailSection) []string {
	lines := compactLines(section.Lines)
	if len(lines) == 0 {
		return nil
	}

	return append([]string{m.styles.header.Render(section.Title)}, lines...)
}

func compactLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		out = append(out, line)
	}

	return out
}

func optionalLine(label, value string) string {
	if value == "" {
		return ""
	}

	return label + ": " + value
}

func metricLine(label string, value *uint64, suffix string) string {
	if value == nil {
		return ""
	}

	return fmt.Sprintf("%s: %d%s", label, *value, suffix)
}

func (m *Model) renderReadLoad() string {
	snap := m.readLoader.Snapshot()
	lines := []string{
		"Device: " + snap.DevicePath,
		"Elapsed: " + format.ElapsedShort(snap.Elapsed),
		"Speed: " + format.RateBytes(snap.BytesPerSecond),
		"Total read: " + format.SizeBytes(snap.BytesRead),
	}
	if snap.DirectIOMessage != "" {
		lines = append(lines, "Note: "+snap.DirectIOMessage)
	}
	lines = append(lines, "", m.styles.focused.Render("Stop"))

	return m.wrapModal("Read Load", lines)
}

func (m *Model) wrapModal(title string, lines []string) string {
	return m.styles.box.Render(m.styles.header.Render(title) + "\n\n" + strings.Join(lines, "\n"))
}

func (m *Model) renderDetailActions(disk *domain.Disk) string {
	actions := []string{
		m.renderDetailButton(detailActionClose, "Close", true, false),
		m.renderDetailButton(detailActionReadLoad, "Read load", disk.Caps.CanReadLoad, false),
		m.renderDetailButton(detailActionRemove, "Remove", !m.readOnly && disk.Caps.CanRemove, m.cfg.ForceRemove),
	}

	return lipgloss.JoinHorizontal(lipgloss.Top, actions...)
}

func (m *Model) renderDetailButton(action detailAction, label string, enabled bool, danger bool) string {
	if !enabled {
		label += " disabled"
	}

	style := m.styles.button
	if danger {
		style = m.styles.danger
	}
	if !enabled {
		style = m.styles.disabled
	}
	if m.detailAction == action {
		style = m.styles.focused
	}

	return style.Render(label)
}

func progressBar(current, total, width int) string {
	if width < 10 {
		width = 10
	}
	if total <= 0 {
		return "[ scanning ]"
	}
	if current > total {
		current = total
	}
	filled := width * current / total

	return "[" + strings.Repeat("=", filled) + strings.Repeat(" ", width-filled) + fmt.Sprintf("] %d/%d", current, total)
}

func trunc(v string, width int) string {
	if width <= 1 {
		return ""
	}
	v = strings.Join(strings.Fields(v), " ")
	if runewidth.StringWidth(v) <= width {
		return v
	}

	var b strings.Builder
	currentWidth := 0
	for _, r := range v {
		rw := runewidth.RuneWidth(r)
		if currentWidth+rw > width {
			break
		}
		b.WriteRune(r)
		currentWidth += rw
	}

	return b.String()
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}

	return b
}
