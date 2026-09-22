package render

import (
	"strings"
	"testing"

	"github.com/ChristopherScot/homelabctl/internal/config"
)

// Every service that does not opt in renders exactly what it did
// before readyPath existed.
//
// This is the whole constraint on the change: pokedex is not the only
// scaffolded service. pokedex-web and pokedex-htmx serve only
// /healthz, approvald likewise, and go-shlink-redirector overrides
// the pair to /health. Pointing readiness at a /readyz they do not
// serve would fail every probe and take them out of their Service.
func TestProbesAreUnchangedWithoutReadyPath(t *testing.T) {
	for _, tc := range []struct {
		name   string
		probes *config.Probes
		want   string // the path BOTH probes must use
	}{
		// pokedex-web, pokedex-htmx, approvald: no probes block at all.
		{"unset", nil, config.DefaultProbePath},
		// An explicit block that only names path.
		{"path only", &config.Probes{Path: "/healthz"}, "/healthz"},
		// go-shlink-redirector, which moves the pair off the default.
		{"shlink's override", &config.Probes{Path: "/health"}, "/health"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := tc.probes.Readiness()
			if tc.probes == nil {
				// A nil block is completed before rendering; the
				// accessor must still answer safely rather than panic.
				if got != config.DefaultProbePath {
					t.Fatalf("Readiness() on a nil Probes = %q, want %q", got, config.DefaultProbePath)
				}
				return
			}
			if got != tc.want {
				t.Errorf("Readiness() = %q, want %q - a service that did not opt in "+
					"must keep the probe it had", got, tc.want)
			}
			if got != tc.probes.Path {
				t.Errorf("readiness %q and liveness %q differ for a service that "+
					"never asked them to", got, tc.probes.Path)
			}
		})
	}
}

// And opting in actually splits them.
func TestReadyPathSplitsTheProbes(t *testing.T) {
	p := &config.Probes{Path: "/healthz", ReadyPath: "/readyz"}
	if got := p.Readiness(); got != "/readyz" {
		t.Errorf("Readiness() = %q, want /readyz", got)
	}
	if p.Path != "/healthz" {
		t.Errorf("liveness moved to %q; readyPath must not touch it", p.Path)
	}
}

// The rendered manifest is what actually reaches the cluster, so
// assert on that rather than only on the accessor.
func probePaths(t *testing.T, c *config.Config) string {
	t.Helper()
	for _, o := range mustAll(t, mustConfig(t, c)) {
		if strings.Contains(o.Body, "readinessProbe") {
			return o.Body
		}
	}
	t.Fatal("no rendered output contains a readinessProbe")
	return ""
}

func TestRenderedManifestKeepsBothProbesTogetherByDefault(t *testing.T) {
	dep := probePaths(t, base())
	if n := strings.Count(dep, "path: "+config.DefaultProbePath); n < 2 {
		t.Errorf("manifest has %d %q probe paths, want both probes on it - a "+
			"service that did not opt in must be byte-identical to before",
			n, config.DefaultProbePath)
	}
}

// go-shlink-redirector's shape: both probes moved off the default
// together, which must keep working.
func TestAnOverriddenPathStillMovesBothProbes(t *testing.T) {
	c := base()
	c.Probes = &config.Probes{Path: "/health"}
	dep := probePaths(t, c)
	if n := strings.Count(dep, "path: /health"); n < 2 {
		t.Errorf("manifest has %d /health probe paths, want 2 - overriding path "+
			"alone must still move readiness AND liveness", n)
	}
	if strings.Contains(dep, config.DefaultProbePath) {
		t.Error("the default path leaked into a manifest that overrode it")
	}
}

// And opting in splits them.
func TestRenderedManifestSplitsWhenAsked(t *testing.T) {
	c := base()
	c.Probes = &config.Probes{Path: "/healthz", ReadyPath: "/readyz"}
	dep := probePaths(t, c)
	if !strings.Contains(dep, "path: /readyz") {
		t.Error("readyPath was set but /readyz is not in the manifest")
	}
	if !strings.Contains(dep, "path: /healthz") {
		t.Error("liveness should still be /healthz; readyPath must not move it")
	}
}
