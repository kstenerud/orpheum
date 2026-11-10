// SPDX-License-Identifier: AGPL-3.0-or-later

package main

import (
	"strconv"

	"github.com/kstenerud/orpheum/internal/core/certificate"
)

// keyListen is the key of a table's address and port to listen on: the server's, HTTPS's and the
// voice relay's.
const keyListen = "listen"

// The file's tables.
const (
	tableServer    = "server"
	tableHTTPS     = "https"
	tableTURN      = "turn"
	tableDiscovery = "discovery"
	tableWebRTC    = "webrtc"
	tableDatabase  = "database"
	tableNode      = "node"
	tableLog       = "log"
)

// configKey is one key of the file. A key with a starting value runs on it when the file lacks it
// (D202); one without is specific to the machine, and the server refuses to start without it.
type configKey struct {
	table         string
	name          string
	startingValue string // the starting value as TOML; "" when there is none
	description   string
}

func (key configKey) path() string {
	return key.table + "." + key.name
}

// configKeys is every key of the full config of an instance of kind, in the order the file is
// written: the core's server and database, or the node's, and the log.
func configKeys(kind instanceKind) []configKey {
	if kind == kindNode {
		return append(nodeConfigKeys(), logConfigKeys()...)
	}
	return append(coreConfigKeys(), logConfigKeys()...)
}

// serverConfigKeys are the core's [server] keys, in the order the file is written.
func serverConfigKeys() []configKey {
	return []configKey{
		{tableServer, keyListen, "", "the address and port to serve on, which only this machine's owner can choose"},
		{tableServer, "read_header_timeout_seconds", "10", "seconds a client has to send its request headers"},
		{
			tableServer, "host_names", "[]",
			"public names the house reaches this listener by; it always answers its IP addresses and names " +
				"public DNS never answers (nas, server.lan, nas.local), and refuses others against DNS rebinding",
		},
		{tableServer, "shutdown_grace_seconds", "10", "seconds in-flight requests get to finish when the server stops"},
		{
			tableServer, "this_machine_listen", `""`,
			"plain HTTP for a browser on this machine alone, published on its loopback: a request on it is from " +
				"home (D334 §4); empty serves none",
		},
		{
			tableServer, "install_route", `""`,
			"how the server was installed, \"script\" (the install script) or a compose file's route (omv, " +
				"synology, portainer, casaos, docker), which the deployment writes: the Update page shows only " +
				"its steps; empty shows every one (D450 §2, D462 §9)",
		},
		{tableServer, "compose_version", "0", "the version of the compose file that started the server (D461 §3)"},
		{tableServer, "image_tag", `""`, "the image tag the compose file runs, a pinned one said (D462 §10.7)"},
		{tableServer, "compose_media", "[]", "the compose file's media folders, filled into a newer one (D462 §4)"},
		{
			tableServer, "home_networks", "[]",
			"networks home besides the private ranges, a VPN's say. Home is always every private address " +
				"(10.0.0.0/8, 172.16.0.0/12, 192.168.0.0/16, fc00::/7), loopback and the /64 of the server's " +
				"own public IPv6 address, but not_home_networks' and the networks of this machine's own " +
				"interfaces, which apply to a listed network too (D452)",
		},
		{
			tableServer, "not_home_networks", "[]",
			"private networks never home: a tunnel's, through which the internet arrives (D452)",
		},
		{
			tableServer, "address_at_home_from_default_route", "false",
			"whether the server shares the machine's network, and so takes the address of the interface " +
				"carrying the default route as its address at home until one is learned or set; never in a " +
				"container's own network, whose default route is Docker's bridge (D452 §3)",
		},
	}
}

// coreConfigKeys are the full config's keys for the core, in the order the file is written.
func coreConfigKeys() []configKey {
	return append(append(append(serverConfigKeys(), []configKey{
		{
			tableHTTPS, "domain", `""`,
			`the public name served over HTTPS, "media.example.com"; empty serves none (D243 §6)`,
		},
		{tableHTTPS, "contact", `""`, "the e-mail address the certificate's CA writes to about it"},
		{tableHTTPS, keyListen, `":443"`, "the address and port HTTPS is served on"},
		{
			tableHTTPS, "acme_directory", strconv.Quote(certificate.LetsEncrypt),
			"the ACME directory the certificate comes from; Let's Encrypt's staging one, " +
				"https://acme-staging-v02.api.letsencrypt.org/directory, spends no production quota (D261 §5)",
		},
		{
			tableTURN, "public_address", `""`,
			"the IP address browsers reach the voice relay at, the VPS's; empty serves none (D243 §7, §8)",
		},
		{tableTURN, keyListen, `":3478"`, "the UDP address and port the voice relay is served on (D243 §8)"},
		{tableTURN, "relay_port_min", "49160", "the least UDP port relayed voice flows through (D243 §8)"},
		{tableTURN, "relay_port_max", "49199", "the greatest UDP port relayed voice flows through (D243 §8)"},
	}...), append(discoveryConfigKeys(), webRTCConfigKeys()...)...), []configKey{
		{
			tableDatabase, "read_connections_minimum", "4",
			"the smallest pool of read connections; a machine with more CPUs gets one per CPU",
		},
		{
			tableDatabase, "migrate_free_space_db_multiple", "3",
			"free space needed before migrating, in multiples of the database: the snapshot plus a rebuild",
		},
		{
			tableDatabase, "when_unmigratable", `"refuse"`,
			`what the core does with a database it cannot bring forward: "refuse" to start, or ` +
				`"start_over", setting it aside in the backups directory and starting empty (staging's, D249 §4)`,
		},
	}...)
}

