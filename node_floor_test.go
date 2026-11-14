package main

import (
	"errors"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/ChristopherScot/homelabctl/internal/runtime"
)

// The Node version the docs ask for is the one the scaffold enforces.
//
// They disagreed: the README said 22 or newer, while package.json.tmpl
// declared ">=24" alongside engine-strict=true, so anyone who followed
// the install table got a scaffold whose dependencies would not
// install - npm error EBADENGINE, at the end of `init`, after the repo
// had already been written.
//
// Both numbers are read rather than written down here. A test that
// pinned "24" would have to be edited to raise the floor and would
// pass while the docs stayed stale, which is the failure it exists to
// catch.
func TestDocsAskForTheNodeTheScaffoldEnforces(t *testing.T) {
	r, err := runtime.Get("node-service")
	if err != nil {
		t.Fatal(err)
	}
	var pkg string
	for _, f := range r.Artifacts(runtime.Params{Name: "svc", Module: "m", Owner: "o", Port: 3000}).Files {
		if f.Path == "package.json" {
			pkg = f.Body
		}
	}
	if pkg == "" {
		t.Fatal("node-service scaffolds no package.json")
	}

	engines := regexp.MustCompile(`"node"\s*:\s*">=\s*(\d+)"`).FindStringSubmatch(pkg)
	if engines == nil {
		t.Fatalf("no engines.node floor in the scaffolded package.json:\n%s", pkg)
	}
	floor := engines[1]

	// Every place a reader is told which Node to install.
	for _, doc := range []string{"README.md", "docs/runtimes/node-service.md"} {
		b, err := os.ReadFile(doc)
		if err != nil {
			t.Fatal(err)
		}
		body := string(b)

		// Two spellings, both of which a reader acts on:
		// "| [Node.js 24+](...)" in the install table, and
		// "Node must be **version 24 or newer**" in the prose. A
		// pattern anchored tight to "Node <n>" matched only the
		// first, so reverting the prose line alone - the very line
		// this test was written for - left it green.
		asks := regexp.MustCompile(`Node[^0-9\n]{0,24}?(\d+)`).FindAllStringSubmatch(body, -1)
		if asks == nil {
			t.Errorf("%s names no Node version; the scaffold needs >=%s", doc, floor)
			continue
		}
		for _, a := range asks {
			if a[1] != floor {
				t.Errorf("%s asks for %q, but the scaffold enforces >=%s", doc, a[0], floor)
			}
		}
	}
}

// The "dependencies did not resolve" hint names the command that
// actually resolves them, for the runtime in hand.
//
// It used to print `cd . && go mod tidy` unconditionally, so a node
// service - which fails that step with npm, not Go - was handed a Go
// command that does nothing for it. Running the suggestion left the
// tree exactly as broken.
//
// Asserting on Lock() rather than on a literal: the hint is right
// because it IS the lock command, and a test naming "npm install"
// would pass while the hint printed something else that happened to
// contain it.
func TestTheUnresolvedHintNamesTheRuntimesOwnLockCommand(t *testing.T) {
	for _, name := range runtime.Names() {
		r, err := runtime.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		// Asserted rather than skipped. A runtime with no lock
		// command still gets the "will not build yet" header and the
		// cause, with no remediation between them - worse than the
		// wrong command this test exists to prevent. Every runtime
		// declares one today, so this holds the line.
		lock := r.Lock()
		if len(lock) == 0 {
			t.Errorf("%s declares no Lock(), so the warning would name no way to fix it", name)
			continue
		}
		t.Run(name, func(t *testing.T) {
			// The function init calls, not a rebuild of it: a test
			// that assembles the hint itself passes no matter what
			// unresolvedWarning prints. It does not cover the call
			// site - inlining this back into runInit would leave
			// this green.
			var buf strings.Builder
			unresolvedWarning(&buf, r, ".", errors.New("boom"))
			got := buf.String()

			// Every lock command has to appear, not just the first:
			// go-service resolves in two languages and a user told
			// only about Go is still stuck on the client.
			for _, cmd := range lock {
				if !strings.Contains(got, strings.Join(cmd, " ")) {
					t.Errorf("the hint does not name %q:\n%s", strings.Join(cmd, " "), got)
				}
			}
			// A Go command must not be suggested to a runtime that
			// does not lock with Go, which is the bug itself.
			locksWithGo := false
			for _, cmd := range lock {
				if len(cmd) > 0 && cmd[0] == "go" {
					locksWithGo = true
				}
			}
			if !locksWithGo && strings.Contains(got, "go mod tidy") {
				t.Errorf("%s does not resolve with Go, but the hint says go mod tidy:\n%s", name, got)
			}
		})
	}
}
