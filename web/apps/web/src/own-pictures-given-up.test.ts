// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import {
  isOwnPictureGivenUp,
  keepOwnPictureGivenUp,
  ownPicturesGivenUpKey,
  ownPicturesGivenUpMost,
} from "./own-pictures-given-up.ts";
import { servedBy, type Heard } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { testServices } from "./test-services.tsx";
import { requestPlayback } from "./watching.ts";

/** A storage over a map. */
function storageOver(held: Map<string, string>): () => StringStorage {
  const storage: StringStorage = {
    getItem: (key) => held.get(key) ?? null,
    setItem: (key, value) => {
      held.set(key, value);
    },
    removeItem: (key) => {
      held.delete(key);
    },
  };
  return () => storage;
}

describe("the films whose own picture this device gave up (D492 §3)", () => {
  it("are kept by the device, each once, and no other", () => {
    const held = new Map<string, string>();
    const storage = storageOver(held);
    expect(isOwnPictureGivenUp(5, storage)).toBe(false);
    keepOwnPictureGivenUp(5, storage);
    keepOwnPictureGivenUp(9, storage);
    keepOwnPictureGivenUp(5, storage);
    expect(isOwnPictureGivenUp(5, storage)).toBe(true);
    expect(isOwnPictureGivenUp(9, storage)).toBe(true);
    expect(isOwnPictureGivenUp(7, storage)).toBe(false);
    expect(held.get(ownPicturesGivenUpKey)).toBe("[9,5]");
  });

  it("are bounded, the one given up longest ago dropped first", () => {
    const storage = storageOver(new Map());
    for (let fileId = 1; fileId <= ownPicturesGivenUpMost + 1; fileId += 1) {
      keepOwnPictureGivenUp(fileId, storage);
    }
    expect(isOwnPictureGivenUp(1, storage)).toBe(false);
    expect(isOwnPictureGivenUp(2, storage)).toBe(true);
    expect(isOwnPictureGivenUp(ownPicturesGivenUpMost + 1, storage)).toBe(true);
  });

  it("are none where what is kept is not a list of films", () => {
    for (const kept of ["oops", '{"5":true}', "5"]) {
      expect(isOwnPictureGivenUp(5, storageOver(new Map([[ownPicturesGivenUpKey, kept]])))).toBe(
        false,
      );
    }
  });
});

describe("a start of a film whose own picture this device gave up (D492 §3)", () => {
  it("says so on every start of the film once the page asks again at the twin, the core keeping none", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy({ "POST /playback": [200, directDecision({})] }, heard);
    const playableFormats = testServices().playableFormats();
    await requestPlayback(api, 7, { playableFormats, cause: "playPage" });
    await requestPlayback(api, 7, { playableFormats, cause: "ownPictureGivenUp" });
    await requestPlayback(api, 7, { playableFormats, cause: "filmPage" });
    await requestPlayback(api, 8, { playableFormats, cause: "filmPage" });
    window.localStorage.removeItem(ownPicturesGivenUpKey);
    expect(
      heard.map((each) => (each.body as { hasGivenUpOwnPicture?: boolean }).hasGivenUpOwnPicture),
    ).toEqual([undefined, true, true, undefined]);
  });
});
