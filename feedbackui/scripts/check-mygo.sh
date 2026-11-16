#!/usr/bin/env bash
# check-mygo.sh [--test] [APPS_DIR]: builds and vets feedbackui against the
# MyGo each app vendors (APPS_DIR/<App>/third_party/mygo; APPS_DIR
# defaults to the directory holding the actnowkit checkout, or
# $APPS_DIR), for macOS (darwin/arm64) and the host; --test also runs the
# tests on the host. Every app replaces github.com/egoist/mygo with its
# own copy, so feedbackui must build with each one. Prints one line per
# app and the compiler's errors; exits 1 when one fails.
set -uo pipefail
here="$(cd "$(dirname "$0")/.." && pwd)"
kit="$(cd "$here/.." && pwd)"
test=0
if [ "${1:-}" = "--test" ]; then
	test=1
	shift
fi
apps_dir="${1:-${APPS_DIR:-$(cd "$kit/.." && pwd)}}"
apps=(Dicta Act Inboxly Pik HereNow CaptionCraft Paralign Auralink VoxtMini AudioSmith)
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
fail=0
for app in "${apps[@]}"; do
	mygo="$apps_dir/$app/third_party/mygo"
	if [ ! -f "$mygo/go.mod" ]; then
		printf '%-13s SKIP (no %s)\n' "$app" "$mygo"
		continue
	fi
	work="$tmp/$app.work"
	cat >"$work" <<WORK
go 1.27.4

use (
	$here
	$kit
)

replace github.com/egoist/mygo => $mygo

replace github.com/hehehai/actnowkit v0.22.0 => $kit
WORK
	log="$tmp/$app.log"
	status=ok
	(
		cd "$here" &&
			GOWORK="$work" go build ./... &&
			GOWORK="$work" go vet ./... &&
			GOWORK="$work" GOOS=darwin GOARCH=arm64 go build ./... &&
			GOWORK="$work" GOOS=darwin GOARCH=arm64 go vet ./... &&
			if [ $test = 1 ]; then GOWORK="$work" go test -count=1 ./...; fi
	) >"$log" 2>&1 || status=FAIL
	rev="$(git -C "$apps_dir/$app" log -1 --format=%h -- third_party/mygo 2>/dev/null)"
	printf '%-13s %-4s (third_party/mygo @ %s)\n' "$app" "$status" "${rev:-?}"
	if [ $status = FAIL ]; then
		fail=1
		sed 's/^/    /' "$log" | head -40
	fi
done
exit $fail
