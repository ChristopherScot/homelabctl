package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"go/format"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	runtimes "github.com/ChristopherScot/homelabctl/internal/runtime"
)

// The subprocess uses the production updater and a local release server. Its
// command records its actual build version and process state after startup.
const updateProbe = `package main
import (
 "encoding/json"
 "fmt"
 "io"
 "net/http"
 "os"
 "strings"
 "time"
)
var Version string
func version() string { return Version }
type transport struct{}
func (transport) RoundTrip(r *http.Request) (*http.Response, error) {
 if r.URL.Host == "api.github.com" {
  u := os.Getenv("UPDATE_TEST_SERVER")
  r.URL.Scheme = "http"
  r.URL.Host = strings.TrimPrefix(u, "http://")
 }
 return http.DefaultTransport.RoundTrip(r)
}
func main() {
 releaseClient = &http.Client{Transport: transport{}, Timeout: 100*time.Millisecond}
 if err := autoUpdate(); err != nil { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
 input, _ := io.ReadAll(os.Stdin)
 cwd, _ := os.Getwd()
 json.NewEncoder(os.Stdout).Encode(map[string]any{
  "version": version(), "args": os.Args, "cwd": cwd, "input": string(input),
  "env": os.Getenv("UPDATE_TEST_VALUE"), "marker": os.Getenv(updateRestartEnv), "pid": os.Getpid(),
 })
 fmt.Fprintln(os.Stderr, "command stderr")
 os.Exit(23)
}
`

func archiveBinary(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tr := tar.NewWriter(gz)
	if err := tr.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := tr.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestAutoUpdateReexecutesOriginalCommand(t *testing.T) {
	dir := t.TempDir()
	source, err := os.ReadFile("update.go")
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"update.go": source, "main.go": []byte(updateProbe)} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	builds := map[string][]byte{}
	for _, v := range []string{"v1.0.0", "v2.0.0", "dev"} {
		output := filepath.Join(dir, v)
		cmd := exec.Command("go", "build", "-ldflags", "-X main.Version="+v, "-o", output, filepath.Join(dir, "main.go"), filepath.Join(dir, "update.go"))
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("build: %v\n%s", err, out)
		}
		builds[v], err = os.ReadFile(output)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, scenario := range []string{"update", "current", "offline", "timeout", "broken-download", "corrupt-archive", "dev"} {
		t.Run(scenario, func(t *testing.T) {
			var checks, downloads atomic.Int32
			archive := archiveBinary(t, "homelabctl", builds["v2.0.0"])
			if scenario == "corrupt-archive" {
				archive[len(archive)-8] ^= 0xff
			}
			var server *httptest.Server
			server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/binary" {
					downloads.Add(1)
					if scenario == "broken-download" {
						http.Error(w, "unavailable", 503)
						return
					}
					w.Write(archive)
					return
				}
				checks.Add(1)
				if scenario == "offline" {
					http.Error(w, "unavailable", 503)
					return
				}
				if scenario == "timeout" {
					time.Sleep(250 * time.Millisecond)
				}
				tag := "v2.0.0"
				if scenario == "current" {
					tag = "v1.0.0"
				}
				fmt.Fprintf(w, `[{"tag_name":%q,"assets":[{"name":%q,"browser_download_url":%q}]}]`, tag, assetName(), server.URL+"/binary")
			}))
			defer server.Close()
			work := t.TempDir()
			binary := filepath.Join(work, "homelabctl")
			initial := "v1.0.0"
			if scenario == "dev" {
				initial = "dev"
			}
			if err := os.WriteFile(binary, builds[initial], 0755); err != nil {
				t.Fatal(err)
			}
			alias := filepath.Join(work, "alias")
			if err := os.Symlink(binary, alias); err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			args := []string{"regen", "--config", "a path/config.yaml", "--", "literal $value", ""}
			run := func(wantVersion string, wantChecks int32) {
				t.Helper()
				cmd := exec.CommandContext(ctx, alias, args...)
				cmd.Dir = work
				cmd.Env = append(os.Environ(), "UPDATE_TEST_SERVER="+server.URL, "UPDATE_TEST_VALUE=kept")
				cmd.Stdin = strings.NewReader("original stdin\n")
				var stdout, stderr bytes.Buffer
				cmd.Stdout = &stdout
				cmd.Stderr = &stderr
				err := cmd.Run()
				if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 23 {
					t.Fatalf("exit=%v stderr=%s", err, &stderr)
				}
				var got struct {
					Version                 string
					Args                    []string
					Cwd, Input, Env, Marker string
					PID                     int
				}
				decoder := json.NewDecoder(&stdout)
				if err := decoder.Decode(&got); err != nil {
					t.Fatalf("stdout: %v: %s", err, &stdout)
				}
				var extra any
				if err := decoder.Decode(&extra); err != io.EOF {
					t.Fatalf("command ran more than once or stdout polluted: %v", err)
				}
				resolvedWork, _ := filepath.EvalSymlinks(work)
				if got.Version != wantVersion || !reflect.DeepEqual(got.Args, append([]string{alias}, args...)) || got.Cwd != resolvedWork || got.Input != "original stdin\n" || got.Env != "kept" || got.Marker != "" || got.PID != cmd.Process.Pid {
					t.Fatalf("lost command state: %+v, pid=%d", got, cmd.Process.Pid)
				}
				if !strings.Contains(stderr.String(), "command stderr") {
					t.Fatal("stderr not preserved")
				}
				if checks.Load() != wantChecks {
					t.Fatalf("checks=%d want=%d", checks.Load(), wantChecks)
				}
			}
			want := initial
			count := int32(1)
			if scenario == "update" {
				want = "v2.0.0"
			}
			if scenario == "dev" {
				count = 0
			}
			run(want, count)
			if scenario == "update" {
				if downloads.Load() != 1 {
					t.Fatal("expected exactly one download")
				}
				// A separate invocation checks again; the one-use marker must not persist.
				run("v2.0.0", 2)
				if downloads.Load() != 1 {
					t.Fatal("reinstalled a current version")
				}
			}
			if _, err := os.Readlink(alias); err != nil {
				t.Fatal("update replaced the symlink")
			}
			if scenario != "update" {
				actual, err := os.ReadFile(binary)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(actual, builds[initial]) {
					t.Fatal("unsuccessful update changed binary")
				}
			}
		})
	}
}

