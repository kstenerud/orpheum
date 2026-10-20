// SPDX-License-Identifier: MPL-2.0

/**
 * The browser's local-network permission (webrtc-transport.md W2c item 1): whether a page on a public
 * origin may reach the core's LAN name. Firefox's Local Network Access and Chrome's ask the person
 * first; while the prompt waits Firefox holds the request that raised it, and fails it once the
 * prompt goes unanswered; a private window asks again each time (the owner's Firefox on staging,
 * 2026-10-20). A browser that has no such permission says "unknown", and the page tries the LAN name
 * as it always did.
 */

/** Where the permission stands. */
export type LocalNetworkState = "granted" | "denied" | "prompt" | "unknown";

/** The permission as the page reads it, and is told when it is granted. */
export interface LocalNetwork {
  readonly state: () => Promise<LocalNetworkState>;
  /** Tells listener each time the permission becomes granted; returns its removal. */
  readonly onGranted: (listener: () => void) => () => void;
  /**
   * Tells listener whether the browser is still to ask the person, once it is known and on each
   * change after; returns its removal. A browser with no such permission never asks.
   */
  readonly onAsking: (listener: (isAsking: boolean) => void) => () => void;
}

/**
 * The permission's names, newest first: "local-network" by the current draft, and
 * "local-network-access", which Chrome 142 to 144 named it and later browsers keep as an alias
 * (MDN's Local network access, 2026-10).
 */
const permissionNames = ["local-network", "local-network-access"];

/**
 * The part of the Permissions API the page reads, by any name: the browser's own types know no
 * local-network permission yet.
 */
export interface PermissionsByName {
  query(descriptor: { readonly name: string }): Promise<PermissionStatus>;
}

/** The browser's Permissions API, or none, as in a test's DOM or a browser without it. */
function browserPermissions(): PermissionsByName | undefined {
  return "permissions" in globalThis.navigator ? globalThis.navigator.permissions : undefined;
}

/** The permission as a browser's Permissions API answers it, or "unknown" where it has none. */
export function browserLocalNetwork(
  permissions: PermissionsByName | undefined = browserPermissions(),
): LocalNetwork {
  let status: Promise<PermissionStatus | undefined> | undefined;
  const statusOf = (): Promise<PermissionStatus | undefined> => {
    status ??= (async () => {
      for (const name of permissionNames) {
        try {
          // A name the browser does not know is a TypeError: the next is tried.
          return await permissions?.query({ name });
        } catch {
          // Unknown here.
        }
      }
      return undefined;
    })();
    return status;
  };
  /**
   * Tells listener the state once it is known, when shouldTellNow, and on each change after; returns its
   * removal.
   */
  const watch = (
    listener: (state: LocalNetworkState) => void,
    shouldTellNow: boolean,
  ): (() => void) => {
    let isRemoved = false;
    let remove = (): void => {
      isRemoved = true;
    };
    void statusOf().then((held) => {
      if (isRemoved) {
        return;
      }
      if (shouldTellNow) {
        listener(held?.state ?? "unknown");
      }
      if (held === undefined) {
        return;
      }
      const changed = (): void => {
        listener(held.state);
      };
      held.addEventListener("change", changed);
      remove = () => {
        held.removeEventListener("change", changed);
      };
    });
    return () => {
      remove();
    };
  };
  return {
    state: async () => (await statusOf())?.state ?? "unknown",
    onGranted: (listener) =>
      watch((state) => {
        if (state === "granted") {
          listener();
        }
      }, false),
    onAsking: (listener) =>
      watch((state) => {
        listener(state === "prompt");
      }, true),
  };
}
