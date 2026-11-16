package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ChristopherScot/homelabctl/internal/runtime"
)

func TestInitLocksOnlyGeneratedDependencies(t *testing.T) {
	for _, spec := range []bool{false, true} {
		name := "without-spec"
		if spec {
			name = "with-spec"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			t.Chdir(dir)
			bin := t.TempDir()
			log := filepath.Join(t.TempDir(), "commands")
			t.Setenv("LOCK_TEST_LOG", log)
			t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			// Execute the real scaffold command path without fetching dependencies.
			// The shell's cd still fails if init locks a client it never generated.
			for _, tool := range []string{"go", "npm"} {
				body := "#!/bin/sh\nprintf '%s\\n' \"" + tool + " $*\" >> \"$LOCK_TEST_LOG\"\n"
				if err := os.WriteFile(filepath.Join(bin, tool), []byte(body), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			o := initOpts{name: "svc", owner: "tester", runtimeID: "go-service", noSpec: !spec, localOnly: true, yes: true}
			c, err := buildConfig(o)
			if err != nil {
				t.Fatal(err)
			}
			r, err := runtime.Get(o.runtimeID)
			if err != nil {
				t.Fatal(err)
			}
			if err := setupLocal(o, c, r, dir); err != nil {
				t.Fatal(err)
			}
			commands, err := os.ReadFile(log)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(commands), "go mod tidy\n") {
				t.Fatalf("Go dependencies were not locked: %s", commands)
			}
			if got := strings.Contains(string(commands), "npm install "); got != spec {
				t.Fatalf("npm invoked=%v, spec=%v: %s", got, spec, commands)
			}
			var warning strings.Builder
			unresolvedWarning(&warning, r, artifactParams(c, o.owner, o.parentRepo), dir, errors.New("dependency failure"))
			if got := strings.Contains(warning.String(), "npm install "); got != spec {
				t.Fatalf("warning suggests npm=%v, spec=%v: %s", got, spec, warning.String())
			}
		})
	}
}
