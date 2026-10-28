package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// api.NewClient is ogen's constructor: http.DefaultClient, no timeout,
// no retries. api.New installs all of it. Three of three consumers
// called the wrong one (#28), so every Go runtime's `make test` greps
// for it - and this runs that Makefile rather than reading it, with
// `go` stubbed out so only the grep can fail.
func TestGoMakefileTestRefusesOgensConstructor(t *testing.T) {
	if _, err := exec.LookPath("make"); err != nil {
		t.Skipf("make unavailable: %v", err)
	}

	checked := 0
	for _, name := range Names() {
		makefile := renderedMakefile(t, name)
		// A runtime whose Makefile does not run the Go toolchain has
		// no Go callers to guard.
		if !strings.Contains(makefile, "go test") {
			continue
		}
		checked++
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, filepath.Join(dir, "Makefile"), makefile)

			// Generated code is where NewClient is defined, so a
			// mention there is not a caller's mistake. Nor is another
			// package whose name merely ends in "api".
			writeFile(t, filepath.Join(dir, "api", "oas_client_gen.go"), `package api

// Prefer api.NewClient(serverURL) over NewClient.
func NewClient() {}
`)
			writeFile(t, filepath.Join(dir, "main.go"), `package main

func main() {
	c, _ := api.New(url, "svc")
	d, _ := otherapi.NewClient(url)
}
`)
			if out, err := makeTest(t, dir); err != nil {
				t.Fatalf("make test failed with no bare constructor in sight:\n%s", out)
			}

			writeFile(t, filepath.Join(dir, "main.go"), `package main

func main() { c, _ := api.NewClient(url) }
`)
			out, err := makeTest(t, dir)
			if err == nil {
				t.Fatalf("make test passed a call to api.NewClient:\n%s", out)
			}
			// grep's own match line names the file; only the
			// Makefile's message names the fix.
			if !strings.Contains(out, "main.go") {
				t.Errorf("the failure does not name the offending file:\n%s", out)
			}
			if !strings.Contains(out, "Call api.New(") {
				t.Errorf("the failure does not say to call api.New:\n%s", out)
			}
		})
	}
	if checked == 0 {
		t.Fatal("no runtime's Makefile runs go test; the guard has nothing to protect")
	}
}

// renderedMakefile is the Makefile a runtime scaffolds.
func renderedMakefile(t *testing.T, runtime string) string {
	t.Helper()
	r, err := Get(runtime)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range r.Artifacts(testParams()).Files {
		if f.Path == "Makefile" {
			return f.Body
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
	if err := os.WriteFile(filepath.Join(bin, "go"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("make", "test")
	cmd.Dir = dir
	// A second PATH after the inherited one: os/exec keeps the last.
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
