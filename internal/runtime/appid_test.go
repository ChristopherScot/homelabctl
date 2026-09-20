package runtime

import "testing"

// An app id Android rejects fails at INSTALL time, on a phone, after
// CI has published the APK - so it is worth pinning here.
func TestAppID(t *testing.T) {
	for _, tc := range []struct {
		name   string
		module string
		want   string
	}{
		{"single repo", "github.com/owner/pokedex", "com.github.owner.pokedex"},
		{"monorepo path", "github.com/owner/pokemon/services/dex", "com.github.owner.pokemon.services.dex"},
		// A hyphen is legal in a repo name and not in a Java package
		// segment, which is what an app id is.
		{"hyphen becomes underscore", "github.com/owner/pokedex-tui", "com.github.owner.pokedex_tui"},
		// Android rejects a segment starting with a digit.
		{"leading digit is prefixed", "github.com/owner/2fa", "com.github.owner.a2fa"},
		{"uppercase is lowered", "github.com/ChristopherScot/Pokemon", "com.github.christopherscot.pokemon"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := Params{Module: tc.module, Name: "x"}
			if got := p.AppID(); got != tc.want {
				t.Errorf("AppID() = %q, want %q", got, tc.want)
			}
		})
	}
}

// No module - a Config built in Go rather than decoded from a repo.
// Android needs at least one dot, so a bare name gets a prefix.
func TestAppIDWithoutAModule(t *testing.T) {
	p := Params{Name: "pokedex"}
	if got := p.AppID(); got != "app.pokedex" {
		t.Errorf("AppID() = %q, want app.pokedex", got)
	}
}

func TestTitle(t *testing.T) {
	for name, want := range map[string]string{
		"pokedex":        "Pokedex",
		"pokedex-tui":    "Pokedex Tui",
		"my_cool_app":    "My Cool App",
		"pokedex-mobile": "Pokedex Mobile",
	} {
		if got := (Params{Name: name}).Title(); got != want {
			t.Errorf("Title(%q) = %q, want %q", name, got, want)
		}
	}
}
