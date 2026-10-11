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
# ./staging; MTU comes from the environment. Stopping takes the tunnel down with the network.
set -u

conf=/etc/orpheum-tunnel/wg0.conf
mtu="${ORPHEUM_TUNNEL_MTU:-1420}"
table=51820

down() {
	ip -4 rule del from 10.77.0.2 lookup "$table" 2>/dev/null || true
	ip -6 rule del from fd77::2 lookup "$table" 2>/dev/null || true
	ip link del wg0 2>/dev/null || true
}

up() {
	ip link add wg0 type wireguard &&
		wg setconf wg0 "$conf" &&
		ip -4 address add 10.77.0.2/24 dev wg0 &&
		ip -6 address add fd77::2/64 dev wg0 &&
		ip link set mtu "$mtu" up dev wg0 &&
		ip -4 rule add from 10.77.0.2 lookup "$table" &&
		ip -4 route replace default dev wg0 table "$table" &&
		ip -6 rule add from fd77::2 lookup "$table" &&
		ip -6 route replace default dev wg0 table "$table"
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
echo "orpheum-tunnel: wg0 is up as 10.77.0.2 and fd77::2, MTU $mtu; the VPS's end is $(wg show wg0 endpoints | cut -f2)"

trap 'down; exit 0' TERM INT
# Waiting in the background, so the trap runs as soon as the signal comes.
sleep infinity &
wait $!
