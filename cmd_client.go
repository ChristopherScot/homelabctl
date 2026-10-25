package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// `client` generates typed clients for an API somebody ELSE owns.
//
// The rest of this tool is for a service whose spec you write: init
// scaffolds the server, regen keeps the clients level with it, and CI
// fails when they drift. None of that applies to Zoom. You do not own
// their spec, cannot fix it, and get a new one whenever they ship one -
// so the spec is an input to be fetched and corrected rather than a
// source file to be maintained.
//
// That inversion is why this is its own command group rather than a flag
// on regen: regen asks "is the generated code level with MY spec", and
// here there is no my-spec to be level with.
const clientManifestName = "clients.yaml"

// clientManifest is clients.yaml: every third-party API this repo
// generates a client for.
//
// Committed, because a generated client is only reproducible if the spec
// it came from and the patches that made it generatable are both pinned.
// Without the file, regenerating after an upstream change means
// rediscovering by hand which edits the spec needed.
type clientManifest struct {
	Clients map[string]clientSpec `yaml:"clients"`
}

type clientSpec struct {
	// Spec is a local path (relative to the manifest) or an https URL.
	Spec string `yaml:"spec"`

	// Vendor is where a fetched spec is written. A remote spec is
	// vendored rather than fetched at generation time so a build does
	// not depend on a third party being up, and so a diff shows when
	// their API changed.
	Vendor string `yaml:"vendor,omitempty"`

	// Patches are files of apispec.Op applied in order before
	// generating.
	Patches []string `yaml:"patches,omitempty"`

	// Out is the directory generated code is written to.
	Out string `yaml:"out"`

	// Languages to emit: go, ts. Defaults to go.
	Languages []string `yaml:"languages,omitempty"`

	// Package is the Go package name for the generated code. Defaults
	// to the client's key in the manifest.
	Package string `yaml:"package,omitempty"`
}

func clientCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "client",
		Short: "generate typed clients for APIs you do not own",
		Long: `Generate typed Go and TypeScript clients from a third-party OpenAPI
spec - an API somebody else publishes and you only consume.

Unlike regen, which keeps a service's clients level with the spec that
service owns, this treats the spec as an input: fetched, vendored,
patched into a state the generators accept, and regenerated on demand.`,
	}
	cmd.AddCommand(clientGenerateCmd(), clientListCmd())
	return cmd
}

func clientListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "list the clients declared in clients.yaml",
		Args:  cobra.NoArgs,
		RunE: func(*cobra.Command, []string) error {
			path, m, err := loadClientManifest("")
			if err != nil {
				return err
			}
			fmt.Println(path)
			for _, name := range sortedClients(m) {
				c := m.Clients[name]
				fmt.Printf("  %-16s %s -> %s\n", name, c.Spec, c.Out)
			}
			return nil
		},
	}
}

func clientGenerateCmd() *cobra.Command {
	var (
		manifest  string
		specFlag  string
		outFlag   string
		pkgFlag   string
		langFlag  []string
		patchFlag []string
		refresh   bool
	)
	cmd := &cobra.Command{
		Use:   "generate [name...]",
		Short: "generate clients from a third-party OpenAPI spec",
		Long: `Generate typed clients from a third-party OpenAPI spec.

With no arguments, generates every client in clients.yaml. Named
arguments generate only those. With --spec, generates one client
without a manifest, which is the way to try a spec before committing
to it.

A remote spec is vendored on first use and reused after that, so a
build does not depend on a third party being reachable. --refresh
re-fetches it, which is how an upstream change enters the repo: as a
reviewable diff.`,
		Example: `  homelabctl client generate
  homelabctl client generate zoom --refresh
  homelabctl client generate --spec ./zoom.json --out ./internal/zoomapi`,
		RunE: func(_ *cobra.Command, args []string) error {
			// --spec is the no-manifest path: one client, named by its
			// output directory, nothing committed. It exists so a spec
			// can be tried before anyone decides to keep it.
			if specFlag != "" {
				if len(args) > 0 {
					return fmt.Errorf("--spec generates one client; drop the name argument or use the manifest")
				}
				if outFlag == "" {
					return fmt.Errorf("--out is required with --spec")
				}
				name := pkgFlag
				if name == "" {
					name = packageFromDir(outFlag)
				}
				return generateClient(".", name, clientSpec{
					Spec:      specFlag,
					Patches:   patchFlag,
					Out:       outFlag,
					Languages: langFlag,
					Package:   name,
				}, refresh)
			}

			path, m, err := loadClientManifest(manifest)
			if err != nil {
				return err
			}
			dir := filepath.Dir(path)

			names := args
			if len(names) == 0 {
				names = sortedClients(m)
			}
			for _, name := range names {
				c, ok := m.Clients[name]
				if !ok {
					return fmt.Errorf("no client %q in %s; declared: %v", name, path, sortedClients(m))
				}
				if err := generateClient(dir, name, c, refresh); err != nil {
					return fmt.Errorf("%s: %w", name, err)
				}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&manifest, "manifest", "", "path to clients.yaml (default: nearest one)")
	cmd.Flags().StringVar(&specFlag, "spec", "", "generate one client from this spec path or URL, without a manifest")
	cmd.Flags().StringVar(&outFlag, "out", "", "output directory, with --spec")
	cmd.Flags().StringVar(&pkgFlag, "package", "", "Go package name, with --spec (default: the output directory's name)")
	cmd.Flags().StringSliceVar(&langFlag, "languages", nil, "languages to emit: go, ts (default: go)")
	cmd.Flags().StringSliceVar(&patchFlag, "patch", nil, "patch files to apply, with --spec")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "re-fetch a remote spec even if it is already vendored")
	return cmd
}

func sortedClients(m *clientManifest) []string {
	out := make([]string, 0, len(m.Clients))
	for name := range m.Clients {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// packageFromDir is the Go package name implied by an output directory.
// Hyphens are legal in a directory name and not in a package name.
func packageFromDir(dir string) string {
	base := filepath.Base(filepath.Clean(dir))
	return strings.ReplaceAll(base, "-", "")
}

func loadClientManifest(explicit string) (string, *clientManifest, error) {
	path := explicit
	if path == "" {
		found, start, err := findUpwards(clientManifestName)
		if err != nil {
			return "", nil, err
		}
		if found == "" {
			return "", nil, fmt.Errorf("no %s in %s or any parent up to the repository root.\n"+
				"create one, or pass --manifest", clientManifestName, start)
		}
		path = found
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", nil, err
	}
	var m clientManifest
	// KnownFields, as config.yaml does: a typo'd key would otherwise
	// decode cleanly and do nothing. `languges: [ts]` silently
	// generating Go only is the kind of failure that gets blamed on the
	// generator.
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	if err := d.Decode(&m); err != nil && !errors.Is(err, io.EOF) {
		return "", nil, fmt.Errorf("parsing %s: %w", path, err)
	}
	if len(m.Clients) == 0 {
		return "", nil, fmt.Errorf("%s declares no clients", path)
	}
	return path, &m, nil
}
