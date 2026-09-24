// Command release inspects the local checkout. Its plan subcommand prints a
// non-mutating release plan and gates on the agent skill stamp; its
// stamp-skill subcommand writes the expected metadata.version into the skill.
package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
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

// gitRunner runs a git subcommand and returns its stdout.
type gitRunner func(args ...string) (string, error)

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr, git))
}

func run(args []string, stdout, stderr io.Writer, git gitRunner) int {
	if len(args) < 1 || (args[0] != "plan" && args[0] != "stamp-skill") {
		_, _ = fmt.Fprintln(stderr, usage)
		return 64
	}
	command := args[0]

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(stderr)
	jsonOutput := false
	if command == "plan" {
		flags.BoolVar(&jsonOutput, "json", false, "write the plan as JSON")
	}
	manifestPath := flags.String("manifest", "release/readiness.json", "release readiness manifest")
	skillPath := flags.String("skill", "skills/kaneo-cli/SKILL.md", "agent skill carrying the metadata.version stamp")
	if err := flags.Parse(args[1:]); err != nil {
		return 64
	}
	fail := func(err error) int {
		_, _ = fmt.Fprintln(stderr, "release "+command+":", err)
		return 1
	}

	ready, err := loadReadiness(*manifestPath)
	if err != nil {
		return fail(err)
	}
	if err := requireFullHistory(git); err != nil {
		return fail(err)
	}
	current, err := currentTag(git)
	if err != nil {
		return fail(err)
	}
	log, err := commitLog(git, current)
	if err != nil {
		return fail(err)
	}
	entry, err := makePlan(ready, current, log)
	if err != nil {
		return fail(err)
	}
	skill, err := os.ReadFile(*skillPath)
	if err != nil {
		return fail(fmt.Errorf("read agent skill: %w", err))
	}

	if command == "stamp-skill" {
		expected := expectedSkillVersion(entry)
		if expected == "" {
			return fail(errors.New("no current tag and no releasable change, so there is no expected skill version"))
		}
		stamped, old, err := stampSkill(skill, expected)
		if err != nil {
			return fail(err)
		}
		if err := os.WriteFile(*skillPath, stamped, 0o644); err != nil { //nolint:gosec // SKILL.md is a public, version-controlled document.
			return fail(fmt.Errorf("write agent skill: %w", err))
		}
		_, _ = fmt.Fprintf(stderr, "%s metadata.version: %s -> %s\n", *skillPath, display(old), expected)
		return 0
	}

	entry.SkillVersion, err = checkSkill(entry, skill)
	if err != nil {
		return fail(fmt.Errorf("%s: %w", *skillPath, err))
	}
	if jsonOutput {
		data, err := json.Marshal(entry)
		if err != nil {
			return fail(err)
		}
		if _, err := fmt.Fprintln(stdout, string(data)); err != nil {
			return fail(fmt.Errorf("write plan: %w", err))
		}
		return 0
	}
	if _, err := fmt.Fprintf(stdout, "enabled: %t\ncurrent tag: %s\nnext tag: %s\nrelease: %t\nreason: %s\nrequired evidence: %d\nskill version: %s\n", entry.Enabled, display(entry.CurrentTag), display(entry.NextTag), entry.Release, entry.Reason, entry.EvidenceSize, display(entry.SkillVersion)); err != nil {
		return fail(fmt.Errorf("write plan: %w", err))
	}
	return 0
}

// requireFullHistory refuses shallow clones: without every tag reachable the
// planner would mistake the checkout for a first release.
func requireFullHistory(git gitRunner) error {
	output, err := git("rev-parse", "--is-shallow-repository")
	if err != nil {
		return err
	}
	if strings.TrimSpace(output) == "true" {
		return errors.New("shallow clone: the release plan needs full history and tags; run `git fetch --unshallow --tags`")
	}
	return nil
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

func currentTag(git gitRunner) (string, error) {
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

func commitLog(git gitRunner, current string) (string, error) {
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

// skillVersionLine locates the version entry that is a direct child of the
// top-level metadata map in the leading YAML frontmatter. It returns the
// split lines, the line index, and the byte offsets of the raw value within
// that line; for an empty value start == end marks the end of the key.
func skillVersionLine(skill []byte) (lines []string, index, start, end int, err error) {
	lines = strings.SplitAfter(string(skill), "\n")
	if len(lines) == 0 || !isFence(lines[0]) {
		return nil, 0, 0, 0, errors.New("skill has no leading YAML frontmatter")
	}
	inMetadata := false
	childIndent := -1
	for i := 1; i < len(lines); i++ {
		if isFence(lines[i]) {
			return nil, 0, 0, 0, errors.New("skill frontmatter has no metadata.version")
		}
		content := strings.TrimRight(lines[i], "\r\n")
		trimmed := strings.TrimLeft(content, " \t")
		if trimmed == "" || trimmed[0] == '#' {
			continue
		}
		indent := len(content) - len(trimmed)
		if indent == 0 {
			key := strings.TrimRight(content[:valueEnd(content, 0)], " \t")
			inMetadata = key == "metadata:"
			childIndent = -1
			continue
		}
		if !inMetadata {
			continue
		}
		if childIndent < 0 {
			childIndent = indent
		}
		if indent < childIndent {
			inMetadata = false
			continue
		}
		if indent > childIndent || !strings.HasPrefix(trimmed, "version:") {
			continue
		}
		keyEnd := indent + len("version:")
		end = len(strings.TrimRight(content[:valueEnd(content, keyEnd)], " \t"))
		if end <= keyEnd {
			return lines, i, keyEnd, keyEnd, nil
		}
		start = keyEnd
		for content[start] == ' ' || content[start] == '\t' {
			start++
		}
		return lines, i, start, end, nil
	}
	return nil, 0, 0, 0, errors.New("skill frontmatter is not terminated")
}

func isFence(line string) bool {
	return strings.TrimRight(line, " \t\r\n") == "---"
}

// valueEnd returns the offset of a trailing " #" comment at or after from,
// ignoring '#' inside quotes, or len(content) when there is none.
func valueEnd(content string, from int) int {
	var quote byte
	for i := from; i < len(content); i++ {
		c := content[i]
		switch {
		case quote == '"' && c == '\\':
			i++
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '#' && i > 0 && (content[i-1] == ' ' || content[i-1] == '\t'):
			return i
		}
	}
	return len(content)
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
// quoting, any trailing comment, and every other byte. An empty value is
// written as ` "X.Y.Z"`. It returns the previous unquoted value.
func stampSkill(skill []byte, version string) ([]byte, string, error) {
	lines, index, start, end, err := skillVersionLine(skill)
	if err != nil {
		return nil, "", err
	}
	line := lines[index]
	raw := line[start:end]
	value := version
	switch {
	case raw == "":
		value = ` "` + version + `"`
	case len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0]:
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
