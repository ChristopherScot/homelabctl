#!/bin/sh
# Builds the generated clients this service depends on, so npm can
# install them. Does not depend on the caller's cwd. The Dockerfile and
# `make deps` both run it from the service directory as
# `sh scripts/build-clients.sh`: init writes every file 0644, so there
# is no exec bit to rely on. It is the one place the recipe is written,
# so the two cannot disagree.
#
# A generated client ships TypeScript and compiles to dist/, which npm
# resolves its `main` into. A `file:` dependency's devDependencies are
# not installed by the consumer, so the client's own prepare/prepack
# never runs - nothing builds it, and tsc reports the package as
# missing entirely rather than unbuilt. That is why this runs before
# `npm ci` rather than relying on any install hook.
#
# The list comes from this package.json's own `file:` dependencies,
# which is the same thing npm will link. A glob of ../*/clients/ts
# asked the filesystem what exists instead, and answered with
# directories this service does not depend on: in a single-service repo
# the service directory IS the repository root, so ../* reached every
# neighbouring checkout on the machine and ran npm inside it.
set -eu

# Relative paths below are the service's own, wherever this is run from.
cd "$(dirname "$0")/.."

# node, not grep: package.json is JSON, and a dependency's path can
# contain anything a path can. The image this runs in is node:alpine
# and `make deps` needs node anyway, so requiring it costs nothing.
#
# Printed one per line. A path with a newline in it would split, which
# is not a case worth carrying complexity for - npm could not install
# it either.
clients=$(node -e '
  const pkg = require("./package.json");
  const deps = { ...pkg.dependencies, ...pkg.devDependencies };
  for (const spec of Object.values(deps)) {
    if (typeof spec === "string" && spec.startsWith("file:")) {
      console.log(spec.slice(5));
    }
  }
')

# A service that consumes no client prints nothing and does nothing.
[ -n "$clients" ] || exit 0

# Word-split on newlines only, so a path with spaces survives. A for
# loop rather than `… | while read`, because the pipeline would put the
# body in a subshell and a failure there reaches this script only
# through set -e - the kind of indirection that let the old loop report
# success after a client failed to build.
IFS='
'
for c in $clients; do
  # A declared path that is not there is the author's mistake, not
  # something to skip silently: npm ci would fail on it moments later
  # with a worse message.
  if [ ! -f "$c/package.json" ]; then
    echo "build-clients: $c is declared as a file: dependency but has no package.json" >&2
    exit 1
  fi
  # npm ci, not npm install: the client ships a committed
  # package-lock.json, and npm install is free to rewrite it.
  ( cd "$c" && npm ci --no-audit --no-fund && npm run build )
done
