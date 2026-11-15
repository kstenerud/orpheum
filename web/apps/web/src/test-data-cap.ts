// SPDX-License-Identifier: MPL-2.0

import type { StringStorage } from "./browser-storage.ts";

/** What a test of a device's data cap stands in for (data-cap.ts, D488). */

/** A storage in memory, holding what is set on it. */
export function heldStorage(): {
  readonly storage: () => StringStorage;
  readonly held: Map<string, string>;
} {
  const held = new Map<string, string>();
  const storage: StringStorage = {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => {
      held.set(key, value);
    },
    removeItem: (key) => {
      held.delete(key);
    },
  };
  return { storage: () => storage, held };
}

/** A browser's NetworkInformation as a test fakes it: a type it says, which a test changes. */
export class FakeConnection extends EventTarget {
  type: string | undefined;

  constructor(type: string | undefined) {
    super();
    this.type = type;
  }

  /** Moves the device to the network of type, telling `change` as the browser does. */
  moveTo(type: string | undefined): void {
    this.type = type;
    this.dispatchEvent(new Event("change"));
  }
}
