#!/bin/sh
# Builds the generated client beside this service, and every sibling
# service's, so npm can install them. Does not depend on the caller's
# cwd. The Dockerfile and `make deps` both run it from the service
# directory as `sh scripts/build-clients.sh`: init writes every file
# 0644, so there is no exec bit to rely on. It is the one place the
# recipe is written, so the two cannot disagree.
#
# A generated client ships TypeScript and compiles to dist/, which npm
# resolves its `main` into. A `file:` dependency's devDependencies are
# not installed by the consumer, so the client's own prepare/prepack
# never runs - nothing builds it, and tsc reports the package as
# missing entirely rather than unbuilt.
#
# Both layouts: clients/ts beside the service in a single-service repo,
# and a sibling service's in a monorepo. Every sibling's is built, not
# only the ones this service declares with `file:`.
set -eu

# The globs below are relative to the service, wherever this is run from.
cd "$(dirname "$0")/.."

for c in ./clients/ts ../*/clients/ts; do
  # A service that consumes no client matches neither glob and does
  # nothing: an unmatched glob stays literal in POSIX sh, and this
  # guard rejects it.
  [ -f "$c/package.json" ] || continue
  # npm ci, not npm install: the client ships a committed
  # package-lock.json, and npm install is free to rewrite it.
  #
  # -e above stops at the first client that fails to build. A for loop
  # otherwise exits with the LAST iteration's status - in either layout
  # that is an unmatched glob the guard skips with status 0 - so a
  # broken client would pass silently and surface later as the tsc
  # error above.
  ( cd "$c" && npm ci --no-audit --no-fund && npm run build )
done
