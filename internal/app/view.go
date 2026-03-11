package app

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/mattn/go-runewidth"

	"github.com/skobkin/simple-hdd-tool/internal/buildinfo"
	"github.com/skobkin/simple-hdd-tool/internal/domain"
	"github.com/skobkin/simple-hdd-tool/internal/format"
)

// View renders the current Bubble Tea screen.
func (m *Model) View() tea.View {
	view := tea.NewView(m.viewContent())
	view.AltScreen = m.altScreen

	return view
}

func (m *Model) viewContent() string {
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
	return m.renderTableHeader(m.tableLayout())
}

func (m *Model) renderDiskRow(d domain.Disk) string {
	return m.renderTableDiskRow(d, m.tableLayout(d))
}

type tableColumnLayout struct {
	width int
	last  bool
}

func (m *Model) renderTableHeader(layout map[string]tableColumnLayout) string {
	return strings.Join([]string{
		renderTableCell("size", layout["size"]),
		renderTableCell("model", layout["model"]),
		renderTableCell("family", layout["family"]),
		renderTableCell("serial", layout["serial"]),
		renderTableCell("dev", layout["dev"]),
		renderTableCell("time", layout["time"]),
		renderTableCell("health", layout["health"]),
		renderTableCell("problems", layout["problems"]),
	}, " ")
}

func (m *Model) renderTableDiskRow(d domain.Disk, layout map[string]tableColumnLayout) string {
	return strings.Join([]string{
		renderTableCell(format.SizeBytes(d.SizeBytes), layout["size"]),
		renderTableCell(d.Model, layout["model"]),
		renderTableCell(d.Family, layout["family"]),
		renderTableCell(d.Serial, layout["serial"]),
		renderTableCell(d.DevicePath, layout["dev"]),
		renderTableCell(format.DurationHoursCompact(d.Smart.PowerOnHours, layout["time"].width), layout["time"]),
		renderStyledTableCell(smartctlHealthIndicator(m.styles, d.Smart.OverallHealth), layout["health"]),
		renderTableCell(d.Problem, layout["problems"]),
	}, " ")
}

func (m *Model) tableLayout(extra ...domain.Disk) map[string]tableColumnLayout {
	type columnSpec struct {
		key      string
		header   string
		minWidth int
		maxWidth int
	}

	columns := []columnSpec{
		{key: "size", header: "size", minWidth: 6, maxWidth: 9},
		{key: "model", header: "model", minWidth: 8, maxWidth: 24},
		{key: "family", header: "family", minWidth: 8, maxWidth: 24},
		{key: "serial", header: "serial", minWidth: 6, maxWidth: 20},
		{key: "dev", header: "dev", minWidth: 8, maxWidth: 10},
		{key: "time", header: "time", minWidth: 6, maxWidth: 13},
		{key: "health", header: "health", minWidth: 6, maxWidth: 6},
		{key: "problems", header: "problems", minWidth: 8, maxWidth: 48},
	}

	desired := make(map[string]int, len(columns))
	for _, column := range columns {
		desired[column.key] = runewidth.StringWidth(column.header)
	}

	disks := append(append([]domain.Disk(nil), m.disks...), extra...)
	for _, d := range disks {
		desired["size"] = maxInt(desired["size"], runewidth.StringWidth(format.SizeBytes(d.SizeBytes)))
		desired["model"] = maxInt(desired["model"], runewidth.StringWidth(normalizeTableValue(d.Model)))
		desired["family"] = maxInt(desired["family"], runewidth.StringWidth(normalizeTableValue(d.Family)))
		desired["serial"] = maxInt(desired["serial"], runewidth.StringWidth(normalizeTableValue(d.Serial)))
		desired["dev"] = maxInt(desired["dev"], runewidth.StringWidth(normalizeTableValue(d.DevicePath)))
		desired["time"] = maxInt(desired["time"], runewidth.StringWidth(format.DurationHoursCompact(d.Smart.PowerOnHours, 64)))
		desired["health"] = maxInt(desired["health"], lipgloss.Width(smartctlHealthIndicator(m.styles, d.Smart.OverallHealth)))
		desired["problems"] = maxInt(desired["problems"], runewidth.StringWidth(normalizeTableValue(d.Problem)))
	}

	layout := make(map[string]tableColumnLayout, len(columns))
	totalWidth := 0
	for idx, column := range columns {
		width := desired[column.key]
		if width < column.minWidth {
			width = column.minWidth
		}
		if width > column.maxWidth {
			width = column.maxWidth
		}
		layout[column.key] = tableColumnLayout{
			width: width,
			last:  idx == len(columns)-1,
		}
		totalWidth += width
	}

	if len(columns) > 1 {
		totalWidth += len(columns) - 1
	}
	if m.width > 0 && totalWidth > m.width {
		deficit := totalWidth - m.width
		shrinkOrder := []string{"problems", "model", "family", "serial"}
		minimum := map[string]int{
			"problems": 8,
			"model":    8,
			"family":   8,
			"serial":   6,
		}
		for deficit > 0 {
			shrank := false
			for _, key := range shrinkOrder {
				column := layout[key]
				if column.width <= minimum[key] {
					continue
				}
				column.width--
				layout[key] = column
				deficit--
				shrank = true
				if deficit == 0 {
					break
				}
			}
			if !shrank {
				break
			}
		}

		return layout
	}

	extraWidth := m.width - totalWidth
	growOrder := []string{"problems", "family", "model"}
	maximum := map[string]int{
		"problems": 64,
		"family":   32,
		"model":    32,
	}
	for extraWidth > 0 {
		grew := false
		for _, key := range growOrder {
			column := layout[key]
			limit := maximum[key]
			if column.width >= limit {
				continue
			}
			column.width++
			layout[key] = column
			extraWidth--
			grew = true
			if extraWidth == 0 {
				break
			}
		}
		if !grew {
			break
		}
	}

	return layout
}

