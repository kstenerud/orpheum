#!/bin/sh
# SPDX-License-Identifier: AGPL-3.0-or-later
# orpheum-tunnel: the core's end of the WireGuard tunnel to the VPS (D243 §7, D249 §2), the entry
# point of the deployment's tunnel service. The core shares this service's network, so the tunnel
# is in the core's network and nowhere else: nothing on the host is configured.
#
# wg0 is not this network's default route: the core's own requests, and its answers to the house's
# network on the published HTTP port, leave the usual way. Only what leaves from the tunnel's
# addresses, the answers to what came in through it, goes back through it, by a rule per address
# family that sends those to a table whose default route is wg0.
#
# /etc/orpheum-tunnel/wg0.conf is `wg setconf`'s format (a key and a peer), rendered by
# write-deployment; the MTU and this end's addresses (the values file's vps.tunnel_ipv4 and
# vps.tunnel_ipv6) come from the environment. Stopping takes the tunnel down with the network.
set -u

conf=/etc/orpheum-tunnel/wg0.conf
mtu="${ORPHEUM_TUNNEL_MTU:-1420}"
ipv4="${ORPHEUM_TUNNEL_IPV4:?the tunnel IPv4 address of the deployment}"
ipv6="${ORPHEUM_TUNNEL_IPV6:?the tunnel IPv6 address of the deployment}"
table=51820

# What arrives through the tunnel comes from the internet, the VPS keeping each packet's source: a
# source at home or on a private network is forged, by whoever holds the VPS, to pass as the house (the
# Plex-chain review's L2). The tunnel's own subnets, its peers', are the exception. Each is dropped by a
# rule on wg0's packets, ahead of the local table, which moves from its place at the head to make room.
tunnel4=10.77.0.0/24
tunnel6=fd77::/64
forged4="0.0.0.0/8 10.0.0.0/8 100.64.0.0/10 127.0.0.0/8 169.254.0.0/16 172.16.0.0/12 192.168.0.0/16"
forged6="::1/128 fc00::/7 fe80::/10"
peers_pref=10
forged_pref=20
local_pref=100

# drop_forged adds the rules for one family: $1 is -4 or -6, $2 the tunnel's subnet, $3 the sources.
drop_forged() {
	{ ip "$1" rule show pref "$local_pref" | grep -q . || ip "$1" rule add pref "$local_pref" lookup local; } &&
		{ ip "$1" rule del pref 0 lookup local 2>/dev/null || true; } &&
		ip "$1" rule add pref "$peers_pref" iif wg0 from "$2" lookup local || return 1
	for source in $3; do
		ip "$1" rule add pref "$forged_pref" iif wg0 from "$source" blackhole || return 1
	done
}

down() {
	ip -4 rule del from "$ipv4" lookup "$table" 2>/dev/null || true
	ip -6 rule del from "$ipv6" lookup "$table" 2>/dev/null || true
	for family in -4 -6; do
		for pref in "$peers_pref" "$forged_pref"; do
			while ip "$family" rule del pref "$pref" 2>/dev/null; do :; done
		done
	done
	ip link del wg0 2>/dev/null || true
}

up() {
	ip link add wg0 type wireguard &&
		wg setconf wg0 "$conf" &&
		ip -4 address add "$ipv4/24" dev wg0 &&
		ip -6 address add "$ipv6/64" dev wg0 &&
		ip link set mtu "$mtu" up dev wg0 &&
		ip -4 rule add from "$ipv4" lookup "$table" &&
		ip -4 route replace default dev wg0 table "$table" &&
		ip -6 rule add from "$ipv6" lookup "$table" &&
		ip -6 route replace default dev wg0 table "$table" &&
		drop_forged -4 "$tunnel4" "$forged4" &&
		drop_forged -6 "$tunnel6" "$forged6"
}

# A restart of this service may find the last run's interface and rules still here.
down
# This service does not exit on a failure: the core shares its network, and a restart of this
# container alone gives it a new network the running core is not in. So a tunnel that cannot come
# up says why and tries again, in the same network, until it does.
until up; do
	echo "orpheum-tunnel: could not bring wg0 up; trying again in 10 s" >&2
	down
	sleep 10
done
echo "orpheum-tunnel: wg0 is up as $ipv4 and $ipv6, MTU $mtu; the VPS's end is $(wg show wg0 endpoints | cut -f2)"

trap 'down; exit 0' TERM INT
# Waiting in the background, so the trap runs as soon as the signal comes.
sleep infinity &
wait $!
