// SPDX-License-Identifier: AGPL-3.0-or-later

// Package nodeproto is the wire between core and node: the JSON messages of cut-a §4 (hello, the
// status, the commands, and the results and progress of the jobs they carry), the byte socket's
// routes and the probe record, as plain types. The version is the path's (/node/v1, /bytes/v1);
// no message carries one. It holds no logic, so both sides can import it without importing each
// other.
//
// # Invariants
//
//   - Imports nothing from the module except internal/platform. Enforced by the depguard rule
//     nodeproto-is-types and TestArch_NodeprotoHasNoLogic.
//   - Every field is camelCase on the wire with an explicit tag. Enforced by tagliatelle and
//     musttag.
//   - A closed set of record or command types is a sum type, and every type switch over it is
//     exhaustive. Enforced by gochecksumtype.
//   - Every hello says the protocol version it speaks (ProtocolVersion), raised with any change the
//     other side would misread (D233 §9). A change to any message's fields, their types or wire
//     names, fails until the version is raised and its shape recorded:
//     TestProtocolVersion_IsRaisedWithAnyChangeToTheWire. So does a route added, moved or dropped,
//     until the version is raised and its routes recorded:
//     TestProtocolVersion_IsRaisedWithAnyChangeToTheRoutes. A change to what a value means is the
//     review's.
//   - Every probe says the ProbeVersion that read it, raised with any change to what the probe says
//     of a file, so a file an older probe read is read again (D535 §1, §2). A change to the probe's
//     fields fails until it is raised and its shape recorded:
//     TestProbeVersion_IsRaisedWithAnyChangeToTheProbe; the wire's shape counts the version, so a
//     raise raises ProtocolVersion too. A change to how a fact is read is the review's.
//   - Each thing the node makes has a recipe (recipe.go), raised with any change to the FFmpeg
//     arguments it is made by, and named in the key of what it made, so what an older recipe made is
//     never served as the new one's (D536 §1, §2). Each recipe's first version adds nothing to a key
//     (D536 §4). Its arguments for a fixed set of commands are recorded beside its version, a change
//     failing until it is raised: TestChunkRecipes_AreRaisedWithAnyChangeToTheirArguments
//     (internal/node/chunks), TestFrameStillRecipe_IsRaisedWithAnyChangeToItsArguments
//     (internal/node/artwork), TestLoudnessRecipe_IsRaisedWithAnyChangeToItsArguments
//     (internal/node/readthrough); the wire's shape counts every recipe, so a raise raises
//     ProtocolVersion too. A change to what the node does with FFmpeg's output is the review's.
//   - A command and a result name their job, and the attempt at it (its token), in the envelope
//     alone, and the node answers every job on one path (D234 §2, D238 §2).
//     TestEnvelope_AJobIsNamedByTheEnvelopeAloneNeverByWhatItCarries.
//
// # Read before changing
//
//   - D240: a scan is a listing and an examining, each a job, whose records are a result's.
//   - D100: a command names its target by path and payload hash.
//   - D107: the node channel is authenticated by socket permissions in Cut A.
//   - D234 §2: the core sends requests the node answers, each matched by id.
//   - D127: the node dials out; the file channel is deleted (D193).
//   - D85: the node protocol is under the compatibility contract.
//   - docs/contributors/design/plans/cut-a.md §4.
//
// # Where to change
//
//   - One file per feature, each holding both directions: hello.go, report.go (the scan records and
//     the node's status), listing.go (a scan's list and examine commands and their answers),
//     commands.go (the long-poll, each command naming its job), result.go (the node's word on how
//     an attempt ended, on one path), progress.go (its word on where one is, best effort, on
//     another), and artwork.go, chunks.go, subtitle.go, readthrough.go, repeats.go and
//     trickplay.go, each a command's payload and its answer's; speedtrial.go, a row of the
//     node's start-up speed trials, which its status carries; trickplay.go also posts each sheet a
//     run finishes, a piece, on a path of its own. stall.go: what a node saw of an FFmpeg run it
//     stopped as stalled, which
//     those answers carry. bytes.go names a file on the node's byte socket, and frame.go asks it for a
//     key frame (D545). probe.go: the probe
//     record a scan reports and the core keeps.
package nodeproto
