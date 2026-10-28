package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// goRuntimes are the runtimes whose Makefile runs the Go toolchain,
// and so the ones whose `make test` must refuse ogen's constructor.
var goRuntimes = []string{"go-service", "go-cli", "go-tui", "go-mobile"}

// api.NewClient is ogen's constructor: http.DefaultClient, no timeout,
// no retries. api.New installs all of it. Three of three consumers
// called the wrong one, so the scaffolded `make test` greps for it -
// and this runs that Makefile rather than reading it, with `go`
// stubbed out so only the grep can fail.
func TestGoMakefileTestRefusesOgensConstructor(t *testing.T) {
	for _, name := range goRuntimes {
		t.Run(name, func(t *testing.T) {
			dir := renderMakefile(t, name)

			// Generated code is where NewClient lives; a mention there
			// is not a caller's mistake and must not trip the check.
			writeFile(t, filepath.Join(dir, "api", "oas_client_gen.go"),
				"package api\n\n// Prefer api.NewClient(serverURL) over NewClient.\nfunc NewClient() {}\n")
			writeFile(t, filepath.Join(dir, "main.go"),
				"package main\n\nfunc main() { c, _ := api.New(url, \"svc\") }\n")

			if out, err := makeTest(t, dir); err != nil {
				t.Fatalf("make test failed with no bare constructor in sight:\n%s", out)
			}

			writeFile(t, filepath.Join(dir, "main.go"),
				"package main\n\nfunc main() { c, _ := api.NewClient(url) }\n")

			out, err := makeTest(t, dir)
			if err == nil {
				t.Fatalf("make test passed a call to api.NewClient:\n%s", out)
			}
			if !strings.Contains(out, "main.go") || !strings.Contains(out, "api.New") {
				t.Errorf("the failure names neither the file nor the fix:\n%s", out)
			}
		})
	}
}

// renderMakefile writes a runtime's Makefile to a fresh directory and
// returns it.
func renderMakefile(t *testing.T, runtime string) string {
	t.Helper()
	r, err := Get(runtime)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for _, f := range r.Artifacts(testParams()).Files {
		if f.Path == "Makefile" {
			writeFile(t, filepath.Join(dir, "Makefile"), f.Body)
			return dir
		}
	}
	t.Fatalf("%s has no Makefile", runtime)
	return ""
}

// makeTest runs `make test` in dir with a `go` on PATH that always
// succeeds, so the Makefile's own checks are the only thing that can
// fail.
func makeTest(t *testing.T, dir string) (string, error) {
	t.Helper()
	bin := t.TempDir()
	writeFile(t, filepath.Join(bin, "go"), "#!/bin/sh\nexit 0\n")
	if err := os.Chmod(filepath.Join(bin, "go"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("make", "test")
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
