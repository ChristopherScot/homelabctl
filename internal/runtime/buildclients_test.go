package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scripts/build-clients.sh builds what the service depends on, and
// nothing else.
//
// It used to glob ../*/clients/ts, which asks the filesystem what
// exists rather than what this service declares. That answered with
// the service's own client a second time (issue #55), with sibling
// services it does not depend on, and - in a single-service repo,
// where the service directory IS the repository root - with every
// neighbouring checkout on the developer's machine, running npm
// inside repositories that have nothing to do with this one
// (issue #62).
//
// This runs the real script with npm stubbed, because the bug was in
// what it DID. The recipe's text was correct throughout: the clients
// it reached were never named in it, only implied by a glob.
func TestNodeServiceBuildsOnlyDeclaredClients(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}
	script := fileBody(t, nodeServiceArtifacts(t), clientBuildScript)

	for _, tc := range []struct {
		name string
		// where the service sits, relative to the temp root
		service string
		// every client directory to create, relative to the temp root
		clients []string
		// file: paths the service declares, relative to itself
		declares []string
		// client directory names expected to be built, by owner
		want []string
	}{
		{
			// The one the glob could not express: a neighbouring
			// checkout is a sibling of the repository, so ../* found
			// it. Nothing declares it, so nothing builds it.
			name:     "single-service repo beside an unrelated checkout",
			service:  "repo",
			clients:  []string{"neighbour/clients/ts"},
			declares: nil,
			want:     nil,
		},
		{
			// Declared, so built - once, though the old glob saw it
			// twice (./clients/ts and ../repo/clients/ts).
			name:     "service depending on its own client",
			service:  "repo",
			clients:  []string{"repo/clients/ts"},
			declares: []string{"./clients/ts"},
			want:     []string{"repo"},
		},
		{
			// A monorepo sibling it depends on is built; one it does
			// not depend on is left alone.
			name:     "monorepo, one sibling of two declared",
			service:  "mono/services/web",
			clients:  []string{"mono/services/api/clients/ts", "mono/services/other/clients/ts"},
			declares: []string{"../api/clients/ts"},
			want:     []string{"api"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, c := range tc.clients {
				dir := filepath.Join(root, c)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				owner := filepath.Base(filepath.Dir(filepath.Dir(dir)))
				// A generated client has a build script; that is how
				// the script tells one from any other local package.
				writeFile(t, filepath.Join(dir, "package.json"),
					`{"name":"`+owner+`-client","version":"1.0.0","scripts":{"build":"tsc"}}`)
			}

			svc := filepath.Join(root, tc.service)
			writeFile(t, filepath.Join(svc, "scripts", "build-clients.sh"), script)

			var deps []string
			for i, d := range tc.declares {
				deps = append(deps, `"@o/c`+string(rune('0'+i))+`":"file:`+d+`"`)
			}
			writeFile(t, filepath.Join(svc, "package.json"),
				`{"name":"svc","version":"0.1.0","dependencies":{`+strings.Join(deps, ",")+`}}`)

			// npm prints where it ran, so the test counts real
			// invocations rather than trusting the script.
			bin := t.TempDir()
			npm := filepath.Join(bin, "npm")
			// writeFile is 0644; a stub has to be executable.
			if err := os.WriteFile(npm, []byte("#!/bin/sh\necho RAN \"$PWD\"\n"), 0o755); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("sh", "scripts/build-clients.sh")
			cmd.Dir = svc
			// A second PATH after the inherited one: os/exec keeps the
			// last, so the stub wins. The real PATH stays because the
			// script needs dirname and node.
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("build-clients.sh failed: %v\n%s", err, out)
			}

			// The script runs npm twice per client - ci, then run
			// build - so a client built once accounts for exactly two
			// invocations. Asserting the number rather than a range is
			// what catches a half-build: one call means the install
			// ran and the compile did not, leaving a client whose
			// dist/ never appears and which the consumer then reports
			// as missing entirely.
			const perClient = 2
			seen := map[string]int{}
			for _, l := range strings.Split(string(out), "\n") {
				dir, ok := strings.CutPrefix(l, "RAN ")
				if !ok {
					continue
				}
				if !strings.HasSuffix(dir, filepath.Join("clients", "ts")) {
					t.Errorf("npm ran somewhere that is not a client: %q", dir)
					continue
				}
				seen[filepath.Base(filepath.Dir(filepath.Dir(dir)))]++
			}

			for _, w := range tc.want {
				if n := seen[w]; n != perClient {
					t.Errorf("npm ran %d times in %s's client, want %d (ci, then run build):\n%s",
						n, w, perClient, out)
				}
				delete(seen, w)
			}
			for extra, n := range seen {
				t.Errorf("built %s's client (%d npm calls), which this service does not depend on:\n%s",
					extra, n, out)
			}
		})
	}
}

