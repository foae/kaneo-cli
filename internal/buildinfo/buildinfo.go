// Package buildinfo reports the provenance embedded in the executable.
package buildinfo

import (
	"runtime/debug"
	"strings"
)

// These values are replaced at link time for release builds.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// Info is the build provenance emitted by the CLI.
type Info struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Date    string `json:"date"`
}

var readBuildInfo = debug.ReadBuildInfo

// Current returns linker-injected provenance, falling back to metadata supplied
// by go install when a value retains its development default.
func Current() Info {
	current := Info{
		Version: Version,
		Commit:  Commit,
		Date:    Date,
	}

	info, ok := readBuildInfo()
	if !ok {
		return current
	}

	if current.Version == "dev" && installedVersion(info.Main.Version) {
		current.Version = info.Main.Version
	}

	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if current.Commit == "none" && setting.Value != "" {
				current.Commit = setting.Value
			}
		case "vcs.time":
			if current.Date == "unknown" && setting.Value != "" {
				current.Date = setting.Value
			}
		}
	}

	return current
}

func installedVersion(version string) bool {
	return version != "" && version != "(devel)" && !strings.HasPrefix(version, "(devel) ")
}
