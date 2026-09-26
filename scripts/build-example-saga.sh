#!/usr/bin/env bash
# Package this repository's Saga, change.saga, as a deterministic, platform-neutral
# release asset.
#
# Usage: scripts/build-example-saga.sh [dist-dir]
#
# Produces <dist-dir>/change-saga-example.saga.zip and a matching checksum
# sidecar. The archive expands to one change.saga directory and contains regular,
# read-only data files only.
set -euo pipefail

if [ "$#" -gt 1 ]; then
	echo "usage: $0 [dist-dir]" >&2
	exit 2
fi

dist="${1:-dist}"
caller_dir="$PWD"
repo_root="$(cd "$(dirname "$0")/.." && pwd)"
case "$dist" in
/*) ;;
*) dist="$caller_dir/$dist" ;;
esac
cd "$repo_root"

checkout_commit="$(git rev-parse --verify 'HEAD^{commit}' 2>/dev/null)" || {
	echo "error: cannot determine release commit" >&2
	exit 1
}
commit_full="${CHANGE_SAGA_COMMIT:-$checkout_commit}"
if [[ ! "$commit_full" =~ ^[[:xdigit:]]{40}([[:xdigit:]]{24})?$ ]]; then
	echo "error: CHANGE_SAGA_COMMIT must be a full 40- or 64-character hexadecimal object ID" >&2
	exit 2
fi
resolved_commit="$(git rev-parse --verify "${commit_full}^{commit}" 2>/dev/null)" || {
	echo "error: release commit does not identify a commit in this checkout" >&2
	exit 2
}
if [ "$resolved_commit" != "$checkout_commit" ]; then
	echo "error: release commit does not match the checked-out source" >&2
	exit 2
fi
if ! git diff --quiet HEAD -- change.saga ||
	[ -n "$(git ls-files --others --exclude-standard -- change.saga)" ]; then
	echo "error: example Saga packaging requires a clean change.saga at the checked-out commit" >&2
	exit 1
fi

required_go="$(tr -d '\r\n' < .go-version)"
case "$required_go" in
go*) ;;
*) required_go="go$required_go" ;;
esac
go_env=(env GOENV=off GOFLAGS= GOTOOLCHAIN=local GOEXPERIMENT= GOAMD64=v1 GOARM64=v8.0)
actual_go="$("${go_env[@]}" go env GOVERSION)"
if [ "$actual_go" != "$required_go" ]; then
	echo "error: example Saga packaging requires $required_go, found $actual_go" >&2
	exit 1
fi

source_date_epoch="${SOURCE_DATE_EPOCH:-}"
if [ -z "$source_date_epoch" ]; then
	source_date_epoch="$(git show -s --format=%ct "$resolved_commit" 2>/dev/null)" || {
		echo "error: cannot determine commit timestamp; set SOURCE_DATE_EPOCH" >&2
		exit 1
	}
fi
if [[ ! "$source_date_epoch" =~ ^[0-9]+$ ]]; then
	echo "error: SOURCE_DATE_EPOCH must be a non-negative integer" >&2
	exit 2
fi

mkdir -p -- "$dist"
dist_abs="$(cd "$dist" && pwd)"
archive="change-saga-example.saga.zip"
sidecar="$archive.sha256"
bundle_stage="$(mktemp -d "$dist_abs/.example-saga-bundle.XXXXXX")"
publishing=0
had_archive=0
had_sidecar=0

restore_release_file() { # restore_release_file <backup> <destination>
	local backup="$1" destination="$2" restore
	restore="$bundle_stage/.release-restore.$$"
	cp -p "$backup" "$restore" && mv -f "$restore" "$destination"
}

cleanup() {
	local status=$?
	if [ "$publishing" -eq 1 ]; then
		if [ "$had_archive" -eq 1 ]; then
			restore_release_file "$bundle_stage/previous.archive" "$dist_abs/$archive" || true
		else
			rm -f "$dist_abs/$archive"
		fi
		if [ "$had_sidecar" -eq 1 ]; then
			restore_release_file "$bundle_stage/previous.sha256" "$dist_abs/$sidecar" || true
		else
			rm -f "$dist_abs/$sidecar"
		fi
	fi
	rm -rf "$bundle_stage"
	exit "$status"
}
trap cleanup EXIT
trap 'exit 130' INT TERM HUP

"${go_env[@]}" go run -mod=readonly ./internal/cmd/sagaarchive \
	"$bundle_stage/$archive" "$source_date_epoch" change.saga change.saga
"$repo_root/scripts/sha256.sh" "$bundle_stage/$archive" > "$bundle_stage/$sidecar"
chmod 0644 "$bundle_stage/$sidecar"

if [ -e "$dist_abs/$archive" ] || [ -L "$dist_abs/$archive" ]; then
	cp -p "$dist_abs/$archive" "$bundle_stage/previous.archive"
	had_archive=1
fi
if [ -e "$dist_abs/$sidecar" ] || [ -L "$dist_abs/$sidecar" ]; then
	cp -p "$dist_abs/$sidecar" "$bundle_stage/previous.sha256"
	had_sidecar=1
fi
publishing=1
mv -f "$bundle_stage/$archive" "$dist_abs/$archive"
mv -f "$bundle_stage/$sidecar" "$dist_abs/$sidecar"
publishing=0

echo "wrote $dist_abs/$archive"
cat "$dist_abs/$sidecar"