func renderTableCell(value string, layout tableColumnLayout) string {
	value = trunc(value, layout.width)
	if layout.last {
		return value
	}

	return lipgloss.NewStyle().Width(layout.width).Render(value)
}

func renderStyledTableCell(value string, layout tableColumnLayout) string {
	if layout.last {
		return value
	}

	style := lipgloss.NewStyle().Width(layout.width)
	if lipgloss.Width(value) >= layout.width {
		return style.Render(value)
	}

	return value + strings.Repeat(" ", layout.width-lipgloss.Width(value))
}

func normalizeTableValue(v string) string {
	if strings.TrimSpace(v) == "" {
		return "—"
	}

	return strings.Join(strings.Fields(v), " ")
}

func (m *Model) renderDetails() string {
	disk := m.selectedDisk()
	if disk == nil {
		return m.renderTable()
	}
	m.clampDetailScroll()

	bodyLines := m.renderDetailBodyLines(disk)
	bodyHeight := m.detailBodyHeight()
	scrollTop := minInt(m.detailScroll, len(bodyLines))
	scrollBottom := minInt(scrollTop+bodyHeight, len(bodyLines))
	visibleBody := bodyLines[scrollTop:scrollBottom]
	lines := []string{m.styles.header.Render("Disk Details")}

	if len(visibleBody) > 0 {
		lines = append(lines, "")
		lines = append(lines, visibleBody...)
	}

	lines = append(lines, "", m.renderDetailActions(disk))

	if m.detailShowHelp() {
		help := "Left/Right or Tab select  Enter activate  Esc back"
		if len(bodyLines) > bodyHeight {
			help = fmt.Sprintf("Up/Down/PgUp/PgDn scroll (%d/%d)  %s", scrollBottom, len(bodyLines), help)
		}
		lines = append(lines, help)
	}

	boxStyle := m.styles.box
	if m.width > 0 {
		boxStyle = boxStyle.MaxWidth(m.width)
	}

	return boxStyle.Render(strings.Join(lines, "\n"))
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

func (m *Model) renderDetailBodyLines(disk *domain.Disk) []string {
	sections := detailSections(m.styles, disk)
	lines := make([]string, 0, len(sections)*6)
	for _, section := range sections {
		rendered := m.renderDetailSection(section)
		if len(rendered) == 0 {
			continue
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, rendered...)
	}

	return wrapLines(lines, m.detailInnerWidth())
}

func detailSections(styles styles, disk *domain.Disk) []detailSection {
	health := string(disk.Health)
	switch disk.Health {
	case domain.HealthHealthy:
		health = styles.good.Render(health)
	case domain.HealthWarning:
		health = styles.warn.Render(health)
	case domain.HealthFailing:
		health = styles.bad.Render(health)
	}

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
				"smartctl health: " + renderSmartctlHealthText(styles, disk.Smart.OverallHealth),
				"App diagnosis: " + health,
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
	if disk.Smart.OverallHealthNote != "" {
		sections[1].Lines = append(sections[1].Lines, "smartctl note: "+disk.Smart.OverallHealthNote)
	}
	if disk.Usage.ChecksPartial {
		sections[3].Lines = append(sections[3].Lines, "Warning: Could not fully verify device usage; proceed carefully")
	}

	return sections
}

func (m *Model) detailInnerWidth() int {
	width := m.width - m.styles.box.GetHorizontalFrameSize()
	if width < 1 {
		return 80
	}

	return width
}

func (m *Model) detailBodyHeight() int {
	if m.height <= 0 {
		return 1 << 20
	}

	innerHeight := m.height - m.styles.box.GetVerticalFrameSize()
	bodyHeight := innerHeight - 3
	if m.detailShowHelp() {
		bodyHeight--
	}
	if bodyHeight < 0 {
		return 0
	}

	return bodyHeight
}

func (m *Model) detailShowHelp() bool {
	if m.height <= 0 {
		return false
	}

	return m.height-m.styles.box.GetVerticalFrameSize() >= 4
}

func wrapLines(lines []string, width int) []string {
	if width < 1 {
		return lines
	}

	style := lipgloss.NewStyle().Width(width)
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			out = append(out, "")

			continue
		}
		out = append(out, strings.Split(style.Render(line), "\n")...)
	}

	return out
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

