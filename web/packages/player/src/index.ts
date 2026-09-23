// SPDX-License-Identifier: MPL-2.0

/**
 * The public entry point of the Orpheum player: the only module other packages and apps import
 * from this package (`exports` in package.json forbids deeper paths).
 *
 * The player will wrap hls.js, JASSUB and the subtitle layer (plans/cut-a.md §3). It holds
 * nothing yet.
 */

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const playerPackageName = "@orpheum/player";
