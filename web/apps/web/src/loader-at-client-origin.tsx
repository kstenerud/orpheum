// SPDX-License-Identifier: MPL-2.0

/**
 * The discovery's loader at a core's client origin (D308): it reaches the core this origin is
 * bound to, by a link, a realm sign-in's hand-off or the session this origin keeps, and runs the
 * core's own client, checked. It refuses any start that names another core.
 */

import {
  createApi,
  noteStep,
  PageNotControlled,
  readLinkFragment,
  refusedClientVersions,
  reportFailure,
  type FetchRequest,
  type KeptCore,
} from "@orpheum/sdk";
import type { JSX } from "react";

import { DiscoveryGate, type GateParts } from "./discovery-gate.tsx";
import {
  heldWith,
  readHandOffFragment,
  whenSignedOut,
  type Reaching,
  type Start,
} from "./discovery-start.ts";
import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";
import { keepPath, type KeptPath } from "./kept-path.ts";
import type { ClientFilesMessage } from "./client-files.ts";
import { ClientFailedPage, clientFailedText } from "./loader-client-failed.tsx";
import { PathRefusedNote } from "./loader-path-refused.tsx";
import type { LoaderConfig } from "./loader-config.ts";
import { loaderMajor, loaderMinor, type OrpheumLoader } from "./loader-interface.ts";
import {
  goneAddress,
  keyRefusedAddress,
  signedOutAddress,
  switchAddress,
} from "./loader-households.ts";
import { checkedClient, ClientRefused, mountClient } from "./loader-run-client.ts";
import { corePrefix, createSession, type Session } from "./session.ts";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { forgetMembership, membershipText, readMembership } from "./together-storage.ts";
import { browserStorage, tabStorage } from "./browser-storage.ts";

/** Where this origin keeps the core it is bound to: its fingerprint and realm, once reached. */
const keptCoreKey = "orpheum.loader.core";

/** What the page at a client origin starts from. */
export interface AtClientOrigin {
  readonly config: LoaderConfig;
  readonly coreId: string;
  readonly storage: StringStorage | undefined;
  readonly location: Pick<Location, "hash" | "pathname" | "search" | "assign">;
  readonly history: Pick<History, "replaceState">;
  /** The origin the discovery's own page, the loader's, is at, which the household list is at. */
  readonly loaderOrigin: string;
  /** Takes the loader's page down, so the client mounts on its root. */
  readonly unmount: () => void;
  /** Tells the service worker the client's files, settling once it kept them. */
  readonly tellWorker: (message: ClientFilesMessage) => Promise<void>;
  /** Shows page in place of the client, once the loader's page is down and the client did not load. */
  readonly showFailure: (page: JSX.Element) => void;
}

/** The core this origin reached before, when it did. */
function keptCore(storage: StringStorage | undefined, coreId: string): KeptCore | undefined {
  const text = storageThatNeverThrows(storage).getItem(keptCoreKey);
  try {
    const kept: unknown = text === null ? undefined : JSON.parse(text);
    if (typeof kept !== "object" || kept === null) {
      return undefined;
    }
    const fingerprint: unknown = Reflect.get(kept, "fingerprint");
    const realm: unknown = Reflect.get(kept, "realm");
    return typeof fingerprint === "string"
      ? {
          coreId,
          fingerprint,
          realm: typeof realm === "string" && realm !== "" ? realm : undefined,
        }
      : undefined;
  } catch {
    return undefined;
  }
}

/**
 * How the page starts: from a link or a hand-off its address carries, which must name this origin's
 * core, else the core it reached before, else nothing. The fragment is cleared once read, so neither
 * stays in the address bar or the history.
 */
