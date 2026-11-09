// SPDX-License-Identifier: AGPL-3.0-or-later

// Package deployment writes a deployment's directory (D249, D272 §3): the values file the owner
// fills in, the tunnel's key pair, and, from the values and the templates embedded here, the
// compose file, the .env file it reads, the override giving the node this machine's media and GPU,
// the tunnel's config and the core's and the node's full configs. `orpheum write-deployment` runs
// it inside the runtime image, so installing needs the image and Docker alone; staging and release
// are written the same way, so the one path stays tested. A server installed from one compose file
// (deploy/omv/compose.yaml, D447, or a Synology's deploy/synology/compose.yaml, the same with its
// values in the file) has each start, `orpheum start`, write the values file and the instances'
// configs alone from its command line (D449 §1), by WriteStartValues and WriteConfigs, which Write
// writes them with too.
//
// # Invariants
//
//   - Every file but the values file and the keys is written again each time, from the values and
//     this binary's templates, so a new image brings its own (D272 §7); the values file and the
//     keys are made once and never overwritten. TestWrite_RewritesTheFilesAndKeepsTheValuesAndKeys.
//     A compose file's start writes the values file too, each time, from its command line, its header
//     saying the compose file is where a change goes (D449 §1).
//     TestWriteStartValues_IsWrittenEachTimeSayingTheComposeFileRules.
//   - The core's config names how the deployment was installed, the install script (Write) or a
//     compose file's start (WriteConfigs given ByComposeFile), so the System page shows only that
//     way's steps to update (D450 §2). TestWriteConfigs_TheCoreSaysHowTheDeploymentWasInstalled.
//   - Nothing is written while the values file lacks a value or holds a wrong one, and each problem
//     names its key. TestValues_ProblemsNameEachKeyThatLacksAValue.
//   - A value reaches a TOML config quoted, and a template naming a value it was not given is an
//     error, not a blank. TestQuoteTOML_QuotesOrRefuses, TestRender_ANameWithNoValueIsAnError.
//   - Release refuses a database it cannot migrate; staging sets it aside and starts again (D249
//     §4). A release node requires FFmpeg's confinement, and staging's reports it (D273 §5 as
//     revised 2026-10-20). TestWrite_ReleaseRefusesAnUnmigratableDatabaseAndStagingStartsOver.
//   - The tunnel's key pair is WireGuard's: a clamped X25519 key and its public key.
//     TestNewKeyPair_IsAClampedX25519PairAsWgMakesOne.
//   - Each deployment's end of the tunnel is the values file's (10.77.0.2 and fd77::2 when it names
//     none), so several deployments, each with its own key, share one VPS (D292 §7); the tunnel and
//     TURN take it from there. TestWrite_ADeploymentTakesItsOwnTunnelAddressesForTheTunnelAndTURN.
//   - A values file naming no VPS writes a deployment with no tunnel (D314 §3): the core
//     publishes its own ports, serves no relay, and asks for nothing but the media and the address
//     at home; a tunnel half described is a problem, not no tunnel.
//     TestWrite_ACoreWithNoTunnelTakesItsOwnPortsAndServesNoRelay,
//     TestValues_ATunnelHalfDescribedIsAProblemNotNoTunnel.
//   - The address at home is required, and its /24 is the only home network an admin administers
//     from: no private range stands in for it (the Plex-chain review's L1).
//     TestWrite_TheCoreAdministersOnlyFromTheHousesNetwork.
//   - The machine's router at home, which the core's container cannot see, goes to the core for its
//     port mapping to ask, and one that is not a router at home is a problem; left empty, nothing is mapped
//     (item 15 of the owner's comments of 2026-11-02). TestWrite_TheCoreAsksTheRouterTheValuesName.
//   - Each deployment's outside network has a private IPv6 /64 of its own, chosen by its directory,
//     so two on one machine both start; one whose directory is not known keeps fd77:5e::/64.
//     TestWrite_EachDeploymentsNetworkHasASubnetOfItsOwn.
//   - A household's compose file names no kernel setting or feature (no sysctl, no IPv6 network),
//     which Docker refuses on a kernel without it, and the tunnel's names no MPTCP (D454 §2, §4).
//     TestCompose_AHouseholdsFileAsksTheKernelForNothing.
//   - With no tunnel the core shares this machine's network and takes its ports itself, this
//     machine's on its loopback; with one, it listens in its own network on 8440 and 443, published
//     (D454 §5). TestCompose_AHouseholdsCoreSharesThisMachinesNetwork,
//     TestWrite_ACoreWithNoTunnelTakesItsOwnPortsAndServesNoRelay.
//   - The node's root is read-only and its /tmp a tmpfs mounted noexec, nosuid and nodev, so what an
//     exploited FFmpeg writes outside the node's volumes can be neither run nor loaded; the node and
//     FFmpeg write nothing outside them (noexec step 1). TestCompose_TheNodesRootIsReadOnlyAndItsTmpRunsNothing.
//   - The pasted compose file keeps the node's root unwritable, and
//     the install page names every variable it reads.
//     TestOMVCompose_KeepsTheDeploymentsGuardsAndStartsFromItsCommandLine,
//     TestOMVCompose_TheInstallPageNamesEveryVariableItReads.
//   - Keys and the configs naming one are readable by their owner alone.
//     TestWrite_RewritesTheFilesAndKeepsTheValuesAndKeys.
//
// # Read before changing
//
//   - D249: the deployment, its three services and the tunnel in a container; D314: the
//     deployment with no tunnel, two services, that a household installs; D447: installed from one
//     compose file, its first start writing it.
//   - D272: release on the production NAS, the image's own setup, the build re-rendering (§7), one shared
//     tunnel key (§8), which D292 §7 revises: a peer and addresses per deployment.
//   - deploy/staging/README.md and deploy/release/README.md: how a person runs it.
//
// # Where to change
//
//   - templates/: the compose file and the configs. values.go: the values file, its starting
//     values and its checks. write.go: what is written where, the keys.
package deployment
