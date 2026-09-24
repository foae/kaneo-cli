package main

import (
	"strings"
	"testing"
)

func TestReleaseKind(t *testing.T) {
	tests := []struct {
		name string
		log  string
		want string
	}{
		{
			name: "fix title produces patch",
			log:  "fix(cli): return useful error\x00",
			want: "patch",
		},
		{
			name: "feature title produces minor",
			log:  "feat(api): add export\x00",
			want: "minor",
		},
		{
			name: "body conventional commit markers are ignored",
			log:  "docs: describe commit syntax\n\nfeat: example feature\nfix: example fix\nchore!: example breaking title\x00",
		},
		{
			name: "breaking change footer produces major",
			log:  "docs: document migration\n\nBREAKING CHANGE: the configuration format changed\x00",
			want: "major",
		},
		{
			name: "breaking change takes precedence across commits",
			log:  "fix: repair output\x00\nfeat: add command\x00\nchore!: remove legacy output\x00\n",
			want: "major",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := releaseKind(test.log); got != test.want {
				t.Fatalf("releaseKind() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestNextTagStableBumps(t *testing.T) {
	tests := []struct {
		name    string
		current string
		kind    string
		want    string
	}{
		{
			name: "first releasable change is policy fixed",
			kind: "patch",
			want: "v1.0.0",
		},
		{
			name:    "stable patch",
			current: "v1.2.3",
			kind:    "patch",
			want:    "v1.2.4",
		},
		{
			name:    "stable minor",
			current: "v1.2.3",
			kind:    "minor",
			want:    "v1.3.0",
		},
		{
			name:    "stable major",
			current: "v1.2.3",
			kind:    "major",
			want:    "v2.0.0",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := nextTag(test.current, test.kind)
			if err != nil {
				t.Fatalf("nextTag() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("nextTag() = %q, want %q", got, test.want)
			}
		})
	}
}

const stampedSkill = "---\nname: kaneo-cli\ndescription: Example.\nmetadata:\n  version: \"1.8.0\"\n---\n\n# Body\n\nversion: 9.9.9\n"

func TestParseSkillVersion(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		want    string
		wantErr bool
	}{
		{name: "quoted stamp", skill: stampedSkill, want: "1.8.0"},
		{name: "unquoted stamp with CRLF", skill: "---\r\nmetadata:\r\n  version: 1.2.3\r\n---\r\n", want: "1.2.3"},
		{name: "missing metadata", skill: "---\nname: kaneo-cli\n---\n\nmetadata:\n  version: 1.0.0\n", wantErr: true},
		{name: "version outside metadata ignored", skill: "---\nversion: 1.0.0\nmetadata:\n  author: x\n---\n", wantErr: true},
		{name: "body version lines ignored", skill: "---\nname: kaneo-cli\n---\n\n  version: 1.0.0\n", wantErr: true},
		{name: "malformed version", skill: "---\nmetadata:\n  version: \"v1.8\"\n---\n", wantErr: true},
		{name: "prefixed version", skill: "---\nmetadata:\n  version: v1.8.0\n---\n", wantErr: true},
		{name: "empty version", skill: "---\nmetadata:\n  version:\n---\n", wantErr: true},
		{name: "no frontmatter", skill: "# Title\n", wantErr: true},
		{name: "unterminated frontmatter", skill: "---\nname: kaneo-cli\n", wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := parseSkillVersion([]byte(test.skill))
			if test.wantErr {
				if err == nil {
					t.Fatalf("parseSkillVersion() = %q, want error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseSkillVersion() error = %v", err)
			}
			if got != test.want {
				t.Fatalf("parseSkillVersion() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestSkillStampGate(t *testing.T) {
	ready := readiness{RequiredEvidence: make([]readinessClause, 5)}
	tests := []struct {
		name    string
		current string
		log     string
		skill   string
		wantErr bool
	}{
		{name: "release matches next tag", current: "v1.7.0", log: "feat: add stamp\x00", skill: stampedSkill},
		{name: "release mismatch", current: "v1.8.0", log: "fix: repair\x00", skill: stampedSkill, wantErr: true},
		{name: "release with missing stamp", current: "v1.7.0", log: "feat: add stamp\x00", skill: "---\nname: kaneo-cli\n---\n", wantErr: true},
		{name: "no release matches current tag", current: "v1.8.0", log: "docs: prose\x00", skill: stampedSkill},
		{name: "no release mismatch", current: "v1.7.0", log: "docs: prose\x00", skill: stampedSkill, wantErr: true},
		{name: "no tags and no release skips check", log: "chore: init\x00", skill: "---\nname: kaneo-cli\n---\n"},
		{name: "no tags first release expects 1.0.0", log: "feat: init\x00", skill: stampedSkill, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			entry, err := makePlan(ready, test.current, test.log)
			if err != nil {
				t.Fatalf("makePlan() error = %v", err)
			}
			_, err = checkSkill(entry, []byte(test.skill))
			if test.wantErr {
				if err == nil {
					t.Fatal("checkSkill() succeeded, want error")
				}
				if !strings.Contains(err.Error(), "just stamp-skill") {
					t.Fatalf("checkSkill() error %q does not name the fix", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("checkSkill() error = %v", err)
			}
		})
	}
}

func TestStampSkill(t *testing.T) {
	tests := []struct {
		name    string
		skill   string
		version string
		want    string
		wantOld string
	}{
		{
			name:    "quoted value keeps quotes and body",
			skill:   stampedSkill,
			version: "1.9.0",
			want:    strings.Replace(stampedSkill, `version: "1.8.0"`, `version: "1.9.0"`, 1),
			wantOld: "1.8.0",
		},
		{
			name:    "unquoted CRLF value keeps line endings and trailing text",
			skill:   "---\r\nmetadata:\r\n  author: x\r\n  version: 1.2.3  \r\nname: n\r\n---\r\nversion: 1.2.3\r\n",
			version: "2.0.0",
			want:    "---\r\nmetadata:\r\n  author: x\r\n  version: 2.0.0  \r\nname: n\r\n---\r\nversion: 1.2.3\r\n",
			wantOld: "1.2.3",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, old, err := stampSkill([]byte(test.skill), test.version)
			if err != nil {
				t.Fatalf("stampSkill() error = %v", err)
			}
			if string(got) != test.want || old != test.wantOld {
				t.Fatalf("stampSkill() = %q, %q; want %q, %q", got, old, test.want, test.wantOld)
			}
			again, _, err := stampSkill(got, test.version)
			if err != nil {
				t.Fatalf("second stampSkill() error = %v", err)
			}
			if string(again) != string(got) {
				t.Fatalf("stampSkill() not idempotent: %q then %q", got, again)
			}
		})
	}
	if _, _, err := stampSkill([]byte("---\nname: kaneo-cli\n---\n"), "1.0.0"); err == nil {
		t.Fatal("stampSkill() without metadata.version succeeded, want error")
	}
}