export function startAtClient(
  at: Pick<AtClientOrigin, "coreId" | "storage" | "location" | "history">,
): Start {
  const { location } = at;
  const link = readLinkFragment(location.hash);
  const handOff = readHandOffFragment(location.hash);
  if (handOff !== undefined) {
    at.history.replaceState(null, "", `${location.pathname}${location.search}`);
    const core = {
      coreId: at.coreId,
      fingerprint: handOff.fingerprint,
      realm: handOff.realm === "" ? undefined : handOff.realm,
    };
    return { kind: "handoff", core, handOff };
  }
  if (link?.coreId === at.coreId) {
    return { kind: "link", link };
  }
  const core = keptCore(at.storage, at.coreId);
  return core === undefined ? { kind: "none" } : { kind: "remembered", core };
}

/**
 * Goes to address at the loader's origin by a link whose referrer policy is the origin alone, under
 * the page's no-referrer: the list takes what a client origin tells it only from that origin's page
 * (D308 §8).
 */
function goBy(address: string): void {
  const link = document.createElement("a");
  link.href = address;
  link.referrerPolicy = "origin";
  document.body.append(link);
  link.click();
}

/**
 * Whether a person signed out at this client origin goes to the discovery's sign-in: wherever the
 * discovery has one, its realms' (D297), however the core was reached, through a realm or by a link
 * alone (the owner, 2026-10-30: one place to land, "the discovery's sign-in, except where there is no
 * discovery"). A link's page knows no realm, so a page reached by one, as over the core's LAN name
 * from the owner's S23, signed out to the core's own sign-in while a page reached through a realm, as
 * the Fire TV's over WebRTC, went to the discovery's. Only at a discovery with no realms, whose page
 * has no sign-in to land on, does the core's own stay.
 */
export function leavesWhenSignedOut(core: KeptCore, hasRealms: boolean): boolean {
  return hasRealms || core.realm !== undefined;
}

/**
 * Goes, by go, to the discovery's page once the session at this origin ends, signed out here or by
 * its device's removal, where leavesWhenSignedOut says its person signs in at the discovery: there
 * the household is no longer gone to and the discovery's sign-in is offered. Left here, the page
 * showed the core's own sign-in, a stranger's to a person who never met it (the owner's Fire TV,
 * 2026-10-27). Returns what stops the watch.
 */
export function leaveWhenSignedOut(
  session: Session,
  to: {
    readonly core: KeptCore;
    /** The client origin's page: whether its discovery has realms, and the loader's origin. */
    readonly at: {
      readonly config: Pick<LoaderConfig, "hasRealms">;
      readonly loaderOrigin: string;
    };
    readonly go: (address: string) => void;
  },
): () => void {
  const { core, at, go } = to;
  if (!leavesWhenSignedOut(core, at.config.hasRealms)) {
    return () => undefined;
  }
  return whenSignedOut(session, () => {
    go(signedOutAddress(at.loaderOrigin, core.coreId));
  });
}

/**
 * What a client origin does once its core refused the key this device remembered for it, as once
 * the device was removed there: forgets what the offers were proven by, the session, else the Watch
 * Together membership, as heldWith chose them, so it is offered no more; and, for a core reached
 * through a realm, goes by go to the discovery's sign-in, which says the device needs to sign in
 * again. A core reached by a link alone stays, saying so, its person needing a new link. Left as it
 * was, the page told the owner's Fire TV to change networks (2026-10-28).
 */
export function forgetRefusedKey(
  held: { readonly session: Session; readonly forgetMembership: () => void },
  to: {
    readonly core: KeptCore;
    readonly loaderOrigin: string;
    readonly go: (address: string) => void;
  },
): void {
  const { session } = held;
  if (session.token() !== undefined && session.locator() !== undefined) {
    session.signOut();
  } else {
    held.forgetMembership();
  }
  if (to.core.realm !== undefined) {
    to.go(keyRefusedAddress(to.loaderOrigin, to.core.coreId));
  }
}

/** What this client origin may hold its core's offers by, for forgetRefusedKey to forget. */
function heldAt(
  at: Pick<AtClientOrigin, "storage" | "coreId">,
): Parameters<typeof forgetRefusedKey>[0] {
  return {
    session: createSession(at.storage, corePrefix(at.coreId)),
    forgetMembership: () => {
      forgetMembership(browserStorage(), "viewer");
    },
  };
}

