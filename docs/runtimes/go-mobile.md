# go-mobile

An Android app written in Go. You build an `.apk`, put it on a phone,
and open it — no app store, no review, no account.

```sh
homelabctl init myapp --runtime go-mobile
```

Not deployed to the cluster. Like a [`go-cli`](go-cli.md), it is built by
CI and published as a downloadable release asset.

## Android only

There is no iPhone build, and this is not an oversight.

Apple will not let you install an app on an iPhone without a signing
certificate from them. Without a paid developer account ($99/year) the
best you can do expires after **seven days**, after which the app stops
opening until you rebuild and reinstall it. Building that into a template
would mean scaffolding a weekly chore.

If you get a developer account later, Gio does support iOS — the Go code
below does not change. What is missing is the release pipeline, which
needs a Mac to run on.

## The interface is Go

Gio draws its own buttons, text and layout straight to the screen. There
is no Android toolkit underneath, and you write no Java or Kotlin.

The practical upshot: **`go run .` opens the app in a window on your
laptop.** Same code, same layout, no emulator, no device, no cable. That
is the loop you develop in; the phone is only for checking it feels right
under a thumb.

The tradeoff is that it looks like Gio rather than like an Android app.
No system-native widgets and no Material You theming.

## What you get

```
myapp/
├── main.go             the window, the event loop, and the layout
├── state.go            your app's logic
├── scaffold_test.go    tests for state.go
├── go.mod / go.sum
├── VERSION
└── .github/workflows/  builds and releases the .apk
```

Run it right now:

```sh
cd myapp
go run .
```

A window opens with a title, a button and a counter. That is a complete,
working app — every piece you need is in it.

## How the app is structured

Gio is **immediate mode**. There is no widget tree to build and keep in
sync. Every frame, your `layout` function runs and draws whatever your
state currently says. Change a field, and the next frame shows it.

Three pieces, in `main.go`:

```go
type ui struct {
	count  int              // your state
	button widget.Clickable // and the widgets that change it
}
```

`loop` waits for events and draws a frame when one arrives. `layout`
describes what the frame looks like — a vertical `layout.Flex` holding a
heading, a line of text and a button.

Reading a click looks slightly odd and is worth knowing:

```go
for u.button.Clicked(gtx) {
	u.count++
}
```

Asking whether it was clicked also *consumes* the click, which is why it
is a loop and why you ask exactly once per frame.

## Where your logic goes

`state.go`, not `main.go`.

Anything that draws needs a real frame to run, and CI has no screen — so
a test of the layout would need an emulator. Anything in `state.go` is a
plain function over plain values, and `go test ./...` runs it anywhere.

The scaffold shows the seam with a deliberately trivial example:

```go
func describe(count int) string
```

`layout` calls it and draws the result. The rule to follow: **layout
reads state and draws it; every decision about *what* to draw is a
function in `state.go`.** Keep to that and your app stays testable
without a device.

## Adding to the interface

Everything comes from `gioui.org/widget/material`, already themed:

| you want | draw it with | state field on `ui` |
|---|---|---|
| a heading | `material.H4(th, "text")` | none |
| body text | `material.Body1(th, "text")` | none |
| a button | `material.Button(th, &u.button, "Tap me")` | `widget.Clickable` |
| a text box | `material.Editor(th, &u.editor, "hint")` | `widget.Editor` |
| a checkbox | `material.CheckBox(th, &u.check, "label")` | `widget.Bool` |
| a scrolling list | `material.List(th, &u.list)` | `widget.List` |

Add the state field to `ui`, draw it in `layout`, and read it the way the
button is read. The state type is not always named after the widget — a
checkbox is a `widget.Bool`. The [Gio documentation](https://gioui.org/) has
the full set.

Touch, scrolling and the on-screen keyboard are handled for you — a
`widget.Clickable` responds to a tap exactly as it responds to a click.

## The whole repo is yours

Same as `go-cli`: no `config.yaml`, so nothing regenerates. Every file is
yours to edit, and a later change to homelabctl's conventions will not
reach your app.

## Releasing

Edit `VERSION` and push to `main`. CI builds the `.apk`, tags the
release and attaches it.

To install: open the release on your phone, download the `.apk`, and tap
it. Android will ask permission to install from this source the first
time.

### ⚠️ Upgrades need an uninstall first

Android identifies an app by its name *and* the key it was signed with,
and it refuses to replace an app whose key has changed. CI generates a
throwaway key on every run, so every release has a different one.

So: **uninstall the old version before installing a new one.** You will
lose anything the app had stored locally.

Fixing this properly means generating a signing key once, keeping it
somewhere safe, and giving it to CI — worth doing when the app has real
users, and not worth it before then.

### The build needs two JDKs

CI installs JDK 17, fetches the Android SDK, then drops to JDK 8 for
`gogio`. That is not belt-and-braces - the two tools genuinely disagree:

- `sdkmanager` refuses anything below 17
- `gogio`'s `d8` step is stuck on 8, and Gio's install guide says newer
  Java "will break the build"

Installing 8 first fails, because fetching the SDK is what runs
`sdkmanager`. The generated workflow already has the right order; it is
worth knowing before editing that section.

### Two couplings that fail quietly

Neither is checked at build time:

- **`VERSION` drives releases.** CI publishes only when the first line
  changes on `main`. Push app changes without touching it and no release
  appears.
- **The app id** (in the workflow, derived from your module path) is what
  Android matches on. Change it and a phone treats the next build as a
  completely different app, installing it alongside the old one.
