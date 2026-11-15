// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import {
  carrySoundOffsetOver,
  deviceSoundOffsetMs,
  forgetSoundOffsetSetInThisPage,
  keepDeviceSoundOffsetMs,
  soundOffsetKey,
} from "./sound-offset.ts";

/** A storage over a map, refusing every write when isRefusing. */
function storageOver(held: Map<string, string>, isRefusing = false): () => StringStorage {
  const storage: StringStorage = {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => {
      if (!isRefusing) {
        held.set(key, value);
      }
    },
    removeItem: (key) => {
      held.delete(key);
    },
  };
  return () => storage;
}

afterEach(() => {
  forgetSoundOffsetSetInThisPage();
});

describe("this device's own sound offset (D492 §1)", () => {
  it("is in step until one is kept, then the one kept, within the bound either way", () => {
    const held = new Map<string, string>();
    const storage = storageOver(held);
    expect(deviceSoundOffsetMs(storage)).toBe(0);
    keepDeviceSoundOffsetMs(-450, storage);
    expect(held.get(soundOffsetKey)).toBe("-450");
    keepDeviceSoundOffsetMs(2050, storage);
    expect(deviceSoundOffsetMs(storage)).toBe(2000);
  });

  it("reads none from what is not an offset this device may make", () => {
    for (const kept of ["oops", "2500", "1.5", '"300"']) {
      expect(deviceSoundOffsetMs(storageOver(new Map([[soundOffsetKey, kept]])))).toBe(0);
    }
  });

  it("lasts as long as the page where the browser refuses to keep it", () => {
    const storage = storageOver(new Map(), true);
    keepDeviceSoundOffsetMs(300, storage);
    expect(deviceSoundOffsetMs(storage)).toBe(300);
  });

  it("carries over the core's where the device has none, so the owner's TV keeps its delay", () => {
    const held = new Map<string, string>();
    carrySoundOffsetOver(500, storageOver(held));
    expect(held.get(soundOffsetKey)).toBe("500");
  });

  it("carries over nothing over one the device keeps, an in-step one among them, nor an in-step one", () => {
    const keptZero = new Map([[soundOffsetKey, "0"]]);
    carrySoundOffsetOver(500, storageOver(keptZero));
    expect(keptZero.get(soundOffsetKey)).toBe("0");
    const none = new Map<string, string>();
    carrySoundOffsetOver(0, storageOver(none));
    carrySoundOffsetOver(undefined, storageOver(none));
    expect(none.has(soundOffsetKey)).toBe(false);
  });
});