// A file: path that is not there fails the build, loudly.
//
// Skipping it instead would hand the problem to `npm ci` moments
// later, which reports the same mistake as a resolution failure
// naming a directory the author never typed. Worth its own case
// because the old glob could not have this bug: a glob that matches
// nothing has nothing to report.
func TestNodeServiceRefusesAFileDependencyThatIsNotThere(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}

	svc := t.TempDir()
	writeFile(t, filepath.Join(svc, "scripts", "build-clients.sh"),
		fileBody(t, nodeServiceArtifacts(t), clientBuildScript))
	writeFile(t, filepath.Join(svc, "package.json"),
		`{"name":"svc","version":"0.1.0","dependencies":{"@o/gone":"file:../nope/clients/ts"}}`)

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "scripts/build-clients.sh")
	cmd.Dir = svc
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("build-clients.sh passed a file: dependency that is not there:\n%s", out)
	}
	if !strings.Contains(string(out), "../nope/clients/ts") {
		t.Errorf("the failure does not name the missing path:\n%s", out)
	}
}

// A local package that is not a generated client is left alone.
//
// The list comes from file: dependencies, and not every one of those
// is a client - a shared utils package is a local package too. It has
// no build script, so running the client recipe on it fails on a
// script that is not there, with an error about the client build. The
// glob could not have this problem: it selected on the path. Skipping
// by "has no build script" keeps the loud failure for a path that is
// genuinely missing.
func TestNodeServiceSkipsALocalPackageThatIsNotAClient(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	if _, err := exec.LookPath("node"); err != nil {
		t.Skipf("node unavailable: %v", err)
	}

	root := t.TempDir()
	// A local package with no build script, beside one with one.
	writeFile(t, filepath.Join(root, "util", "package.json"),
		`{"name":"@o/util","version":"1.0.0","main":"index.js"}`)
	writeFile(t, filepath.Join(root, "api", "clients", "ts", "package.json"),
		`{"name":"@o/api-client","version":"1.0.0","scripts":{"build":"tsc"}}`)

	svc := filepath.Join(root, "web")
	writeFile(t, filepath.Join(svc, "scripts", "build-clients.sh"),
		fileBody(t, nodeServiceArtifacts(t), clientBuildScript))
	writeFile(t, filepath.Join(svc, "package.json"),
		`{"name":"svc","version":"0.1.0","dependencies":{`+
			`"@o/util":"file:../util",`+
			`"@o/api-client":"file:../api/clients/ts"}}`)

	bin := t.TempDir()
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte("#!/bin/sh\necho RAN \"$PWD\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command("sh", "scripts/build-clients.sh")
	cmd.Dir = svc
	cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build-clients.sh failed on a local package that is not a client: %v\n%s", err, out)
	}
	if strings.Contains(string(out), filepath.Join(root, "util")) {
		t.Errorf("ran npm in a local package with no build script:\n%s", out)
	}
	if !strings.Contains(string(out), filepath.Join("api", "clients", "ts")) {
		t.Errorf("did not build the client declared beside it:\n%s", out)
	}
}
