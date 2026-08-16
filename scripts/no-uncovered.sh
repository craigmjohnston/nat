#!/bin/sh
# Fail if any statement of the module went unrun.
#
# `go test -cover` prints one decimal, so a single uncovered statement in a
# package of thousands reads as 100.0% — which is exactly how one hid here once.
# `go tool cover -func` rounds the same way, just per function.
#
# The coverage profile itself does not round. Every line after the header is
# `file:start.col,end.col numstmt count`, and a count of zero is a block nothing
# ran; printing those says where to look rather than what fraction was missed.
#
# It takes the profile a single `go test -coverprofile ./...` run produced,
# because the blocks have to be merged across packages before they are counted:
# a helper covered only by another package's tests is still covered.
set -eu

profile="${1:-coverage.out}"

awk 'NR > 1 && $NF == "0" { print; n++ }
     END { if (n) { printf "%d uncovered block(s)\n", n; exit 1 } }' "$profile"