func smartctlHealthIndicator(styles styles, health domain.SmartctlHealth) string {
	switch health {
	case domain.SmartctlHealthPassed:
		return styles.good.Render("●")
	case domain.SmartctlHealthFailed:
		return styles.bad.Render("✕")
	default:
		return styles.faint.Render("?")
	}
}

func renderSmartctlHealthText(styles styles, health domain.SmartctlHealth) string {
	switch health {
	case domain.SmartctlHealthPassed:
		return styles.good.Render(string(health))
	case domain.SmartctlHealthFailed:
		return styles.bad.Render(string(health))
	default:
		return styles.faint.Render(string(domain.SmartctlHealthUnknown))
	}
}

func (m *Model) renderReadLoad() string {
	snap := snapshotReadLoad(m.readLoader)
	innerWidth := m.modalInnerWidth()
	rateLine := m.styles.focused.Render(" Read rate: " + format.RateBytes(snap.BytesPerSecond) + " ")
	summaryLine := "Elapsed: " + format.ElapsedShort(snap.Elapsed) + "  Data read: " + format.SizeBytes(snap.BytesRead)
	summaryLines := []string{summaryLine}
	if runewidth.StringWidth(summaryLine) > innerWidth {
		summaryLines = []string{
			"Elapsed: " + format.ElapsedShort(snap.Elapsed),
			"Data read: " + format.SizeBytes(snap.BytesRead),
		}
	}

	modeLine := "Mode: direct I/O"
	if snap.DirectIOMessage != "" {
		modeLine = m.styles.warn.Render("Warning: " + snap.DirectIOMessage)
	}

	lines := []string{
		m.styles.faint.Render("Generating sustained read activity"),
		"",
		rateLine,
	}
	lines = append(lines, summaryLines...)
	lines = append(lines,
		"",
		"Device: "+snap.DevicePath,
		modeLine,
		"",
		m.styles.focused.Render("Stop"),
		m.styles.faint.Render("Enter/S/Esc/Q stop"),
	)

	return m.wrapModal("Read Load", wrapLines(lines, innerWidth))
}

func (m *Model) wrapModal(title string, lines []string) string {
	return m.styles.box.Render(m.styles.header.Render(title) + "\n\n" + strings.Join(lines, "\n"))
}

func (m *Model) modalInnerWidth() int {
	width := m.width - m.styles.box.GetHorizontalFrameSize()
	if width < 24 {
		return 24
	}

	return width
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

func minInt(a, b int) int {
	if a < b {
		return a
	}

	return b
}
