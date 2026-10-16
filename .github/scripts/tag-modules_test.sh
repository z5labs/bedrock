#!/usr/bin/env bash
#
# tag-modules_test.sh tests tag-modules.sh against throwaway repositories, each
# with a bare repository as its remote. It needs git and nothing else, and
# touches nothing outside a temporary directory.
#
#   .github/scripts/tag-modules_test.sh

set -euo pipefail

script=$(cd "$(dirname "$0")" && pwd)/tag-modules.sh
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

export GIT_CONFIG_GLOBAL=/dev/null GIT_CONFIG_NOSYSTEM=1
export GIT_AUTHOR_NAME=test GIT_AUTHOR_EMAIL=test@example.com
export GIT_COMMITTER_NAME=test GIT_COMMITTER_EMAIL=test@example.com

failures=0
fail() {
	echo "FAIL: $*" >&2
	failures=$((failures + 1))
}

# newrepo creates a repository whose remote origin is a bare repository, with
# one go.mod per <dir>=<module path> argument (. for the root), commits it,
# pushes it and tags it v0.22.0 on both sides, and prints its directory.
newrepo() {
	local dir
	dir=$(mktemp -d "$tmp/repo.XXXXXX")
	git init -q --bare "$dir.git"
	git init -q -b main "$dir"
	git -C "$dir" remote add origin "$dir.git"
	local arg
	for arg in "$@"; do
		mkdir -p "$dir/${arg%%=*}"
		printf 'module %s\n\ngo 1.27.0\n' "${arg#*=}" >"$dir/${arg%%=*}/go.mod"
	done
	git -C "$dir" add -A
	git -C "$dir" commit -q -m initial
	git -C "$dir" tag v0.22.0
	git -C "$dir" push -q origin main v0.22.0
	echo "$dir"
}

# run runs tag-modules.sh in <repo> with the remaining arguments, leaving its
# combined output in $out and its exit status in $status.
run() {
	local repo=$1
	shift
	status=0
	out=$(cd "$repo" && "$script" "$@" 2>&1) || status=$?
}

remote_tags() {
	git -C "$1" ls-remote --tags origin | awk '{ print $2 }' | sort | tr '\n' ' '
}

expect_status() {
	[ "$status" = "$1" ] || fail "$2: exit $status, want $1; output:"$'\n'"$out"
}

expect_output() {
	grep -qF -- "$1" <<<"$out" || fail "$2: output lacks \"$1\"; output:"$'\n'"$out"
}

root=github.com/z5labs/bedrock

# Discovery tags nested modules and skips the root, testdata, . and _ dirs.
repo=$(newrepo .=$root fit=$root/fit a/b=$root/a/b \
	fit/testdata/x=example.com/x _tools=example.com/tools a/.hidden/m=example.com/m)
run "$repo" --dry-run v0.22.0
expect_status 0 "dry run"
expect_output "would create fit/v0.22.0" "dry run"
expect_output "would create a/b/v0.22.0" "dry run"
expect_output "skip fit/testdata/x/go.mod: under testdata" "dry run"
expect_output "skip _tools/go.mod: under _tools" "dry run"
expect_output "skip a/.hidden/m/go.mod: under .hidden" "dry run"
expect_output "dry run: created no tags" "dry run"
if grep -qE 'would create (v0\.22\.0|fit/testdata|_tools|a/\.hidden)' <<<"$out"; then
	fail "dry run: proposes a tag it should not; output:"$'\n'"$out"
fi
[ "$(remote_tags "$repo")" = "refs/tags/v0.22.0 " ] || fail "dry run: remote tags changed: $(remote_tags "$repo")"
[ "$(git -C "$repo" tag | tr '\n' ' ')" = "v0.22.0 " ] || fail "dry run: local tags changed"

out=$(cd "$repo" && "$script" --list v0.22.0 2>/dev/null) || fail "list: exit $?"
[ "$out" = "$root"$'\n'"$root/a/b"$'\n'"$root/fit" ] || fail "list: got"$'\n'"$out"

