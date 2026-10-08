#!/bin/sh
# Validates and tests nat's embedded Claude Code mod with Claude Code itself.
# Needs `claude` on PATH; neither command signs in or reaches the network.
set -eu
mod="$(dirname "$0")/../mods/embedded"
claude plugin validate --strict "$mod"
claude plugin test "$mod"
