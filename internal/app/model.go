package app

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/skobkin/simple-hdd-tool/internal/buildinfo"
	"github.com/skobkin/simple-hdd-tool/internal/domain"
	"github.com/skobkin/simple-hdd-tool/internal/format"
	"github.com/skobkin/simple-hdd-tool/internal/linux"
	"github.com/skobkin/simple-hdd-tool/internal/updates"
)

var startReadLoad = linux.StartReadLoad
var snapshotReadLoad = func(loader *linux.ReadLoader) domain.ReadLoadSnapshot { return loader.Snapshot() }

// updateChecker is the seam between the UI and the update checker; the
// production implementation is *updates.Checker and tests substitute a stub.
type updateChecker interface {
	Check(ctx context.Context) (updates.Info, error)
}

// newUpdateChecker builds the production update checker; kept as a variable so
// tests can stub construction without a Forgejo server.
var newUpdateChecker = func(currentVersion string) (updateChecker, error) {
	return updates.New(updates.Options{CurrentVersion: currentVersion})
}

type viewMode int

const (
	viewScanning viewMode = iota
	viewTable
	viewDetails
	viewConfirmRemove
	viewInfo
	viewReadLoad
	viewRemoving
	viewUpdates
)

type scanProgressMsg domain.ScanProgress
type scanCompleteMsg domain.ScanResult
type removeProgressMsg domain.RemovalProgress
type removeCompleteMsg domain.RemovalResult
type readLoadTickMsg struct{}

type updateCheckDoneMsg struct {
	result updates.Info
	err    error
	manual bool
}

type scanRunner struct {
	progress chan domain.ScanProgress
	result   chan domain.ScanResult
}

type removeRunner struct {
	progress chan domain.RemovalProgress
	result   chan domain.RemovalResult
}

type row struct {
	Header  string
	Disk    domain.Disk
	HasDisk bool
}

type detailAction int

const (
	detailActionClose detailAction = iota
	detailActionReadLoad
	detailActionRemove
	detailActionCount
)

// Model owns the Bubble Tea application state.
type Model struct {
	cfg       Config
	width     int
	height    int
	mode      viewMode
	infoText  string
	altScreen bool

	disks    []domain.Disk
	rows     []row
	selected int
	readOnly bool

	scanProgress   domain.ScanProgress
	removeProgress string
	scanRunner     *scanRunner
	removeRunner   *removeRunner
	readLoader     *linux.ReadLoader
	detailAction   detailAction
	detailScroll   int

	updateChecker  updateChecker
	updateChecking bool
	updateResult   *updates.Info
	updateErr      error
	pendingUpdates bool

	styles styles
}

type styles struct {
	header   lipgloss.Style
	banner   lipgloss.Style
	selected lipgloss.Style
	faint    lipgloss.Style
	good     lipgloss.Style
	warn     lipgloss.Style
	bad      lipgloss.Style
	box      lipgloss.Style
	button   lipgloss.Style
	focused  lipgloss.Style
	disabled lipgloss.Style
	danger   lipgloss.Style
}

// NewModel builds the initial UI model from CLI config.
func NewModel(cfg Config) *Model {
	model := &Model{
		cfg:    cfg,
		mode:   viewScanning,
		styles: newStyles(cfg.NoColor),
	}
	if checker, err := newUpdateChecker(buildinfo.Version); err == nil {
		model.updateChecker = checker
	}

	return model
}

// SetAltScreen configures whether the app should render in the terminal alternate screen.
func (m *Model) SetAltScreen(enabled bool) {
	m.altScreen = enabled
}

