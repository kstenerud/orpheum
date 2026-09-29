// SPDX-License-Identifier: MPL-2.0

/**
 * The public entry point of the Orpheum SDK: the only module other packages and apps import
 * from this package (`exports` in package.json forbids deeper paths).
 *
 * The SDK is React-free. It holds the generated API types and client, the menu probe (what
 * this browser can play), the wait for a file the server is still making (whenReady,
 * textWhenReady) and the reading of refusals, and will hold the Watch Together client
 * (plans/cut-a.md §3, §5).
 */

export { apiPrefix, artUrl, createApi } from "./client.ts";
export type { Api, ApiOptions, ArtSize } from "./client.ts";
export type { components, operations, paths } from "./gen/api.ts";
export { browserMenu, menuFrom } from "./menu.ts";
export { textWhenReady, whenReady } from "./ready.ts";
export { ApiProblem, dataOrThrow, problemFromAnswer, throwIfRefused } from "./problem.ts";
export type { ErrorDetail } from "./problem.ts";
export type { WaitOptions } from "./ready.ts";
export type { MediaProbe, Menu } from "./menu.ts";

/** The npm name of this package, exported so the workspace wiring has something to resolve. */
export const sdkPackageName = "@orpheum/sdk";
