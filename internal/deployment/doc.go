// SPDX-License-Identifier: AGPL-3.0-or-later

// Package deployment writes a deployment's directory (D249, D272 §3): the values file the owner
// fills in, the tunnel's key pair, and, from the values and the templates embedded here, the
// compose file, the .env file it reads, the override giving the node this machine's media and GPU,
// the tunnel's config and the core's and the node's full configs. `orpheum write-deployment` runs
// it inside the runtime image, so installing needs the image and Docker alone; staging and release
// are written the same way, so the one path stays tested.
//
// # Invariants
//
//   - Every file but the values file and the keys is written again each time, from the values and
//     this binary's templates, so a new image brings its own (D272 §7); the values file and the
//     keys are made once and never overwritten. TestWrite_RewritesTheFilesAndKeepsTheValuesAndKeys.
//   - Nothing is written while the values file lacks a value or holds a wrong one, and each problem
//     names its key. TestValues_ProblemsNameEachKeyThatLacksAValue.
//   - A value reaches a TOML config quoted, and a template naming a value it was not given is an
//     error, not a blank. TestQuoteTOML_QuotesOrRefuses, TestRender_ANameWithNoValueIsAnError.
//   - Release refuses a database it cannot migrate; staging sets it aside and starts again (D249
//     §4). TestWrite_ReleaseRefusesAnUnmigratableDatabaseAndStagingStartsOver.
//   - The tunnel's key pair is WireGuard's: a clamped X25519 key and its public key.
//     TestNewKeyPair_IsAClampedX25519PairAsWgMakesOne.
//   - Each deployment's end of the tunnel is the values file's (10.77.0.2 and fd77::2 when it names
//     none), so several deployments, each with its own key, share one VPS (D292 §7); the tunnel and
//     TURN take it from there. TestWrite_ADeploymentTakesItsOwnTunnelAddressesForTheTunnelAndTURN.
//   - Keys and the configs naming one are readable by their owner alone.
//     TestWrite_RewritesTheFilesAndKeepsTheValuesAndKeys.
//
// # Read before changing
//
//   - D249: the deployment, its three services and the tunnel in a container.
//   - D272: release on nas.example, the image's own setup, the build re-rendering (§7), one shared
//     tunnel key (§8), which D292 §7 revises: a peer and addresses per deployment.
//   - deploy/staging/README.md and deploy/release/README.md: how a person runs it.
//
// # Where to change
//
//   - templates/: the compose file and the configs. values.go: the values file, its starting
//     values and its checks. write.go: what is written where, the keys.
package deployment
