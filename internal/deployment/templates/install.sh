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

# The whole script is one block, which sh reads to its end before it runs any of it: piped to sh, it is
# read from standard input as it runs, and a docker command in it that reads standard input, as
# docker compose exec does, swallowed the rest, and sh stopped on "Unterminated quoted string" (the
# amd box, 2026-10-23). Each docker command that needs no input is given none, besides.
{
image="{{.Image}}"
dir="${HOME:-/root}/orpheum"
media=""
pull=true
prune=true

usage() {
	echo "usage: sh install.sh --media <folder> [--media <folder>...] [--dir <folder>] [--image <image>] [--no-pull] [--keep-images]" >&2
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
	# Leave every older image of this one's repository in place.
	--keep-images) prune=false; shift ;;
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

# Who write-deployment runs as, and so who the services run as (it writes the IDs it runs with):
# this user, who owns the directory. Rootless Docker maps a container's root to the user running it
# and every other ID to one of the user's subordinate IDs, which cannot write the directory, so
# there it is the container's root, which is this user on the host and nobody more.
runAs="$(id -u):$(id -g)"
# docker info lists its security options one to a line, rootless among them when it is.
isRootless=false
if docker info 2>/dev/null | grep -qE '^[[:space:]]+rootless$'; then
	isRootless=true
	runAs=0:0
fi

writeDeployment() {
	set -- --rm --user "$runAs" --hostname "$(hostname)" \
		--volume "$dir:/var/lib/orpheum" --volume "$dir/run:/run/orpheum"
	if [ -d /dev/dri ]; then
		set -- "$@" --volume /dev/dri:/dev/dri:ro
	fi
	docker run "$@" "$image" write-deployment --deployment release --image "$image" --directory "$dir" \
		${nvidia:+--nvidia "$nvidia"}
}

# How Docker hands this machine's NVIDIA GPUs to a container, with the driver's libraries no image can
# carry (D333): cdi, by the NVIDIA container toolkit's CDI spec (toolkit 1.18 or later, or after
# `nvidia-ctk cdi generate`); driver, by Docker's nvidia device driver, which the toolkit's hook
# provides; or empty, when neither works. Each is tried on the image itself, and only where an NVIDIA
# driver is loaded.
nvidiaControl=/dev/nvidiactl
nvidia=""
nvidiaAccess() {
	[ -e "$nvidiaControl" ] || return 0
	if docker run --rm --entrypoint true --device nvidia.com/gpu=all "$image" >/dev/null 2>&1; then
		nvidia=cdi
	elif docker run --rm --entrypoint true --gpus "driver=nvidia,count=all" "$image" >/dev/null 2>&1; then
		nvidia=driver
	fi
}

# The address this machine has at home, for its LAN name (D300); none when it has no private one.
homeAddress() {
	if command -v ip >/dev/null 2>&1; then
		address=$(ip -4 route get 1.1.1.1 2>/dev/null | sed -n 's/.* src \([0-9.]*\).*/\1/p')
	else
		# macOS has no ip: the default route's interface, and its address (the Mac hardware test,
		# 2026-10-23, stopped asking for lan.address by hand).
		interface=$(route -n get default 2>/dev/null | sed -n 's/.*interface: *//p')
		address=$(ipconfig getifaddr "$interface" 2>/dev/null)
	fi
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

# dockerFormat is Docker's template for one field, two braces around .Field, written so that this
# file, a Go template rendered for a household and run raw by deploy/release/build.sh, holds no
# action of its own.
dockerFormat() { printf '{%s}' "{$1}"; }

# The image the deployment ran before this install, by its ID, taken before a pull can move its tag
# to the new one: an update keeps it, to go back to, and removes the older ones (pruneImages).
previousID=""
if [ -f "$dir/.env" ]; then
	previous=$(sed -n 's/^ORPHEUM_IMAGE=//p' "$dir/.env")
	if [ -n "$previous" ]; then
		previousID=$(docker image inspect --format "$(dockerFormat .Id)" "$previous" 2>/dev/null) || previousID=""
	fi
fi

# pruneImages removes every image of this one's repository but this one and the one the deployment
# ran before, once the server is up: each update left its image behind, and nas.example had some fifteen
# (2026-10-23). An image a container still uses is refused by Docker and stays. Only an update
# prunes: a first install ran nothing before, and another deployment's image to go back to, which no
# container uses, would go with the rest (the hardware test's scratch install removed nas.example's,
# 2026-10-23). An image named by its digest has no repository to list, and prunes nothing.
pruneImages() {
	[ -n "$previousID" ] || return 0
	case $image in *@*) return 0 ;; esac
	currentID=$(docker image inspect --format "$(dockerFormat .Id)" "$image" 2>/dev/null) || return 0
	repository=$image
	case ${image##*/} in *:*) repository=${image%:*} ;; esac
	docker image ls --no-trunc --format "$(dockerFormat .ID) $(dockerFormat .Repository):$(dockerFormat .Tag)" "$repository" |
		while read -r id name; do
			if [ "$id" = "$currentID" ] || [ "$id" = "$previousID" ]; then
				continue
			fi
			# An image whose tag moved on has none left, and goes by its ID.
			target=$name
			case $name in *:"<none>") target=$id ;; esac
			if docker image rm "$target" >/dev/null 2>&1; then
				say "  removed the old image $name"
			fi
		done
}

if $pull; then
	say "pulling $image"
	docker pull --quiet "$image" >/dev/null
fi
nvidiaAccess
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
# The kernel's security modules, which a container usually cannot see though its host has them: the
# node asks the kernel itself, and its System page says (internal/node/confine).
lsmList=/sys/kernel/security/lsm
if [ ! -r "$lsmList" ]; then
	say "  Landlock: cannot be seen from here, as in a container; the server's System page says whether FFmpeg runs confined"
elif grep -qs landlock "$lsmList"; then
	say "  Landlock: yes, so every FFmpeg run is confined"
else
	say "  Landlock: no; FFmpeg runs unconfined (add lsm=landlock,... to the kernel's command line to have it)"
fi
if [ -e /dev/dri/renderD128 ]; then
	say "  a GPU's render node: yes, for hardware encoding"
else
	say "  a GPU's render node: no; films that need preparing are encoded in software"
fi
if [ -e "$nvidiaControl" ]; then
	if [ -n "$nvidia" ]; then
		say "  an NVIDIA GPU: yes, for hardware encoding"
	else
		say "  an NVIDIA GPU: yes, but Docker cannot hand it to a container, so films are encoded in software; install the NVIDIA Container Toolkit (https://docs.nvidia.com/datacenter/cloud-native/container-toolkit/latest/install-guide.html), then run this again"
	fi
fi
# A public IPv6 address (2000::/3) to reach the internet from: where the house shares its IPv4, as
# DS-Lite does, it is the way a device away from home reaches the server without a relay (D314 §4).
# A machine that forwards IPv6, as Docker makes it, ignores the router's announcements and with them
# its default route unless the interface's accept_ra is 2 (docs/users/setup-problems.md).
procSys=/proc/sys
ipv6Report() {
	if ip -6 route get 2606:4700:4700::1111 2>/dev/null | grep -qE ' src [23][0-9a-f]{0,3}:'; then
		say "  a public IPv6 address: yes, so devices away from home that have IPv6 reach it directly"
		return
	fi
	dev=$(ip -4 route show default 2>/dev/null | sed -n 's/.* dev \([^ ]*\).*/\1/p' | head -n 1)
	if [ -n "$dev" ] && ip -6 addr show dev "$dev" scope global 2>/dev/null | grep -qE 'inet6 [23][0-9a-f]{0,3}:' &&
		[ "$(cat "$procSys/net/ipv6/conf/all/forwarding" 2>/dev/null)" = 1 ] &&
		[ "$(cat "$procSys/net/ipv6/conf/$dev/accept_ra" 2>/dev/null)" != 2 ]; then
		say "  a public IPv6 address: yes, but no route out by it: this machine forwards IPv6 (Docker turns that on), so it ignores the router's announcements;"
		say "    as root: echo 'net.ipv6.conf.$dev.accept_ra = 2' > /etc/sysctl.d/90-orpheum-ipv6.conf, then restart the machine"
		return
	fi
	say "  a public IPv6 address: no; devices away from home reach it only where IPv4 lets them (give this machine IPv6: a Proxmox container's network takes IPv6 SLAAC)"
}
ipv6Report
# Rootless Docker hands each connection to a published port on through its userland proxy, so the
# server sees every device at Docker's own gateway, none at home, and no one can administer it (D297
# §6). RootlessKit 3 keeps the device's address once the proxy is off, which needs br_netfilter,
# loaded by root; without the module, the daemon will not start with the proxy off.
daemonConfig="${XDG_CONFIG_HOME:-$HOME/.config}/docker/daemon.json"
if $isRootless; then
	if grep -qsE '"userland-proxy"[[:space:]]*:[[:space:]]*false' "$daemonConfig"; then
		say "  rootless Docker: its userland proxy is off, so the server sees each device's own address"
	else
		say "  rootless Docker: its userland proxy hides each device's address, so the server sees none at home and no one can administer it. To fix it:"
		next="add"
		if [ ! -e "$procSys/net/bridge/bridge-nf-call-iptables" ]; then
			say "    as root: modprobe br_netfilter && echo br_netfilter > /etc/modules-load.d/br_netfilter.conf"
			next="then add"
		fi
		say "    $next \"userland-proxy\": false to $daemonConfig, run systemctl --user restart docker, and run this again"
	fi
fi
if grep -qs bbr /proc/sys/net/ipv4/tcp_available_congestion_control || [ -d /sys/module/tcp_bbr ]; then
	say "  BBR: yes, which keeps a film's rate on a lossy link"
else
	say "  BBR: no (modprobe tcp_bbr to have it)"
fi

say "waiting for the server to start"
# The code that makes the first admin, as soon as the server answers; a server that has its admin
# already says so, and is not waited on for the code it will never give (nas.example, 2026-10-19: every
# update sat out the whole two minutes).
setupCode=""
isUp=false
tries=0
while [ "$tries" -lt 60 ]; do
	if answer=$(cd "$dir" && docker compose exec -T orpheum-core orpheum setup-code --instance /var/lib/orpheum/core \
		</dev/null 2>/dev/null); then
		setupCode=$answer
		isUp=true
		break
	fi
	case $answer in
	*"setup is complete"*) isUp=true; break ;;
	esac
	tries=$((tries + 1))
	sleep 2
