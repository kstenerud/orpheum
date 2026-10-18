#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Release's one build command (D272 §4, §7): builds the runtime image from a tag of this
# ABOUTME: checkout, then installs it over the deployment's directory as a household's install does.
#
#   deploy/release/build.sh <tag> <deployment directory> [--media <folder>...]
#   deploy/release/build.sh --load <image archive> <deployment directory> [--media <folder>...]
#
# With --load it builds nothing: it loads an image `docker save` wrote, gzipped or not, and writes
# the deployment from it, so a machine with Docker alone runs the image staging runs, until there is a
# tag (the weekend fallback, deploy/release/README.md §3a).
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
	echo "       deploy/release/build.sh --load <image archive> <deployment directory> [--media <folder>...]" >&2
	exit 2
}

# install runs the install script over the directory with the image named, which is this machine's
# own: nothing is pulled.
install() {
	sh "$(dirname "$0")/../../internal/deployment/templates/install.sh" --no-pull --image "$image" --dir "$dir" "$@"
}

if [ "$#" -ge 3 ] && [ "$1" = --load ]; then
	archive=$2
	mkdir -p "$3/run"
	dir=$(cd "$3" && pwd)
	shift 3
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
tag=$1
dir=$2
shift 2
checkout=$(cd "$(dirname "$0")/../.." && pwd)

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
image="orpheum:$tag"

source=$(mktemp -d)
trap 'rm -rf "$source"' EXIT
echo "build: cloning $tag" >&2
git clone --quiet --depth 1 --branch "$tag" "file://$checkout" "$source/orpheum" 2>/dev/null
echo "build: building $image (the first build fetches FFmpeg, Go and Node, and takes several minutes)" >&2
DOCKER_BUILDKIT=1 docker build --file "$source/orpheum/deploy/Containerfile" --target runtime \
	--tag "$image" "$source/orpheum"

install "$@"
