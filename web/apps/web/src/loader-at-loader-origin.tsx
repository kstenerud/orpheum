// SPDX-License-Identifier: MPL-2.0

/**
 * The discovery's loader at its own origin, `app.` (D308): the first contact, and nothing more. A
 * link is sent on to its core's client origin as it came; a realm sign-in reaches the core, signs in
 * there for a hand-off's code, and sends the browser on with it; and the households this browser was
 * sent to are listed, straight to the one when there is one. It keeps no core's session (§3).
 */

import { createApi, readLinkFragment, type FetchRequest, type KeptCore } from "@orpheum/sdk";
import { useMemo, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { DiscoveryGate, type GateParts, type SignInWith } from "./discovery-gate.tsx";
import { directFetch } from "@orpheum/sdk";
import { handOffFragment, type Reached } from "./discovery-start.ts";
import { thisDevice } from "./device.ts";
import type { LoaderConfig } from "./loader-config.ts";
import { clientOriginOf } from "./loader-config.ts";
import {
  asksForTheList,
  forgetHousehold,
  changedFrom,
  goneFrom,
  keyRefusedFrom,
  listedHouseholds,
  markHouseholdGone,
  markHouseholdSignedOut,
  rememberHousehold,
  signedOutFrom,
  switchedFrom,
  type Household,
} from "./loader-households.ts";
import type { StringStorage } from "./browser-storage.ts";
import { ConfirmButton } from "./confirm-button.tsx";
import { idempotent, newIdempotencyKey } from "./idempotent.ts";
import { CredentialsForm } from "./credentials-form.tsx";
import { DeviceCodeSignIn, type DeviceCodeParts } from "./loader-device-code.tsx";
import { activateHere, ActivateWhere, scannedAt } from "./loader-activate.tsx";
import type { Scanned } from "./router.ts";
import "./loader-households.css";

/** What the page at the loader's origin starts from. */
export interface AtLoaderOrigin {
  readonly config: LoaderConfig;
  readonly storage: StringStorage | undefined;
  readonly location: Pick<Location, "hash" | "pathname" | "search" | "replace" | "assign">;
  readonly referrer: string;
  readonly now: () => number;
}

/** What sends a request on the path a realm sign-in reached. */
function sendOn(path: Reached): FetchRequest {
  return path.kind === "direct" ? directFetch(path.origin) : path.transport.fetch;
}

/**
 * Signs in at the core over path for a hand-off's code, which the client origin takes the session
 * by; the session itself never comes here (D308 §4).
 */
async function handOffCode(path: Reached, signIn: SignInWith): Promise<string> {
  const api = createApi({ getToken: () => undefined, fetch: sendOn(path) });
  const device = thisDevice({ deviceId: () => undefined, deviceProof: () => undefined });
  // One key for the click, so a request the transport sends again signs in once (D244 §4).
  const { data } = await api.POST("/session/handoff", {
    body: { name: signIn.name, password: signIn.password, device },
    ...idempotent(newIdempotencyKey()),
  });
  if (data === undefined) {
    throw new Error("the server refused the sign-in's hand-off");
  }
  return data.code;
}

/** Where the browser goes to reach core: its client origin. */
function clientAddressOf(config: LoaderConfig, coreId: string): string | undefined {
  return clientOriginOf(coreId, config.clientOrigins);
}

/**
 * Where the page at the loader's origin goes, or what it shows: a link goes on to its client origin;
 * a request for the list, or more than one household, shows the list; one household is gone to;
 * none, the sign-in.
 */
export function startAtLoaderOrigin(at: AtLoaderOrigin): JSX.Element | undefined {
  const { config, storage, location } = at;
  const link = readLinkFragment(location.hash);
  if (link !== undefined) {
    const origin = clientAddressOf(config, link.coreId);
    if (origin !== undefined) {
      location.replace(`${origin}${location.pathname}${location.hash}`);
      return undefined;
    }
  }
  const from = switchedFrom(location.search, at.referrer, (coreId) =>
    clientAddressOf(config, coreId),
  );
  if (from !== undefined) {
    rememberHousehold(storage, { ...from, lastUsedAt: at.now() });
  }
  const gone = goneFrom(location.search, at.referrer, (coreId) => clientAddressOf(config, coreId));
  if (gone !== undefined) {
    markHouseholdGone(storage, gone);
  }
  const signedOut = signedOutFrom(location.search, at.referrer, (coreId) =>
    clientAddressOf(config, coreId),
  );
  if (signedOut !== undefined) {
    markHouseholdSignedOut(storage, signedOut);
  }
  const keyRefusedBy = keyRefusedFrom(location.search, at.referrer, (coreId) =>
    clientAddressOf(config, coreId),
  );
  const kept = listedHouseholds(storage);
  const scanned = scannedAt(location);
  // A core that refused this device's key sends its person to the sign-in, which says why, never on
  // to another household (the owner's Fire TV, 2026-10-28).
  if (keyRefusedBy === undefined && wentToTheOne(at, kept, scanned)) {
    return undefined;
  }
  return (
    <LoaderPage
      at={at}
      isListFirst={kept.length > 0 && keyRefusedBy === undefined}
      scanned={scanned}
      keyRefusedBy={keyRefusedBy}
    />
  );
}

/**
 * Goes to the one household kept, unless the list was asked for, and says whether it went: to its
 * home, or for a TV's QR code to its Activate a device, the code filled in, even when the list was
 * asked for (D321).
 */
function wentToTheOne(
  at: AtLoaderOrigin,
  kept: readonly Household[],
  scanned: Scanned | undefined,
): boolean {
  const [only] = kept;
  const isListAsked = asksForTheList(at.location.search) && scanned === undefined;
  if (isListAsked || kept.length !== 1 || only === undefined) {
    return false;
  }
  const origin = clientAddressOf(at.config, only.coreId);
  if (origin === undefined) {
    return false;
  }
  at.location.replace(`${origin}${scanned === undefined ? "/" : activateHere(scanned)}`);
  return true;
}

/** The loader's page: the household list, or the sign-in that adds to it. */
function LoaderPage(props: {
  readonly at: AtLoaderOrigin;
  readonly isListFirst: boolean;
  /** A TV's QR code the page was opened by, which the sign-in carries on to activate (D321). */
  readonly scanned?: Scanned | undefined;
  /** The core whose client origin sent the page here because it refused this device's key. */
  readonly keyRefusedBy?: string | undefined;
}): JSX.Element {
  const { at, scanned, keyRefusedBy } = props;
  const [isJoining, setIsJoining] = useState(!props.isListFirst && scanned === undefined);
  const [isUsingCode, setIsUsingCode] = useState(false);
  // A household gone or signed out of is kept for the warning a realm sign-in gives (D312 §2),
  // never listed.
  const live = (): readonly Household[] => listedHouseholds(at.storage);
  const [listed, setListed] = useState(live);
  if (isUsingCode) {
    return (
      <DeviceCodeSignIn
        parts={codeParts(at)}
        onBack={() => {
          setIsUsingCode(false);
        }}
      />
    );
  }
  if (isJoining) {
    return (
      <JoinByRealm
        at={at}
        then={scanned === undefined ? "/" : activateHere(scanned)}
        keyRefusedBy={keyRefusedBy}
        onUseCode={() => {
          setIsUsingCode(true);
        }}
      />
    );
  }
  if (scanned !== undefined) {
    return (
      <ActivateWhere
        scanned={scanned}
        listed={listed}
        originOf={(coreId) => clientAddressOf(at.config, coreId)}
        link={`${at.config.loaderOrigin}${at.location.pathname}${at.location.search}`}
        onSignInHere={() => {
          setIsJoining(true);
        }}
      />
    );
  }
  return (
    <HouseholdList
      listed={listed}
      originOf={(coreId) => clientAddressOf(at.config, coreId)}
      onForget={(coreId) => {
        forgetHousehold(at.storage, coreId);
        setListed(live());
      }}
      onJoin={() => {
        setIsJoining(true);
      }}
    />
  );
}

/** The households this browser was sent to, each a way back, and a way to join another. */
export function HouseholdList(props: {
  readonly listed: readonly Household[];
  readonly originOf: (coreId: string) => string | undefined;
  readonly onForget: (coreId: string) => void;
  readonly onJoin: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsForm heading={t("loader.households.heading")} onSubmit={props.onJoin}>
      <ul className="loader-households">
        {props.listed.map((household) => {
          const origin = props.originOf(household.coreId);
          return origin === undefined ? null : (
            <li key={household.coreId}>
              <a href={`${origin}/`}>{householdName(household)}</a>
              <ConfirmButton
                label={t("loader.households.forget")}
                labelForReader={t("loader.households.forgetNamed", {
                  name: householdName(household),
                })}
                consequence={t("loader.households.forgetConsequence", {
                  name: householdName(household),
                })}
                confirmLabel={t("loader.households.forgetConfirm")}
                onConfirm={() => {
                  props.onForget(household.coreId);
                }}
              />
            </li>
          );
        })}
      </ul>
      <button type="submit">{t("loader.households.join")}</button>
    </CredentialsForm>
  );
}

/** What a household is listed as: its name, else its realm, else its core's id. */
function householdName(household: Household): string {
  if (household.name !== "") {
    return household.name;
  }
  return household.realm === "" ? household.coreId : household.realm;
}

/**
 * The sign-in by realm, which hands the browser on to the core it finds, at then on its client
 * origin, or a code instead.
 */
function JoinByRealm(props: {
  readonly at: AtLoaderOrigin;
  readonly then: string;
  readonly onUseCode: () => void;
  /** The core that refused this device's key, which the sign-in says (Start's keyRefusedBy). */
  readonly keyRefusedBy?: string | undefined;
}): JSX.Element {
  const { at, then, onUseCode, keyRefusedBy } = props;
  const parts = useMemo(() => ({ ...joinParts(at, then), onUseCode }), [at, then, onUseCode]);
  return <DiscoveryGate start={{ kind: "none", keyRefusedBy }} parts={parts} />;
}

/**
 * What a code from the discovery's service reaches the core with and hands the browser on by, as a
 * realm sign-in does (D293 §2).
 */
export function codeParts(at: AtLoaderOrigin): DeviceCodeParts {
  return {
    client: { clientApi: at.config.clientApi },
    originOf: (coreId) => clientAddressOf(at.config, coreId),
    remember: (core) => {
      rememberHousehold(at.storage, { ...core, name: "", lastUsedAt: at.now() });
    },
    go: (address) => {
      at.location.assign(address);
    },
    loaderOrigin: at.config.loaderOrigin,
  };
}

/**
 * What the realm sign-in at the loader's origin reaches the core with, and hands the browser on by:
 * to then on the core's client origin, its home unless a TV's QR code asked for Activate (D321). The
 * hand-off rides in the fragment, which the client origin clears, so then's query outlives it.
 */
export function joinParts(at: AtLoaderOrigin, then = "/"): GateParts {
  return {
    client: { clientApi: at.config.clientApi },
    hasRealms: at.config.hasRealms,
    realmStorage: at.storage,
    shouldSkipLanNames: true,
    sessionOf: () => undefined,
    forgetCore: () => undefined,
    changedFrom: (found) => changedFrom(at.storage, found),
    onConnected: async (core: KeptCore, reaching, signIn) => {
      const origin = clientAddressOf(at.config, core.coreId);
      if (signIn === undefined || origin === undefined || !("bootstrapToken" in signIn.held)) {
        throw new Error("only a realm sign-in is handed on from here");
      }
      // The transport is closed whether the hand-off's code came or not: left open after a failure,
      // its connections stayed, each one the core turned away replaced at once (staging, 2026-10-25).
      const code = await handOffCode(reaching.first, signIn).finally(() => {
        if (reaching.first.kind === "webrtc") {
          reaching.first.transport.close();
        }
      });
      rememberHousehold(at.storage, {
        coreId: core.coreId,
        fingerprint: core.fingerprint,
        realm: core.realm ?? "",
        name: "",
        lastUsedAt: at.now(),
      });
      at.location.assign(
        `${origin}${then}${handOffFragment({
          code,
          bootstrapToken: signIn.held.bootstrapToken,
          fingerprint: core.fingerprint,
          realm: core.realm ?? "",
        })}`,
      );
    },
  };
}
