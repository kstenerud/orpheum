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
  keep(refusable, [
    { ...household, name: household.name === "" ? (earlier?.name ?? "") : household.name },
    ...others.filter((entry) => entry.coreId !== household.coreId),
  ]);
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
  const coreId = new URLSearchParams(search).get(goneParameter) ?? "";
  const origin = clientOriginOf(coreId);
  return origin !== undefined && isFromOrigin(referrer, origin) ? coreId : undefined;
}

/** Whether a request for the loader's page asks for the list however many it holds. */
export function asksForTheList(search: string): boolean {
  return new URLSearchParams(search).has(listParameter);
}