func newStyles(noColor bool) styles {
	if noColor {
		return styles{
			header:   lipgloss.NewStyle().Bold(true),
			banner:   lipgloss.NewStyle().Bold(true),
			selected: lipgloss.NewStyle().Bold(true),
			faint:    lipgloss.NewStyle().Faint(true),
			good:     lipgloss.NewStyle(),
			warn:     lipgloss.NewStyle().Bold(true),
			bad:      lipgloss.NewStyle().Bold(true),
			box:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).Padding(0, 1),
			button:   lipgloss.NewStyle().Padding(0, 1),
			focused:  lipgloss.NewStyle().Bold(true).Reverse(true).Padding(0, 1),
			disabled: lipgloss.NewStyle().Faint(true).Padding(0, 1),
			danger:   lipgloss.NewStyle().Bold(true).Padding(0, 1),
		}
	}

	return styles{
		header:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("14")),
		banner:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("11")),
		selected: lipgloss.NewStyle().Background(lipgloss.Color("24")).Foreground(lipgloss.Color("255")),
		faint:    lipgloss.NewStyle().Faint(true),
		good:     lipgloss.NewStyle().Foreground(lipgloss.Color("10")),
		warn:     lipgloss.NewStyle().Foreground(lipgloss.Color("11")),
		bad:      lipgloss.NewStyle().Foreground(lipgloss.Color("9")),
		box:      lipgloss.NewStyle().Border(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("8")).Padding(0, 1),
		button:   lipgloss.NewStyle().Padding(0, 1),
		focused:  lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("255")).Background(lipgloss.Color("24")).Padding(0, 1),
		disabled: lipgloss.NewStyle().Faint(true).Foreground(lipgloss.Color("8")).Padding(0, 1),
		danger:   lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("9")).Padding(0, 1),
	}
}

// Init starts the initial disk scan and, when enabled, the startup update
// check.
func (m *Model) Init() tea.Cmd {
	if m.updateChecker != nil && autoUpdateEnabled(m.cfg, buildinfo.Version) {
		return tea.Batch(m.startScan(), m.startUpdateCheck(false))
	}

	return m.startScan()
}

// autoUpdateEnabled reports whether the startup update check should run: never
// on development builds, otherwise per the --no-update-check config.
func autoUpdateEnabled(cfg Config, version string) bool {
	if version == buildinfo.DevVersion {
		return false
	}

	return !cfg.NoUpdateCheck
}

// showUpdateModal presents a completed manual check. It opens immediately from
// the disk list; while another view is active (scan, details, read load,
// removal) the result is deferred until the disk list is shown again, so the
// modal never interrupts a running operation.
func (m *Model) showUpdateModal() {
	if m.mode != viewTable && m.mode != viewUpdates {
		m.pendingUpdates = true

		return
	}
	m.mode = viewUpdates
}

// returnToTable leaves the current view for the disk list, presenting any
// update result that was deferred while another operation was active.
func (m *Model) returnToTable() {
	m.mode = viewTable
	m.flushPendingUpdateModal()
}

func (m *Model) flushPendingUpdateModal() {
	if !m.pendingUpdates {
		return
	}
	m.pendingUpdates = false
	m.showUpdateModal()
}

// startUpdateCheck issues one update check; manual marks a user-requested
// check so its result, including failures, is presented in the update modal.
func (m *Model) startUpdateCheck(manual bool) tea.Cmd {
	if m.updateChecker == nil || m.updateChecking {
		return nil
	}
	m.updateChecking = true
	m.updateErr = nil
	checker := m.updateChecker

	return func() tea.Msg {
		result, err := checker.Check(context.Background())

		return updateCheckDoneMsg{result: result, err: err, manual: manual}
	}
}

func (m *Model) startScan() tea.Cmd {
	runner := &scanRunner{
		progress: make(chan domain.ScanProgress),
		result:   make(chan domain.ScanResult, 1),
	}
	m.scanRunner = runner

	return tea.Batch(func() tea.Msg {
		go func() {
			result := linux.Scanner{PerDiskTimeout: 3 * time.Second}.Scan(context.Background(), runner.progress)
			runner.result <- result
			close(runner.progress)
		}()

		return nil
	}, waitScanProgress(runner.progress), waitScanResult(runner.result))
}

func waitScanProgress(ch <-chan domain.ScanProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}

		return scanProgressMsg(p)
	}
}