# Tagging pushes every nested tag at the release commit.
run "$repo" v0.22.0
expect_status 0 "tag"
expect_output "created fit/v0.22.0" "tag"
expect_output "created a/b/v0.22.0" "tag"
want="refs/tags/a/b/v0.22.0 refs/tags/fit/v0.22.0 refs/tags/v0.22.0 "
[ "$(remote_tags "$repo")" = "$want" ] || fail "tag: remote tags are $(remote_tags "$repo"), want $want"
release=$(git -C "$repo" rev-parse v0.22.0)
for tag in fit/v0.22.0 a/b/v0.22.0; do
	got=$(git -C "$repo.git" rev-parse "$tag^{commit}")
	[ "$got" = "$release" ] || fail "tag: $tag at $got, want $release"
done

# A re-run against tags already at the release commit changes nothing.
run "$repo" v0.22.0
expect_status 0 "re-run"
expect_output "exists fit/v0.22.0 at the release commit, skipping" "re-run"
expect_output "nothing to create" "re-run"
[ "$(remote_tags "$repo")" = "$want" ] || fail "re-run: remote tags changed"

# The release commit defaults to the release tag, not to HEAD, and a module
# added after it is not tagged.
mkdir -p "$repo/later"
printf 'module %s\n' "$root/later" >"$repo/later/go.mod"
git -C "$repo" add -A
git -C "$repo" commit -q -m later
run "$repo" --dry-run v0.22.0
expect_status 0 "later module"
if grep -qF later <<<"$out"; then
	fail "later module: tagged a module added after the release; output:"$'\n'"$out"
fi

# A nested tag at a different commit fails the run, and nothing is pushed.
repo=$(newrepo .=$root fit=$root/fit a=$root/a)
git -C "$repo" commit -q --allow-empty -m other
git -C "$repo" push -q origin HEAD:refs/tags/fit/v0.22.0
run "$repo" v0.22.0
expect_status 1 "conflict"
expect_output "fit/v0.22.0 already exists at" "conflict"
[ "$(remote_tags "$repo")" = "refs/tags/fit/v0.22.0 refs/tags/v0.22.0 " ] || fail "conflict: pushed $(remote_tags "$repo")"

# An annotated tag is compared by the commit it points to.
repo=$(newrepo .=$root fit=$root/fit)
git -C "$repo" tag -a -m annotated fit/v0.22.0 v0.22.0
git -C "$repo" push -q origin fit/v0.22.0
run "$repo" v0.22.0
expect_status 0 "annotated"
expect_output "exists fit/v0.22.0 at the release commit, skipping" "annotated"

# Malformed versions, v2 and later, and mismatched module paths are refused
# before anything is created.
repo=$(newrepo .=$root fit=$root/fit)
for version in 0.22.0 v0.22 v0.22.0+build v01.2.3 v0.22.0- v0.22.0-rc..1 v0.22.0-rc. v0.22.0-01; do
	run "$repo" "$version"
	expect_status 1 "malformed $version"
	expect_output "$version is not a Go module version" "malformed $version"
done
run "$repo" v0.22.0+build
expect_output "build metadata" "build metadata"
for version in v0.22.0-rc.1 v1.0.0 v1.0.0-0.alpha-1; do
	git -C "$repo" tag "$version"
	run "$repo" --dry-run "$version"
	expect_status 0 "valid $version"
	expect_output "would create fit/$version" "valid $version"
done
run "$repo" v2.0.0
expect_status 1 "v2"
expect_output "v2.0.0 has major version 2" "v2"

repo=$(newrepo .=$root fit=$root/fit wrong=$root/right)
run "$repo" v0.22.0
expect_status 1 "mismatch"
expect_output "wrong/go.mod declares module $root/right, want $root/wrong" "mismatch"
[ "$(remote_tags "$repo")" = "refs/tags/v0.22.0 " ] || fail "mismatch: pushed $(remote_tags "$repo")"

# --commit names the release commit before the release tag exists.
repo=$(newrepo .=$root fit=$root/fit)
run "$repo" --dry-run --commit main v0.23.0
expect_status 0 "commit"
expect_output "would create fit/v0.23.0" "commit"
run "$repo" --dry-run v0.23.0
expect_status 1 "no release tag"
expect_output "refs/tags/v0.23.0 does not name a commit" "no release tag"

# Usage errors.
for args in "" "--dry-run --list v0.22.0" "--bogus v0.22.0" "v0.22.0 v0.23.0" "--commit"; do
	# shellcheck disable=SC2086 # the arguments are split on purpose
	run "$repo" $args
	expect_status 2 "usage: $args"
done

if [ "$failures" != 0 ]; then
	echo "$failures failure(s)" >&2
	exit 1
fi
echo "ok"
