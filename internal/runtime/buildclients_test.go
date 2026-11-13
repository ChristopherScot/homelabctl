package runtime

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The service's own client is built once, not twice.
//
// scripts/build-clients.sh loops over "./clients/ts ../*/clients/ts".
// The second glob also matches the service's own directory - as
// ../<service>/clients/ts in a monorepo, and as ../<repo>/clients/ts
// in a single-service repo - so the own client was installed and
// compiled twice on every make deps, test, build, run and image build
// (issue #55).
//
// This runs the real script with npm stubbed, because the bug is in
// what it DOES: the recipe's text was correct throughout, and the two
// matches differ only in spelling, so no assertion on the script's
// source could tell them apart.
func TestNodeServiceBuildsEachClientOnce(t *testing.T) {
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skipf("sh unavailable: %v", err)
	}
	script := fileBody(t, nodeServiceArtifacts(t), clientBuildScript)

	for _, tc := range []struct {
		name string
		// where the service sits, relative to the temp root
		service string
		// every client directory to create, relative to the temp root
		clients []string
		want    []string
	}{
		{
			// The service directory IS the repository root, so
			// ../*/clients/ts matches it back as ../<repo>/clients/ts.
			name:    "single-service repo",
			service: "repo",
			clients: []string{"repo/clients/ts"},
			want:    []string{"repo"},
		},
		{
			// A sibling's client is the reason the glob exists, so it
			// still has to be built - exactly once, alongside our own.
			name:    "monorepo with a sibling",
			service: "mono/services/web",
			clients: []string{"mono/services/web/clients/ts", "mono/services/api/clients/ts"},
			want:    []string{"api", "web"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			for _, c := range tc.clients {
				dir := filepath.Join(root, c)
				if err := os.MkdirAll(dir, 0o755); err != nil {
					t.Fatal(err)
				}
				// The name the stub reports is the client's owning
				// service directory, two levels up from clients/ts.
				owner := filepath.Base(filepath.Dir(filepath.Dir(dir)))
				writeFile(t, filepath.Join(dir, "package.json"),
					`{"name":"`+owner+`-client","version":"1.0.0"}`)
			}

			svc := filepath.Join(root, tc.service)
			writeFile(t, filepath.Join(svc, "scripts", "build-clients.sh"), script)

			// npm prints the directory it was run in, so the test
			// counts real invocations rather than trusting the script.
			bin := t.TempDir()
			writeFile(t, filepath.Join(bin, "npm"), "#!/bin/sh\necho RAN \"$PWD\"\n")
			if err := os.Chmod(filepath.Join(bin, "npm"), 0o755); err != nil {
				t.Fatal(err)
			}

			cmd := exec.Command("sh", "scripts/build-clients.sh")
			cmd.Dir = svc
			cmd.Env = append(os.Environ(), "PATH="+bin+string(os.PathListSeparator)+os.Getenv("PATH"))
			out, err := cmd.CombinedOutput()
			if err != nil {
				t.Fatalf("build-clients.sh failed: %v\n%s", err, out)
			}

			// npm is called twice per client (ci, then run build), so
			// count distinct directories and how often each appears.
			seen := map[string]int{}
			for _, l := range strings.Split(string(out), "\n") {
				dir, ok := strings.CutPrefix(l, "RAN ")
				if !ok {
					continue
				}
				// .../<service>/clients/ts -> <service>
				seen[filepath.Base(filepath.Dir(filepath.Dir(dir)))]++
			}

			for _, w := range tc.want {
				switch n := seen[w]; {
				case n == 0:
					t.Errorf("%s's client was never built:\n%s", w, out)
				case n > 2:
					t.Errorf("%s's client was built %d times, want once (npm runs twice per build: ci, then run build):\n%s",
						w, n/2, out)
				}
				delete(seen, w)
			}
			for extra := range seen {
				t.Errorf("built a client the service does not own: %s\n%s", extra, out)
			}
		})
	}
}
