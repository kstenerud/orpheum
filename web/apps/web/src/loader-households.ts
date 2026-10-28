// SPDX-License-Identifier: MPL-2.0

/**
 * The households a browser reached through the discovery's loader (D308 §8): kept at the loader's
 * origin alone, which never holds their sessions, only what leads back to each: its core's id, the
 * fingerprint held for it, its realm and its name, and when it was last used.
 */

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";

/** One household in the list. */
export interface Household {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly realm: string;
  /** What the household calls itself, as its client said, or "" until one did. */
  readonly name: string;
  /** When the browser was last handed on to it, in milliseconds since the epoch. */
  readonly lastUsedAt: number;
  /**
   * Whether its client origin said the discovery finds its core no more: kept, not gone to, so a
   * realm sign-in that finds another core for its realm warns first (D312 §2).
   */
  readonly isGone?: boolean;
  /**
   * Whether its person signed out there, or was signed out: kept, as one gone is, not gone to, where
   * only the core's own sign-in waited, the page offering the discovery's sign-in instead (the
   * owner's Fire TV, 2026-10-27).
   */
  readonly isSignedOut?: boolean;
}

/** Where the list is kept. */
const householdsKey = "orpheum.loader.households";

/** How many households the list keeps, the least lately used dropped first. */
const householdsAtMost = 20;

function stringField(kept: object, name: string): string | undefined {
  const value: unknown = Reflect.get(kept, name);
  return typeof value === "string" ? value : undefined;
}

/** One kept entry read back, or undefined for one that is not whole. */
function householdOf(kept: unknown): Household | undefined {
  if (typeof kept !== "object" || kept === null) {
    return undefined;
  }
  const coreId = stringField(kept, "coreId");
  const fingerprint = stringField(kept, "fingerprint");
  const lastUsedAt: unknown = Reflect.get(kept, "lastUsedAt");
  if (coreId === undefined || fingerprint === undefined || typeof lastUsedAt !== "number") {
    return undefined;
  }
  return {
    coreId,
    fingerprint,
    realm: stringField(kept, "realm") ?? "",
    name: stringField(kept, "name") ?? "",
    lastUsedAt,
    isGone: Reflect.get(kept, "isGone") === true,
    isSignedOut: Reflect.get(kept, "isSignedOut") === true,
  };
}

/** The households kept, the last used first. */
export function households(refusable: StringStorage | undefined): readonly Household[] {
  const text = storageThatNeverThrows(refusable).getItem(householdsKey);
  if (text === null) {
    return [];
  }
  try {
    const kept: unknown = JSON.parse(text);
    if (!Array.isArray(kept)) {
      return [];
    }
    return kept
      .map((entry: unknown) => householdOf(entry))
      .filter((entry) => entry !== undefined)
      .toSorted((first, second) => second.lastUsedAt - first.lastUsedAt);
  } catch {
    return [];
  }
}

function keep(refusable: StringStorage | undefined, kept: readonly Household[]): void {
  storageThatNeverThrows(refusable).setItem(
    householdsKey,
    JSON.stringify(kept.slice(0, householdsAtMost)),
  );
}

/** The households kept that the list shows and goes to: neither gone nor signed out of. */
export function listedHouseholds(refusable: StringStorage | undefined): readonly Household[] {
  return households(refusable).filter(
    (household) => household.isGone !== true && household.isSignedOut !== true,
  );
}

/**
 * Keeps household as the last used, in place of any entry for the same core; a name it does not say
 * keeps the one the entry had.
 */
export function rememberHousehold(
  refusable: StringStorage | undefined,
  household: Household,
): void {
  const others = households(refusable);
  const earlier = others.find((entry) => entry.coreId === household.coreId);
  // One core per realm: the one reached now takes the place of any other kept for it, gone or not.
  const isReplaced = (entry: Household): boolean =>
    entry.coreId === household.coreId ||
    (household.realm !== "" && entry.realm === household.realm);
  keep(refusable, [
    {
      ...household,
      name: household.name === "" ? (earlier?.name ?? "") : household.name,
      isGone: false,
      isSignedOut: false,
    },
    ...others.filter((entry) => !isReplaced(entry)),
  ]);
}

/** Keeps the household of coreId as gone: its core found no more, its realm and fingerprint kept. */
export function markHouseholdGone(refusable: StringStorage | undefined, coreId: string): void {
  keep(
    refusable,
    households(refusable).map((entry) =>
      entry.coreId === coreId ? { ...entry, isGone: true } : entry,
    ),
  );
}

/** Keeps the household of coreId as signed out of: its realm and fingerprint kept, not gone to. */
export function markHouseholdSignedOut(refusable: StringStorage | undefined, coreId: string): void {
  keep(
    refusable,
    households(refusable).map((entry) =>
      entry.coreId === coreId ? { ...entry, isSignedOut: true } : entry,
    ),
  );
}

/**
 * The household this browser remembers for found's realm by another core's fingerprint, gone or
 * not: a realm sign-in that found found stops with a warning first (D312 §2).
 */
export function changedFrom(
  refusable: StringStorage | undefined,
  found: {
    readonly coreId: string;
    readonly fingerprint: string;
    readonly realm?: string | undefined;
  },
): Household | undefined {
  const realm = found.realm ?? "";
  return households(refusable).find(
    (entry) => realm !== "" && entry.realm === realm && entry.fingerprint !== found.fingerprint,
  );
}

