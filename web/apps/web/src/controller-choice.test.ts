// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { beforeEach, describe, expect, it, vi } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import type * as ControllerChoice from "./controller-choice.ts";

/** The module afresh, its choice not yet read. */
async function freshChoice(): Promise<typeof ControllerChoice> {
  vi.resetModules();
  return await import("./controller-choice.ts");
}

/** A storage holding kept under the choice's key, counting its reads. */
function countedStorage(kept: string | null): {
  readonly storage: () => StringStorage;
  readonly reads: () => number;
} {
  let reads = 0;
  const storage: StringStorage = {
    getItem: () => {
      reads += 1;
      return kept;
    },
    setItem: () => undefined,
    removeItem: () => undefined,
  };
  return { storage: () => storage, reads: () => reads };
}

describe("the choice of a controller's OK", () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it("is read from storage once, however often the controller poll asks for it", async () => {
    const choice = await freshChoice();
    const counted = countedStorage("right");
    for (let frame = 0; frame < 120; frame++) {
      expect(choice.okChoice(counted.storage)).toBe("right");
    }
    expect(counted.reads()).toBe(1);
  });

  it("takes the choice another tab makes, without reading storage again", async () => {
    const choice = await freshChoice();
    const counted = countedStorage("right");
    expect(choice.okChoice(counted.storage)).toBe("right");
    let told = 0;
    const changed = (): void => {
      told += 1;
    };
    // As useOkChoice listens, so a page showing the choice is drawn again.
    const stop = choice.listenToOkChoice(changed);
    globalThis.dispatchEvent(
      new StorageEvent("storage", { key: choice.okChoiceKey, newValue: "bottom" }),
    );
    expect(choice.okChoice(counted.storage)).toBe("bottom");
    expect(choice.chosenOkButton()).toBe("bottom");
    globalThis.dispatchEvent(
      new StorageEvent("storage", { key: "another.key", newValue: "right" }),
    );
    expect(choice.okChoice(counted.storage)).toBe("bottom");
    // The whole storage cleared, by another tab: automatic.
    globalThis.dispatchEvent(new StorageEvent("storage", { key: null }));
    expect(choice.okChoice(counted.storage)).toBe("automatic");
    expect(counted.reads()).toBe(1);
    expect(told).toBe(2);
    stop();
  });
});
