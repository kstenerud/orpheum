#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Installs Orpheum, checking the release before running anything of it (the root key audit's §3.9;
# D393 §8). It holds the keys of Orpheum's publisher root, fetches the signed release record, checks
# its signature with ssh-keygen, reads the latest release's image digest, pulls the image by that
# digest, and only then runs that image's own install script, its arguments passed on:
#
#   curl -fsSL <this script's address> | sh -s -- --media /path/to/films
#
# Written by `discovery-sign bootstrap` from the publisher's chain; its own hash is printed beside it.
set -eu
{
say() { echo "orpheum: $*" >&2; }
for tool in curl ssh-keygen docker awk; do
	command -v "$tool" >/dev/null 2>&1 || { say "$tool is missing; install it, then run this again"; exit 1; }
done
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
cat >"$work/allowed_signers" <<'SIGNERS'
{{SIGNERS}}
SIGNERS
# The places the record is served at, origins separated by spaces.
locations='{{LOCATIONS}}'
fetched=false
for location in $locations; do
	if curl -fsS "$location/.well-known/orpheum/release" -o "$work/release" &&
		curl -fsS "$location/.well-known/orpheum/release.sig" -o "$work/release.sig"; then
		fetched=true
		break
	fi
done
$fetched || { say "no place served the release record; nothing was installed"; exit 1; }
if ! ssh-keygen -Y verify -f "$work/allowed_signers" -I orpheum -n orpheum-release-v1 \
	-s "$work/release.sig" <"$work/release" >/dev/null; then
	say "the release record's signature does not hold; nothing was installed"
	exit 1
fi
[ "$(sed -n 1p "$work/release")" = "orpheum-signed release 1" ] || {
	say "the release record is of a format this script does not read; fetch the script again"
	exit 1
}
latest=$(awk '$1 == "latest" { print $2 }' "$work/release")
digest=$(awk -v release="$latest" '$1 == "image" && $2 == release { print $3 }' "$work/release")
pull=$(awk '$1 == "pull" { print $2; exit }' "$work/release")
case $digest in
sha256:[0-9a-f]*) ;;
*) say "the release record names no image for $latest; nothing was installed"; exit 1 ;;
esac
if [ "${#digest}" -ne 71 ] || [ -z "$pull" ]; then
	say "the release record names no image to pull; nothing was installed"
	exit 1
fi
image="$pull@$digest"
say "installing Orpheum $latest, $image"
docker pull --quiet "$image" >/dev/null
docker run --rm "$image" install-script | sh -s -- --image "$image" "$@"
}
