// SPDX-License-Identifier: MPL-2.0

/**
 * This device's data cap (D488): the most a film may use, chosen in ⁝ in GB per hour from the rungs
 * the core offers it (offeredRungsOf), kept as the chosen rung's rate, kbit/s, its sound's with it.
 * A device whose browser says what kind of network it is on (navigator.connection's type, Chrome on
 * Android in practice) keeps two caps, one for Wi-Fi and one for mobile data, and plays by the one
 * for the network it is on, changing with it mid-film; where the browser does not say, it keeps one.
 * Its kind is never guessed from the link's speed. Each starts at No limit (the owner, 2026-11-14).
 * Kept in this browser's storage, not on the core: see D488 for why, and what is left open.
 */

import { browserStorage, type StringStorage } from "./browser-storage.ts";

/** Where this browser keeps its caps, kbit/s by kind of network, as JSON. */
export const dataCapsKey = "orpheum.dataCaps";

/**
 * The kind of network a cap is for: Wi-Fi (a wired link with it), mobile data, or one the browser
 * does not say, which has a cap of its own.
 */
export type NetworkKind = "wifi" | "mobile" | "unknown";

const kinds: readonly NetworkKind[] = ["wifi", "mobile", "unknown"];

/** The browser's NetworkInformation, where it gives one: an EventTarget, told `change`. */
export function connectionOf(navigatorNow: object): EventTarget | undefined {
  const connection: unknown = Reflect.get(navigatorNow, "connection");
  return connection instanceof EventTarget ? connection : undefined;
}

/** The kind of network connection says it is on: its type, which only some browsers give. */
export function networkKindOf(connection: EventTarget | undefined): NetworkKind {
  const type: unknown = connection === undefined ? undefined : Reflect.get(connection, "type");
  if (type === "cellular") {
    return "mobile";
  }
  return type === "wifi" || type === "ethernet" ? "wifi" : "unknown";
}

/** A film's rate, kbit/s, as GB per hour, to one decimal: what a monthly plan is counted in. */
export function gbPerHourOf(kbps: number): number {
  return Math.round((kbps * 3600) / 8 / 100_000) / 10;
}

/**
 * Of rungs, a film's (offeredRungsOf), the one capKbps plays: the highest within it, the lowest
 * where none is; undefined for no cap, or no rungs (underDataCap, @orpheum/player).
 */
export function rungUnderCap(
  rungs: readonly number[],
  capKbps: number | undefined,
): number | undefined {
  if (capKbps === undefined) {
    return undefined;
  }
  return rungs.filter((rung) => rung <= capKbps).at(-1) ?? rungs[0];
}

/** The caps kept in storage, by kind; a value that is not a positive rate is none. */
function capsIn(storage: StringStorage): Partial<Record<NetworkKind, number>> {
  let kept: unknown;
  try {
    kept = JSON.parse(storage.getItem(dataCapsKey) ?? "{}");
  } catch {
    return {};
  }
  if (typeof kept !== "object" || kept === null) {
    return {};
  }
  const caps: Partial<Record<NetworkKind, number>> = {};
  for (const kind of kinds) {
    const cap: unknown = Reflect.get(kept, kind);
    if (typeof cap === "number" && Number.isFinite(cap) && cap > 0) {
      caps[kind] = cap;
    }
  }
  return caps;
}

/** This device's caps, the network it is on, and who is told when either changes. */
export class DataCaps {
  readonly #storage: () => StringStorage;
  readonly #connection: EventTarget | undefined;
  readonly #listeners = new Set<() => void>();

  /** The caps kept in storage, for the network connection says, where it says one. */
  constructor(storage: () => StringStorage, connection: EventTarget | undefined) {
    this.#storage = storage;
    this.#connection = connection;
  }

  /** The kind of network the device is on now, which the cap chosen now is for. */
  kind(): NetworkKind {
    return networkKindOf(this.#connection);
  }

  /** The cap now, kbit/s, for the network the device is on; undefined for No limit. */
  capKbps(): number | undefined {
    return capsIn(this.#storage())[this.kind()];
  }

  /** Keeps kbps as the cap for the network the device is on; undefined is No limit. */
  choose(kbps: number | undefined): void {
    const caps = { ...capsIn(this.#storage()), [this.kind()]: kbps };
    // JSON leaves a kind of No limit out.
    this.#storage().setItem(dataCapsKey, JSON.stringify(caps));
    for (const listener of this.#listeners) {
      listener();
    }
  }

  /**
   * Tells listener of each cap chosen and of each change of network, after which capKbps may differ;
   * returns the stopping.
   */
  subscribe(listener: () => void): () => void {
    this.#listeners.add(listener);
    this.#connection?.addEventListener("change", listener);
    return () => {
      this.#listeners.delete(listener);
      this.#connection?.removeEventListener("change", listener);
    };
  }
}

const page: { caps: DataCaps | undefined } = { caps: undefined };

/** This device's caps, in this browser's storage, for the network its browser says it is on. */
export function deviceDataCaps(): DataCaps {
  page.caps ??= new DataCaps(browserStorage, connectionOf(navigator));
  return page.caps;
}

/** This device's cap now, kbit/s, for a playback to choose under (ChoosingOptions.dataCapKbps). */
export function deviceDataCapKbps(): number | undefined {
  return deviceDataCaps().capKbps();
}
