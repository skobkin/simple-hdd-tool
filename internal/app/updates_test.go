package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/skobkin/simple-hdd-tool/internal/buildinfo"
	"github.com/skobkin/simple-hdd-tool/internal/updates"
)

type stubUpdateChecker struct {
	result updates.Info
	err    error
	calls  int
}

func (s *stubUpdateChecker) Check(context.Context) (updates.Info, error) {
	s.calls++

	return s.result, s.err
}

func availableUpdateInfo() updates.Info {
	return updates.Info{
		CurrentVersion: "0.3.0",
		Status:         updates.StatusUpdateAvailable,
		LatestVersion:  "0.9.0",
		LatestURL:      "https://git.example.invalid/skobkin/simple-hdd-tool/releases/tag/0.9.0",
		ReleasesURL:    "https://git.example.invalid/skobkin/simple-hdd-tool/releases",
		Changelog:      "## 0.9.0\n\n- fix: something (#12)",
	}
}

func newUpdatesTestModel(t *testing.T, stub *stubUpdateChecker) *Model {
	t.Helper()

	m := NewModel(Config{NoColor: true})
	m.updateChecker = stub
	m.width = 120

	return m
}

func TestAutoUpdateEnabled(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		noUpdateCheck bool
		want          bool
	}{
		{name: "release build checks", version: "0.3.0", want: true},
		{name: "dev build skips", version: buildinfo.DevVersion},
		{name: "flag suppresses", version: "0.3.0", noUpdateCheck: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := autoUpdateEnabled(Config{NoUpdateCheck: tt.noUpdateCheck}, tt.version)
			if got != tt.want {
				t.Errorf("autoUpdateEnabled = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestUpdateKeyStartsManualCheck(t *testing.T) {
	stub := &stubUpdateChecker{result: availableUpdateInfo()}
	m := newUpdatesTestModel(t, stub)
	m.mode = viewTable

	_, cmd := m.handleKey(keyPress('u', "u", 0))
	if cmd == nil {
		t.Fatal("expected a command from the update key")
	}
	msg, ok := cmd().(updateCheckDoneMsg)
	if !ok {
		t.Fatalf("msg = %T, want updateCheckDoneMsg", msg)
	}
	if !msg.manual {
		t.Error("expected the check to be marked manual")
	}
	if stub.calls != 1 {
		t.Fatalf("checker calls = %d, want 1", stub.calls)
	}
	if !m.updateChecking {
		t.Error("expected the busy flag to be set while the check runs")
	}
	if m.mode != viewTable {
		t.Errorf("mode = %v, want viewTable", m.mode)
	}
}

func TestUpdateCheckBusyGuard(t *testing.T) {
	stub := &stubUpdateChecker{result: availableUpdateInfo()}
	m := newUpdatesTestModel(t, stub)
	m.mode = viewTable
	m.updateChecking = true

	m.handleKey(keyPress('u', "u", 0))

	if stub.calls != 0 {
		t.Errorf("checker calls = %d, want 0 while a check is in flight", stub.calls)
	}
}

func TestUpdateUnavailableWithoutChecker(t *testing.T) {
	m := newUpdatesTestModel(t, nil)
	m.updateChecker = nil
	m.mode = viewTable

	m.handleKey(keyPress('u', "u", 0))

	if m.mode != viewInfo {
		t.Fatalf("mode = %v, want viewInfo", m.mode)
	}
	if !strings.Contains(m.infoText, "unavailable") {
		t.Errorf("infoText = %q, want an availability notice", m.infoText)
	}
}

func TestInitStartsUpdateCheckForReleaseBuilds(t *testing.T) {
	stub := &stubUpdateChecker{}
	previous := buildinfo.Version
	buildinfo.Version = "0.3.0"
	t.Cleanup(func() { buildinfo.Version = previous })

	m := newUpdatesTestModel(t, stub)
	m.Init()
	if !m.updateChecking {
		t.Error("expected the startup check to run for release builds")
	}

	m = newUpdatesTestModel(t, stub)
	buildinfo.Version = buildinfo.DevVersion
	m.Init()
	if m.updateChecking {
		t.Error("expected no startup check for dev builds")
	}
}

func TestManualCheckResultOpensModal(t *testing.T) {
	stub := &stubUpdateChecker{result: availableUpdateInfo()}
	m := newUpdatesTestModel(t, stub)

	m.Update(updateCheckDoneMsg{result: stub.result, manual: true})

	if m.mode != viewUpdates {
		t.Fatalf("mode = %v, want viewUpdates", m.mode)
	}
	if m.updateChecking {
		t.Error("expected the busy flag to clear after the check")
	}

	view := ansi.Strip(m.renderUpdates())
	for _, want := range []string{"Update Available", "0.9.0", "Download: " + stub.result.LatestURL, "fix: something", "Press Enter, Esc, or q to close."} {
		if !strings.Contains(view, want) {
			t.Errorf("update modal missing %q, got %q", want, view)
		}
	}
}

func TestManualCheckErrorShowsModal(t *testing.T) {
	stub := &stubUpdateChecker{err: errors.New("offline")}
	m := newUpdatesTestModel(t, stub)

	m.Update(updateCheckDoneMsg{err: stub.err, manual: true})

	if m.mode != viewUpdates {
		t.Fatalf("mode = %v, want viewUpdates", m.mode)
	}
	if view := ansi.Strip(m.renderUpdates()); !strings.Contains(view, "Update check failed: offline") {
		t.Errorf("update modal = %q, want the failure notice", view)
	}
}

func TestAutoCheckResultDoesNotChangeMode(t *testing.T) {
	stub := &stubUpdateChecker{result: availableUpdateInfo()}
	m := newUpdatesTestModel(t, stub)
	m.mode = viewDetails

	m.Update(updateCheckDoneMsg{result: stub.result})

	if m.mode != viewDetails {
		t.Errorf("mode = %v, want viewDetails", m.mode)
	}
	if m.updateResult == nil || !m.updateResult.UpdateAvailable() {
		t.Fatal("expected the automatic result to be stored")
	}
	if view := ansi.Strip(m.renderTable()); !strings.Contains(view, "Update available: 0.9.0") {
		t.Errorf("table view = %q, want the update banner", view)
	}
}

func TestAutoCheckErrorSilent(t *testing.T) {
	stub := &stubUpdateChecker{err: errors.New("offline")}
	m := newUpdatesTestModel(t, stub)
	m.mode = viewTable

	m.Update(updateCheckDoneMsg{err: stub.err})

	if m.mode != viewTable {
		t.Errorf("mode = %v, want viewTable", m.mode)
	}
	if m.updateErr != nil {
		t.Errorf("updateErr = %v, want nil", m.updateErr)
	}
}

func TestUpdateBannerStates(t *testing.T) {
	stub := &stubUpdateChecker{result: availableUpdateInfo()}
	m := newUpdatesTestModel(t, stub)

	m.updateChecking = true
	if banner := m.updateBanner(); !strings.Contains(ansi.Strip(banner), "Checking for updates") {
		t.Errorf("banner = %q, want the checking notice", banner)
	}

	m.updateChecking = false
	m.updateResult = &updates.Info{CurrentVersion: "0.9.0", Status: updates.StatusUpToDate}
	if banner := m.updateBanner(); banner != "" {
		t.Errorf("banner = %q, want empty when up to date", banner)
	}
	if view := ansi.Strip(m.renderTable()); strings.Contains(view, "Update available") {
		t.Errorf("table view = %q, want no banner when up to date", view)
	}
}

func TestUpdateModalClosesToTable(t *testing.T) {
	keys := []struct {
		name string
		code rune
		text string
	}{
		{name: "enter", code: tea.KeyEnter},
		{name: "esc", code: tea.KeyEscape},
		{name: "q", code: 'q', text: "q"},
	}

	for _, key := range keys {
		t.Run(key.name, func(t *testing.T) {
			m := newUpdatesTestModel(t, nil)
			m.mode = viewUpdates
			m.updateErr = errors.New("offline")

			m.handleKey(keyPress(key.code, key.text, 0))

			if m.mode != viewTable {
				t.Fatalf("mode = %v, want viewTable", m.mode)
			}
			if m.updateErr != nil {
				t.Errorf("updateErr = %v, want it cleared", m.updateErr)
			}
		})
	}
}
