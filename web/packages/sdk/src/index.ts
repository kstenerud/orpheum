// SPDX-License-Identifier: MPL-2.0

/**
 * The public entry point of the Orpheum SDK: the only module other packages and apps import
 * from this package (`exports` in package.json forbids deeper paths).
 *
 * The SDK is React-free. It will hold the generated API types and client, probe-and-pick,
 * and the Watch Together client (plans/cut-a.md §3, §5). It holds nothing yet.
 */

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const sdkPackageName = "@orpheum/sdk";
