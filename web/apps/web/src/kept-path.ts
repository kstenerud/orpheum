// SPDX-License-Identifier: MPL-2.0

/**
 * The path a page served by a discovery keeps to its core once the gate reached it (path-keeper.ts;
 * webrtc-transport.md W2c items 5 and 6): its requests sent on the path in use, moved to another when
 * it fails and to a better one when that answers later, the Watch Together socket opened on it, and
 * media carried over it by the service worker. The loader keeps it and hands the client what it needs
 * of it (D308).
 */

import {
  browserTogetherSocket,
  carryMediaOver,
  directFetch,
  namingFailures,
  reportFailure,
  rtcTogetherSocket,
  PathKeeper,
  type FetchRequest,
  type MediaCredentials,
  type OpenTogetherSocket,
} from "@orpheum/sdk";

import type { Reached, Reaching } from "./discovery-start.ts";
import { pathName } from "./transport-choice.ts";

/** What a page keeps of the paths to its core. */
export interface KeptPath {
  /** Sends a request on the path in use, moving to another when it fails. */
  readonly fetch: FetchRequest;
  /** Opens the Watch Together socket on the path in use. */
  readonly openTogetherSocket: OpenTogetherSocket;
  /** Tells listener each time the requests move to another path; returns its removal. */
  readonly onPathMove: (listener: () => void) => () => void;
  /** Names what media loads are signed in with. */
  readonly setMediaCredentials: (credentials: () => MediaCredentials) => void;
  /**
   * Settles once the service worker carries the page's media and controls the page, so loads the
   * page makes by itself go through it: rejects where no service worker runs.
   */
  readonly carried: Promise<void>;
  /** Stops looking for better paths and closes a WebRTC connection in use. */
  readonly close: () => void;
}

/** What a path is called in the console, and what sends on it. */
function sendingOn(path: Reached): { readonly send: FetchRequest; readonly name: string } {
  const name = path.kind === "direct" ? `the core's name, ${path.origin}` : pathName;
  const send = path.kind === "direct" ? directFetch(path.origin) : path.transport.fetch;
  return { send: namingFailures(send, name), name };
}

/** The Watch Together socket's opener on path. */
function socketOn(path: Reached): OpenTogetherSocket {
  return path.kind === "direct"
    ? browserTogetherSocket(path.origin)
    : rtcTogetherSocket(() => path.transport.openChannel());
}

/** How often the page asks whether a better path than the one in use answers now (W2c item 6). */
const betterPathEveryMs = 30_000;

/**
 * Keeps the paths reaching found, moving to another when the one in use fails and to a better one
 * when it answers later, as at home once Firefox's local-network prompt is allowed; media loads are
 * signed in with the session's token until setMediaCredentials says more.
 */
export function keepPath(
  reaching: Reaching,
  session: { readonly token: () => string | undefined },
): KeptPath {
  let isFirst = true;
  let openSocket = socketOn(reaching.first);
  let credentials = (): MediaCredentials => ({ token: session.token(), viewer: undefined });
  const moveListeners = new Set<() => void>();
  const keeper = new PathKeeper<Reached>(reaching.first, {
    sendOn: (path) => sendingOn(path).send,
    another: reaching.another,
    onUse: (path) => {
      // eslint-disable-next-line no-console -- which path a tab's requests take is read in its console (D273 §7)
      console.info(`Orpheum: requests now go over ${sendingOn(path).name}`);
      openSocket = socketOn(path);
      if (!isFirst) {
        for (const listener of moveListeners) {
          listener();
        }
        // Media follows the keeper's path (carryMediaOver below), so the console says it moved too.
        // eslint-disable-next-line no-console -- which path a tab's media takes is read in its console (D273 §7)
        console.info(`Orpheum: media now goes over ${sendingOn(path).name}`);
      }
      isFirst = false;
    },
  });
  // Media is carried over once, on whichever path the keeper holds when each load is asked for: one
  // carried over per path would answer each load once for every path the page ever used.
  const carried = carryMediaOver(keeper.fetch, () => credentials(), sendingOn(reaching.first).name);
  carried.catch(reportFailure);
  const lookForBetter = (): void => {
    void reaching.better(keeper.path).then((better) => {
      if (better !== undefined) {
        keeper.moveTo(better);
      }
    });
  };
  const every = setInterval(lookForBetter, betterPathEveryMs);
  window.addEventListener("online", lookForBetter);
  return {
    fetch: keeper.fetch,
    openTogetherSocket: (listeners) => openSocket(listeners),
    onPathMove: (listener) => {
      moveListeners.add(listener);
      return () => {
        moveListeners.delete(listener);
      };
    },
    setMediaCredentials: (named) => {
      credentials = named;
    },
    carried,
    close: () => {
      clearInterval(every);
      window.removeEventListener("online", lookForBetter);
      if (keeper.path.kind === "webrtc") {
        keeper.path.transport.close();
      }
    },
  };
}
