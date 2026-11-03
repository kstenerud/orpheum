// SPDX-License-Identifier: MPL-2.0

/**
 * A stand-in for hls.js's events, its listeners called as hls.js calls them, with none of hls.js's
 * own controllers answering first (which, for one, makes any error of a film with no qualities
 * loaded fatal).
 */
export class HlsEvents {
  readonly #listeners = new Map<string, ((event: unknown, data: unknown) => void)[]>();

  on(event: string, listener: (event: unknown, data: unknown) => void): void {
    this.#listeners.set(event, [...(this.#listeners.get(event) ?? []), listener]);
  }

  off(event: string, listener: (event: unknown, data: unknown) => void): void {
    this.#listeners.set(
      event,
      (this.#listeners.get(event) ?? []).filter((each) => each !== listener),
    );
  }

  trigger(event: string, data: unknown): void {
    for (const listener of this.#listeners.get(event) ?? []) {
      listener(event, data);
    }
  }

  /** How many listeners are on, of every event. */
  listening(): number {
    return [...this.#listeners.values()].reduce((sum, each) => sum + each.length, 0);
  }
}
