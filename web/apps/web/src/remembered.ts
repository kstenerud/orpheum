// SPDX-License-Identifier: MPL-2.0

import { useEffect } from "react";

import type { Resource } from "./use-resource.ts";

/**
 * How many pages' last values a tab remembers of one kind: a few dozen films or series, each a few
 * kilobytes.
 */
const rememberedAtMost = 50;

/** The last value each of a kind of page loaded in this tab, by its resource key, the latest last. */
export class Remembered<T> {
  readonly #values = new Map<string, T>();

  get(key: string): T | undefined {
    return this.#values.get(key);
  }

  set(key: string, value: T): void {
    this.#values.delete(key);
    this.#values.set(key, value);
    const oldest = this.#values.keys().next();
    if (this.#values.size > rememberedAtMost && oldest.done !== true) {
      this.#values.delete(oldest.value);
    }
  }
}

/**
 * A page's resource, shown as its last value while it loads again when this tab loaded it before:
 * going back to a film's page showed "Loading…" each time, longer on a phone (the owner,
 * 2026-10-07). What it loads replaces it as it arrives.
 */
export function useRemembered<T>(
  key: string,
  resource: Resource<T>,
  remembered: Remembered<T>,
): Resource<T> {
  useEffect(() => {
    if (resource.state === "loaded") {
      remembered.set(key, resource.value);
    }
  }, [key, resource, remembered]);
  const last = remembered.get(key);
  return resource.state === "loading" && last !== undefined
    ? { state: "loaded", value: last }
    : resource;
}
