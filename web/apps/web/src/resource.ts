// SPDX-License-Identifier: MPL-2.0

// Apart from use-resource.ts, which re-exports it, so remembered.ts, which use-resource.ts keeps
// the tab's answers in (D478), can name it without importing its reader.

/**
 * Something loaded from the server: still loading, loaded, or failed with why. A failure keeps
 * its cause, which a screen tells the person in their words (problemText), and the console is
 * told it as it is (reportFailure); one a poller will ask again after says so (willAskAgain), so
 * the person is told they need not.
 */
export type Resource<T> =
  | { readonly state: "loading" }
  | { readonly state: "loaded"; readonly value: T }
  | { readonly state: "failed"; readonly cause: unknown; readonly willAskAgain?: boolean };