func waitScanResult(ch <-chan domain.ScanResult) tea.Cmd {
	return func() tea.Msg {
		return scanCompleteMsg(<-ch)
	}
}

func waitRemoveProgress(ch <-chan domain.RemovalProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}

		return removeProgressMsg(p)
	}
}

func waitRemoveResult(ch <-chan domain.RemovalResult) tea.Cmd {
	return func() tea.Msg {
		return removeCompleteMsg(<-ch)
	}
}

func readLoadTick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return readLoadTickMsg{} })
}

// Update handles terminal events and async worker messages.
func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.clampDetailScroll()
	case tea.KeyPressMsg:
		return m.handleKey(msg)
	case scanProgressMsg:
		m.scanProgress = domain.ScanProgress(msg)

		return m, waitScanProgress(m.scanRunner.progress)
	case scanCompleteMsg:
		res := domain.ScanResult(msg)
		m.readOnly = res.ReadOnly
		m.mode = viewTable
		if res.Err != nil {
			m.infoText = "Scan failed: " + res.Err.Error()
			m.mode = viewInfo
		}
		m.disks = res.Disks
		m.rebuildRows()
		m.flushPendingUpdateModal()
	case removeProgressMsg:
		m.removeProgress = domain.RemovalProgress(msg).Step

		return m, waitRemoveProgress(m.removeRunner.progress)
	case removeCompleteMsg:
		res := domain.RemovalResult(msg)
		m.removeRunner = nil
		if res.Err != nil {
			m.mode = viewInfo
			m.infoText = "Remove failed: " + res.Err.Error()

			return m, nil
		}
		m.mode = viewInfo
		m.infoText = "Disk removed"

		return m, m.startScan()
	case readLoadTickMsg:
		if m.mode == viewReadLoad && m.readLoader != nil {
			if snap := m.readLoader.Snapshot(); snap.LastError != nil {
				m.readLoader.Stop()
				m.readLoader = nil
				m.mode = viewInfo
				m.infoText = "Read load failed: " + snap.LastError.Error()

				return m, nil
			}

			return m, readLoadTick()
		}
	case updateCheckDoneMsg:
		m.updateChecking = false
		if msg.err != nil {
			if msg.manual {
				m.updateErr = msg.err
				m.showUpdateModal()
			}

			return m, nil
		}
		result := msg.result
		m.updateResult = &result
		if msg.manual {
			m.showUpdateModal()
		}
	}

	return m, nil
}

