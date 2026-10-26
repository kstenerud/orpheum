// SPDX-License-Identifier: AGPL-3.0-or-later

// Package assembly assembles the core's services: Build makes every service the core runs over its
// stores and wires them to each other, the API's dependencies among them, and the jobs table, each
// kind's runner and each sweep (D351). cmd/orpheum, the composition root, calls it, and so do the
// API's test harness and the metadata scenarios, so the tests run the server's own wiring; the root
// opens what Build is given, adds what its config alone makes (the certificates, the discovery, port
// mapping, the reachability check, the relay) and runs the lifecycle: the node channel, the runners
// and the loops on their tickers.
//
// # Invariants
//
//   - Build starts nothing: no goroutine, listener or ticker; the composition root runs what it
//     returns (D351 §2).
//   - It is core code under every core rule: it reads no environment, opens no file but through
//     internal/core/instancedir, makes no root context and builds no logger (D351 §5). Enforced by
//     the forbidigo rules tagged [ambient], [core-fs] and [context], and the depguard rule
//     core-boundary.
//   - What a test fakes is a part, nil in production: the node channel (Nodes, the union of what each
//     service asks of nodes.Service and the setters that wire them to it), its bytes, the scan
//     requests, the file reads, OpenSubtitles and the metadata providers (D351 §3). A faked node
//     channel with real bytes is refused, since only the core's own reads them. Enforced by
//     TestBuild_AFakedNodeChannelNeedsItsBytesFakedToo.
//   - The metadata scenarios run the queue, TMDB's provider service and the matcher made as Build
//     makes them (newMetadataJobs), against a TMDB on loopback:
//     TestScenario_S25_ATMDBOutageOvernightParksAndMatchesAllByMorning,
//     TestScenario_S70_APersonsRefreshWhileTMDBIsParkedLooksAtOnceAndRunsFirstWhenItReturns,
//     TestScenario_S73_ARefusedKeyParksUntilTheKeyChanges and
//     TestScenario_S78_DiscardingCacheDBLosesOnlyTiming.
//   - Every kind of job runs one at a time, and its work is derived by a sweep or asked for again by
//     a play or a scan request; a failed job is its item's own fact where it records one (D231 §3,
//     §4). Enforced by TestRunners_EveryKindRunsOneAtATime and
//     TestRunners_EveryKindsWorkIsDerivedByASweepOrAskedForByAPlayAndItsOwnFailureIsAFact.
//
// # Read before changing
//
//   - D351: why the assembly is a package of its own, and what stays in cmd/orpheum.
//   - D231 §3, §4: the sweeps and the runners.
//   - D234 §7: the exchanges, which give the nodes their work.
//   - D212: the artwork service and identify never import each other; the wiring here joins them.
//
// # Where to change
//
//   - assembly.go: Parts, Services and Build; the accounts, the Watch Together sessions, the jobs'
//     services, the library admin, and what is wired into the API.
//   - nodes.go: the node channel (Nodes), the exchanges, the ingest and the scans, and the byte
//     socket.
//   - metadata.go: the metadata jobs' queue, pass and matcher, made one way for Build and the
//     scenarios (newMetadataJobs), and the outside hosts' provider services.
//   - artwork.go: the artwork service, and the images identify found handed to it.
//   - playback.go: the playback service and what it asks of the nodes: read-throughs, subtitles,
//     chunks.
//   - jobs.go: the jobs table, each kind's runner (KindRunners) and each sweep (AddSweeps).
package assembly
