#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Release's one build command (D272 §4, §7): builds the runtime image from a tag of this
# ABOUTME: checkout, then installs it over the deployment's directory as a household's install does.
#
#   deploy/release/build.sh <tag> <deployment directory> [--media <folder>...]
#   deploy/release/build.sh --checkout <deployment directory> [--force] [--media <folder>...]
#   deploy/release/build.sh --load <image archive> <deployment directory> [--media <folder>...]
#   deploy/release/build.sh --save <image archive>
#
# With --checkout (`make deploy`) it builds whatever this checkout is on: the commit, cloned, when the
# tree is clean, or the working files as they are when it is not. A commit with no version tag, or a
# tree with uncommitted changes, is warned of and asked about, unless --force; the image is named by
# the tag, or by a pseudo-version, v0.0.0-<the commit's time>-<commit>, and a modified tree's by the
# time of the build to the second, v0.0.0-<now>-<commit>-dirty. The binary records the same itself
# (internal/platform/build), as Go stamps it from the .git the build is given.
#
# With --load it builds nothing: it loads an image `docker save` wrote, gzipped or not, and writes
# the deployment from it, so a machine with Docker alone runs the image staging runs, until there is a
# tag (the weekend fallback, deploy/release/README.md §3a).
#
# With --save it installs nothing: it builds the checkout's commit as --checkout does, named the same,
# from a clone when the tree is clean, and writes the image as `docker save` does into the archive,
# gzipped when its name ends in .gz, for --load on another machine (orpheum-ops' prod-deploy SHA=
# builds on the owner's machine, not on the production NAS). It asks nothing: nothing is deployed
# until --load.
#
# It clones the tag into a temporary directory, with its own .git so the binary records its version
# and commit (D225), builds the image orpheum:<tag> from it, and runs the image's write-deployment
# over the directory, which then names that image, by the install script a household runs
# (internal/deployment/templates/install.sh, D314), taken from this checkout so an older tag's image,
# which has no install-script, installs the same way: the first time it needs --media; it writes the
# deployment from the values file and starts or updates it. Going back is the same with an older tag
# (deploy/release/README.md says when it cannot).
#
# It needs git and Docker with BuildKit, and is run as the deployment directory's owner.
set -eu

usage() {
	echo "usage: deploy/release/build.sh <tag> <deployment directory> [--media <folder>...]" >&2
	echo "       deploy/release/build.sh --checkout <deployment directory> [--force] [--media <folder>...]" >&2
	echo "       deploy/release/build.sh --load <image archive> <deployment directory> [--media <folder>...]" >&2
	echo "       deploy/release/build.sh --save <image archive>" >&2
	exit 2
}

# install runs the install script over the directory with the image named, which is this machine's
# own: nothing is pulled. Then the build cache no build has used for a week goes: each build left its
# layers there, 6.4 GB on the production NAS after a few days of deploys (2026-10-23), while what the next build
# reuses stays. The install script removes the older images themselves.
install() {
	sh "$(dirname "$0")/../../internal/deployment/templates/install.sh" --no-pull --image "$image" --dir "$dir" "$@"
	docker builder prune --force --filter until=168h >/dev/null 2>&1 || true
}

# mediaFirst refuses, before anything is built or loaded, a first install into $dir that names no
# folder of films and series, which the install script would refuse only after the build.
mediaFirst() {
	[ ! -f "$dir/values.toml" ] || return 0
	for arg in "$@"; do
		[ "$arg" != --media ] || return 0
	done
	echo "build: $dir holds no deployment yet: name the folders that hold the films and series," \
		"--media /path/to/films (MEDIA=\"/path/to/films /path/to/shows\" with make)" >&2
	exit 2
}

if [ "$#" -ge 3 ] && [ "$1" = --load ]; then
	archive=$2
	mkdir -p "$3/run"
	dir=$(cd "$3" && pwd)
	shift 3
	mediaFirst "$@"
	case $archive in
	*.gz) loaded=$(gunzip -c "$archive" | docker load) ;;
	*) loaded=$(docker load --input "$archive") ;;
	esac
	echo "$loaded" >&2
	image=$(echo "$loaded" | sed -n 's/^Loaded image: //p' | tail -n 1)
	if [ -z "$image" ]; then
		echo "build: the archive named no image; was it made by docker save of a tagged image?" >&2
		exit 1
	fi
	install "$@"
	exit 0
fi
[ "$#" -ge 2 ] || usage
checkout=$(cd "$(dirname "$0")/../.." && pwd)