// nodeConfigKeys are the file's keys for the media node, in the order the file is written.
func nodeConfigKeys() []configKey {
	return append([]configKey{
		{tableNode, "name", `"local"`, "the name the core knows this node by (D207)"},
		{
			tableNode, "core_socket", "",
			"the core's socket, in its instance's run directory, which this node dials (D242 §7)",
		},
		{tableNode, "root_paths", "", "the media directories this node serves; set on the node, never remotely (D107)"},
		{tableNode, "ffmpeg_dir", "", "where the pinned FFmpeg is on this machine"},
		{
			tableNode, "confinement_policy", `"report"`,
			`"report" runs FFmpeg with what confinement the kernel offers and says what is missing; "available" ` +
				`does too where the kernel lacks a mechanism, but starts the node locked where one the kernel has ` +
				`cannot be put in force; "require" starts a node that lacks any locked, running no FFmpeg until ` +
				`an admin lets it run unconfined on the System page (D273 §5, D450 §1, D454 §3)`,
		},
		{tableNode, "probe_timeout_seconds", "60", "seconds ffprobe gets for one file before it is stopped"},
		{
			tableNode, "ended_work_kept_hours", "168",
			"hours the node remembers work whose result the core took, so the same command is not run twice (D238 §2)",
		},
		{
			tableNode, "read_header_timeout_seconds", "10",
			"seconds the core has to send a request's headers to this node's byte socket",
		},
		{
			tableNode, "prepared_budget_gib", "200",
			"GiB the stores of chunks may take; the least recently played go first past it (D217 §5)",
		},
		{
			tableNode, "prepared_kept_after_play_hours", "72",
			"hours after its last play a store of chunks is kept before any past it is evicted (D217 §5)",
		},
		{
			// A playing hls.js fetches a segment every few seconds once its 30 s buffer is full; ten
			// minutes also covers a short pause, after which a paused film may go.
			tableNode, "prepared_watched_minutes", "10",
			"minutes after it last served a file that a store of chunks counts as being watched; eviction " +
				"never removes one being watched, and a run of chunks the disk then has no room for is refused",
		},
		{
			tableNode, "prepared_disk_used_limit_percent", "95",
			"how full the prepared directory's disk may be, whatever the budget; past it, stores are " +
				"evicted, and a run of chunks with nothing to evict is refused (D217 §5)",
		},
		{
			tableNode, "prepared_disk_min_free_mib", "100",
			"MiB the prepared directory's disk keeps free, whatever its fullness limit; below it, stores are " +
				"evicted, and a run of chunks with nothing to evict is refused",
		},
	}, nodeCapacityConfigKeys()...)
}

// nodeCapacityConfigKeys are the node's keys for what its machine can do, and how it watches.
func nodeCapacityConfigKeys() []configKey {
	return []configKey{
		{
			tableNode, "encode_slot_count", "1",
			"encodes this node runs at once, which its processor or GPU bounds (cut-a §5.3)",
		},
		{
			tableNode, "remux_slot_count", "2",
			"remuxes this node runs at once, which its disk bounds (cut-a §5.3)",
		},
		{
			tableNode, "slowest_read_mib_per_second", "20",
			"the slowest this node reads a whole file, a busy spinning disk or a network mount: how long a " +
				"run that reads one is allowed, and how long the core waits for a file's read-through",
		},
		{
			// Long enough for a download's or a copy's next write to come, short enough that a new film
			// is in the library soon after it lands; a copy that never pauses is noticed anyway.
			tableNode, "watch_quiet_seconds", "30",
			"seconds a root goes without a change before what its watcher saw there is scanned, so a copy " +
				"in progress settles first (D272 §9)",
		},
	}
}

// logConfigKeys are the file's keys for logging.
func logConfigKeys() []configKey {
	return []configKey{
		{tableLog, "level", `"info"`, "debug, info, warn or error"},
		{tableLog, "format", `"json"`, "json for the service's journal, text for a person's terminal"},
		{
			tableLog, "destination", `"stderr"`,
			`"stderr", or the absolute path of a file the log is appended to (D242 §3)`,
		},
	}
}
