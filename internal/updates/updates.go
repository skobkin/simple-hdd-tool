// Package updates wires the go4updates library into the application: it owns
// the Forgejo release source and one-shot update checks, and shapes results
// for the UI. The package is UI-free; every external input (server URL, API
// URL, HTTP client, timeout) is injected through Options, which doubles as
// the test seam.
package updates

import (
	"context"
	"fmt"
	"net/http"
	"time"

	updates "github.com/skobkin/go4updates"
	"github.com/skobkin/go4updates/releasefmt"
	"github.com/skobkin/go4updates/source/forgejo"
)

const (
	// ServerURL is the Forgejo instance hosting the application releases.
	ServerURL = "https://git.skobk.in"

	// Repository is the owner/name path of the application repository.
	Repository = "skobkin/simple-hdd-tool"

	// DefaultTimeout bounds a single check attempt.
	DefaultTimeout = 15 * time.Second
)

const (
	// releaseLimit caps how many releases are fetched per check.
	releaseLimit = 10

	// changelogLimit caps how many releases the changelog covers.
	changelogLimit = 5
)

// Status is the outcome of comparing the running version against the latest
// upstream release.
type Status = updates.Status

const (
	// StatusUnknown means the versions could not be compared, for example on
	// a development build or when the release feed was empty.
	StatusUnknown = updates.StatusUnknown

	// StatusUpToDate means the running version is at least as new as the
	// latest upstream release.
	StatusUpToDate = updates.StatusUpToDate

	// StatusUpdateAvailable means a newer release exists upstream.
	StatusUpdateAvailable = updates.StatusUpdateAvailable
)

// Options configures a Checker. Zero values resolve to the documented
// defaults.
type Options struct {
	// ServerURL is the Forgejo instance hosting the releases. Empty means
	// ServerURL.
	ServerURL string

	// Repository is the owner/name path of the repository. Empty means
	// Repository.
	Repository string

	// APIURL optionally overrides the API base URL, for example to point a
	// test at an httptest server.
	APIURL string

	// CurrentVersion is the running version, usually buildinfo.Version.
	CurrentVersion string

	// Timeout bounds a single check attempt. Zero means DefaultTimeout.
	Timeout time.Duration

	// HTTPClient optionally replaces the HTTP client used for API requests.
	HTTPClient *http.Client
}

// Checker performs one-shot update checks against the application release
// feed. It is immutable after construction and safe for concurrent use.
type Checker struct {
	source         *forgejo.Source
	checker        *updates.Checker
	currentVersion string
	timeout        time.Duration
}

// Info is the outcome of one check, shaped for display. Changelog is
// pre-rendered so the UI never has to interpret releases.
type Info struct {
	// CurrentVersion is the version that was checked.
	CurrentVersion string

	// Status is the comparison outcome.
	Status Status

	// LatestVersion is the newest upstream version, empty when the feed was
	// empty.
	LatestVersion string

	// LatestURL is the human-facing location of the latest release.
	LatestURL string

	// ReleasesURL is the human-facing releases listing page.
	ReleasesURL string

	// Changelog holds the rendered release notes for the modal.
	Changelog string
}

// UpdateAvailable reports whether the info carries a newer release.
func (i Info) UpdateAvailable() bool {
	return i.Status == StatusUpdateAvailable
}

// New builds a Checker. It performs no network I/O and starts no goroutines.
func New(opts Options) (*Checker, error) {
	serverURL := opts.ServerURL
	if serverURL == "" {
		serverURL = ServerURL
	}

	repository := opts.Repository
	if repository == "" {
		repository = Repository
	}

	sourceOpts := make([]forgejo.Option, 0, 2)
	if opts.APIURL != "" {
		sourceOpts = append(sourceOpts, forgejo.WithAPIURL(opts.APIURL))
	}
	if opts.HTTPClient != nil {
		sourceOpts = append(sourceOpts, forgejo.WithHTTPClient(opts.HTTPClient))
	}

	source, err := forgejo.New(serverURL, repository, sourceOpts...)
	if err != nil {
		return nil, fmt.Errorf("updates: %w", err)
	}

	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}

	return &Checker{
		source:         source,
		checker:        updates.NewChecker(updates.CheckerOptions{}),
		currentVersion: opts.CurrentVersion,
		timeout:        timeout,
	}, nil
}

// Check fetches the release feed once and compares versions. Errors are
// wrapped diagnostics (go4updates v0.1 exports no sentinels); callers should
// display, not match, them.
func (c *Checker) Check(ctx context.Context) (Info, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	result, err := c.checker.Check(ctx, updates.Target{
		CurrentVersion: c.currentVersion,
		Source:         c.source,
		Fetch:          updates.FetchOptions{Limit: releaseLimit},
	})
	if err != nil {
		return Info{}, fmt.Errorf("check for updates: %w", err)
	}

	return c.infoFromResult(result), nil
}

// infoFromResult shapes a raw check result for display.
func (c *Checker) infoFromResult(result updates.Result) Info {
	info := Info{
		CurrentVersion: result.CurrentVersion,
		Status:         result.Status,
		ReleasesURL:    result.Feed.ReleasesURL,
	}

	if result.Latest != nil {
		info.LatestVersion = result.Latest.Version
		info.LatestURL = result.Latest.URL
		if info.LatestURL == "" {
			info.LatestURL = result.Feed.ReleasesURL
		}
	}

	info.Changelog = releasefmt.Changelog(c.changelogReleases(result), releasefmt.Options{
		Linker:         releasefmt.ForgejoLinker(result.Feed.RepositoryURL),
		ShortCommitSHA: true,
	})

	return info
}

// changelogReleases picks the releases the changelog covers: everything newer
// than the running version (capped at changelogLimit), the latest release
// only when versions cannot be compared, and nothing when up to date.
func (c *Checker) changelogReleases(result updates.Result) []updates.Release {
	releases := result.Feed.Releases
	if len(releases) == 0 {
		return nil
	}

	if result.Status == StatusUnknown {
		return releases[:1]
	}

	var newer []updates.Release
	comparator := updates.SemVerComparator{}
	for _, release := range releases {
		if comparator.Compare(result.CurrentVersion, release.Version) != StatusUpdateAvailable {
			break
		}
		newer = append(newer, release)
		if len(newer) >= changelogLimit {
			break
		}
	}

	return newer
}