/** Forgets the household of coreId. */
export function forgetHousehold(refusable: StringStorage | undefined, coreId: string): void {
  keep(
    refusable,
    households(refusable).filter((entry) => entry.coreId !== coreId),
  );
}

/** The query a client origin asks for the list by, naming the household it came from. */
export interface SwitchAsk {
  readonly coreId: string;
  readonly fingerprint: string;
  readonly realm: string;
  readonly name: string;
}

/** The query parameter that asks for the list however many households it holds. */
export const listParameter = "households";

/** The address of the list at loaderOrigin, asked for by the household ask names. */
export function switchAddress(loaderOrigin: string, ask: SwitchAsk): string {
  const query = new URLSearchParams({
    [listParameter]: "",
    from: ask.coreId,
    fingerprint: ask.fingerprint,
    realm: ask.realm,
    name: ask.name,
  });
  return `${loaderOrigin}/?${query.toString()}`;
}

/**
 * The household a request for the list says it came from, taken only when the page that sent it is
 * that household's own client origin, as its referrer says: no other page can put an entry in the
 * list (D308 §8). Undefined for anything else.
 */
export function switchedFrom(
  search: string,
  referrer: string,
  clientOriginOf: (coreId: string) => string | undefined,
): SwitchAsk | undefined {
  const query = new URLSearchParams(search);
  const coreId = query.get("from") ?? "";
  const fingerprint = query.get("fingerprint") ?? "";
  const origin = clientOriginOf(coreId);
  if (origin === undefined || fingerprint === "" || !isFromOrigin(referrer, origin)) {
    return undefined;
  }
  return { coreId, fingerprint, realm: query.get("realm") ?? "", name: query.get("name") ?? "" };
}

/** Whether referrer is a page of origin. */
function isFromOrigin(referrer: string, origin: string): boolean {
  try {
    return new URL(referrer).origin === origin;
  } catch {
    return false;
  }
}

/** The query parameter a client origin names its core by when the discovery no longer finds it. */
export const goneParameter = "gone";

/**
 * The address of the list at loaderOrigin, telling it the household of coreId is gone: the
 * discovery found no core by it, so the list forgets it and the page offers the realm sign-in.
 */
export function goneAddress(loaderOrigin: string, coreId: string): string {
  const query = new URLSearchParams({ [listParameter]: "", [goneParameter]: coreId });
  return `${loaderOrigin}/?${query.toString()}`;
}

/**
 * The core a request for the list says is gone, taken only from that core's own client origin, as
 * its referrer says, as switchedFrom takes a household: no other page strikes one off the list.
 */
export function goneFrom(
  search: string,
  referrer: string,
  clientOriginOf: (coreId: string) => string | undefined,
): string | undefined {
  return coreTold(goneParameter, { search, referrer, clientOriginOf });
}

/**
 * The query parameter a client origin names its core by when its person signed out there, or was
 * signed out, the device removed.
 */
export const signedOutParameter = "signedout";

/**
 * The address of the loader's page at loaderOrigin, telling it its person signed out of the household
 * of coreId: the list goes to it no more, and the page offers the discovery's sign-in, not the core's
 * own a client origin with no session shows.
 */
export function signedOutAddress(loaderOrigin: string, coreId: string): string {
  const query = new URLSearchParams({ [signedOutParameter]: coreId });
  return `${loaderOrigin}/?${query.toString()}`;
}

/**
 * The query parameter beside signedOutParameter that says why: the core refused the key this device
 * remembered for it, so the realm sign-in says the device needs to sign in again.
 */
export const keyRefusedParameter = "keyrefused";

/**
 * The address of the loader's page at loaderOrigin, telling it the household of coreId was signed
 * out of because its core refused the key this device remembered for it, as once the device was
 * removed there (the owner's Fire TV, 2026-10-28): signed out of, and the sign-in says so.
 */
export function keyRefusedAddress(loaderOrigin: string, coreId: string): string {
  const query = new URLSearchParams({ [signedOutParameter]: coreId, [keyRefusedParameter]: "" });
  return `${loaderOrigin}/?${query.toString()}`;
}

/**
 * The core a request for the loader's page says refused this device's key, taken only from that
 * core's own client origin, as signedOutFrom takes one signed out of.
 */
export function keyRefusedFrom(
  search: string,
  referrer: string,
  clientOriginOf: (coreId: string) => string | undefined,
): string | undefined {
  return new URLSearchParams(search).has(keyRefusedParameter)
    ? signedOutFrom(search, referrer, clientOriginOf)
    : undefined;
}

/**
 * The core a request for the loader's page says was signed out of, taken only from that core's own
 * client origin, as goneFrom takes one gone.
 */
export function signedOutFrom(
  search: string,
  referrer: string,
  clientOriginOf: (coreId: string) => string | undefined,
): string | undefined {
  return coreTold(signedOutParameter, { search, referrer, clientOriginOf });
}

/**
 * The core the query's parameter names, when the page that sent it is that core's own client
 * origin, as its referrer says; undefined for any other.
 */
function coreTold(
  parameter: string,
  told: {
    readonly search: string;
    readonly referrer: string;
    readonly clientOriginOf: (coreId: string) => string | undefined;
  },
): string | undefined {
  const coreId = new URLSearchParams(told.search).get(parameter) ?? "";
  const origin = told.clientOriginOf(coreId);
  return origin !== undefined && isFromOrigin(told.referrer, origin) ? coreId : undefined;
}

/** Whether a request for the loader's page asks for the list however many it holds. */
export function asksForTheList(search: string): boolean {
  return new URLSearchParams(search).has(listParameter);
}
