#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# Installs an Orpheum core on this machine (D314): Docker is all it needs. `orpheum install-script`
# prints it from the image a household pulled, which it then installs:
#
#   docker run --rm IMAGE install-script | sh -s -- --media /path/to/films [--media /path/to/shows]
#
# It writes the deployment into a directory of its own (--dir, ~/orpheum unless named), fills in
# the media and this machine's address at home, starts it, says what this machine has and lacks, and
# prints the address to open and the one-time code that makes the first admin. Each --media is one
# folder, so films on one drive and series on another are both served. Run again, it adds any folder
# named that the values file lacks, writes the deployment again from it and restarts it, keeping
# everything.
set -eu

image="{{.Image}}"
dir="${HOME:-/root}/orpheum"
media=""
pull=true

usage() {
	echo "usage: sh install.sh --media <folder> [--media <folder>...] [--dir <folder>] [--image <image>] [--no-pull]" >&2
	exit 2
}

while [ "$#" -gt 0 ]; do
	case $1 in
	--media) [ "$#" -ge 2 ] || usage; media="$media
$2"; shift 2 ;;
	--dir) [ "$#" -ge 2 ] || usage; dir=$2; shift 2 ;;
	--image) [ "$#" -ge 2 ] || usage; image=$2; shift 2 ;;
	# An image built on this machine, as deploy/release/build.sh builds one from a tag.
	--no-pull) pull=false; shift ;;
	*) usage ;;
	esac
done

say() { echo "orpheum: $*" >&2; }

if ! command -v docker >/dev/null 2>&1; then
	say "Docker is not installed; install it (https://docs.docker.com/engine/install/), then run this again"
	exit 1
fi
if ! docker compose version >/dev/null 2>&1; then
	say "Docker's compose plugin is missing; install docker-compose-plugin, then run this again"
	exit 1
fi
if ! docker info >/dev/null 2>&1; then
	say "Docker does not answer this user; run this as a member of the docker group, or as root"
	exit 1
fi

mkdir -p "$dir/run"
dir=$(cd "$dir" && pwd)
values="$dir/values.toml"
isNew=false
[ -f "$values" ] || isNew=true
if $isNew && [ -z "$media" ]; then
	say "name the folders that hold your films and series: --media /path/to/films"
	exit 2
fi
# Each folder named, one per line; a path may hold spaces.
printf '%s\n' "$media" | sed '/^$/d' | while IFS= read -r folder; do
	case $folder in
	/*) [ -d "$folder" ] || { say "$folder is not a folder on this machine"; exit 1; } ;;
	*) say "$folder: name the folder by its full path, /..."; exit 1 ;;
	esac
done

writeDeployment() {
	set -- --rm --user "$(id -u):$(id -g)" --hostname "$(hostname)" \
		--volume "$dir:/var/lib/orpheum" --volume "$dir/run:/run/orpheum"
	if [ -d /dev/dri ]; then
		set -- "$@" --volume /dev/dri:/dev/dri:ro
	fi
	docker run "$@" "$image" write-deployment --deployment release --image "$image"
}

# The address this machine has at home, for its LAN name (D300); none when it has no private one.
homeAddress() {
	address=$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p')
	case $address in
	10.* | 192.168.* | 172.1[6-9].* | 172.2[0-9].* | 172.3[01].*) echo "$address" ;;
	esac
}

# Fills this machine's address at home into a values file just begun (D300).
fillAddress() {
	address=$(homeAddress)
	awk -v address="$address" '
		/^\[/ { section = $0 }
		section == "[lan]" && $0 == "address = \"\"" && address != "" { print "address = \"" address "\""; next }
		{ print }
	' "$values" >"$values.new" && mv "$values.new" "$values"
}

# Adds each folder named to the values file's media, once: a folder there already stays as it is, so
# a drive added later is named on a run of its own. Each is a TOML basic string, its backslashes and
# quotes escaped one character at a time (gsub's replacement escapes differ between awks).
addMedia() {
	if ! ORPHEUM_MEDIA=$(printf '%s\n' "$media" | sed '/^$/d') awk '
		function quoted(s, out, c, i) {
			for (i = 1; i <= length(s); i++) {
				c = substr(s, i, 1)
				if (c == "\\" || c == "\"") out = out "\\"
				out = out c
			}
			return "\"" out "\""
		}
		/^\[/ { section = $0 }
		section == "[media]" && /^roots = \[.*\]$/ {
			line = substr($0, 1, length($0) - 1)
			n = split(ENVIRON["ORPHEUM_MEDIA"], folders, "\n")
			for (i = 1; i <= n; i++) {
				q = quoted(folders[i])
				if (index(line, q)) continue
				line = line (line ~ /\[$/ ? "" : ", ") q
			}
			print line "]"; found = 1; next
		}
		{ print }
		END { if (!found) exit 3 }
	' "$values" >"$values.new"; then
		rm -f "$values.new"
		say "$values lists its media over more than one line; add the folders to media.roots there by hand"
		exit 1
	fi
	mv "$values.new" "$values"
}

if $pull; then
	say "pulling $image"
	docker pull --quiet "$image" >/dev/null
fi
if $isNew; then
	# Beginning writes the values file and fails, saying what to fill in, which this script fills in
	# itself; it failed for another reason when it wrote none.
	if ! began=$(writeDeployment 2>&1 >/dev/null) && [ ! -f "$values" ]; then
		printf '%s\n' "$began" >&2
		exit 1
	fi
	fillAddress
fi
[ -z "$media" ] || addMedia
writeDeployment
(cd "$dir" && docker compose up -d --remove-orphans)

say "what this machine has (the server runs without each, and says so on its System page):"
if grep -qs landlock /sys/kernel/security/lsm; then
	say "  Landlock: yes, so every FFmpeg run is confined"
else
	say "  Landlock: no; FFmpeg runs unconfined (add lsm=landlock,... to the kernel's command line to have it)"
fi
if [ -e /dev/dri/renderD128 ]; then
	say "  a GPU's render node: yes, for hardware encoding"
else
	say "  a GPU's render node: no; films that need preparing are encoded in software"
fi
if grep -qs bbr /proc/sys/net/ipv4/tcp_available_congestion_control || [ -d /sys/module/tcp_bbr ]; then
	say "  BBR: yes, which keeps a film's rate on a lossy link"
else
	say "  BBR: no (modprobe tcp_bbr to have it)"
fi

say "waiting for the server to start"
tries=0
until (cd "$dir" && docker compose exec -T orpheum-core orpheum setup-code --instance /var/lib/orpheum/core) \
	>"$dir/run/setup-code" 2>/dev/null || [ "$tries" -ge 60 ]; do
	tries=$((tries + 1))
	sleep 2
done
address=$(homeAddress)
[ -n "$address" ] || address="<this machine's address>"
say "installed in $dir"
if [ -s "$dir/run/setup-code" ]; then
	say "open http://$address:8440 and make the first admin with this one-time code:"
	cat "$dir/run/setup-code" >&2
else
	say "open http://$address:8440: it has its first admin already"
fi
rm -f "$dir/run/setup-code"