# buildImage builds the runtime image $image from the folder $1, which holds a .git.
buildImage() {
	echo "build: building $image (the first build fetches FFmpeg, Go and Node, and takes several minutes)" >&2
	DOCKER_BUILDKIT=1 docker build --file "$1/deploy/Containerfile" --target runtime --tag "$image" "$1"
}

# checkoutImage names the image of the commit this checkout is on: $commit, $short, its version $tag
# (none when untagged), $isDirty, $version and $image, by the tag or a pseudo-version.
checkoutImage() {
	commit=$(git -C "$checkout" rev-parse HEAD)
	short=$(printf '%s' "$commit" | cut -c1-12)
	tag=$(git -C "$checkout" tag --points-at HEAD | grep -E '^v[0-9]+\.[0-9]+\.[0-9]+' | head -n 1 || true)
	isDirty=false
	[ -z "$(git -C "$checkout" status --porcelain)" ] || isDirty=true
	if $isDirty; then
		version="v0.0.0-$(date -u +%Y%m%d%H%M%S)-$short-dirty"
	elif [ -n "$tag" ]; then
		version=$tag
	else
		version="v0.0.0-$(TZ=UTC git -C "$checkout" log -1 --format=%cd --date=format-local:%Y%m%d%H%M%S)-$short"
	fi
	image="orpheum:$version"
}

# buildCheckout builds $image from the checkout: from a clone of the commit when the tree is clean,
# from the working files as they are when it is not.
buildCheckout() {
	if $isDirty; then
		buildImage "$checkout"
	else
		source=$(mktemp -d)
		trap 'rm -rf "$source"' EXIT
		echo "build: cloning $short" >&2
		git clone --quiet --no-checkout "file://$checkout" "$source/orpheum" 2>/dev/null
		git -C "$source/orpheum" checkout --quiet "$commit"
		buildImage "$source/orpheum"
	fi
}

if [ "$1" = --save ]; then
	[ "$#" -eq 2 ] || usage
	archive=$2
	checkoutImage
	if $isDirty; then
		echo "build: warning: the tree has uncommitted changes, which are built as they are, as $version" >&2
	fi
	buildCheckout
	case $archive in
	*.gz)
		docker save --output "${archive%.gz}" "$image"
		gzip -f "${archive%.gz}"
		;;
	*) docker save --output "$archive" "$image" ;;
	esac
	echo "build: saved $image to $archive" >&2
	exit 0
fi

if [ "$1" = --checkout ]; then
	mkdir -p "$2/run"
	dir=$(cd "$2" && pwd)
	shift 2
	force=false
	if [ "${1:-}" = --force ]; then
		force=true
		shift
	fi
	mediaFirst "$@"
	checkoutImage
	warned=false
	if [ -z "$tag" ]; then
		echo "build: warning: $short carries no version tag; it deploys as $version" >&2
		warned=true
	fi
	if $isDirty; then
		echo "build: warning: the tree has uncommitted changes, which deploy as they are, as $version" >&2
		git -C "$checkout" status --short >&2
		warned=true
	fi
	if $warned && ! $force; then
		printf 'build: deploy it anyway? [y/N] ' >&2
		answer=""
		read -r answer || true
		case $answer in
		y | Y | yes) ;;
		*)
			echo "build: not deployed; FORCE=yes deploys without asking" >&2
			exit 1
			;;
		esac
	fi
	buildCheckout
	install "$@"
	exit 0
fi
tag=$1
dir=$2
shift 2

# A version tag, vMAJOR.MINOR.PATCH with an optional -prerelease: Go records only such a tag as the
# binary's version (D225).
case $tag in
v[0-9]*.[0-9]*.[0-9]*) ;;
*)
	echo "build: $tag is not a version tag such as v0.1.0" >&2
	exit 1
	;;
esac
if ! git -C "$checkout" rev-parse --quiet --verify "refs/tags/$tag" >/dev/null; then
	echo "build: $tag is not a tag of $checkout; \`git fetch --tags\` brings new ones" >&2
	exit 1
fi
mkdir -p "$dir/run"
dir=$(cd "$dir" && pwd)
mediaFirst "$@"
image="orpheum:$tag"

source=$(mktemp -d)
trap 'rm -rf "$source"' EXIT
echo "build: cloning $tag" >&2
git clone --quiet --depth 1 --branch "$tag" "file://$checkout" "$source/orpheum" 2>/dev/null
buildImage "$source/orpheum"

install "$@"