done
if $isUp && $prune; then
	pruneImages
fi

# valueOf is the values file's value of key in section, its quotes taken off, or nothing.
valueOf() {
	awk -v section="[$1]" -v key="$2" '
		/^\[/ { current = $0 }
		current == section && $1 == key && $2 == "=" { value = $3; gsub(/"/, "", value); print value; exit }
	' "$values"
}

# The terminal the person runs this at, which the script, itself read from standard input, asks at
# (D334 §2): opened once, so each answer is read in turn; there is none in a NAS's panel, Portainer
# or a script, where the one-time setup code is printed instead.
ttyIn=/dev/tty
ttyOut=/dev/tty
hasTerminal=false
if (exec 3<"$ttyIn" 4>"$ttyOut") 2>/dev/null; then
	exec 3<"$ttyIn" 4>"$ttyOut"
	hasTerminal=true
	trap 'stty echo <&3 2>/dev/null || true' EXIT
fi

# askHidden asks at the terminal without echoing the answer, and prints it.
askHidden() {
	printf '%s' "$1" >&4
	stty -echo <&3 2>/dev/null || true
	readStatus=0
	IFS= read -r hidden <&3 || readStatus=$?
	stty echo <&3 2>/dev/null || true
	printf '\n' >&4
	[ "$readStatus" -eq 0 ] && printf '%s' "$hidden"
}

# The first admin, made at the terminal, and then, if the person has an account's key, the core
# paired with its discovery with the password just given (D334 §2, §3): no setup code to copy, and no
# key to paste on a System page afterwards. A password goes to the core on standard input alone.
admin=""
realm=""
if [ -n "$setupCode" ] && $hasTerminal; then
	say "the server is up; choose who administers it"
	attempts=0
	while [ -z "$admin" ] && [ "$attempts" -lt 3 ]; do
		attempts=$((attempts + 1))
		printf 'Administrator name: ' >&4
		IFS= read -r name <&3 || break
		[ -n "$name" ] || continue
		password=$(askHidden "Password: ") || break
		again=$(askHidden "The same again: ") || break
		if [ "$password" != "$again" ]; then
			say "the two passwords differ; once more"
			continue
		fi
		if printf '%s\n' "$password" | (cd "$dir" && docker compose exec -T orpheum-core orpheum reset-admin \
			--name "$name" --instance /var/lib/orpheum/core) >/dev/null; then
			admin=$name
		fi
	done
	if [ -n "$admin" ]; then
		setupCode=""
		say "$admin administers it"
		printf 'The key from your Orpheum account, to reach it from away (Enter to skip): ' >&4
		IFS= read -r key <&3 || key=""
		if [ -n "$key" ]; then
			if paired=$(printf '%s\n%s\n' "$password" "$key" | (cd "$dir" && docker compose exec -T orpheum-core \
				orpheum pair --name "$admin" --instance /var/lib/orpheum/core)); then
				case $paired in "paired: realm "*) realm=${paired#paired: realm } ;; esac
				say "paired with your account"
			else
				say "not paired; enter the key later on the server's System page"
			fi
		fi
	fi
	password=""
	again=""
fi

address=$(valueOf lan address)
[ -n "$address" ] || address="<this machine's address>"
port=$(valueOf http port)
thisMachinePort=$(valueOf http this_machine_port)
say "installed in $dir"
if ! $isUp; then
	say "the server did not answer within two minutes; \`docker compose logs orpheum-core\` in $dir says why"
	exit 1
fi
say "open it:"
say "  from a phone, laptop or TV at home:  http://$address:${port:-8440}"
say "  in a browser on this computer:       http://localhost:${thisMachinePort:-8441}"
if [ -n "$realm" ]; then
	say "  from away: sign in at app.orpheum-app.com with the realm $realm, as $admin"
fi
if [ -n "$setupCode" ]; then
	say "make the first admin there with this one-time code:"
	printf '%s\n' "$setupCode" >&2
elif [ -z "$admin" ]; then
	say "it has its first admin already"
fi
}
