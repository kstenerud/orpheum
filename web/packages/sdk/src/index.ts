// SPDX-License-Identifier: MPL-2.0

/**
 * The public entry point of the Orpheum SDK: the only module other packages and apps import
 * from this package (`exports` in package.json forbids deeper paths).
 *
 * The SDK is React-free. It holds the generated API types and client, and will hold
 * probe-and-pick and the Watch Together client (plans/cut-a.md §3, §5).
 */

export { apiPrefix, createApi } from "./client.ts";
export type { Api, ApiOptions } from "./client.ts";
export type { components, operations, paths } from "./gen/api.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const sdkPackageName = "@orpheum/sdk";