func TestScaffoldsShareHomelabctlUpdater(t *testing.T) {
	own, err := os.ReadFile("update.go")
	if err != nil {
		t.Fatal(err)
	}
	own, err = format.Source(own)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"go-cli", "go-tui"} {
		rt, err := runtimes.Get(name)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range rt.Artifacts(runtimes.Params{Name: "homelabctl", Owner: "ChristopherScot", Module: "github.com/ChristopherScot/homelabctl"}).Files {
			if f.Path != "update.go" {
				continue
			}
			found = true
			rendered, err := format.Source([]byte(f.Body))
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(own, rendered) {
				t.Errorf("%s updater differs from tested homelabctl updater", name)
			}
		}
		if !found {
			t.Errorf("%s missing update.go", name)
		}
	}
}

func TestInstallArchiveRejectsInvalidEntries(t *testing.T) {
	for _, data := range [][]byte{archiveBinary(t, "other", []byte("new")), archiveBinary(t, "homelabctl", nil), []byte("bad gzip")} {
		path := filepath.Join(t.TempDir(), "homelabctl")
		if err := os.WriteFile(path, []byte("old"), 0755); err != nil {
			t.Fatal(err)
		}
		if err := installArchive(bytes.NewReader(data), path); err == nil {
			t.Fatal("accepted invalid archive")
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "old" {
			t.Fatalf("lost original: %q, %v", got, err)
		}
	}
}

func TestAutoUpdateLeavesExplicitUpdaterInControl(t *testing.T) {
	previousVersion, previousArgs, previousClient := Version, os.Args, releaseClient
	t.Cleanup(func() { Version, os.Args, releaseClient = previousVersion, previousArgs, previousClient })
	Version = "v1.0.0"
	t.Setenv(updateRestartEnv, "")
	releaseClient = &http.Client{Transport: updateRoundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("startup checked before the explicit update command")
		return nil, nil
	})}
	for _, args := range [][]string{{"homelabctl", "update"}, {"homelabctl", "update", "--check"}} {
		os.Args = args
		if err := autoUpdate(); err != nil {
			t.Fatal(err)
		}
	}
}

type updateRoundTripFunc func(*http.Request) (*http.Response, error)

func (f updateRoundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
