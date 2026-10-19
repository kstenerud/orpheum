// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import {
  asksForTheList,
  forgetHousehold,
  households,
  rememberHousehold,
  switchAddress,
  switchedFrom,
  goneAddress,
  goneFrom,
} from "./loader-households.ts";

function memoryStorage(): StringStorage {
  const items = new Map<string, string>();
  return {
    getItem: (key) => items.get(key) ?? null,
    setItem: (key, value) => {
      items.set(key, value);
    },
    removeItem: (key) => {
      items.delete(key);
    },
  };
}

const household = {
  coreId: "k7xq2",
  fingerprint: "a-fingerprint",
  realm: "nas",
  name: "",
  lastUsedAt: 1,
};

describe("the households a browser reached (D308 §8)", () => {
  it("lists the last used first, one entry a core, a name kept until another is said, and forgets", () => {
    const storage = memoryStorage();
    rememberHousehold(storage, { ...household, name: "Karl's" });
    rememberHousehold(storage, { ...household, coreId: "m3dz9", realm: "friends", lastUsedAt: 2 });
    rememberHousehold(storage, { ...household, lastUsedAt: 3 });
    expect(households(storage).map((entry) => [entry.coreId, entry.name])).toEqual([
      ["k7xq2", "Karl's"],
      ["m3dz9", ""],
    ]);
    forgetHousehold(storage, "k7xq2");
    expect(households(storage).map((entry) => entry.coreId)).toEqual(["m3dz9"]);
  });

  it("reads nothing from what is not a list of households", () => {
    const storage = memoryStorage();
    storage.setItem("orpheum.loader.households", "not json");
    expect(households(storage)).toEqual([]);
    storage.setItem("orpheum.loader.households", JSON.stringify([{ coreId: "k7xq2" }, 7]));
    expect(households(storage)).toEqual([]);
  });

  it("takes the household a switch names only from that household's own client origin", () => {
    const originOf = (coreId: string): string | undefined =>
      /^[a-z0-9]+$/u.test(coreId) ? `https://${coreId}.client.example` : undefined;
    const address = switchAddress("https://app.example", { ...household, name: "Karl's" });
    const search = new URL(address).search;
    expect(asksForTheList(search)).toBe(true);
    expect(switchedFrom(search, "https://k7xq2.client.example/", originOf)).toEqual({
      coreId: "k7xq2",
      fingerprint: "a-fingerprint",
      realm: "nas",
      name: "Karl's",
    });
    for (const referrer of [
      "",
      "https://m3dz9.client.example/",
      "https://evil.example/",
      "not a url",
    ]) {
      expect(switchedFrom(search, referrer, originOf)).toBeUndefined();
    }
    expect(asksForTheList("")).toBe(false);
  });

  it("takes a household as gone only from that household's own client origin", () => {
    const originOf = (coreId: string): string | undefined =>
      /^[a-z0-9]+$/u.test(coreId) ? `https://${coreId}.client.example` : undefined;
    const search = new URL(goneAddress("https://app.example", "k7xq2")).search;
    expect(asksForTheList(search)).toBe(true);
    expect(goneFrom(search, "https://k7xq2.client.example/", originOf)).toBe("k7xq2");
    for (const referrer of ["", "https://m3dz9.client.example/", "https://evil.example/"]) {
      expect(goneFrom(search, referrer, originOf)).toBeUndefined();
    }
  });
});
