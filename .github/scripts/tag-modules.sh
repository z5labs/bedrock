#!/usr/bin/env bash
#
# tag-modules.sh tags every nested Go module in this repository with the
# version of a release, so the Go toolchain can resolve that version of each
# of them. A nested module's versions are tags prefixed with its directory:
# fit/v0.22.0 is version v0.22.0 of github.com/z5labs/bedrock/fit.
#
# Every module is tagged with the same version on every release, whether or not
# it changed, after OpenTelemetry Go: one version identifies a consistent set of
# modules. The GitHub release creates the root tag; this script creates the
# rest, all pointing at the commit the root tag points to.
#
# Usage:
#
#   tag-modules.sh [--dry-run | --list] [--commit <rev>] [--remote <name>] <version>
#
#   <version>        the release tag, vMAJOR.MINOR.PATCH with an optional
#                    pre-release suffix, e.g. v0.22.0 or v0.22.0-rc.1
#   --commit <rev>   the release commit; defaults to the commit the local tag
#                    <version> points to. A dry run before the release exists
#                    names it explicitly, e.g. --commit origin/main
#   --remote <name>  the remote the tags are read from and pushed to; defaults
#                    to origin
#   --dry-run        print the tags that would be created and create none
#   --list           print the path of the root module and of every nested
#                    module, one per line, and touch no remote
#
# Before any tag is created, the script checks that <version> is a Go module
# version of major version 0 or 1, that every nested module's path is its
# directory under the root module's path, and that no nested tag already exists
# at a different commit. A nested tag that already exists at the release commit
# is skipped, so a re-run is safe. The tags it does create are pushed in one
# atomic push, so a failure leaves no partial set.
#
# A nested module is every go.mod tracked at the release commit except the
# root's own and any the Go tool ignores: one under a testdata directory or
# under a directory whose name begins with . or _.

set -euo pipefail

usage() {
	echo "usage: tag-modules.sh [--dry-run | --list] [--commit <rev>] [--remote <name>] <version>" >&2
	exit 2
}

die() {
	echo "tag-modules: $*" >&2
	exit 1
}

mode=push
commit=
remote=origin
version=

while [ $# -gt 0 ]; do
	case "$1" in
	--dry-run)
		[ "$mode" = push ] || usage
		mode=dry-run
		;;
	--list)
		[ "$mode" = push ] || usage
		mode=list
		;;
	--commit)
		[ $# -ge 2 ] || usage
		commit=$2
		shift
		;;
	--remote)
		[ $# -ge 2 ] || usage
		remote=$2
		shift
		;;
	-*)
		usage
		;;
	*)
		[ -z "$version" ] || usage
		version=$1
		;;
	esac
	shift
done
[ -n "$version" ] || usage

# The version must be one the Go toolchain accepts as a module version: a
# canonical semantic version with a leading v, an optional pre-release and no
# build metadata.
case "$version" in
*+*) die "$version is not a Go module version: build metadata (+...) is not allowed" ;;
esac
numeric='(0|[1-9][0-9]*)'
if ! [[ "$version" =~ ^v${numeric}\.${numeric}\.${numeric}(-([0-9A-Za-z.-]+))?$ ]]; then
	die "$version is not a Go module version: want vMAJOR.MINOR.PATCH with an optional -pre-release suffix"
fi
major=${BASH_REMATCH[1]}
prerelease=${BASH_REMATCH[5]}
if [ -n "$prerelease" ]; then
	IFS=. read -r -a identifiers <<<"$prerelease."
	for identifier in "${identifiers[@]}"; do
		if [ -z "$identifier" ]; then
			die "$version is not a Go module version: its pre-release has an empty identifier"
		fi
		if [[ "$identifier" =~ ^0[0-9]+$ ]]; then
			die "$version is not a Go module version: numeric pre-release identifier $identifier has a leading zero"
		fi
	done
	# "a." splits to a single identifier, so a trailing dot is checked here.
	case "$prerelease" in
	*.) die "$version is not a Go module version: its pre-release has an empty identifier" ;;
	esac
fi
if [ "$major" -gt 1 ]; then
	die "$version has major version $major: from v2 on every module path needs a /v$major suffix, which this repository does not have"
fi

if [ -z "$commit" ]; then
	commit=refs/tags/$version
fi
sha=$(git rev-parse --verify --quiet "$commit^{commit}") || die "$commit does not name a commit"

# module_path prints the module path declared by the go.mod at <path> in the
# release commit.
module_path() {
	git show "$sha:$1" | awk '
		$1 == "module" {
			path = $2
			gsub(/"/, "", path)
			print path
			exit
		}
	'
}

root=$(module_path go.mod)
[ -n "$root" ] || die "go.mod at $sha declares no module path"

# Discover the nested modules at the release commit, not at the head of any
# branch, so a module added after it is not tagged for it.
dirs=()
while IFS= read -r file; do
	[ "$file" = go.mod ] && continue
	case "$file" in
	*/go.mod) ;;
	*) continue ;;
	esac
	dir=${file%/go.mod}
	ignored=
	IFS=/ read -r -a parts <<<"$dir"
	for part in "${parts[@]}"; do
		case "$part" in
		testdata | .* | _*)
			ignored=$part
			break
			;;
		esac
	done
	if [ -n "$ignored" ]; then
		echo "skip $file: under $ignored, which the Go tool ignores" >&2
		continue
	fi
	dirs+=("$dir")
done < <(git ls-tree -r --name-only "$sha")

# Every nested module's tag resolves only if its path is its directory under
# the root module's path. Every mismatch is reported before anything happens.
mismatched=0
for dir in "${dirs[@]}"; do
	path=$(module_path "$dir/go.mod")
	if [ "$path" != "$root/$dir" ]; then
		echo "tag-modules: $dir/go.mod declares module ${path:-<none>}, want $root/$dir: its tag $dir/$version would never resolve" >&2
		mismatched=1
	fi
done
[ "$mismatched" = 0 ] || exit 1

if [ "$mode" = list ]; then
	echo "$root"
	for dir in "${dirs[@]}"; do
		echo "$root/$dir"
	done
	exit 0
fi

echo "release $version at $sha"

# The commit every existing tag on the remote points to, peeled through any
# annotated tag object.
remote_tags=$(git ls-remote --tags "$remote") || die "could not list the tags of $remote"
tagged_commit() {
	awk -v ref="refs/tags/$1" '
		$2 == ref { sha = $1 }
		$2 == ref "^{}" { peeled = $1 }
		END { print (peeled != "" ? peeled : sha) }
	' <<<"$remote_tags"
}

refspecs=()
conflicts=0
for dir in "${dirs[@]}"; do
	tag=$dir/$version
	existing=$(tagged_commit "$tag")
	if [ -z "$existing" ]; then
		refspecs+=("$sha:refs/tags/$tag")
		if [ "$mode" = dry-run ]; then
			echo "would create $tag"
		fi
	elif [ "$existing" = "$sha" ]; then
		echo "exists $tag at the release commit, skipping"
	else
		echo "tag-modules: $tag already exists at $existing, not at the release commit $sha" >&2
		conflicts=1
	fi
done
[ "$conflicts" = 0 ] || exit 1

if [ "${#refspecs[@]}" = 0 ]; then
	echo "nothing to create"
	exit 0
fi

if [ "$mode" = dry-run ]; then
	echo "dry run: created no tags"
	exit 0
fi

git push --atomic "$remote" "${refspecs[@]}"
for refspec in "${refspecs[@]}"; do
	echo "created ${refspec#*:refs/tags/}"
done
