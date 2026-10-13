#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Release's one build command (D272 §4, §7): builds the runtime image from a tag of this
# ABOUTME: checkout, then runs that image's write-deployment over the deployment's directory.
#
#   deploy/release/build.sh <tag> <deployment directory>
#   deploy/release/build.sh --load <image archive> <deployment directory>
#
# With --load it builds nothing: it loads an image `docker save` wrote, gzipped or not, and writes
# the deployment from it, so a machine with Docker alone runs the image staging runs, until there is a
# tag (the weekend fallback, deploy/release/README.md §3a).
#
# It clones the tag into a temporary directory, with its own .git so the binary records its version
# and commit (D225), builds the image orpheum:<tag> from it, and runs the image's write-deployment
# over the directory, which then names that image: the first time it writes the values file and the
# tunnel's keys and stops; after that it writes the compose file and the configs from the image's
# templates. `docker compose up -d` in the directory then starts or updates the deployment. Going
# back is the same with an older tag (deploy/release/README.md says when it cannot).
#
# It needs git and Docker with BuildKit, and is run as the deployment directory's owner.
set -eu

usage() {
	echo "usage: deploy/release/build.sh <tag> <deployment directory>" >&2
	echo "       deploy/release/build.sh --load <image archive> <deployment directory>" >&2
	exit 2
}

# writeDeployment runs the image's write-deployment over the directory, as cmd/orpheum's
# write-deployment says to run it: the directory and its run/ where the image looks for them, as
# their owner, with the machine's name, and the GPU when there is one.
writeDeployment() {
	set -- --rm --user "$(id -u):$(id -g)" --hostname "$(hostname)" \
		--volume "$dir:/var/lib/orpheum" --volume "$dir/run:/run/orpheum"
	if [ -d /dev/dri ]; then
		set -- "$@" --volume /dev/dri:/dev/dri:ro
	fi
	docker run "$@" "$image" write-deployment --deployment release --image "$image"
}

if [ "$#" -eq 3 ] && [ "$1" = --load ]; then
	mkdir -p "$3/run"
	dir=$(cd "$3" && pwd)
	case $2 in
	*.gz) loaded=$(gunzip -c "$2" | docker load) ;;
	*) loaded=$(docker load --input "$2") ;;
	esac
	echo "$loaded" >&2
	image=$(echo "$loaded" | sed -n 's/^Loaded image: //p' | tail -n 1)
	if [ -z "$image" ]; then
		echo "build: the archive named no image; was it made by docker save of a tagged image?" >&2
		exit 1
	fi
	writeDeployment
	exit 0
fi
[ "$#" -eq 2 ] || usage
tag=$1
dir=$2
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

writeDeployment