func (m *Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch m.mode {
	case viewTable:
	case viewScanning, viewRemoving:
		if msg.String() == "ctrl+c" || msg.String() == "q" {
			return m, tea.Quit
		}

		return m, nil
	case viewInfo:
		if msg.String() == "enter" || msg.String() == "esc" || msg.String() == "q" {
			m.returnToTable()
			m.infoText = ""
		}

		return m, nil
	case viewConfirmRemove:
		switch msg.String() {
		case "y":
			m.mode = viewRemoving
			runner := &removeRunner{progress: make(chan domain.RemovalProgress), result: make(chan domain.RemovalResult, 1)}
			m.removeRunner = runner
			disk := m.selectedDisk()

			return m, tea.Batch(func() tea.Msg {
				go func() {
					res := linux.Remover{}.Remove(context.Background(), *disk, m.cfg.ForceRemove, runner.progress)
					runner.result <- res
					close(runner.progress)
				}()

				return nil
			}, waitRemoveProgress(runner.progress), waitRemoveResult(runner.result))
		case "n", "esc":
			m.mode = viewDetails
			m.clampDetailScroll()
		}

		return m, nil
	case viewDetails:
		switch msg.String() {
		case "esc", "q":
			m.returnToTable()
		case "up":
			m.scrollDetails(-1)
		case "down":
			m.scrollDetails(1)
		case "pgup":
			m.scrollDetails(-m.detailPageSize())
		case "pgdown":
			m.scrollDetails(m.detailPageSize())
		case "home":
			m.detailScroll = 0
		case "end":
			m.detailScroll = m.maxDetailScroll()
		case "left", "shift+tab":
			m.detailAction = (m.detailAction + detailActionCount - 1) % detailActionCount
		case "right", "tab":
			m.detailAction = (m.detailAction + 1) % detailActionCount
		case "enter":
			return m.runDetailAction()
		case "l":
			m.detailAction = detailActionReadLoad

			return m.runDetailAction()
		case "x":
			m.detailAction = detailActionRemove

			return m.runDetailAction()
		}

		return m, nil
	case viewReadLoad:
		switch msg.String() {
		case "enter", "s", "esc", "q":
			if m.readLoader != nil {
				m.readLoader.Stop()
				m.readLoader = nil
			}
			m.mode = viewDetails
		}

		return m, nil
	case viewUpdates:
		if msg.String() == "enter" || msg.String() == "esc" || msg.String() == "q" {
			m.returnToTable()
			m.updateErr = nil
		}

		return m, nil
	}

	switch msg.String() {
	case "ctrl+c", "q":
		return m, tea.Quit
	case "j", "down":
		if m.selected < len(m.rows)-1 {
			m.selected++
			for m.selected < len(m.rows) && !m.rows[m.selected].HasDisk {
				m.selected++
			}
			if m.selected >= len(m.rows) {
				m.selected = len(m.rows) - 1
			}
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
			for m.selected >= 0 && !m.rows[m.selected].HasDisk {
				m.selected--
			}
			if m.selected < 0 {
				m.selected = 0
			}
		}
	case "g":
		m.cfg.GroupBy = m.cfg.GroupBy.Next()
		m.rebuildRows()
	case "s":
		m.cfg.SortBy = m.cfg.SortBy.Next()
		m.rebuildRows()
	case "r":
		m.mode = viewScanning
		m.scanProgress = domain.ScanProgress{}

		return m, m.startScan()
	case "u":
		if m.updateChecker == nil {
			m.mode = viewInfo
			m.infoText = "Update checking is unavailable"

			return m, nil
		}

		return m, m.startUpdateCheck(true)
	case "enter":
		if m.selectedDisk() != nil {
			m.detailAction = detailActionClose
			m.detailScroll = 0
			m.mode = viewDetails
		}
	}

	return m, nil
}

func (m *Model) runDetailAction() (tea.Model, tea.Cmd) {
	disk := m.selectedDisk()
	if disk == nil {
		m.returnToTable()

		return m, nil
	}

	switch m.detailAction {
	case detailActionClose:
		m.returnToTable()

		return m, nil
	case detailActionReadLoad:
		if !disk.Caps.CanReadLoad {
			m.mode = viewInfo
			m.infoText = "Read load is unavailable in read-only mode"

			return m, nil
		}
		loader, err := startReadLoad(disk.DevicePath, disk.SizeBytes)
		if err != nil {
			m.mode = viewInfo
			m.infoText = "Read load failed: " + err.Error()

			return m, nil
		}
		m.readLoader = loader
		m.mode = viewReadLoad

		return m, readLoadTick()
	case detailActionRemove:
		if m.readOnly || !disk.Caps.CanRemove {
			m.mode = viewInfo
			m.infoText = "Remove is unavailable in read-only mode"

			return m, nil
		}
		m.mode = viewConfirmRemove

		return m, nil
	default:
		return m, nil
	}
}

func (m *Model) selectedDisk() *domain.Disk {
	if m.selected < 0 || m.selected >= len(m.rows) {
		return nil
	}
	if !m.rows[m.selected].HasDisk {
		return nil
	}

	return &m.rows[m.selected].Disk
}

