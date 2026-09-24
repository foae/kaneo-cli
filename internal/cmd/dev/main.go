// Command dev runs the repository's local development workflows.
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

const (
	goreleaserModule   = "github.com/goreleaser/goreleaser/v2@v2.18.2"
	govulncheckModule  = "golang.org/x/vuln/cmd/govulncheck@v1.8.0"
	golangciLintModule = "github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.13.2"
)

var crossTargets = []struct {
	goos   string
	goarch string
}{
	{goos: "linux", goarch: "amd64"},
	{goos: "linux", goarch: "arm64"},
	{goos: "darwin", goarch: "amd64"},
	{goos: "darwin", goarch: "arm64"},
	{goos: "windows", goarch: "amd64"},
	{goos: "windows", goarch: "arm64"},
}

func main() {
	if len(os.Args) != 2 {
		usage()
		os.Exit(2)
	}

	root, err := moduleRoot()
	if err != nil {
		fatal(err)
	}

	var runErr error
	switch os.Args[1] {
	case "check":
		runErr = check(root)
	case "fmt":
		runErr = format(root)
	case "lint":
		runErr = run(root, nil, "go", "run", golangciLintModule, "run", "./...")
	case "build":
		runErr = build(root, runtime.GOOS, runtime.GOARCH)
	case "cross":
		runErr = cross(root)
	case "snapshot":
		runErr = snapshot(root)
	case "hooks":
		runErr = installHooks(root)
	case "race":
		runErr = run(root, nil, "go", "test", "-race", "./...")
	case "vuln":
		runErr = run(root, nil, "go", "run", govulncheckModule, "./...")
	case "help", "-h", "--help":
		usage()
		return
	default:
		usage()
		os.Exit(2)
	}
	if runErr != nil {
		fatal(runErr)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: go run ./internal/cmd/dev <check|fmt|lint|build|cross|snapshot|hooks|race|vuln>")
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "dev:", err)
	os.Exit(1)
}

func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("could not find go.mod")
		}
		dir = parent
	}
}

func check(root string) error {
	if err := checkFormatting(root); err != nil {
		return err
	}
	for _, command := range [][]string{
		{"vet", "./..."},
		{"run", golangciLintModule, "run", "./..."},
		{"test", "./..."},
		{"mod", "tidy", "-diff"},
		{"run", "./internal/cmd/specinventory", "--check"},
		{"run", "./internal/cmd/release", "plan"},
	} {
		if err := run(root, nil, "go", command...); err != nil {
			return err
		}
	}
	return nil
}

func format(root string) error {
	files, err := goFiles(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}
	return run(root, nil, "gofmt", append([]string{"-w"}, files...)...)
}

func checkFormatting(root string) error {
	files, err := goFiles(root)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return nil
	}

	cmd := exec.Command("gofmt", append([]string{"-l"}, files...)...)
	cmd.Dir = root
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("gofmt -l: %w", err)
	}
	if output.Len() == 0 {
		return nil
	}
	fmt.Fprintln(os.Stderr, "files require gofmt:")
	fmt.Fprint(os.Stderr, output.String())
	return errors.New("formatting check failed")
}

func goFiles(root string) ([]string, error) {
	var files []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "dist", "vendor":
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasSuffix(entry.Name(), ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	return files, nil
}

func build(root, goos, goarch string) error {
	if err := os.MkdirAll(filepath.Join(root, "dist"), 0o755); err != nil {
		return err
	}
	name := "kaneo-cli_" + goos + "_" + goarch
	if goos == "windows" {
		name += ".exe"
	}
	return run(root, []string{"CGO_ENABLED=0", "GOOS=" + goos, "GOARCH=" + goarch}, "go", "build", "-trimpath", "-o", filepath.Join("dist", name), "./cmd/kaneo-cli")
}

func cross(root string) error {
	for _, target := range crossTargets {
		if err := build(root, target.goos, target.goarch); err != nil {
			return err
		}
	}
	return nil
}

func snapshot(root string) error {
	return run(root, nil, "go", "run", goreleaserModule, "release", "--snapshot", "--clean", "--config", ".goreleaser.yaml")
}

func installHooks(root string) error {
	if _, err := os.Stat(filepath.Join(root, ".githooks", "pre-commit")); err != nil {
		return fmt.Errorf("pre-commit hook is unavailable: %w", err)
	}
	current := exec.Command("git", "config", "--get", "core.hooksPath")
	current.Dir = root
	value, err := current.Output()
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			return fmt.Errorf("read existing hooks configuration: %w", err)
		}
	}
	if path := strings.TrimSpace(string(value)); path != "" && path != ".githooks" {
		return fmt.Errorf("refusing to replace existing core.hooksPath %q", path)
	}
	return run(root, nil, "git", "config", "--local", "core.hooksPath", ".githooks")
}

func run(root string, env []string, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s %s: %w", name, strings.Join(args, " "), err)
	}
	return nil
}
