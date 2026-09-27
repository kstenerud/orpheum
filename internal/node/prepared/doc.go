// SPDX-License-Identifier: GPL-3.0-or-later

// Package prepared keeps a node's prepared renditions within their budget (D217 §5): the node
// alone deletes in its prepared directory, and this is where it does. A rendition's run
// directory's modification time is its last use: the byte socket sets it each time it serves one
// of its files, so it survives a restart. Eviction removes the least recently used first: those
// past a grace after their last use before any within it, never one being written, until the
// directory is within its budget and its filesystem within its fullness limit. A prepare that
// would find the disk past that limit with nothing left to evict is refused rather than fill it.
//
// # Invariants
//
//   - Nothing but this package and internal/node/trash deletes on the node. Enforced by the
//     forbidigo rules tagged [delete].
//   - It deletes only whole run directories under its own directory, three levels down
//     (<file_id>/<profile>/<run_id>), and the empty folders they leave. Enforced by
//     TestEvict_RemovesTheLeastRecentlyUsedPastTheGraceFirstAndNeverOneBeingWritten.
//   - A rendition within its grace is evicted only when the budget or the disk still needs it,
//     after every rendition past its grace; one being written, never.
//     TestEvict_RemovesTheLeastRecentlyUsedPastTheGraceFirstAndNeverOneBeingWritten.
//   - A prepare the disk has no room for is refused, with why.
//     TestRoom_RefusesWhenTheDiskIsPastItsLimitWithNothingToEvict.
//
// # Read before changing
//
//   - D217 §5: a node owns its prepared renditions, their budget, grace and disk limit.
//   - principles §2: the user's files are not ours to touch; these are ours.
//
// # Where to change
//
//   - shelf.go: the budget, last use, eviction and room.
package prepared
