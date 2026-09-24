// SPDX-License-Identifier: GPL-3.0-or-later

// Command orpheum is the Orpheum media server: one binary whose subcommand chooses the role
// (run, core, node) or the operation (migrate, backup, restore, doctor, setup-code). The
// systemd unit runs `orpheum run`, which starts the core, and the core supervises the node as a
// child process.
//
// This is the composition root. It is the only package that reads argv, the environment or the
// process's standard streams, and the only one that exits; it builds the logger, the clock and
// every dependency, and passes typed values down.
//
// # Invariants
//
//   - Only this package reads the environment, argv or stdio, or calls os.Exit. Enforced by the
//     forbidigo rule tagged [ambient].
//   - Only this package prints. Enforced by the forbidigo rule tagged [print].
//   - A planned subcommand that is not built yet exits 2 and says so; it never exits 0.
//     Enforced by TestRun_PlannedSubcommandsAreNotImplementedYet in main_test.go.
//   - The dev subcommand, which seeds public passwords, exists only in -tags dev builds. Enforced
//     by TestRun_DevSubcommandExistsOnlyInDevBuilds.
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
//   - D43: one binary, one process per role, and a thin optional supervisor.
//   - D44: prefer the platform's supervisor, and never exit when you could report.
//   - D181: migrate is also a command, so an operator can run it without starting the server.
//   - D195: the binary is named orpheum; environment variables take an ORPHEUM_ prefix.
//   - docs/contributors/design/plans/cut-a.md §2 and §3.
//
// # Where to change
//
//   - main.go: the subcommand table and dispatch. A subcommand's work lives in the package it
//     names (internal/store for migrate, internal/core/backup for backup and restore); main
//     only parses flags and wires dependencies.
//   - config.go: the config file (cut-a §2a), every key required, each with a starting value or
//     none (D200, D201). configfill.go: running a missing key on its starting value (D202). serve.go:
//     orpheum run, which seeds the settings at start. operations.go: migrate, backup, restore,
//     setup-code and openapi. doctor.go: doctor, the one list of what to fix. dev_seed.go: orpheum dev seed,
//     in dev builds only.
package main
