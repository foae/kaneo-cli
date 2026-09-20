package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestCurrentPrefersLinkerValues(t *testing.T) {
	previousVersion, previousCommit, previousDate := Version, Commit, Date
	previousReadBuildInfo := readBuildInfo
	t.Cleanup(func() {
		Version, Commit, Date = previousVersion, previousCommit, previousDate
		readBuildInfo = previousReadBuildInfo
	})

	Version, Commit, Date = "v0.1.0", "linked-commit", "2026-09-20T00:00:00Z"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "v0.2.0"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "installed-commit"},
				{Key: "vcs.time", Value: "2026-09-21T00:00:00Z"},
			},
		}, true
	}

	got := Current()
	want := Info{Version: "v0.1.0", Commit: "linked-commit", Date: "2026-09-20T00:00:00Z"}
	if got != want {
		t.Fatalf("Current() = %#v, want %#v", got, want)
	}
}

func TestCurrentFallsBackToInstalledMetadata(t *testing.T) {
	previousVersion, previousCommit, previousDate := Version, Commit, Date
	previousReadBuildInfo := readBuildInfo
	t.Cleanup(func() {
		Version, Commit, Date = previousVersion, previousCommit, previousDate
		readBuildInfo = previousReadBuildInfo
	})

	Version, Commit, Date = "dev", "none", "unknown"
	readBuildInfo = func() (*debug.BuildInfo, bool) {
		return &debug.BuildInfo{
			Main: debug.Module{Version: "v0.1.0"},
			Settings: []debug.BuildSetting{
				{Key: "vcs.revision", Value: "installed-commit"},
				{Key: "vcs.time", Value: "2026-09-20T00:00:00Z"},
			},
		}, true
	}

	got := Current()
	want := Info{Version: "v0.1.0", Commit: "installed-commit", Date: "2026-09-20T00:00:00Z"}
	if got != want {
		t.Fatalf("Current() = %#v, want %#v", got, want)
	}
}
