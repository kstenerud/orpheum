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
  mediaCarrierOf,
  tellMediaCarriers,
  directFetch,
  namingFailures,
  reportFailure,
  rtcTogetherSocket,
  PathKeeper,
  type CarrierSeen,
  type FetchRequest,
  type MediaCredentials,
  type OpenTogetherSocket,
  type RtcTransport,
} from "@orpheum/sdk";

import type { Reached, Reaching } from "./discovery-start.ts";
import { reportPathChoice, type PathChoice } from "./path-choice.ts";
import { pathName } from "./transport-choice.ts";

/** What a page keeps of the paths to its core. */
export interface KeptPath {
  /** Sends a request on the path in use, moving to another when it fails. */
  readonly fetch: FetchRequest;
  /** Opens the Watch Together socket on the path in use. */
  readonly openTogetherSocket: OpenTogetherSocket;
  /** Tells listener each time the requests move to another path; returns its removal. */
  readonly onPathMove: (listener: (to: string) => void) => () => void;
  /** The kind of path the requests and the media go over now. */
  readonly pathInUse: () => Reached["kind"];
  /** What carries the load of a media url now, for a stuck chunk's log (the SDK's mediaCarrierOf). */
  readonly mediaCarrierOf: (url: string) => string;
  /**
   * Tells listener each time the way to the core may work again: on WebRTC, its transport's word
   * (RtcTransport.onBack: the browser back online, or a connection up after none); on a core's name,
   * the browser back online. Returns its removal.
   */
  readonly onPathBack: (listener: () => void) => () => void;
  /**
   * Tells listener once the WebRTC transport in use is refused for good: the core refused what the
   * page proves its connections by, and what else it holds (RtcTransport.onRefused). Returns its
   * removal.
   */
  readonly onPathRefused: (listener: () => void) => () => void;
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
/** The WebRTC connection carrying url's load on path, for a stuck chunk's log; none on a direct path. */
function carrierOn(path: Reached, url: string): CarrierSeen | undefined {
  return path.kind === "direct" ? undefined : path.transport.carrierOf(url);
}

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
 * Tells the core's log how the page chose its path, over send's fetch, with the pairs of transport
 * where it is WebRTC's (D354 §5): for the first path, and again for each WebRTC transport moved to
 * later, once each, as a page whose LAN name failed moves to WebRTC (D300 §4) and nothing said
 * whether its pairs worked (code debt, 2026-10-27). A failure to tell it is dropped: it is for the
 * log alone. The choice is read when told, its race over (D414 §4).
 */
function choiceTeller(
  send: () => FetchRequest,
  session: { readonly token: () => string | undefined },
  reaching: { readonly choice?: PathChoice },
): (transport: RtcTransport | undefined) => void {
  return (transport) => {
    reportPathChoice(send(), {
      token: session.token,
      choice: () => reaching.choice,
      transport,
    }).catch(() => undefined);
  };
}

/**
 * Keeps the paths reaching found, moving to another when the one in use fails and to a better one
 * when it answers later, as at home once Firefox's local-network prompt is allowed; media loads are
 * signed in with the session's token until setMediaCredentials says more. How the first path was
 * chosen, when reaching says, is told to the core's log over it once signed in (path-choice.ts), and
 * again with the pairs of each WebRTC transport it moves to later, once for each.
 */
export function keepPath(
  reaching: Omit<Reaching, "choice"> & { readonly choice?: PathChoice },
  session: { readonly token: () => string | undefined },
): KeptPath {
  let isFirst = true;
  let inUse: Reached = reaching.first;
  let openSocket = socketOn(reaching.first);
  let credentials = (): MediaCredentials => ({ token: session.token(), viewer: undefined });
  const moveListeners = new Set<(to: string) => void>();
  const used = transportsUsed(() => inUse);
  const tellChoice = choiceTeller(() => keeper.fetch, session, reaching);
  const keeper = new PathKeeper<Reached>(reaching.first, {
    sendOn: (path) => sendingOn(path).send,
    another: reaching.another,
    onUse: (path) => {
      const isNewTransport = used.add(path);
      if (!isFirst) {
        // Never the first path's use, which the keeper tells as it is made, before looking is.
        looking.moved(inUse);
        if (isNewTransport && path.kind === "webrtc") {
          tellChoice(path.transport); // the first path's is told below
        }
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
  tellMediaCarriers((url) => carrierOn(inUse, url));
  const carried = carryMediaOver(keeper.fetch, () => credentials(), sendingOn(reaching.first).name);
  tellChoice(reaching.first.kind === "webrtc" ? reaching.first.transport : undefined);
  carried.catch(reportFailure);
  const looking = lookForBetterPaths(keeper, reaching, pathJudge(session));
  used.onClose(looking.stop);
  return {
    fetch: keeper.fetch,
    openTogetherSocket: (listeners) => openSocket(listeners),
    onPathMove: (listener) => {
      moveListeners.add(listener);
      return () => moveListeners.delete(listener);
    },
    pathInUse: () => inUse.kind,
    mediaCarrierOf,
    onPathBack: used.onPathBack,
    onPathRefused: used.onPathRefused,
    setMediaCredentials: (named) => {
      credentials = named;
    },
    carried,
    close: used.close,
  };
}

/** The word that the way to the core may work again, from the path in use. */
interface PathBack {
  /** Hears path's transport, when it is one, from now on, telling only while it is in use. */
  readonly hear: (path: Reached) => void;
  readonly listen: (listener: () => void) => () => void;
  readonly stop: () => void;
}

/**
 * Tells its listeners when the way to the core may work again, as the path inUse says: a WebRTC
 * transport's onBack, which hears the browser back online itself, so it is told once; on a core's
 * name, the browser's online event.
 */
function pathBack(inUse: () => Reached): PathBack {
  const listeners = new Set<() => void>();
  const tell = (): void => {
    for (const listener of listeners) {
      listener();
    }
  };
  const heard = new Set<unknown>();
  const stops: (() => void)[] = [];
  const online = (): void => {
    if (inUse().kind === "direct") {
      tell();
    }
  };
  window.addEventListener("online", online);
  stops.push(() => {
    window.removeEventListener("online", online);
  });
  return {
    hear: (path) => {
      if (path.kind !== "webrtc" || heard.has(path.transport)) {
        return;
      }
      const { transport } = path;
      heard.add(transport);
      stops.push(
        transport.onBack(() => {
          const now = inUse();
          if (now.kind === "webrtc" && now.transport === transport) {
            tell();
          }
        }),
      );
    },
    listen: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    stop: () => {
      for (const stop of stops.splice(0)) {
        stop();
      }
    },
  };
}

/** The WebRTC transports a page used, closed together. */
interface TransportsUsed {
  /** Keeps path's transport, when it is one, to close; whether it is a transport not kept before. */
  readonly add: (path: Reached) => boolean;
  /** Names what else closing does. */
  readonly onClose: (also: () => void) => void;
  /** Closes every transport kept, and does what onClose named. */
  readonly close: () => void;
  /** Tells listener each time the way to the core may work again (pathBack); returns its removal. */
  readonly onPathBack: (listener: () => void) => () => void;
  /** Tells listener once a transport is refused for good while in use (pathRefused); returns its removal. */
  readonly onPathRefused: (listener: () => void) => () => void;
}

/** The word that the transport in use is refused for good. */
interface PathRefused {
  /** Hears path's transport, when it is one, telling only while it is in use. */
  readonly hear: (path: Reached) => void;
  readonly listen: (listener: () => void) => () => void;
  readonly stop: () => void;
}

/** Tells its listeners, once, when the WebRTC transport inUse is refused for good. */
function pathRefused(inUse: () => Reached): PathRefused {
  const listeners = new Set<() => void>();
  const stops: (() => void)[] = [];
  let isTold = false;
  return {
    hear: (path) => {
      if (path.kind !== "webrtc") {
        return;
      }
      const { transport } = path;
      stops.push(
        transport.onRefused(() => {
          const now = inUse();
          if (!isTold && now.kind === "webrtc" && now.transport === transport) {
            isTold = true;
            for (const listener of listeners) {
              listener();
            }
          }
        }),
      );
    },
    listen: (listener) => {
      listeners.add(listener);
      return () => {
        listeners.delete(listener);
      };
    },
    stop: () => {
      for (const stop of stops.splice(0)) {
        stop();
      }
    },
  };
}

/**
 * Each WebRTC transport a page used, closed with its path or when the page leaves for good: one it
 * moved away from kept its connections, a share of its network's at the core, for as long as the tab
 * stayed open. A page leaving closes them itself, so the core frees them at once rather than when
 * their checks time out; a page kept for the back button keeps them. Each one's word that the way to
 * the core is back is told while it is the path inUse.
 */
function transportsUsed(inUse: () => Reached): TransportsUsed {
  const transports = new Set<{ readonly close: () => void }>();
  const back = pathBack(inUse);
  const refused = pathRefused(inUse);
  let also = (): void => undefined;
  const close = (): void => {
    window.removeEventListener("pagehide", onPageHide);
    also();
    back.stop();
    refused.stop();
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
      const isNew = path.kind === "webrtc" && !transports.has(path.transport);
      if (isNew) {
        transports.add(path.transport);
        refused.hear(path);
      }
      back.hear(path);
      return isNew;
    },
    onClose: (named) => {
      also = named;
    },
    close,
    onPathBack: back.listen,
    onPathRefused: refused.listen,
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
