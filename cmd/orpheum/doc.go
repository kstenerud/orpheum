// SPDX-License-Identifier: AGPL-3.0-or-later

// Command orpheum is the Orpheum media server: one binary whose subcommand chooses the role
// (core, node) or the operation (migrate, backup, restore, doctor, setup-code, reset-admin, openapi,
// version, and dev in dev builds). The core and each node are services of their own, `orpheum core` and
// `orpheum node`, which the runtime starts and restarts (D233 §2, D235 §4); the core starts no node.
//
// This is the composition root. It is the only package that reads argv, the environment or the
// process's standard streams, and the only one that exits; it builds the logger and the clock, opens
// the instance's directories and databases, and runs the lifecycle: the listeners, the node channel,
// the job runners and loops on their tickers, the shutdown. The core's services are assembled by
// internal/core/assembly, which it gives what it opened and read, and which the tests share; what
// the config alone makes (the certificates, the discovery, port mapping, the reachability check, the
// relay, WebRTC, the debug endpoints) it makes here, beside them (D351).
//
// # Invariants
//
//   - Only this package reads the environment, argv or stdio, or calls os.Exit. Enforced by the
//     forbidigo rule tagged [ambient].
//   - A release build reads no environment but TZ, once, at the edge, as the default of the
//     server's time zone (D246 §7): its subcommands get an environment with TZ alone, and only a
//     dev build passes ORPHEUM_DEV_* on (D204). Enforced by TestEnvironmentLookup_SeesOnlyTZInARelease,
//     and by TestArch_ReleaseBinaryReadsNoEnvironment, which fails on any environment read in a
//     file a release build compiles but os.LookupEnv(timeZoneVariable). The zone found is
//     TestMachineZone_IsTZsElseTheSystemsElseUTC's.
//   - Every time the server formats is UTC, whatever TZ says (D204). Enforced by
//     TestUseUTC_TimesAreUTCWhateverTZSays.
//   - core starts no node, and node runs the node's loop on its own: a node's work is its own, which a
//     core restart does not touch (D233 §2). The node refuses to run while
//     node.confinement_policy is "require" and a confinement mechanism is not in force (D112).
//     Enforced by TestRunNode_RefusesToRunUnconfinedWhenConfinementIsRequired.
//   - The node role, node.go, imports nothing from internal/core or internal/store, as the node's
//     own packages do not (D33); what both roles need is in internal/platform. Enforced by the
//     depguard rule node-role-boundary.
//   - Only this package prints. Enforced by the forbidigo rule tagged [print].
//   - A planned subcommand that is not built yet exits 2 and says so; it never exits 0.
//     Enforced by TestDispatch_PlannedSubcommandsAreNotImplementedYet in main_test.go.
//   - The dev subcommand, which seeds public passwords, exists only in -tags dev builds. Enforced
//     by TestDispatch_DevSubcommandExistsOnlyInDevBuilds.
//   - run answers /health before migrating, says "migrating" before each step, and stays up to
//     report a failed start rather than exiting. Enforced by
//     TestServe_HealthSaysMigratingBeforeTheMigrationStarts and
//     TestServe_ReportsAFailedStartInsteadOfExiting.
//   - Bootstrap sweeps the settings — writing missing rows, moving unmodified ones to a changed
//     starting value — before anything reads one or the server reports ready (D201). Enforced by
//     TestServe_SweepsUnmodifiedSettingsToTheirStartingValuesBeforeItIsReady.
//   - A secret appears in no API response, log, doctor output, backup or database file except as
//     ciphertext (D194 §5). Enforced by TestCanary_ASecretAppearsNowhereButAsCiphertext, seen to
//     fail against a planted leak.
//
// # Read before changing
//
//   - D43: one binary, one process per role; D233 §2 and D235 §4 ended its thin supervisor.
//   - D44: prefer the platform's supervisor, and never exit when you could report.
//   - D181: migrate is also a command, so an operator can run it without starting the server.
//   - D351: the core's services are assembled in internal/core/assembly; this package reads the
//     outside world and runs the lifecycle.
//   - D195: the binary is named orpheum; environment variables take an ORPHEUM_ prefix.
//   - docs/contributors/design/plans/cut-a.md §2 and §3.
//
// # Where to change
//
//   - main.go: the subcommand table and dispatch. A subcommand's work lives in the package it
//     names (internal/store for migrate, internal/core/backup for backup and restore); main
//     only parses flags and passes what it read down.
//   - instance.go: an instance directory's top-level config, its kind and its directories, and
//     making one (default-config, D242). config.go: its full config (cut-a §2a), the keys of its
//     kind, each with a starting value or none (D200, D201). configfill.go: running a missing key
//     on its starting value (D202). serve.go: orpheum core, which seeds the settings at start.
//     operations.go: default-config, migrate, backup, restore, setup-code and openapi.
//     reset_admin.go: reset-admin, the operator's way back in (D294), terminal_linux.go its
//     password read without echo.
//     doctor.go: doctor, the one list of what to fix. dev_seed.go: orpheum dev seed, in dev
//     builds only.
//   - serve.go's openAndWire and wireCore: what is opened, seeded and read for the assembly, and what
//     is made beside it. A service of the core's, or its wiring to another, is
//     internal/core/assembly's.
//   - metadata.go: the runners, sweeps and loops the core runs on their tickers. artwork.go: the
//     artwork cache's upkeep. nodes.go: the node channel's listener. node.go: orpheum node, the media
//     node itself.
package main
