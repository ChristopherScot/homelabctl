package main

import (
	"testing"
)

// A binary that cannot name itself cannot update itself: runUpdate
// refuses to act on "dev".
func TestVersionAlwaysAnswers(t *testing.T) {
	if version() == "" {
		t.Error("version() is empty; it must fall back to the build info, then to \"dev\"")
	}
}

func TestPickVersion(t *testing.T) {
	for _, tc := range []struct {
		name, ldflags, module, want string
	}{
		{"release binary", "v1.2.3", "v1.2.3", "v1.2.3"},
		// go install sets no ldflags, but the module version it was
		// installed at is embedded. Without this the binary reports
		// "dev" and refuses to update itself.
		{"go install", "", "v1.2.3", "v1.2.3"},
		{"local go build", "", "(devel)", "dev"},
		{"no build info at all", "", "", "dev"},
		// A build from a working tree, which go embeds as a
		// pseudo-version. It names no release, so updating from it
		// would overwrite someone's own build.
		{"pseudo-version", "", "v1.21.1-0.20261026015910-0fa740d72a1f", "dev"},
		// What `go install` reports for an untagged module, and the
		// form that slipped through a narrower check.
		{"untagged pseudo-version", "", "v0.0.0-20261026020159-df3906439210", "dev"},
		// A real prerelease tag is not a pseudo-version.
		{"prerelease tag", "", "v1.2.3-rc1", "v1.2.3-rc1"},
		{"dirty working tree", "", "v1.21.1-0.20261026015910-0fa740d72a1f+dirty", "dev"},
		// A clean tag with +dirty appended: what go embeds for a
		// working-tree build of a tagged checkout. IsPseudoVersion
		// says no to this one, so Canonical has to catch it.
		{"tagged but dirty", "", "v0.1.0+dirty", "dev"},
		{"not a version at all", "", "garbage", "dev"},
		{"ldflags wins", "v9.9.9", "v1.2.3", "v9.9.9"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := pickVersion(tc.ldflags, tc.module); got != tc.want {
				t.Errorf("pickVersion(%q, %q) = %q, want %q",
					tc.ldflags, tc.module, got, tc.want)
			}
		})
	}
}

func TestIsNewer(t *testing.T) {
	for _, tc := range []struct {
		latest, current string
		want            bool
		why             string
	}{
		{"v1.2.0", "v1.1.0", true, "ordinary bump"},
		{"v1.1.0", "v1.2.0", false, "older is not newer"},
		{"v1.1.0", "v1.1.0", false, "equal is not newer"},
		{"v1.1.1", "v1.1", true, "an omitted patch reads as .0"},

		// Double digits. A comparison that reads components pairwise
		// gets these backwards and then reports "already up to date"
		// forever - a failure nobody notices, because it looks like
		// there simply being no new release.
		{"v1.10.0", "v1.2.0", true, "10 is newer than 2, not older"},
		{"v1.2.0", "v1.10.0", false, "and the reverse still holds"},
		{"v2.0.0", "v1.99.99", true, "major wins over any minor"},

		// A prerelease sorts before its release.
		{"v1.0.0", "v1.0.0-rc1", true, "release supersedes its rc"},
		{"v1.0.0-rc1", "v1.0.0", false, "an rc does not supersede the release"},

		// Unparseable means "not newer": leaving someone on a working
		// binary beats talking them into replacing it.
		{"not-a-version", "v1.0.0", false, "unparseable latest"},
		{"v1.0.0", "garbage", false, "unparseable current"},
		{"dev", "v1.0.0", false, "a dev build is not a version"},
	} {
		if got := isNewer(tc.latest, tc.current); got != tc.want {
			t.Errorf("isNewer(%q, %q) = %v, want %v (%s)",
				tc.latest, tc.current, got, tc.want, tc.why)
		}
	}
}

// The caller hands isNewer whatever the tag and the ldflags say, which
// may or may not carry a v. semver.IsValid rejects the unprefixed
// spelling, and an invalid version means "not newer" - so if this
// normalisation broke, update would go silent rather than fail loudly.
func TestEnsureVAcceptsEitherSpelling(t *testing.T) {
	for _, in := range []string{"1.2.3", "v1.2.3"} {
		if got := ensureV(in); got != "v1.2.3" {
			t.Errorf("ensureV(%q) = %q, want %q", in, got, "v1.2.3")
		}
	}
	if !isNewer(ensureV("1.10.0"), ensureV("v1.2.0")) {
		t.Error("a mixed-spelling comparison did not reach semver intact")
	}
}
