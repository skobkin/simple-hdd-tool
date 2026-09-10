package buildinfo

// AppName is the user-facing application name shown in the CLI and UI.
const AppName = "hddtool"

// DevVersion marks builds that were not made from a release tag. Automatic
// update checks are skipped for them.
const DevVersion = "dev"

// Version is overridden at build time for tagged releases and defaults to
// DevVersion.
var Version = DevVersion
