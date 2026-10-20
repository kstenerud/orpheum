// SPDX-License-Identifier: MPL-2.0

/**
 * The discovery's loader at a core's client origin (D308): it reaches the core this origin is
 * bound to, by a link, a realm sign-in's hand-off or the session this origin keeps, and runs the
 * core's own client, checked. It refuses any start that names another core.
 */

import {
  createApi,
  readLinkFragment,
  refusedClientVersions,
  reportFailure,
  type FetchRequest,
} from "@orpheum/sdk";
import type { JSX } from "react";

import { DiscoveryGate, type GateParts } from "./discovery-gate.tsx";
import {
  heldWith,
  readHandOffFragment,
  type KeptCore,
  type Reaching,
  type Start,
} from "./discovery-start.ts";
import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";
import { keepPath, type KeptPath } from "./kept-path.ts";
import type { ClientFilesMessage } from "./client-files.ts";
import type { LoaderConfig } from "./loader-config.ts";
import { loaderMajor, loaderMinor, type OrpheumLoader } from "./loader-interface.ts";
import { goneAddress, switchAddress } from "./loader-households.ts";
import { checkedClient, mountClient } from "./loader-run-client.ts";
import { corePrefix, createSession, type Session } from "./session.ts";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { readMembership } from "./together-storage.ts";
import { browserStorage } from "./browser-storage.ts";

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
export function startAtClient(at: AtClientOrigin): Start {
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

/** The loader's interface over a kept path, for the client to take. */
function loaderOver(
  at: AtClientOrigin,
  reached: { readonly core: KeptCore; readonly session: Session; readonly kept: KeptPath },
): OrpheumLoader {
  const { core, session, kept } = reached;
  return {
    major: loaderMajor,
    minor: loaderMinor,
    core,
    fetch: kept.fetch,
    openTogetherSocket: kept.openTogetherSocket,
    onPathMove: kept.onPathMove,
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
 * spent and the page on the core's own sign-in (the owner's nas.example sign-in, 2026-10-20).
 */
export async function redeem(send: FetchRequest, session: Session, code: string): Promise<void> {
  const api = createApi({ getToken: () => undefined, fetch: send });
  const { data, response } = await api.POST("/session/redeem", {
    body: { code },
    ...idempotent(newIdempotencyKey()),
  });
  if (data === undefined) {
    throw new Error(`the server answered ${String(response.status)} for the hand-off's code`);
  }
  session.signIn(data.token, {
    deviceId: data.user.device?.id,
    deviceSecret: data.deviceSecret,
    locator: data.locator,
  });
}

/** The page a client origin opens on until the core's client runs. */
export function AtClientOriginPage(props: { readonly at: AtClientOrigin }): JSX.Element {
  const { at } = props;
  const start = startAtClient(at);
  const sessionOfCore = (): Session => createSession(at.storage, corePrefix(at.coreId));
  const parts: GateParts = {
    client: { clientApi: at.config.clientApi },
    hasRealms: false,
    realmStorage: undefined,
    sessionOf: (core) =>
      heldWith(core, sessionOfCore(), readMembership(browserStorage(), "viewer")),
    forgetCore: () => {
      storageThatNeverThrows(at.storage).removeItem(keptCoreKey);
    },
    onGone: () => {
      storageThatNeverThrows(at.storage).removeItem(keptCoreKey);
      goBy(goneAddress(at.loaderOrigin, at.coreId));
    },
    onConnected: async (core: KeptCore, reaching: Reaching) => {
      const session = sessionOfCore();
      const kept = keepPath(reaching, session);
      try {
        if (start.kind === "handoff") {
          await redeem(kept.fetch, session, start.handOff.code);
        }
        storageThatNeverThrows(at.storage).setItem(
          keptCoreKey,
          JSON.stringify({ fingerprint: core.fingerprint, realm: core.realm ?? "" }),
        );
        await kept.carried;
        const run = await checkedClient({
          send: kept.fetch,
          fingerprint: core.fingerprint,
          refusedVersions: () => refusedClientVersions({ clientApi: at.config.clientApi }),
          inertPage: document,
          mount: (loader, parts) => {
            mountClient(document, loader, parts);
          },
          tellWorker: at.tellWorker,
        });
        at.unmount();
        run(loaderOver(at, { core, session, kept }));
      } catch (cause: unknown) {
        kept.close();
        reportFailure(cause);
        throw cause;
      }
    },
  };
  return <DiscoveryGate start={start} parts={parts} />;
}