func (m *Model) rebuildRows() {
	disks := append([]domain.Disk(nil), m.disks...)
	sortDisks(disks, m.cfg.SortBy)
	selectedID := ""
	if disk := m.selectedDisk(); disk != nil {
		selectedID = disk.ID
	}

	m.rows = m.rows[:0]
	if m.cfg.GroupBy == domain.GroupByNone {
		for i := range disks {
			m.rows = append(m.rows, row{Disk: disks[i], HasDisk: true})
		}
		m.restoreSelection(selectedID)

		return
	}

	grouped := make(map[string][]domain.Disk)
	order := make([]string, 0)
	for _, disk := range disks {
		key := groupKey(disk, m.cfg.GroupBy)
		if _, ok := grouped[key]; !ok {
			order = append(order, key)
		}
		grouped[key] = append(grouped[key], disk)
	}
	sort.Strings(order)
	for _, key := range order {
		header := fmt.Sprintf("%s (%d)", key, len(grouped[key]))
		m.rows = append(m.rows, row{Header: header})
		for i := range grouped[key] {
			m.rows = append(m.rows, row{Disk: grouped[key][i], HasDisk: true})
		}
	}
	m.restoreSelection(selectedID)
	m.clampDetailScroll()
}

func (m *Model) scrollDetails(delta int) {
	if delta == 0 {
		return
	}

	m.detailScroll += delta
	m.clampDetailScroll()
}

func (m *Model) detailPageSize() int {
	height := m.detailBodyHeight()
	if height < 1 {
		return 1
	}

	return height
}

func (m *Model) clampDetailScroll() {
	if m.detailScroll < 0 {
		m.detailScroll = 0
	}

	maxScroll := m.maxDetailScroll()
	if m.detailScroll > maxScroll {
		m.detailScroll = maxScroll
	}
}

func (m *Model) maxDetailScroll() int {
	disk := m.selectedDisk()
	if disk == nil {
		return 0
	}

	bodyLines := m.renderDetailBodyLines(disk)
	bodyHeight := m.detailBodyHeight()
	if bodyHeight < 1 || len(bodyLines) <= bodyHeight {
		return 0
	}

	return len(bodyLines) - bodyHeight
}

func (m *Model) restoreSelection(selectedID string) {
	for idx, row := range m.rows {
		if row.HasDisk && row.Disk.ID == selectedID {
			m.selected = idx

			return
		}
	}
	for idx, row := range m.rows {
		if row.HasDisk {
			m.selected = idx

			return
		}
	}
	m.selected = 0
}

func groupKey(d domain.Disk, mode domain.GroupMode) string {
	switch mode {
	case domain.GroupBySize:
		return format.SizeBytes(d.SizeBytes)
	case domain.GroupByFamily:
		return fallback(d.Family)
	default:
		return fallback(d.Model)
	}
}

func sortDisks(disks []domain.Disk, mode domain.SortMode) {
	sort.SliceStable(disks, func(i, j int) bool {
		a, b := disks[i], disks[j]
		switch mode {
		case domain.SortBySerial:
			return sortString(a.Serial, b.Serial, a.DevicePath, b.DevicePath)
		case domain.SortByHours:
			ah := ^uint64(0)
			bh := ^uint64(0)
			if a.Smart.PowerOnHours != nil {
				ah = *a.Smart.PowerOnHours
			}
			if b.Smart.PowerOnHours != nil {
				bh = *b.Smart.PowerOnHours
			}
			if ah == bh {
				return a.DevicePath < b.DevicePath
			}

			return ah < bh
		default:
			if a.SizeBytes == b.SizeBytes {
				return a.DevicePath < b.DevicePath
			}
			if a.SizeBytes == 0 {
				return false
			}
			if b.SizeBytes == 0 {
				return true
			}

			return a.SizeBytes < b.SizeBytes
		}
	})
}

func sortString(a, b, afallback, bfallback string) bool {
	av := fallback(a)
	bv := fallback(b)
	if av == "—" {
		return false
	}
	if bv == "—" {
		return true
	}
	if av == bv {
		return afallback < bfallback
	}

	return av < bv
}

func fallback(v string) string {
	if strings.TrimSpace(v) == "" || v == "—" {
		return "—"
	}

	return v
}
