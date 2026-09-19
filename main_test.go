package main

// Tests for homelabctl itself, as opposed to scaffold_test.go, which holds
// the tests every go-cli starts with. Keeping the two apart is what makes
// drift from the template visible: if scaffold_test.go stops matching what
// `init` generates, this repo has fallen behind its own runtime.

import "testing"

// The root wires every subcommand; if one is dropped the CLI still compiles
// and the command silently disappears.
func TestRootHasExpectedCommands(t *testing.T) {
	want := map[string]bool{
		"init": false, "render": false, "check": false,
		"update": false, "version": false, "completion": false, "vault": false,
	}
	for _, c := range rootCmd().Commands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("root command is missing %q", name)
		}
	}
}

// Cobra builds its completion command during Execute, so `install` has to
// be attached after forcing it into existence. If that ordering regresses
// the subcommand silently disappears.
func TestCompletionInstallIsWired(t *testing.T) {
	for _, c := range rootCmd().Commands() {
		if c.Name() != "completion" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() == "install" {
				if sub.Flags().Lookup("file") == nil {
					t.Error("completion install has no --file flag")
				}
				return
			}
		}
		t.Fatal("completion has no install subcommand")
	}
	t.Fatal("no completion command on root")
}
