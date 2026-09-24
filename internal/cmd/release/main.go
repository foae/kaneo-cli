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
	SkillVersion string `json:"skillVersion"`
}

var (
	versionPattern       = regexp.MustCompile(`^v([0-9]+)\.([0-9]+)\.([0-9]+)$`)
	breakingBodyPattern  = regexp.MustCompile(`(?mi).*BREAKING[ -]CHANGE:.*`)
	breakingTitlePattern = regexp.MustCompile(`(?im).*(\w+)(\(.*\))?!:.*`)
	featureTitlePattern  = regexp.MustCompile(`(?im).*feat(\(.*\))?:.*`)
	fixTitlePattern      = regexp.MustCompile(`(?im).*fix(\(.*\))?:.*`)
	skillVersionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
)

const usage = "usage: go run ./internal/cmd/release plan [--json] [--manifest path] [--skill path]\n       go run ./internal/cmd/release stamp-skill [--manifest path] [--skill path]"

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "plan" && os.Args[1] != "stamp-skill") {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(64)
	}
	command := os.Args[1]

	flags := flag.NewFlagSet(command, flag.ExitOnError)
	jsonOutput := false
	if command == "plan" {
		flags.BoolVar(&jsonOutput, "json", false, "write the plan as JSON")
	}
	manifestPath := flags.String("manifest", "release/readiness.json", "release readiness manifest")
	skillPath := flags.String("skill", "skills/kaneo-cli/SKILL.md", "agent skill carrying the metadata.version stamp")
	_ = flags.Parse(os.Args[2:])

	ready, err := loadReadiness(*manifestPath)
	if err != nil {
		fail(err)
	}
	current, err := currentTag()
	if err != nil {
		fail(err)
	}
	log, err := commitLog(current)
	if err != nil {
		fail(err)
	}
	entry, err := makePlan(ready, current, log)
	if err != nil {
		fail(err)
	}
	skill, err := os.ReadFile(*skillPath)
	if err != nil {
		fail(fmt.Errorf("read agent skill: %w", err))
	}

	if command == "stamp-skill" {
		expected := expectedSkillVersion(entry)
		if expected == "" {
			fail(errors.New("stamp-skill: no current tag and no releasable change, so there is no expected skill version"))
		}
		stamped, old, err := stampSkill(skill, expected)
		if err != nil {
			fail(err)
		}
		if err := os.WriteFile(*skillPath, stamped, 0o644); err != nil { //nolint:gosec // SKILL.md is a public, version-controlled document.
			fail(fmt.Errorf("write agent skill: %w", err))
		}
		fmt.Fprintf(os.Stderr, "%s metadata.version: %s -> %s\n", *skillPath, display(old), expected)
		return
	}

	entry.SkillVersion, err = checkSkill(entry, skill)
	if err != nil {
		fail(fmt.Errorf("%s: %w", *skillPath, err))
	}
	if jsonOutput {
		data, err := json.Marshal(entry)
		if err != nil {
			fail(err)
		}
		fmt.Println(string(data))
		return
	}
	fmt.Printf("enabled: %t\ncurrent tag: %s\nnext tag: %s\nrelease: %t\nreason: %s\nrequired evidence: %d\nskill version: %s\n", entry.Enabled, display(entry.CurrentTag), display(entry.NextTag), entry.Release, entry.Reason, entry.EvidenceSize, display(entry.SkillVersion))
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

func commitLog(current string) (string, error) {
	logRange := "HEAD"
	if current != "" {
		logRange = current + "..HEAD"
	}
	return git("log", "--format=%B%x00", logRange)
}

func makePlan(ready readiness, current, log string) (plan, error) {
	kind := releaseKind(log)
	entry := plan{Enabled: ready.Enabled, CurrentTag: current, EvidenceSize: len(ready.RequiredEvidence)}
	if kind == "" {
		entry.Reason = "no feat, fix, or breaking Conventional Commit since the current tag"
		return entry, nil
	}
	nextTag, err := nextTag(current, kind)
	if err != nil {
		return plan{}, err
	}
	entry.NextTag = nextTag
	entry.Release = true
	if current == "" {
		entry.Reason = "first releasable change; policy fixes the first release at v1.0.0"
		return entry, nil
	}
	entry.Reason = kind + " release under the stable policy"
	return entry, nil
}

