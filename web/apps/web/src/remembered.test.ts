// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { createElement } from "react";
import { describe, expect, it } from "vitest";

import { Remembered, useRemembered } from "./remembered.ts";
import { mount } from "./test-dom.tsx";
import type { Resource } from "./use-resource.ts";

/** What a page shows for resource, its first render, with remembered. */
async function shownFor(
  resource: Resource<string>,
  remembered: Remembered<string>,
): Promise<Resource<string>> {
  const seen: Resource<string>[] = [];
  function Probe(): null {
    seen.push(useRemembered("title/23", resource, remembered));
    return null;
  }
  const mounted = await mount(createElement(Probe));
  mounted.unmount();
  const first = seen[0];
  if (first === undefined) {
    throw new Error("the probe never rendered");
  }
  return first;
}

describe("a page this tab showed before", () => {
  it("shows its last value at once while it loads again, and loading the first time", async () => {
    const remembered = new Remembered<string>();
    expect(await shownFor({ state: "loading" }, remembered)).toEqual({ state: "loading" });
    await shownFor({ state: "loaded", value: "Bram" }, remembered);
    expect(await shownFor({ state: "loading" }, remembered)).toEqual({
      state: "loaded",
      value: "Bram",
    });
  });

  it("forgets the oldest past fifty", () => {
    const remembered = new Remembered<number>();
    for (let index = 0; index <= 50; index++) {
      remembered.set(`title/${String(index)}`, index);
    }
    expect(remembered.get("title/0")).toBeUndefined();
    expect(remembered.get("title/50")).toBe(50);
  });
});
