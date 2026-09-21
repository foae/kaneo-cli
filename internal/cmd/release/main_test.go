package main

import "testing"

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
