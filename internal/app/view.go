package app

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"github.com/skobkin/simple-hdd-tool/internal/domain"
	"github.com/skobkin/simple-hdd-tool/internal/format"
)

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
		m.styles.header.Render("simple-hdd-tool"),
		"",
		"Scanning SATA/SAS disks",
		progressBar(m.scanProgress.Current, m.scanProgress.Total, max(24, m.width-10)),
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
		text := row.Header
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
	return fmt.Sprintf("%-9s %-18s %-10s %-14s %-10s %-12s %s",
		"size", "model", "family", "serial", "block dev", "time", "problems")
}

func (m *Model) renderDiskRow(d domain.Disk) string {
	return fmt.Sprintf("%-9s %-18s %-10s %-14s %-10s %-12s %s",
		trunc(format.SizeBytes(d.SizeBytes), 9),
		trunc(d.Model, 18),
		trunc(d.Family, 10),
		trunc(d.Serial, 14),
		trunc(d.DevicePath, 10),
		trunc(format.DurationHours(d.Smart.PowerOnHours), 12),
		trunc(d.Problem, max(8, m.width-80)),
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
	lines := []string{
		m.styles.header.Render("Disk Details"),
		"",
		"Family: " + disk.Family,
		"Model: " + disk.Model,
		"Size: " + format.SizeBytes(disk.SizeBytes),
		"Serial: " + disk.Serial,
		"Block device: " + disk.DevicePath,
		"Time: " + format.DurationHours(disk.Smart.PowerOnHours),
		"Health: " + health,
		"Problem: " + fallback(disk.Problem),
	}
	if disk.Smart.TemperatureC != nil {
		lines = append(lines, fmt.Sprintf("Temperature: %d C", *disk.Smart.TemperatureC))
	}
	if disk.Smart.ReallocatedSectors != nil {
		lines = append(lines, fmt.Sprintf("Reallocated sectors: %d", *disk.Smart.ReallocatedSectors))
	}
	if disk.Smart.PendingSectors != nil {
		lines = append(lines, fmt.Sprintf("Pending sectors: %d", *disk.Smart.PendingSectors))
	}
	if disk.Smart.UncorrectableErrors != nil {
		lines = append(lines, fmt.Sprintf("Reported uncorrectable errors: %d", *disk.Smart.UncorrectableErrors))
	}
	if disk.Smart.StartStopCount != nil {
		lines = append(lines, fmt.Sprintf("Start/stop count: %d", *disk.Smart.StartStopCount))
	}
	if disk.Smart.PowerCycleCount != nil {
		lines = append(lines, fmt.Sprintf("Power cycle count: %d", *disk.Smart.PowerCycleCount))
	}
	if disk.ProblemNote != "" {
		lines = append(lines, "Note: "+disk.ProblemNote)
	}
	if disk.Usage.ChecksPartial {
		lines = append(lines, "Warning: Could not fully verify device usage; proceed carefully")
	}

	lines = append(lines, "", m.renderDetailActions(disk), "Left/Right or Tab select  Enter activate  Esc back")
	return m.styles.box.Render(strings.Join(lines, "\n"))
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
	lines = append(lines, "s Stop  Esc Stop  q Stop")
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

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
