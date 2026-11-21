// SPDX-License-Identifier: MPL-2.0

import { useEffect } from "react";

import { resetOnEachTestMount } from "./screen-load.tsx";
import type { Resource } from "./resource.ts";

/**
 * How many pages' last values a tab remembers of one kind: a few dozen films or series, each a few
 * kilobytes.
 */
const rememberedAtMost = 50;

/** The last value each of a kind of page loaded in this tab, by its resource key, the latest last. */
export class Remembered<T> {
  readonly #values = new Map<string, { readonly value: T; readonly size: number }>();
  readonly #atMost: number;
  readonly #sizeAtMost: number;
  #size = 0;

  /**
   * atMost: how many it keeps, rememberedAtMost unless a kind's values are larger; sizeAtMost: how
   * large they may be together, as set measures each (a tab's answers by their characters, D478).
   */
  constructor(atMost: number = rememberedAtMost, sizeAtMost: number = Number.POSITIVE_INFINITY) {
    this.#atMost = atMost;
    this.#sizeAtMost = sizeAtMost;
  }

  get(key: string): T | undefined {
    return this.#values.get(key)?.value;
  }

  /** Keeps value as key's latest, size its measure; the oldest go while too many or too large. */
  set(key: string, value: T, size = 0): void {
    this.#forget(key);
    if (size > this.#sizeAtMost) {
      return; // larger than all it may keep: kept, it would push out everything else
    }
    this.#values.set(key, { value, size });
    this.#size += size;
    for (const oldest of this.#values.keys()) {
      if (this.#values.size <= this.#atMost && this.#size <= this.#sizeAtMost) {
        break;
      }
      this.#forget(oldest);
    }
  }

  /** Forgets every value. */
  clear(): void {
    this.#values.clear();
    this.#size = 0;
  }

  #forget(key: string): void {
    const held = this.#values.get(key);
    if (held !== undefined) {
      this.#size -= held.size;
      this.#values.delete(key);
    }
  }
}

/**
 * A Remembered held by a module for the whole tab, as a kind of page's (title.tsx, series.tsx):
 * forgotten at each test's mount (resetOnEachTestMount), as a new tab would start, so a test's page
 * never shows, while it loads, the details an earlier test in its file left under the same id (the
 * series agent's tests in Firefox, 2026-11-04).
 */
export function rememberedInTheTab<T>(atMost?: number, sizeAtMost?: number): Remembered<T> {
  const remembered = new Remembered<T>(atMost, sizeAtMost);
  resetOnEachTestMount(() => {
    remembered.clear();
  });
  return remembered;
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