/**
 * Everything this client origin holds with its core, as text that changes whenever any of it does:
 * the session's token and locator, and the Watch Together viewer's membership (GateParts.holdingOf).
 */
export function holdingAt(session: Session, membership: () => string | null): string {
  return JSON.stringify([session.token() ?? "", session.locator() ?? "", membership() ?? ""]);
}

/**
 * Once kept's server refused the page's way back for good, shows the person over the client
 * (PathRefusedNote): a reload, or signing in again, which forgets what was refused as a refusal at
 * the gate does (forgetRefusedKey) and, for a core reached by a link alone, reloads.
 */
export function showWhenPathRefused(
  kept: Pick<KeptPath, "onPathRefused">,
  shown: {
    readonly at: Pick<AtClientOrigin, "storage" | "coreId" | "loaderOrigin" | "showFailure">;
    readonly core: KeptCore;
  },
  reload: () => void = () => {
    window.location.reload();
  },
): void {
  const { at, core } = shown;
  kept.onPathRefused(() => {
    at.showFailure(
      <PathRefusedNote
        onReload={reload}
        onSignInAgain={() => {
          forgetRefusedKey(heldAt(at), { core, loaderOrigin: at.loaderOrigin, go: goBy });
          if (core.realm === undefined) {
            reload();
          }
        }}
      />,
    );
  });
}

/** The loader's interface over a kept path, for the client to take. */
export function loaderOver(
  at: AtClientOrigin,
  reached: {
    readonly core: KeptCore;
    readonly session: Session;
    readonly kept: KeptPath;
    readonly reaching: Reaching;
  },
): OrpheumLoader {
  const { core, session, kept, reaching } = reached;
  return {
    major: loaderMajor,
    minor: loaderMinor,
    core,
    fetch: kept.fetch,
    openTogetherSocket: kept.openTogetherSocket,
    onPathMove: kept.onPathMove,
    pathInUse: kept.pathInUse,
    directOriginInUse: kept.directOriginInUse,
    mediaCarrierOf: kept.mediaCarrierOf,
    onPathBack: kept.onPathBack,
    onLocalNetworkAsking: reaching.onLocalNetworkAsking,
    willLeaveWhenSignedOut: leavesWhenSignedOut(core, at.config.hasRealms),
    session,
    setMediaCredentials: kept.setMediaCredentials,
    switchHousehold: () => {
      goBy(
        switchAddress(at.loaderOrigin, {
          coreId: core.coreId,
          fingerprint: core.fingerprint,
          realm: core.realm ?? "",
          name: "",
        }),
      );
    },
  };
}

/**
 * Takes the session a hand-off kept, by its code, over the path just reached. Its code works once, so
 * the request carries an idempotency key (D244 §4): sent again on another path after its answer was
 * lost, the core answers the session it gave the first time. Without one, Firefox's Local Network
 * Access, which lets a LAN name's request reach the core and then drops its answer, left the code
 * spent and the page on the core's own sign-in (the owner's production NAS sign-in, 2026-10-20).
 * The key is the page's for its code (redeemOnce), so a try on a new connection asks under it too.
 */
export async function redeem(
  send: FetchRequest,
  session: Session,
  handOff: { readonly code: string; readonly key: string },
): Promise<void> {
  const { code, key } = handOff;
  const api = createApi({ getToken: () => undefined, fetch: send });
  noteStep("redeeming the link's hand-off");
  const { data, response } = await api.POST("/session/redeem", {
    body: { code },
    ...idempotent(key),
  });
  noteStep(`the core answered the hand-off: ${String(response.status)}`);
  if (data === undefined) {
    throw new HandOffRefused(
      `the server answered ${String(response.status)} for the hand-off's code`,
    );
  }
  session.signIn(data.token, {
    deviceId: data.user.device?.id,
    deviceSecret: data.deviceSecret,
    locator: data.locator,
    user: data.user,
  });
}

