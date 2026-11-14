package main

import (
	"os"
	"regexp"
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

		// "Node.js 24+" in the install table, "**Node 24 or newer**"
		// in the prose - which wraps across a line, so anything after
		// the number is not matched on. Every "Node <n>" a reader
		// could act on has to name the floor.
		asks := regexp.MustCompile(`Node(?:\.js)? (\d+)`).FindAllStringSubmatch(body, -1)
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
