package updates

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func newTestChecker(t *testing.T, serverURL, currentVersion string) *Checker {
	t.Helper()

	checker, err := New(Options{ServerURL: serverURL, CurrentVersion: currentVersion})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	return checker
}

func TestCheckStatuses(t *testing.T) {
	prerelease := stableRelease("0.10.0-rc1", "2026-02-01T10:00:00Z")
	prerelease.Prerelease = true
	draft := stableRelease("0.10.0", "2026-02-02T10:00:00Z")
	draft.Draft = true

	tests := []struct {
		name           string
		currentVersion string
		releases       []fakeRelease
		wantStatus     Status
		wantLatest     string
		wantURL        string
	}{
		{
			name:           "update available",
			currentVersion: "0.8.0",
			releases: []fakeRelease{
				stableRelease("0.9.0", "2026-01-03T10:00:00Z"),
				stableRelease("0.8.0", "2026-01-02T10:00:00Z"),
			},
			wantStatus: StatusUpdateAvailable,
			wantLatest: "0.9.0",
			wantURL:    "https://git.example.invalid/skobkin/simple-hdd-tool/releases/tag/0.9.0",
		},
		{
			name:           "up to date",
			currentVersion: "0.9.0",
			releases:       []fakeRelease{stableRelease("0.9.0", "2026-01-03T10:00:00Z")},
			wantStatus:     StatusUpToDate,
			wantLatest:     "0.9.0",
			wantURL:        "https://git.example.invalid/skobkin/simple-hdd-tool/releases/tag/0.9.0",
		},
		{
			name:           "newer than latest",
			currentVersion: "0.9.1",
			releases:       []fakeRelease{stableRelease("0.9.0", "2026-01-03T10:00:00Z")},
			wantStatus:     StatusUpToDate,
			wantLatest:     "0.9.0",
			wantURL:        "https://git.example.invalid/skobkin/simple-hdd-tool/releases/tag/0.9.0",
		},
		{
			name:           "prerelease excluded",
			currentVersion: "0.9.0",
			releases: []fakeRelease{
				prerelease,
				stableRelease("0.9.0", "2026-01-03T10:00:00Z"),
			},
			wantStatus: StatusUpToDate,
			wantLatest: "0.9.0",
			wantURL:    "https://git.example.invalid/skobkin/simple-hdd-tool/releases/tag/0.9.0",
		},
		{
			name:           "draft excluded",
			currentVersion: "0.9.0",
			releases: []fakeRelease{
				draft,
				stableRelease("0.9.0", "2026-01-03T10:00:00Z"),
			},
			wantStatus: StatusUpToDate,
			wantLatest: "0.9.0",
			wantURL:    "https://git.example.invalid/skobkin/simple-hdd-tool/releases/tag/0.9.0",
		},
		{
			name:           "empty feed",
			currentVersion: "0.9.0",
			releases:       nil,
			wantStatus:     StatusUnknown,
			wantLatest:     "",
			wantURL:        "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := newForgejoFake(t, tt.releases...)
			checker := newTestChecker(t, fake.url(), tt.currentVersion)

			info, err := checker.Check(context.Background())
			if err != nil {
				t.Fatalf("Check: %v", err)
			}
			if info.Status != tt.wantStatus {
				t.Errorf("Status = %v, want %v", info.Status, tt.wantStatus)
			}
			if info.LatestVersion != tt.wantLatest {
				t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, tt.wantLatest)
			}
			if info.LatestURL != tt.wantURL {
				t.Errorf("LatestURL = %q, want %q", info.LatestURL, tt.wantURL)
			}
			if info.CurrentVersion != tt.currentVersion {
				t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, tt.currentVersion)
			}
			if got := info.UpdateAvailable(); got != (tt.wantStatus == StatusUpdateAvailable) {
				t.Errorf("UpdateAvailable() = %v for status %v", got, tt.wantStatus)
			}
		})
	}
}

func TestCheckReportsReleasesURL(t *testing.T) {
	fake := newForgejoFake(t, stableRelease("0.9.0", "2026-01-03T10:00:00Z"))
	checker := newTestChecker(t, fake.url(), "0.8.0")

	info, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	want := fake.url() + "/skobkin/simple-hdd-tool/releases"
	if info.ReleasesURL != want {
		t.Errorf("ReleasesURL = %q, want %q", info.ReleasesURL, want)
	}
}

func TestCheckDevVersionIsUnknown(t *testing.T) {
	fake := newForgejoFake(t, stableRelease("0.9.0", "2026-01-03T10:00:00Z"))
	checker := newTestChecker(t, fake.url(), "dev")

	info, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if info.Status != StatusUnknown {
		t.Errorf("Status = %v, want %v", info.Status, StatusUnknown)
	}
	if info.LatestVersion != "0.9.0" {
		t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "0.9.0")
	}
	if !strings.Contains(info.Changelog, "## 0.9.0") {
		t.Errorf("Changelog should cover the latest release, got %q", info.Changelog)
	}
}

func TestCheckServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)

	checker := newTestChecker(t, server.URL, "0.8.0")

	info, err := checker.Check(context.Background())
	if err == nil {
		t.Fatal("Check expected an error on HTTP 500")
	}
	if info != (Info{}) {
		t.Errorf("Info = %+v, want zero value on error", info)
	}
}

func TestCheckChangelogOnlyNewerReleases(t *testing.T) {
	fake := newForgejoFake(t,
		stableRelease("0.2.0", "2026-01-04T10:00:00Z"),
		stableRelease("0.1.5", "2026-01-03T10:00:00Z"),
		stableRelease("0.1.0", "2026-01-02T10:00:00Z"),
		stableRelease("0.0.9", "2026-01-01T10:00:00Z"),
	)
	checker := newTestChecker(t, fake.url(), "0.1.0")

	info, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	for _, want := range []string{"## 0.2.0", "## 0.1.5"} {
		if !strings.Contains(info.Changelog, want) {
			t.Errorf("Changelog missing %q, got %q", want, info.Changelog)
		}
	}
	for _, unwanted := range []string{"## 0.1.0", "## 0.0.9"} {
		if strings.Contains(info.Changelog, unwanted) {
			t.Errorf("Changelog should not contain %q, got %q", unwanted, info.Changelog)
		}
	}
}

func TestCheckChangelogCapsAtLimit(t *testing.T) {
	releases := make([]fakeRelease, 0, 7)
	for i := 8; i >= 2; i-- {
		releases = append(releases, stableRelease("0."+strconv.Itoa(i)+".0", "2026-01-01T10:00:00Z"))
	}
	fake := newForgejoFake(t, releases...)
	checker := newTestChecker(t, fake.url(), "0.1.0")

	info, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	// Sections are joined by the releasefmt separator; body headings would
	// skew a "## " count.
	if got := strings.Count(info.Changelog, "\n\n---\n\n") + 1; got != changelogLimit {
		t.Errorf("Changelog covers %d releases, want %d", got, changelogLimit)
	}
}

func TestCheckChangelogUnknownShowsLatestOnly(t *testing.T) {
	fake := newForgejoFake(t,
		stableRelease("0.3.0", "2026-01-04T10:00:00Z"),
		stableRelease("0.2.0", "2026-01-03T10:00:00Z"),
	)
	checker := newTestChecker(t, fake.url(), "dev")

	info, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if !strings.Contains(info.Changelog, "## 0.3.0") {
		t.Errorf("Changelog missing latest release, got %q", info.Changelog)
	}
	if strings.Contains(info.Changelog, "## 0.2.0") {
		t.Errorf("Changelog should show the latest release only, got %q", info.Changelog)
	}
}

func TestCheckChangelogEmptyWhenUpToDate(t *testing.T) {
	fake := newForgejoFake(t, stableRelease("0.9.0", "2026-01-03T10:00:00Z"))
	checker := newTestChecker(t, fake.url(), "0.9.0")

	info, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	if info.Changelog != "" {
		t.Errorf("Changelog = %q, want empty", info.Changelog)
	}
}

func TestCheckChangelogLinkifiesAgainstRepositoryURL(t *testing.T) {
	release := stableRelease("0.9.0", "2026-01-03T10:00:00Z")
	fake := newForgejoFake(t, release)
	checker := newTestChecker(t, fake.url(), "0.8.0")

	info, err := checker.Check(context.Background())
	if err != nil {
		t.Fatalf("Check: %v", err)
	}
	wantIssue := "[#12](" + fake.url() + "/skobkin/simple-hdd-tool/issues/12)"
	if !strings.Contains(info.Changelog, wantIssue) {
		t.Errorf("Changelog missing linkified issue %q, got %q", wantIssue, info.Changelog)
	}
	if !strings.Contains(info.Changelog, "/commit/abc123d") {
		t.Errorf("Changelog missing linkified commit, got %q", info.Changelog)
	}
}

func TestCheckHonorsContextCancellation(t *testing.T) {
	fake := blockingFake(t)
	checker := newTestChecker(t, fake.url(), "0.8.0")

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := checker.Check(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Errorf("err = %v, want context.Canceled", err)
	}
}

func TestNewRejectsBadOptions(t *testing.T) {
	tests := []struct {
		name string
		opts Options
	}{
		{name: "repository without owner", opts: Options{Repository: "no-slash"}},
		{name: "invalid server URL", opts: Options{ServerURL: "://bad"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := New(tt.opts); err == nil {
				t.Fatal("New expected an error")
			}
		})
	}
}
