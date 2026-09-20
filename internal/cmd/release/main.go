// Command release inspects the local checkout and prints a non-mutating release plan.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type readiness struct {
	Enabled          bool              `json:"enabled"`
	RequiredEvidence []readinessClause `json:"requiredEvidence"`
}

type readinessClause struct {
	ID          string `json:"id"`
	Requirement string `json:"requirement"`
}

type plan struct {
	Enabled      bool   `json:"enabled"`
	CurrentTag   string `json:"currentTag,omitempty"`
	NextTag      string `json:"nextTag,omitempty"`
	Release      bool   `json:"release"`
	Reason       string `json:"reason"`
	EvidenceSize int    `json:"requiredEvidenceCount"`
}

var versionPattern = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)
var breakingPattern = regexp.MustCompile(`(?mi)^.*(BREAKING[ -]CHANGE:|[[:alnum:]]+(\([^\n)]*\))?!:).*$`)
var featurePattern = regexp.MustCompile(`(?mi)^feat(\([^\n)]*\))?:`)
var fixPattern = regexp.MustCompile(`(?mi)^fix(\([^\n)]*\))?:`)

func main() {
	if len(os.Args) < 2 || os.Args[1] != "plan" {
		fmt.Fprintln(os.Stderr, "usage: go run ./internal/cmd/release plan [--json] [--manifest path]")
		os.Exit(64)
	}

	flags := flag.NewFlagSet("plan", flag.ExitOnError)
	jsonOutput := flags.Bool("json", false, "write the plan as JSON")
	manifestPath := flags.String("manifest", "release/readiness.json", "release readiness manifest")
	_ = flags.Parse(os.Args[2:])

	ready, err := loadReadiness(*manifestPath)
	if err != nil {
		fail(err)
	}
	current, err := currentTag()
	if err != nil {
		fail(err)
	}
	entry, err := makePlan(ready, current)
	if err != nil {
		fail(err)
	}
	if *jsonOutput {
		data, err := json.Marshal(entry)
		if err != nil {
			fail(err)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Printf("enabled: %t\ncurrent tag: %s\nnext tag: %s\nrelease: %t\nreason: %s\nrequired evidence: %d\n", entry.Enabled, display(entry.CurrentTag), display(entry.NextTag), entry.Release, entry.Reason, entry.EvidenceSize)
}

func loadReadiness(path string) (readiness, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return readiness{}, fmt.Errorf("read readiness manifest: %w", err)
	}
	var ready readiness
	if err := json.Unmarshal(data, &ready); err != nil {
		return readiness{}, fmt.Errorf("parse readiness manifest: %w", err)
	}
	if len(ready.RequiredEvidence) < 5 {
		return readiness{}, errors.New("readiness manifest must contain all five required evidence criteria")
	}
	for _, clause := range ready.RequiredEvidence {
		if clause.ID == "" || clause.Requirement == "" {
			return readiness{}, errors.New("readiness manifest contains an incomplete evidence criterion")
		}
	}
	return ready, nil
}

func currentTag() (string, error) {
	output, err := git("tag", "--merged", "HEAD", "--list", "v[0-9]*")
	if err != nil {
		return "", err
	}
	var tags []semver
	for _, tag := range strings.Fields(output) {
		if parsed, ok := parseSemver(tag); ok {
			tags = append(tags, parsed)
		}
	}
	if len(tags) == 0 {
		return "", nil
	}
	sort.Slice(tags, func(i, j int) bool { return tags[i].less(tags[j]) })
	return tags[len(tags)-1].String(), nil
}

func makePlan(ready readiness, current string) (plan, error) {
	logRange := "HEAD"
	if current != "" {
		logRange = current + "..HEAD"
	}
	log, err := git("log", "--format=%B%x00", logRange)
	if err != nil {
		return plan{}, err
	}
	kind := releaseKind(log)
	entry := plan{Enabled: ready.Enabled, CurrentTag: current, EvidenceSize: len(ready.RequiredEvidence)}
	if kind == "" {
		entry.Reason = "no feat, fix, or breaking Conventional Commit since the current tag"
		return entry, nil
	}
	if current == "" {
		entry.NextTag = "v0.1.0"
		entry.Release = true
		entry.Reason = "first releasable change; policy fixes the first release at v0.1.0"
		return entry, nil
	}
	currentVersion, ok := parseSemver(current)
	if !ok {
		return plan{}, fmt.Errorf("invalid current tag: %s", current)
	}
	switch kind {
	case "patch":
		currentVersion.patch++
	case "minor":
		currentVersion.minor++
		currentVersion.patch = 0
	default:
		return plan{}, fmt.Errorf("unknown release kind: %s", kind)
	}
	entry.NextTag = currentVersion.String()
	entry.Release = true
	entry.Reason = kind + " release under the 0.x policy"
	return entry, nil
}

func releaseKind(log string) string {
	if breakingPattern.MatchString(log) || featurePattern.MatchString(log) {
		return "minor"
	}
	if fixPattern.MatchString(log) {
		return "patch"
	}
	return ""
}

type semver struct {
	major int
	minor int
	patch int
}

func parseSemver(tag string) (semver, bool) {
	match := versionPattern.FindStringSubmatch(tag)
	if match == nil {
		return semver{}, false
	}
	major, err := strconv.Atoi(match[1])
	if err != nil {
		return semver{}, false
	}
	minor, err := strconv.Atoi(match[2])
	if err != nil {
		return semver{}, false
	}
	patch, err := strconv.Atoi(match[3])
	if err != nil {
		return semver{}, false
	}
	return semver{major: major, minor: minor, patch: patch}, true
}

func (v semver) String() string {
	return fmt.Sprintf("v%d.%d.%d", v.major, v.minor, v.patch)
}

func (v semver) less(other semver) bool {
	if v.major != other.major {
		return v.major < other.major
	}
	if v.minor != other.minor {
		return v.minor < other.minor
	}
	return v.patch < other.patch
}

func git(args ...string) (string, error) {
	command := exec.Command("git", args...)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return string(output), nil
}

func display(value string) string {
	if value == "" {
		return "none"
	}
	return value
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "release plan:", err)
	os.Exit(1)
}
