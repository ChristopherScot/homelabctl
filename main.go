// homelabctl scaffolds services and renders their Kubernetes manifests.
//
// One tool so there is one config schema and one place that encodes the
// cluster's conventions. Languages are plugins (internal/runtime); adding a
// runtime does not touch the manifest, Argo or CI logic.
package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// Version is set at build time via ldflags by the release workflow.
//
// `go install` passes no ldflags, so it stays empty there - but the
// module version it was installed at is embedded in the binary, and
// version() reads that instead. Without the fallback a go-installed
// binary reports "dev" and refuses to self-update.
var Version string

// version is what this binary should call itself: the ldflags value,
// else the module version it was built from, else "dev".
func version() string {
	var module string
	if bi, ok := debug.ReadBuildInfo(); ok {
		module = bi.Main.Version
	}
	return pickVersion(Version, module)
}

// pickVersion is version()'s choice, separated from the build-info
// lookup so it can be tested - a test binary has no module version of
// its own to stand in for one.
//
// Only a released version counts. A build from a working tree reports
// "(devel)", a pseudo-version derived from the commit, or a tag with
// "+dirty" appended; none of them names a release that exists, and
// updating from one would overwrite someone's own build with a release
// they did not ask for.
//
// semver.IsValid rejects "(devel)" and "". Canonical differs from the
// original exactly when there is build metadata, which is how "+dirty"
// is caught - it is a suffix on an otherwise real tag.
func pickVersion(ldflags, modver string) string {
	switch {
	case ldflags != "":
		return ldflags
	case !semver.IsValid(modver),
		semver.Canonical(modver) != modver,
		module.IsPseudoVersion(modver):
		return "dev"
	default:
		return modver
	}
}

func rootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "homelabctl",
		Short: "scaffold services and render their manifests",
		// Usage on every error buries the error itself; cobra still prints
		// usage for genuine usage mistakes.
		SilenceUsage: true,
		// Cobra prints the error itself; main prints it too, so without
		// this every failure appears twice.
		SilenceErrors: true,
	}
	root.AddCommand(initCmd(), renderCmd(), regenCmd(), diffCmd(), checkCmd(), statusCmd(), registerCmd(), imageCmd(), vaultCmd(), updateCmd(), versionCmd())

	// Cobra builds its own `completion` command during Execute, so it does
	// not exist yet here and cannot be extended in place. Force it to be
	// created now, then hang `install` off it.
	root.InitDefaultCompletionCmd()
	for _, c := range root.Commands() {
		if c.Name() == "completion" {
			c.AddCommand(completionInstallCmd("homelabctl"))
		}
	}
	return root
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print the version",
		Args:  cobra.NoArgs,
		Run:   func(_ *cobra.Command, _ []string) { fmt.Println(version()) },
	}
}

func main() {
	if err := rootCmd().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