/** The core's refusal of a hand-off's code, as once it was spent: no path's loss, and not waited out. */
export class HandOffRefused extends Error {}

/**
 * Takes a hand-off's session by its code at most once a page (redeem): asked again on a new
 * connection after the path was lost, it asks under the same idempotency key, so a code whose answer
 * was lost gets the session it gave, and once the session is taken it asks no more. A new key each
 * try spent the code on the first and was refused on the next.
 */
export function redeemOnce(code: string): (send: FetchRequest, session: Session) => Promise<void> {
  const key = newIdempotencyKey();
  let isTaken = false;
  return async (send, session) => {
    if (!isTaken) {
      await redeem(send, session, { code, key });
      isTaken = true;
    }
  };
}

/**
 * Whether problem, which ended the page's reach of its core or its run of the core's client, is the
 * path's loss, which the page waits out on a new connection (GateParts.isPathLost; §28; the owner's
 * train, 2026-11-13): anything but a refusal, which a new connection would not change. The client's
 * own (ClientRefused: an unsigned manifest, a changed page, a refused version) is the loader's
 * security check, not the network's; so is a refused hand-off, and a page no worker controls.
 */
export function isLoaderPathLost(problem: unknown): boolean {
  return !(
    problem instanceof ClientRefused ||
    problem instanceof HandOffRefused ||
    problem instanceof PageNotControlled
  );
}

/** The tab's key for the reload it made because no service worker controlled its page. */
const reloadedUncontrolledKey = "orpheum.loader.reloadedUncontrolled";

/** What reloadOnceWhenUncontrolled needs of the page. */
export interface UncontrolledParts {
  /** This tab's storage, which says whether it reloaded for this before. */
  readonly tab: StringStorage;
  /** Tells the core's log what happened; a failure to tell it is the reload's to drop. */
  readonly tell: (text: string) => Promise<void>;
  readonly reload: () => void;
}

/**
 * Reloads the page, once in a tab, when no service worker controlled it after it found its core
 * (PageNotControlled), having told the core's log: its media would go past the worker to the client
 * origin, which serves none, and every poster 404 there. Chrome can commit a page reloaded as an
 * updated worker activates under the worker before and then stop that one (idiosyncrasies.md); a
 * fresh load is controlled. A second time in the tab it does not, and the failure shows. Whether it
 * reloads; a page whose media is carried clears the tab's mark (markControlled), so a later time
 * reloads again.
 */
export async function reloadOnceWhenUncontrolled(
  cause: unknown,
  parts: UncontrolledParts,
): Promise<boolean> {
  if (!(cause instanceof PageNotControlled)) {
    return false;
  }
  const isReloadedBefore = parts.tab.getItem(reloadedUncontrolledKey) !== null;
  noteStep(isReloadedBefore ? "not controlled again: not reloading" : "reloading the page once");
  await parts
    .tell(
      `${cause.message}; ${isReloadedBefore ? "reloaded for it before in this tab, so not again" : "reloading the page once"}`,
    )
    .catch(() => undefined);
  if (isReloadedBefore) {
    return false;
  }
  parts.tab.setItem(reloadedUncontrolledKey, "1");
  parts.reload();
  return true;
}

/** Clears the tab's mark of a reload for an uncontrolled page, once a worker carries its media. */
export function markControlled(tab: StringStorage): void {
  tab.removeItem(reloadedUncontrolledKey);
}

/** The longest message and page path the core's log takes from a page (client_problems.go). */
const problemMost = 500;
const pageMost = 200;

/** Tells the core's log text as a failure of the page, over send, while session is signed in. */
async function tellCoreLog(send: FetchRequest, session: Session, text: string): Promise<void> {
  if (session.token() === undefined) {
    return;
  }
  const api = createApi({ getToken: session.token, fetch: send });
  await api.POST("/client-problems", {
    body: {
      message: text.slice(0, problemMost),
      page: window.location.pathname.slice(0, pageMost),
    },
  });
}

