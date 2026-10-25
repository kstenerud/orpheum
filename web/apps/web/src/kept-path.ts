// SPDX-License-Identifier: MPL-2.0

/**
 * The path a page served by a discovery keeps to its core once the gate reached it (path-keeper.ts;
 * webrtc-transport.md W2c items 5 and 6): its requests sent on the path in use, moved to another when
 * it fails and to a better one when that answers later, the Watch Together socket opened on it, and
 * media carried over it by the service worker. The loader keeps it and hands the client what it needs
 * of it (D308).
 */

import {
  apiPrefix,
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
  readonly onPathMove: (listener: (to: string) => void) => () => void;
  /** Names what media loads are signed in with. */
  readonly setMediaCredentials: (credentials: () => MediaCredentials) => void;
  /**
   * Settles once the service worker carries the page's media and controls the page, so loads the
   * page makes by itself go through it: rejects where no service worker runs.
   */
  readonly carried: Promise<void>;
  /**
   * Stops looking for better paths and closes every WebRTC transport the page used, the one in use
   * or one it moved away from; the page's hiding for good does too.
   */
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

/**
 * How long the page waits to ask whether a better path than the one in use answers now (W2c item 6):
 * betterPathFirstMs, twice as long after each move away from a path that failed, up to
 * betterPathMostMs. Firefox fails a LAN name's requests once the page's service worker controls it,
 * yet answers its look for a better path: a host's page moved to the LAN name and back off it 171
 * times in a 53-minute session, a request failing at each (the owner's Watch Together session on
 * nas.example, 2026-10-18).
 */
export const betterPathFirstMs = 30_000;
export const betterPathMostMs = 10 * 60_000;

/**
 * How many times a page moves away from a path for a failed request before it moves to it no more:
 * Firefox's Local Network Access fails some of a LAN name's requests and lets others through,
 * whatever they carry, so a page that went back each time a look found it answering lost a request
 * at each move (the owner's Firefox on staging, 2026-10-20). A reload tries it again; at home the
 * page stays on WebRTC, a direct pair in the house, which counts as home (D300).
 */
export const failuresBeforeGivingUp = 2;

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
  let inUse: Reached = reaching.first;
  let openSocket = socketOn(reaching.first);
  let credentials = (): MediaCredentials => ({ token: session.token(), viewer: undefined });
  const moveListeners = new Set<(to: string) => void>();
  const used = transportsUsed();
  const keeper = new PathKeeper<Reached>(reaching.first, {
    sendOn: (path) => sendingOn(path).send,
    another: reaching.another,
    onUse: (path) => {
      used.add(path);
      if (!isFirst) {
        // Never the first path's use, which the keeper tells as it is made, before looking is.
        looking.moved(inUse);
      }
      inUse = path;
      // eslint-disable-next-line no-console -- which path a tab's requests take is read in its console (D273 §7)
      console.info(`Orpheum: requests now go over ${sendingOn(path).name}`);
      openSocket = socketOn(path);
      if (!isFirst) {
        for (const listener of moveListeners) {
          listener(sendingOn(path).name);
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
  const looking = lookForBetterPaths(keeper, reaching, pathJudge(session));
  used.onClose(looking.stop);
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
    close: used.close,
  };
}

/** The WebRTC transports a page used, closed together. */
interface TransportsUsed {
  /** Keeps path's transport, when it is one, to close. */
  readonly add: (path: Reached) => void;
  /** Names what else closing does. */
  readonly onClose: (also: () => void) => void;
  /** Closes every transport kept, and does what onClose named. */
  readonly close: () => void;
}

/**
 * Each WebRTC transport a page used, closed with its path or when the page leaves for good: one it
 * moved away from kept its connections, a share of its network's at the core, for as long as the tab
 * stayed open. A page leaving closes them itself, so the core frees them at once rather than when
 * their checks time out; a page kept for the back button keeps them.
 */
function transportsUsed(): TransportsUsed {
  const transports = new Set<{ readonly close: () => void }>();
  let also = (): void => undefined;
  const close = (): void => {
    window.removeEventListener("pagehide", onPageHide);
    also();
    for (const transport of transports) {
      transport.close();
    }
    transports.clear();
  };
  function onPageHide(event: Event): void {
    if (Reflect.get(event, "persisted") !== true) {
      close();
    }
  }
  window.addEventListener("pagehide", onPageHide);
  return {
    add: (path) => {
      if (path.kind === "webrtc") {
        transports.add(path.transport);
      }
    },
    onClose: (named) => {
      also = named;
    },
    close,
  };
}

/**
 * Whether path carries a request as the page's own requests go: with the session's Authorization
 * header, which needs the CORS preflight a bare health check never makes. Firefox's Local Network
 * Access answers the bare check at a LAN name and then fails every request the page makes there, so a
 * page moved by the check alone went to the LAN name and back off it at each look, losing a request
 * each time and a film's posters with them (staging and nas.example, 2026-10-20; idiosyncrasies.md).
 */
async function carriesRequests(
  path: Reached,
  session: { readonly token: () => string | undefined },
): Promise<boolean> {
  const token = session.token();
  const headers = new Headers();
  if (token !== undefined) {
    headers.set("Authorization", `Bearer ${token}`);
  }
  const request = new Request(`${globalThis.location.origin}${apiPrefix}/health`, { headers });
  try {
    const answer = await sendingOn(path).send(request);
    return answer.ok || answer.status === 503;
  } catch {
    return false;
  }
}

/** How the look judges a path: whether it carries a request, and whether the page gave it up. */
interface PathJudge {
  /** Whether path carries a request as the page's go (carriesRequests). */
  readonly carries: (path: Reached) => Promise<boolean>;
  /** Counts a move away from a path for a request that failed there. */
  readonly count: (from: Reached) => void;
  /** Whether the page moved away from path failuresBeforeGivingUp times. */
  readonly isGivenUp: (path: Reached) => boolean;
}

/** The judge of the paths of a page signed in as session. */
function pathJudge(session: { readonly token: () => string | undefined }): PathJudge {
  const byName = new Map<string, number>();
  return {
    carries: (path) => carriesRequests(path, session),
    count: (from) => {
      const name = sendingOn(from).name;
      byName.set(name, (byName.get(name) ?? 0) + 1);
    },
    isGivenUp: (path) => (byName.get(sendingOn(path).name) ?? 0) >= failuresBeforeGivingUp,
  };
}

/**
 * Tells chance each time a better path may answer at once: the browser back online, or allowed to
 * reach the LAN name at last (W2c item 1). Returns the end of telling.
 */
function hearChances(
  reaching: Pick<Reaching, "onLocalNetworkGranted">,
  chance: () => void,
): () => void {
  window.addEventListener("online", chance);
  const unhear = reaching.onLocalNetworkGranted(chance);
  return () => {
    window.removeEventListener("online", chance);
    unhear();
  };
}

/** The page's look for a better path, as keepPath runs it. */
interface BetterPaths {
  /** Told each move, from the path left: one it did not make itself is away from a path that failed. */
  readonly moved: (from: Reached) => void;
  readonly stop: () => void;
}

/**
 * Looks for a better path than keeper's every betterPathFirstMs, and moves to it once it carries a
 * request as the page's go (carries): twice as long after each move away from a path that failed, or
 * a better path that did not carry one, up to betterPathMostMs, and at once, as often as at first,
 * when the browser is back online. A path the page moved away from failuresBeforeGivingUp times is
 * moved to no more.
 */
function lookForBetterPaths(
  keeper: PathKeeper<Reached>,
  reaching: Pick<Reaching, "better" | "onLocalNetworkGranted">,
  judge: PathJudge,
): BetterPaths {
  let waitMs = betterPathFirstMs;
  let isMoving = false;
  let isStopped = false;
  let next: ReturnType<typeof setTimeout> | undefined;
  const backOff = (): void => {
    waitMs = Math.min(waitMs * 2, betterPathMostMs);
    clearTimeout(next);
    lookLater();
  };
  const look = (): void => {
    void reaching.better(keeper.path).then(async (better) => {
      if (better === undefined || judge.isGivenUp(better)) {
        return;
      }
      const isCarried = await judge.carries(better);
      if (isStopped || !isCarried) {
        if (!isStopped) {
          backOff();
        }
        return;
      }
      isMoving = true;
      keeper.moveTo(better);
      isMoving = false;
    });
  };
  const lookLater = (): void => {
    next = setTimeout(() => {
      look();
      if (!isStopped) {
        lookLater();
      }
    }, waitMs);
  };
  const backOnline = (): void => {
    waitMs = betterPathFirstMs;
    look();
  };
  lookLater();
  const unhear = hearChances(reaching, backOnline);
  return {
    moved: (from) => {
      if (!isMoving) {
        judge.count(from);
        backOff();
      }
    },
    stop: () => {
      isStopped = true;
      clearTimeout(next);
      unhear();
    },
  };
}
