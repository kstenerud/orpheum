#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Hardware playback test of a runtime image on this machine, installed as a household does,
# ABOUTME: in a scratch deployment beside any other; deploy/hwtest/README.md lists the machines.
#
# Hardware playback test of a runtime image on this machine, as a household installs it: installs it
# with the image's own install script into a scratch deployment beside anything else here, plays
# test films of several kinds through it as an H.264-only browser would (hwplay.py), and removes it.
#
#   sh hwtest.sh <image or image archive> [--expect <encoder>] [--port <n>] [--keep]
#
# The deployment is ~/hwtest, its media ~/hwtest-media, its compose project "hwtest", and its HTTP on
# 127.0.0.1:<port> (18440 unless named) alone, so nothing outside this machine reaches it. A request
# from this machine reaches the core from Docker's gateway rather than from the house's network, so
# the core is told that gateway is home, which only this machine can come from. --keep leaves it
# running afterwards.
set -eu

image=""
expect=""
port=18440
keep=false
while [ "$#" -gt 0 ]; do
	case $1 in
	--expect) expect=$2; shift 2 ;;
	--port) port=$2; shift 2 ;;
	--keep) keep=true; shift ;;
	*) image=$1; shift ;;
	esac
done
[ -n "$image" ] || { echo "usage: sh hwtest.sh <image or archive> [--expect <encoder>] [--port <n>] [--keep]" >&2; exit 2; }
here=$(cd "$(dirname "$0")" && pwd)
dir=$HOME/hwtest
media=$HOME/hwtest-media
export COMPOSE_PROJECT_NAME=hwtest

say() { echo "hwtest: $*" >&2; }

if [ -f "$image" ]; then
	image=$( (case $image in *.gz) gunzip -c "$image" ;; *) cat "$image" ;; esac) | docker load | sed -n 's/^Loaded image: //p' | tail -n 1)
	say "loaded $image"
fi

tearDown() {
	if [ -f "$dir/compose.yaml" ]; then
		(cd "$dir" && docker compose down --remove-orphans >/dev/null 2>&1) || true
	fi
	# What the services wrote is the container user's: under rootless Docker, root's in the container.
	docker run --rm -v "$HOME:/h" --user 0:0 --entrypoint sh "$image" -c 'rm -rf /h/hwtest /h/hwtest-media' || true
}
tearDown
mkdir -p "$dir/run" "$media"

say "installing $image into $dir"
docker run --rm "$image" install-script >"$dir.install.sh"
# The first run begins the deployment and starts it; this test then moves it off the house's ports.
sh "$dir.install.sh" --no-pull --image "$image" --dir "$dir" --media "$media" >"$dir.install.log" 2>&1 || true
[ -f "$dir/values.toml" ] || { cat "$dir.install.log" >&2; exit 1; }
sed -i.bak \
	-e "s/^publish = \".*\"/publish = \"127.0.0.1\"/" \
	-e "s/^port = 8440$/port = $port/" \
	-e "s/^this_machine_port = 8441$/this_machine_port = $((port + 1))/" \
	-e "s/^https_port = 8443$/https_port = $((port + 3))/" \
	-e "s/^webrtc_port = 8443$/webrtc_port = $((port + 3))/" \
	"$dir/values.toml"
(cd "$dir" && docker compose down --remove-orphans >/dev/null 2>&1) || true
sh "$dir.install.sh" --no-pull --image "$image" --dir "$dir" >"$dir.install.log" 2>&1 || true
grep "^orpheum:   " "$dir.install.log" >&2 || true

# The test administers through this machine's own port, which is home (D334 §4): the house's port
# arrives from Docker's gateway, one of the core's own interface networks, never home (D452 §1), and a
# network home_networks lists is under the same exceptions.
core="http://127.0.0.1:$((port + 1))/api/v1"
tries=0
until curl -sf "$core/setup" >/dev/null 2>&1; do
	tries=$((tries + 1))
	[ "$tries" -lt 60 ] || { say "the core did not answer"; (cd "$dir" && docker compose logs --tail 30) >&2; exit 1; }
	sleep 2
done

status=0
python3 "$here/hwplay.py" --dir "$dir" --media "$media" --core "$core" \
	${expect:+--expect "$expect"} || status=$?
if $keep; then
	say "kept: $dir (COMPOSE_PROJECT_NAME=hwtest)"
else
	tearDown
	rm -f "$dir.install.sh" "$dir.install.log"
fi
exit "$status"