/** Says the core's client did not load, by the address of its script that did not. */
function showClientFailed(at: AtClientOrigin, coreId: string, address: string): void {
  reportFailure(new Error(`the server's client did not load: ${address}`));
  const isControlled = navigator.serviceWorker.controller !== null;
  const text = clientFailedText({ coreId, address, isControlled }, navigator.userAgent);
  at.showFailure(<ClientFailedPage text={text} isControlled={isControlled} />);
}

/**
 * What the gate reads of what this client origin holds with its core, its session read afresh by
 * sessionOfCore each time: the session or membership offers are proven by, and all of it as text.
 */
function holdingsOf(sessionOfCore: () => Session): Pick<GateParts, "sessionOf" | "holdingOf"> {
  return {
    sessionOf: (core) =>
      heldWith(core, sessionOfCore(), readMembership(browserStorage(), "viewer")),
    holdingOf: () => holdingAt(sessionOfCore(), () => membershipText(browserStorage(), "viewer")),
  };
}

/** The page a client origin opens on until the core's client runs. */
export function AtClientOriginPage(props: { readonly at: AtClientOrigin }): JSX.Element {
  const { at } = props;
  const start = startAtClient(at);
  const sessionOfCore = (): Session => createSession(at.storage, corePrefix(at.coreId));
  const takeHandOff = start.kind === "handoff" ? redeemOnce(start.handOff.code) : undefined;
  const parts: GateParts = {
    client: { clientApi: at.config.clientApi },
    hasRealms: false,
    realmStorage: undefined,
    isPathLost: isLoaderPathLost,
    ...holdingsOf(sessionOfCore),
    forgetCore: () => {
      storageThatNeverThrows(at.storage).removeItem(keptCoreKey);
    },
    onKeyRefused: (core) => {
      forgetRefusedKey(heldAt(at), { core, loaderOrigin: at.loaderOrigin, go: goBy });
    },
    onGone: () => {
      storageThatNeverThrows(at.storage).removeItem(keptCoreKey);
      goBy(goneAddress(at.loaderOrigin, at.coreId));
    },
    onConnected: async (core: KeptCore, reaching: Reaching) => {
      const session = sessionOfCore();
      const kept = keepPath(reaching, session);
      try {
        await takeHandOff?.(kept.fetch, session);
        storageThatNeverThrows(at.storage).setItem(
          keptCoreKey,
          JSON.stringify({ fingerprint: core.fingerprint, realm: core.realm ?? "" }),
        );
        await kept.carried;
        markControlled(tabStorage());
        const run = await checkedClient({
          send: kept.fetch,
          fingerprint: core.fingerprint,
          refusedVersions: () => refusedClientVersions({ clientApi: at.config.clientApi }),
          inertPage: document,
          mount: (loader, parts) => {
            mountClient(document, {
              loader,
              parts,
              onFailed: (address) => {
                showClientFailed(at, core.coreId, address);
              },
            });
          },
          tellWorker: at.tellWorker,
        });
        at.unmount();
        leaveWhenSignedOut(session, { core, at, go: goBy });
        showWhenPathRefused(kept, { at, core });
        run(loaderOver(at, { core, session, kept, reaching }));
      } catch (cause: unknown) {
        await failedToRun(cause, kept, session);
      }
    },
  };
  return <DiscoveryGate start={start} parts={parts} />;
}

/**
 * Ends the page's run of the core's client over kept, which failed for cause: reloads once in the
 * tab for a page no worker controlled (reloadOnceWhenUncontrolled), else throws cause for the gate
 * to show.
 */
async function failedToRun(cause: unknown, kept: KeptPath, session: Session): Promise<void> {
  const isReloading = await reloadOnceWhenUncontrolled(cause, {
    tab: tabStorage(),
    tell: (text) => tellCoreLog(kept.fetch, session, text),
    reload: () => {
      window.location.reload();
    },
  });
  kept.close();
  reportFailure(cause);
  if (!isReloading) {
    throw cause;
  }
}
