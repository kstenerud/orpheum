// SPDX-License-Identifier: MPL-2.0

/**
 * This device's data cap (D488): the most a film may use, chosen in ⁝ in GB per hour from the rungs
 * the core offers it (offeredRungsOf), kept as the chosen rung's rate, kbit/s, its sound's with it.
 * A device whose browser says what kind of network it is on (navigator.connection's type, Chrome on
 * Android in practice) keeps two caps, one for Wi-Fi and one for mobile data, and plays by the one
 * for the network it is on, changing with it mid-film; where the browser does not say, it keeps one.
 * Its kind is never guessed from the link's speed. Each starts at No limit (the owner, 2026-11-14).
 * The film's own picture is one more rung, and its own file plays under a cap at or above its rate
 * (D491). The caps are the core's, kept with the device, which applies them at a start (D493); this
 * browser keeps a copy, refreshed at each read of who is signed in, for hls.js's ceiling and ⋯'s
 * choices, and a choice changes both.
 */

import { reportFailure } from "@orpheum/sdk";

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

/**
 * Whether a film's own file, of fileKbps (the core's fileBitRateKbps, every track of it), plays under
 * capKbps (D491): with no cap, or where its rate, known, is within it; one whose rate is not known,
 * 0, is over every cap.
 */
export function isWithinCap(fileKbps: number, capKbps: number | undefined): boolean {
  return capKbps === undefined || (fileKbps > 0 && fileKbps <= capKbps);
}

/** A device's caps, kbit/s, by kind of network; a kind left out is No limit. */
export type CapsByKind = Partial<Record<NetworkKind, number>>;

/** A device's caps as the core keeps them (D493): a kind absent is No limit. */
export interface CoreDataCaps {
  readonly wifiKbps?: number;
  readonly mobileKbps?: number;
  readonly unknownKbps?: number;
}

/** caps as the core keeps them. */
export function coreDataCapsOf(caps: CapsByKind): CoreDataCaps {
  return {
    ...(caps.wifi === undefined ? {} : { wifiKbps: caps.wifi }),
    ...(caps.mobile === undefined ? {} : { mobileKbps: caps.mobile }),
    ...(caps.unknown === undefined ? {} : { unknownKbps: caps.unknown }),
  };
}

/** Keeps a device's caps on the core: a PUT of /me/devices/{id} (setMyDeviceDataCaps). */
export type KeepOnCore = (caps: CoreDataCaps) => Promise<void>;

/** The caps kept in storage, by kind; a value that is not a positive rate is none. */
function capsIn(storage: StringStorage): CapsByKind {
  let kept: unknown;
  try {
    kept = JSON.parse(storage.getItem(dataCapsKey) ?? "{}");
  } catch {
    return {};
  }
  return positiveRatesOf(kept);
}

/** Of kept, by kind, each value that is a positive rate. */
function positiveRatesOf(kept: unknown): CapsByKind {
  if (typeof kept !== "object" || kept === null) {
    return {};
  }
  const caps: CapsByKind = {};
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
  #keepOnCore: KeepOnCore | undefined;
  // The writes to the core so far, one at a time in the order chosen; it never rejects.
  #written: Promise<void> = Promise.resolve();

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

  /**
   * Keeps kbps as the cap for the network the device is on, here and on the core; undefined is No
   * limit.
   */
  choose(kbps: number | undefined): void {
    const caps = positiveRatesOf({ ...capsIn(this.#storage()), [this.kind()]: kbps });
    this.#keepHere(caps);
    this.#keepOnCoreToo(caps);
  }

  /**
   * Takes the caps the core keeps for this device, core, as GET /me says them, into this browser's
   * copy, and keeps each choice after on the core by keepOnCore (D493). Once, where the core keeps
   * none and this browser some, chosen here before the core kept them (D488), they are kept on the
   * core, and the core's are the truth from then on.
   */
  settleWithCore(core: CoreDataCaps, keepOnCore: KeepOnCore): void {
    this.#keepOnCore = keepOnCore;
    const kept = positiveRatesOf({
      wifi: core.wifiKbps,
      mobile: core.mobileKbps,
      unknown: core.unknownKbps,
    });
    const here = capsIn(this.#storage());
    if (Object.keys(kept).length === 0 && Object.keys(here).length > 0) {
      this.#keepOnCoreToo(here);
      return;
    }
    if (JSON.stringify(coreDataCapsOf(kept)) !== JSON.stringify(coreDataCapsOf(here))) {
      this.#keepHere(kept);
    }
  }

  /**
   * Settles once every choice made so far is kept on the core, or failed to be: a start waits for it,
   * so the core applies the cap just chosen (D489's re-ask, cause dataCapStarted). It never rejects.
   */
  written(): Promise<void> {
    return this.#written;
  }

  /** Keeps caps as this browser's copy, and tells each listener. */
  #keepHere(caps: CapsByKind): void {
    // JSON leaves a kind of No limit out.
    this.#storage().setItem(dataCapsKey, JSON.stringify(caps));
    for (const listener of this.#listeners) {
      listener();
    }
  }

  /** Keeps caps on the core, after the writes before it; before the core is known, nowhere. */
  #keepOnCoreToo(caps: CapsByKind): void {
    const keep = this.#keepOnCore;
    if (keep === undefined) {
      return;
    }
    this.#written = this.#written
      .then(async () => {
        await keep(coreDataCapsOf(caps));
      })
      .catch(reportFailure);
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

  /**
   * Tells onFall each time the cap falls under fileKbps where it was not (isWithinCap), by a choice or
   * a change of network: a cap starting to hold under it, or one lowered under it, at which a film
   * playing its own file of that rate moves to its ladder (D489, D491); returns the stopping.
   */
  whenCapFallsUnder(fileKbps: number, onFall: () => void): () => void {
    let wasWithin = isWithinCap(fileKbps, this.capKbps());
    return this.subscribe(() => {
      const isWithin = isWithinCap(fileKbps, this.capKbps());
      if (wasWithin && !isWithin) {
        onFall();
      }
      wasWithin = isWithin;
    });
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
