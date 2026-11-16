#!/usr/bin/env bash
# work.sh MYGO_DIR: writes feedbackui/go.work so that the module builds
# against the actnowkit next to it and the MyGo in MYGO_DIR (an app's
# third_party/mygo). go.work is not committed (.gitignore).
set -euo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
mygo="${1:-${MYGO_DIR:-}}"
if [ -z "$mygo" ]; then
	echo "usage: $0 <app>/third_party/mygo   (or MYGO_DIR=...)" >&2
	exit 2
fi
mygo="$(cd "$mygo" && pwd)"
[ -f "$mygo/go.mod" ] || { echo "no go.mod in $mygo" >&2; exit 2; }
cat >"$here/go.work" <<WORK
go 1.27.4

use (
	.
	..
)

replace github.com/egoist/mygo => $mygo

// the actnowkit next to it, whatever version go.mod asks (not tagged yet)
replace github.com/hehehai/actnowkit v0.22.0 => ../
WORK
echo "go.work: MyGo from $mygo"