// expectedSkillVersion returns the version the skill stamp must carry: the
// planned tag when releasing, otherwise the current tag, without the "v".
// It is empty when there is neither.
func expectedSkillVersion(entry plan) string {
	if entry.Release {
		return strings.TrimPrefix(entry.NextTag, "v")
	}
	return strings.TrimPrefix(entry.CurrentTag, "v")
}

// checkSkill validates the skill stamp against the plan and returns the
// stamp it found. Without an expected version the check is skipped.
func checkSkill(entry plan, skill []byte) (string, error) {
	expected := expectedSkillVersion(entry)
	found, err := parseSkillVersion(skill)
	if expected == "" {
		return found, nil
	}
	if err != nil {
		return "", fmt.Errorf("%w; expected metadata.version %q; run `just stamp-skill`", err, expected)
	}
	if found != expected {
		return "", fmt.Errorf("skill metadata.version is %q, expected %q; run `just stamp-skill`", found, expected)
	}
	return found, nil
}

// skillVersionLine locates the version entry under the top-level metadata
// map of the leading YAML frontmatter. It returns the line index, the byte
// offsets of the raw value within that line, and the split lines.
func skillVersionLine(skill []byte) (lines []string, index, start, end int, err error) {
	lines = strings.SplitAfter(string(skill), "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r\n") != "---" {
		return nil, 0, 0, 0, errors.New("skill has no leading YAML frontmatter")
	}
	inMetadata := false
	for i := 1; i < len(lines); i++ {
		content := strings.TrimRight(lines[i], "\r\n")
		if content == "---" {
			return nil, 0, 0, 0, errors.New("skill frontmatter has no metadata.version")
		}
		if strings.TrimSpace(content) == "" {
			continue
		}
		indented := content[0] == ' ' || content[0] == '\t'
		if !indented {
			inMetadata = strings.TrimRight(content, " \t") == "metadata:"
			continue
		}
		if !inMetadata {
			continue
		}
		trimmed := strings.TrimLeft(content, " \t")
		if !strings.HasPrefix(trimmed, "version:") {
			continue
		}
		start = len(content) - len(trimmed) + len("version:")
		for start < len(content) && (content[start] == ' ' || content[start] == '\t') {
			start++
		}
		end = max(len(strings.TrimRight(content, " \t")), start)
		return lines, i, start, end, nil
	}
	return nil, 0, 0, 0, errors.New("skill frontmatter is not terminated")
}

func parseSkillVersion(skill []byte) (string, error) {
	lines, index, start, end, err := skillVersionLine(skill)
	if err != nil {
		return "", err
	}
	raw := lines[index][start:end]
	value := unquote(raw)
	if !skillVersionPattern.MatchString(value) {
		return "", fmt.Errorf("skill metadata.version %q is not a plain X.Y.Z version", raw)
	}
	return value, nil
}

// stampSkill rewrites only the metadata.version value, preserving its
// quoting and every other byte. It returns the previous raw value.
func stampSkill(skill []byte, version string) ([]byte, string, error) {
	lines, index, start, end, err := skillVersionLine(skill)
	if err != nil {
		return nil, "", err
	}
	line := lines[index]
	raw := line[start:end]
	value := version
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		value = string(raw[0]) + version + string(raw[0])
	}
	lines[index] = line[:start] + value + line[end:]
	return []byte(strings.Join(lines, "")), unquote(raw), nil
}

func unquote(raw string) string {
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		return raw[1 : len(raw)-1]
	}
	return raw
}

func releaseKind(log string) string {
	kind := ""
	for _, commit := range strings.Split(log, "\x00") {
		title, body, _ := strings.Cut(strings.TrimPrefix(commit, "\n"), "\n")
		switch {
		case breakingBodyPattern.MatchString(body), breakingTitlePattern.MatchString(title):
			return "major"
		case featureTitlePattern.MatchString(title):
			kind = "minor"
		case fixTitlePattern.MatchString(title) && kind == "":
			kind = "patch"
		}
	}
	return kind
}

func nextTag(current, kind string) (string, error) {
	if current == "" {
		return "v1.0.0", nil
	}
	currentVersion, ok := parseSemver(current)
	if !ok {
		return "", fmt.Errorf("invalid current tag: %s", current)
	}
	switch kind {
	case "major":
		currentVersion.major++
		currentVersion.minor = 0
		currentVersion.patch = 0
	case "minor":
		currentVersion.minor++
		currentVersion.patch = 0
	case "patch":
		currentVersion.patch++
	default:
		return "", fmt.Errorf("unknown release kind: %s", kind)
	}
	return currentVersion.String(), nil
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
