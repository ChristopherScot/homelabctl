# Clients for APIs you do not own

`homelabctl client` generates typed Go and TypeScript clients from
somebody else's OpenAPI spec — Zoom's, say — so you call their API
through generated code instead of a hand-written wrapper.

```sh
homelabctl client generate
```

## How this differs from `regen`

`regen` keeps a service's clients level with the spec **that service
owns**. You write `openapi.yml`, CI fails when the generated code drifts
from it, and the fix is to regenerate.

A third-party spec inverts all of that. You cannot edit it, you get a new
one whenever they publish one, and it may not generate at all. So the
spec is treated as an input: fetched, vendored, corrected, and
regenerated on demand.

Use `regen` for your own services. Use `client` for everyone else's.

## Getting started

Declare what you consume in `clients.yaml`:

```yaml
clients:
  zoom:
    spec: https://example.com/zoom/openapi.json
    vendor: specs/zoom.json
    patches: [patches/zoom.yaml]
    out: ./internal/zoomapi
    languages: [go]
    package: zoomapi
```

| field | meaning |
|---|---|
| `spec` | a local path, or an https URL to fetch |
| `vendor` | where a fetched spec is stored. Required for a URL |
| `patches` | patch files applied in order before generating |
| `out` | where generated code goes |
| `languages` | `go`, `ts`, or both. Defaults to `go` |
| `package` | Go package name. Defaults to the output directory's name |

Then:

```sh
homelabctl client generate              # every client
homelabctl client generate zoom         # just one
homelabctl client list                  # what is declared
```

To try a spec before committing to it, skip the manifest:

```sh
homelabctl client generate --spec ./zoom.json --out ./internal/zoomapi
```

## Why the spec is vendored

A remote spec is downloaded once and committed. Generating reads the
committed copy.

This is deliberate. A build that fetches from a third party fails when
they are down, and silently changes behaviour when they ship a new spec.
Vendoring turns an upstream change into a reviewable diff.

```sh
homelabctl client generate zoom --refresh
```

re-fetches, and the diff on `specs/zoom.json` shows exactly what they
changed.

## Patches

A spec you do not own may not generate. Zoom's, for example, is Swagger
2.0 and contains four defects that stop ogen outright.

Conversion to OpenAPI 3 is automatic. The rest go in a patch file, which
is committed, applied in order, and reapplied on every generation:

```yaml
- op: rename-component
  from: parameters/RegistrantStatus
  to: parameters/RegistrantStatusParam
  why: collides with components/schemas/RegistrantStatus; ogen cannot emit two types with one name

- op: set
  path: /components/schemas/UserSettingsRecording/properties/auto_delete_cmr_days/default
  value: 0
  why: upstream declares type integer with default false
```

Three operations are available:

- **`set`** — replace a value at a JSON Pointer path. It will not create
  a key that does not exist, so a typo'd path is an error rather than a
  field the upstream spec never had.
- **`remove`** — delete a key.
- **`rename-component`** — move a component and repoint every `$ref` to
  it. Use this for name collisions; a plain `set` would leave dangling
  references.

`why` is required. A patch whose reason is not written down cannot be
retired, because nobody can tell later whether upstream still has the
problem.

Patch paths are written against **OpenAPI 3**, after conversion. A
Swagger 2.0 document keeps `parameters` and `definitions` at the root,
but by the time patches run those have moved under `components`.

### Finding what to patch

Run the command and read the error. ogen names the offending location:

```
expected schema type is "string", default value is int64
```

Each fix tends to reveal the next, so expect to iterate a few times on
first setup. Once the patch file is complete it keeps working until
upstream changes that part of the spec.

## When upstream changes

```sh
homelabctl client generate zoom --refresh
```

Review the diff on the vendored spec, then regenerate. If a patch no
longer applies, the command fails naming the patch — which usually means
upstream fixed the defect and the patch can be deleted.

## Generated code is not yours to edit

Everything under `out` is rewritten on every run, and `--clean` means a
removed endpoint's code disappears rather than lingering as a method the
API no longer has.

`ogen.yml` in that directory is the exception: it is written once and is
yours to tune afterwards.
